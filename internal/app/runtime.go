package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

const runtimeContextMessageLimit = 40
const runtimeContextBodyLimit = 4000

var errAgentRunStopped = errors.New("agent run stopped")

type agentRunOptions struct {
	Prompt            string
	Context           string
	PermissionMode    string
	OnCompleted       func(context.Context) error
	Result            chan<- agentRunResult
	Team              *domain.TeamMetadata
	TeamForCompletion func(body string) *domain.TeamMetadata
}

type agentRunResult struct {
	Message domain.Message
	Err     error
}

func (a *App) runAgentForMessage(ctx context.Context, userMessage domain.Message, targets ...ConversationAgentContext) {
	runID := id.New("run")
	defer func() {
		if recovered := recover(); recovered != nil {
			a.publishAgentRunFailed(userMessage, runID, fmt.Errorf("agent run panic: %v", recovered))
		}
	}()

	var target ConversationAgentContext
	if len(targets) > 0 {
		target = targets[0]
	} else {
		scope, err := a.conversationScope(ctx, userMessage.ConversationType, userMessage.ConversationID)
		if err != nil {
			a.publishAgentRunFailed(userMessage, runID, err)
			return
		}
		resolved, err := a.conversationAgents(ctx, scope)
		if err != nil {
			if agentID := a.firstAgentIDForFailedResolution(ctx, scope); agentID != "" {
				a.setFailedAgentSession(ctx, agentID, userMessage, "")
			}
			a.publishAgentRunFailed(userMessage, runID, err)
			return
		}
		if len(resolved) == 0 {
			return
		}
		target = resolved[0]
	}

	a.runAgentForMessageWithTarget(ctx, userMessage, target, runID, agentRunOptions{})
}

func (a *App) runAgentForMessageWithTarget(ctx context.Context, userMessage domain.Message, target ConversationAgentContext, runID string, opts agentRunOptions) {
	runCtx, cancelRun := context.WithCancelCause(ctx)
	ctx = runCtx
	defer cancelRun(nil)

	startedAt := time.Now().UTC()
	activeRun := &activeAgentRun{
		runID:            runID,
		agentID:          target.Agent.ID,
		organizationID:   userMessage.OrganizationID,
		conversationType: userMessage.ConversationType,
		conversationID:   userMessage.ConversationID,
		startedAt:        startedAt,
		team:             cloneTeamMetadata(opts.Team),
		cancel:           cancelRun,
	}
	activeKey := activeRunKey{
		conversationType: userMessage.ConversationType,
		conversationID:   userMessage.ConversationID,
		agentID:          target.Agent.ID,
	}
	a.registerActiveAgentRun(activeKey, activeRun)
	defer a.removeActiveAgentRun(activeKey, runID)

	var tracker *agentRunTracker
	resultSent := false
	sendResult := func(message domain.Message, err error) {
		if opts.Result == nil || resultSent {
			return
		}
		resultSent = true
		result := agentRunResult{Message: message, Err: err}
		select {
		case opts.Result <- result:
		default:
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("agent run panic: %v", recovered)
			if tracker != nil {
				a.recordAgentRunMetric(context.WithoutCancel(ctx), tracker.metric(agentRunMetricInput{
					Status:      "failed",
					CompletedAt: time.Now().UTC(),
				}))
			}
			a.publishAgentRunFailedWithContext(userMessage, runID, target.Agent.ID, opts.Team, err)
			sendResult(domain.Message{}, err)
		}
	}()

	agent := target.Agent
	workspace := target.RunWorkspace
	runAttrs := agentRunLogAttrs(runID, userMessage, agent, workspace.Path, target.ConfigWorkspace.Path)
	scope, scopeErr := a.conversationScope(ctx, userMessage.ConversationType, userMessage.ConversationID)
	if scopeErr != nil {
		slog.Warn("agent run metrics scope lookup failed", append(runAttrs, "error", scopeErr)...)
	}
	runTracker := agentRunTracker{
		RunID:       runID,
		UserMessage: userMessage,
		Agent:       agent,
		Scope:       runMetricScope(scope),
		StartedAt:   startedAt,
	}
	tracker = &runTracker
	failRun := func(failCtx context.Context, providerSessionID string, err error) {
		if failCtx.Err() != nil {
			failCtx = context.WithoutCancel(failCtx)
		}
		a.setFailedAgentSession(failCtx, agent.ID, userMessage, providerSessionID)
		a.recordAgentRunMetric(failCtx, runTracker.metric(agentRunMetricInput{
			Status:            "failed",
			ProviderSessionID: providerSessionID,
			CompletedAt:       time.Now().UTC(),
		}))
		a.publishAgentRunFailedWithContext(userMessage, runID, agent.ID, opts.Team, err)
		sendResult(domain.Message{}, err)
	}
	slog.Info("agent run starting", runAttrs...)
	if err := os.MkdirAll(workspace.Path, 0o755); err != nil {
		failRun(ctx, "", err)
		return
	}
	if err := ensureAgentInstructionFiles(target.ConfigWorkspace.Path, agent); err != nil {
		failRun(ctx, "", err)
		return
	}
	rt, ok := a.runtimeForAgent(agent)
	if !ok {
		failRun(ctx, "", fmt.Errorf("runtime for agent kind %q is not configured", agent.Kind))
		return
	}

	previousSessionID, err := a.previousProviderSessionID(ctx, agent.ID, userMessage)
	if err != nil {
		failRun(ctx, "", err)
		return
	}

	runtimeContext, err := a.runtimeContextForMessage(ctx, agent.ID, userMessage)
	if err != nil {
		failRun(ctx, "", err)
		return
	}
	runtimeContext = joinRuntimeContext(runtimeContext, opts.Context)
	if len(userMessage.Attachments) == 0 {
		attachments, err := a.store.MessageAttachments().ListByMessage(ctx, userMessage.ID)
		if err != nil {
			failRun(ctx, "", err)
			return
		}
		userMessage.Attachments = attachments
	}

	prompt := opts.Prompt
	if prompt == "" {
		prompt = userMessage.Body
	}
	prompt, err = a.promptWithReplyReference(ctx, userMessage, prompt)
	if err != nil {
		failRun(ctx, "", err)
		return
	}

	sessionKey := agent.ID + ":" + string(userMessage.ConversationType) + ":" + userMessage.ConversationID
	startedPublished := false
	for {
		session, err := rt.StartSession(ctx, agentruntime.StartSessionRequest{
			AgentID:              agent.ID,
			Workspace:            workspace.Path,
			InstructionWorkspace: target.ConfigWorkspace.Path,
			Model:                agent.Model,
			Effort:               agent.Effort,
			PermissionMode:       opts.PermissionMode,
			FastMode:             agent.FastMode,
			YoloMode:             agent.YoloMode,
			Env:                  agent.Env,
			SessionKey:           sessionKey,
			PreviousSessionID:    previousSessionID,
		})
		if err != nil {
			slog.Error("agent runtime session start failed", append(runAttrs, "error", err)...)
			failRun(ctx, "", err)
			return
		}
		a.setActiveAgentRunSession(activeKey, runID, session)

		providerSessionID := session.CurrentSessionID()
		if err := a.store.Sessions().SetAgentSession(ctx, agent.ID, userMessage.ConversationType, userMessage.ConversationID, providerSessionID, "running"); err != nil {
			_ = session.Close(context.WithoutCancel(ctx))
			failRun(ctx, providerSessionID, err)
			return
		}
		if !startedPublished {
			startedPublished = true
			a.publishConversationEvent(domain.Event{
				Type:             domain.EventAgentRunStarted,
				OrganizationID:   userMessage.OrganizationID,
				ConversationType: userMessage.ConversationType,
				ConversationID:   userMessage.ConversationID,
				Payload:          domain.AgentRunPayload{RunID: runID, AgentID: agent.ID, Team: opts.Team},
			})
		}

		if err := session.Send(ctx, agentruntime.Input{
			Prompt:      prompt,
			Context:     runtimeContext,
			Attachments: runtimeAttachmentsFromMessage(userMessage),
		}); err != nil {
			slog.Error("agent runtime send failed", append(runAttrs, "provider_session_id", session.CurrentSessionID(), "error", err)...)
			failRun(ctx, session.CurrentSessionID(), err)
			_ = session.Close(context.WithoutCancel(ctx))
			return
		}

		var thinkingBuf strings.Builder
		var processBuf []domain.ProcessItem
		var firstTokenAt *time.Time
		var usage *agentruntime.Usage
		retryWithoutPreviousSession := false
	eventLoop:
		for {
			select {
			case <-ctx.Done():
				failRun(context.WithoutCancel(ctx), session.CurrentSessionID(), agentRunContextError(ctx))
				_ = session.Close(context.WithoutCancel(ctx))
				return
			case evt, ok := <-session.Events():
				if !ok {
					if ctx.Err() != nil {
						failRun(context.WithoutCancel(ctx), session.CurrentSessionID(), agentRunContextError(ctx))
						_ = session.Close(context.WithoutCancel(ctx))
						return
					}
					err := errors.New("agent runtime event stream closed")
					a.publishAgentRunFailedWithContext(userMessage, runID, agent.ID, opts.Team, err)
					sendResult(domain.Message{}, err)
					_ = session.Close(context.WithoutCancel(ctx))
					return
				}
				if evt.Usage != nil {
					usage = evt.Usage
				}
				switch evt.Type {
				case agentruntime.EventDelta:
					if firstTokenAt == nil && strings.TrimSpace(evt.Text) != "" {
						now := time.Now().UTC()
						firstTokenAt = &now
					}
					if evt.Thinking != "" {
						thinkingBuf.WriteString(evt.Thinking)
					}
					process := runtimeProcessItems(evt)
					processBuf = append(processBuf, process...)
					if evt.Text == "" && evt.Thinking == "" && len(process) == 0 {
						continue
					}
					activeRun.appendDelta(evt.Text, evt.Thinking, process, opts.Team)
					a.publishConversationEvent(domain.Event{
						Type:             domain.EventAgentOutputDelta,
						OrganizationID:   userMessage.OrganizationID,
						ConversationType: userMessage.ConversationType,
						ConversationID:   userMessage.ConversationID,
						Payload:          domain.AgentOutputDeltaPayload{RunID: runID, AgentID: agent.ID, Text: evt.Text, Thinking: evt.Thinking, Process: process, Team: opts.Team},
					})
				case agentruntime.EventCompleted:
					a.removePendingQuestions(userMessage.ConversationType, userMessage.ConversationID)
					completedAt := time.Now().UTC()
					if firstTokenAt == nil && strings.TrimSpace(evt.Text) != "" {
						firstTokenAt = &completedAt
					}
					if evt.Thinking != "" {
						thinkingBuf.WriteString(evt.Thinking)
					}
					processBuf = append(processBuf, runtimeProcessItems(evt)...)
					team := opts.Team
					if opts.TeamForCompletion != nil {
						team = opts.TeamForCompletion(evt.Text)
					}
					botMessage, err := a.completeAgentRun(ctx, userMessage, agent, runID, session.CurrentSessionID(), evt.Text, thinkingBuf.String(), processBuf, runTracker.metric(agentRunMetricInput{
						Status:            "completed",
						ProviderSessionID: session.CurrentSessionID(),
						FirstTokenAt:      firstTokenAt,
						CompletedAt:       completedAt,
						Usage:             usage,
					}), team)
					if err != nil {
						sendResult(domain.Message{}, err)
						_ = session.Close(context.WithoutCancel(ctx))
						return
					}
					if opts.OnCompleted != nil {
						if err := opts.OnCompleted(ctx); err != nil {
							a.publishAgentRunFailedWithContext(userMessage, runID, agent.ID, opts.Team, err)
							sendResult(domain.Message{}, err)
							_ = session.Close(context.WithoutCancel(ctx))
							return
						}
					}
					sendResult(botMessage, nil)
					_ = session.Close(context.WithoutCancel(ctx))
					return
				case agentruntime.EventInputRequest:
					if evt.InputRequest != nil {
						payload := domain.AgentInputRequestPayload{
							RunID:      runID,
							AgentID:    agent.ID,
							QuestionID: evt.InputRequest.QuestionID,
							Question:   evt.InputRequest.Question,
							Options:    toAgentInputOptions(evt.InputRequest.Options),
							Team:       opts.Team,
						}
						activeRun.setPendingQuestion(payload)
						a.registerPendingQuestion(pendingQuestionKey{
							conversationType: userMessage.ConversationType,
							conversationID:   userMessage.ConversationID,
							questionID:       evt.InputRequest.QuestionID,
						}, &pendingQuestion{session: session, run: activeRun})
						a.publishConversationEvent(domain.Event{
							Type:             domain.EventAgentInputRequest,
							OrganizationID:   userMessage.OrganizationID,
							ConversationType: userMessage.ConversationType,
							ConversationID:   userMessage.ConversationID,
							Payload:          payload,
						})
					}
				case agentruntime.EventFailed:
					a.removePendingQuestions(userMessage.ConversationType, userMessage.ConversationID)
					err := runtimeEventError(evt)
					if evt.StaleSession && previousSessionID != "" {
						slog.Warn("agent runtime stale provider session; retrying without resume", append(runAttrs, "provider_session_id", previousSessionID, "error", err)...)
						if err := a.store.Sessions().SetAgentSession(ctx, agent.ID, userMessage.ConversationType, userMessage.ConversationID, "", "stale"); err != nil {
							failRun(ctx, session.CurrentSessionID(), err)
							_ = session.Close(context.WithoutCancel(ctx))
							return
						}
						previousSessionID = ""
						retryWithoutPreviousSession = true
						break eventLoop
					}
					slog.Error("agent runtime event failed", append(runAttrs, "provider_session_id", session.CurrentSessionID(), "error", err)...)
					a.recordAgentRunMetric(ctx, runTracker.metric(agentRunMetricInput{
						Status:            "failed",
						ProviderSessionID: session.CurrentSessionID(),
						CompletedAt:       time.Now().UTC(),
						FirstTokenAt:      firstTokenAt,
						Usage:             usage,
					}))
					a.setFailedAgentSession(ctx, agent.ID, userMessage, session.CurrentSessionID())
					a.publishAgentRunFailedWithContext(userMessage, runID, agent.ID, opts.Team, err)
					sendResult(domain.Message{}, err)
					_ = session.Close(context.WithoutCancel(ctx))
					return
				}
			}
		}
		_ = session.Close(context.WithoutCancel(ctx))
		if !retryWithoutPreviousSession {
			return
		}
	}
}

func (a *App) runtimeContextForMessage(ctx context.Context, agentID string, userMessage domain.Message) (string, error) {
	var contextStartedAt *time.Time
	session, err := a.store.Sessions().ByConversation(ctx, agentID, userMessage.ConversationType, userMessage.ConversationID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	} else {
		contextStartedAt = session.ContextStartedAt
	}

	messages, err := a.store.Messages().ListRecent(ctx, userMessage.ConversationType, userMessage.ConversationID, runtimeContextMessageLimit)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, message := range messages {
		if message.ID == userMessage.ID {
			continue
		}
		if contextStartedAt != nil && message.CreatedAt.Before(*contextStartedAt) {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("Conversation history visible to this agent. Use it as context, but reply only to the current user message.\n")
		}
		fmt.Fprintf(
			&b,
			"%s %s: %s\n",
			message.CreatedAt.Format(time.RFC3339),
			runtimeSenderLabel(message),
			runtimeMessageBody(message.Body),
		)
	}
	return strings.TrimSpace(b.String()), nil
}

func (a *App) promptWithReplyReference(ctx context.Context, userMessage domain.Message, prompt string) (string, error) {
	replyToMessageID := strings.TrimSpace(userMessage.ReplyToMessageID)
	if replyToMessageID == "" {
		return prompt, nil
	}

	referenced, err := a.store.Messages().ByID(ctx, replyToMessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return deletedReplyReferencePrompt(replyToMessageID, prompt), nil
		}
		return "", err
	}
	if referenced.OrganizationID != userMessage.OrganizationID ||
		referenced.ConversationType != userMessage.ConversationType ||
		referenced.ConversationID != userMessage.ConversationID {
		return deletedReplyReferencePrompt(replyToMessageID, prompt), nil
	}

	var b strings.Builder
	b.WriteString("The current user message is replying to this referenced message.\n")
	fmt.Fprintf(&b, "Referenced message ID: %s\n", referenced.ID)
	fmt.Fprintf(&b, "Referenced sender: %s\n", runtimeSenderLabel(referenced))
	fmt.Fprintf(&b, "Referenced created at: %s\n", referenced.CreatedAt.Format(time.RFC3339))
	b.WriteString("Referenced body:\n")
	b.WriteString(runtimeMessageBody(referenced.Body))
	b.WriteString("\n\nUser message:\n")
	b.WriteString(prompt)
	return b.String(), nil
}

func deletedReplyReferencePrompt(messageID string, prompt string) string {
	return "The current user message is replying to a referenced message, but that referenced message was deleted or is unavailable.\n" +
		"Referenced message ID: " + messageID + "\n\nUser message:\n" + prompt
}

func runtimeSenderLabel(message domain.Message) string {
	switch message.SenderType {
	case domain.SenderUser:
		return "user"
	case domain.SenderBot:
		return "bot:" + message.SenderID
	case domain.SenderSystem:
		return "system"
	default:
		return string(message.SenderType)
	}
}

func runtimeMessageBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "(empty)"
	}
	runes := []rune(body)
	if len(runes) <= runtimeContextBodyLimit {
		return body
	}
	return string(runes[:runtimeContextBodyLimit]) + "\n[truncated]"
}

func joinRuntimeContext(base string, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	if base == "" {
		return extra
	}
	if extra == "" {
		return base
	}
	return base + "\n\n" + extra
}

func runtimeAttachmentsFromMessage(message domain.Message) []agentruntime.Attachment {
	if len(message.Attachments) == 0 {
		return nil
	}
	attachments := make([]agentruntime.Attachment, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		attachments = append(attachments, agentruntime.Attachment{
			ID:          attachment.ID,
			Filename:    attachment.Filename,
			ContentType: attachment.ContentType,
			Kind:        string(attachment.Kind),
			SizeBytes:   attachment.SizeBytes,
			LocalPath:   attachment.StoragePath,
		})
	}
	return attachments
}

func (a *App) firstAgentIDForFailedResolution(ctx context.Context, scope conversationScope) string {
	if scope.legacyBinding != nil {
		return scope.legacyBinding.AgentID
	}
	if scope.channel.ID == "" {
		return ""
	}
	bindings, err := a.store.ChannelAgents().ListByChannel(ctx, scope.channel.ID)
	if err != nil || len(bindings) == 0 {
		return ""
	}
	return bindings[0].AgentID
}

func (a *App) previousProviderSessionID(ctx context.Context, agentID string, message domain.Message) (string, error) {
	session, err := a.store.Sessions().ByConversation(ctx, agentID, message.ConversationType, message.ConversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return session.ProviderSessionID, nil
}

func (a *App) completeAgentRun(ctx context.Context, userMessage domain.Message, agent domain.Agent, runID string, providerSessionID string, body string, thinking string, process []domain.ProcessItem, metric domain.AgentRunMetric, team *domain.TeamMetadata) (domain.Message, error) {
	createdAt := time.Now().UTC()
	if !createdAt.After(userMessage.CreatedAt) {
		createdAt = userMessage.CreatedAt.Add(time.Nanosecond)
	}
	botMessageID := id.New("msg")
	metric.ResponseMessageID = botMessageID
	if metric.CompletedAt == nil {
		completedAt := createdAt
		metric.CompletedAt = &completedAt
	}
	metricSummary := messageMetricsSummary(metric)
	var metadata map[string]any
	if thinking = strings.TrimSpace(thinking); thinking != "" {
		metadata = map[string]any{"thinking": thinking}
	}
	if len(process) > 0 {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["process"] = process
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["metrics"] = metricSummary
	if team != nil {
		metadata["team"] = *team
	}
	botMessage := domain.Message{
		ID:               botMessageID,
		OrganizationID:   userMessage.OrganizationID,
		ConversationType: userMessage.ConversationType,
		ConversationID:   userMessage.ConversationID,
		SenderType:       domain.SenderBot,
		SenderID:         agent.BotUserID,
		Kind:             domain.MessageText,
		Body:             body,
		Metadata:         metadata,
		CreatedAt:        createdAt,
	}
	if err := a.store.Messages().Create(ctx, botMessage); err != nil {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, providerSessionID)
		metric.Status = "failed"
		metric.ResponseMessageID = ""
		now := time.Now().UTC()
		metric.CompletedAt = &now
		a.recordAgentRunMetric(ctx, metric)
		a.publishAgentRunFailedWithContext(userMessage, runID, agent.ID, team, err)
		return domain.Message{}, err
	}
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventMessageCreated,
		OrganizationID:   botMessage.OrganizationID,
		ConversationType: botMessage.ConversationType,
		ConversationID:   botMessage.ConversationID,
		Payload:          domain.MessageCreatedPayload{Message: botMessage},
	})
	a.notifyAgentMessageCreated(context.WithoutCancel(ctx), agent.Name, botMessage)
	if err := a.store.Sessions().SetAgentSession(ctx, agent.ID, userMessage.ConversationType, userMessage.ConversationID, providerSessionID, "completed"); err != nil {
		metric.Status = "failed"
		a.recordAgentRunMetric(ctx, metric)
		a.publishAgentRunFailedWithContext(userMessage, runID, agent.ID, team, err)
		return domain.Message{}, err
	}
	a.recordAgentRunMetric(ctx, metric)
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentRunCompleted,
		OrganizationID:   userMessage.OrganizationID,
		ConversationType: userMessage.ConversationType,
		ConversationID:   userMessage.ConversationID,
		Payload:          domain.AgentRunPayload{RunID: runID, AgentID: agent.ID, Team: team},
	})
	slog.Info(
		"agent run completed",
		"run_id", runID,
		"agent_id", agent.ID,
		"agent_kind", agent.Kind,
		"provider_session_id", providerSessionID,
		"organization_id", userMessage.OrganizationID,
		"conversation_type", userMessage.ConversationType,
		"conversation_id", userMessage.ConversationID,
		"message_id", userMessage.ID,
		"response_chars", len([]rune(body)),
		"thinking_chars", len([]rune(thinking)),
		"process_items", len(process),
	)
	return botMessage, nil
}

func (a *App) setFailedAgentSession(ctx context.Context, agentID string, message domain.Message, providerSessionID string) {
	_ = a.store.Sessions().SetAgentSession(ctx, agentID, message.ConversationType, message.ConversationID, providerSessionID, "failed")
}

type agentRunMetricScope struct {
	ProjectID string
	ChannelID string
	ThreadID  string
}

type agentRunTracker struct {
	RunID       string
	UserMessage domain.Message
	Agent       domain.Agent
	Scope       agentRunMetricScope
	StartedAt   time.Time
}

type agentRunMetricInput struct {
	Status            string
	ProviderSessionID string
	ResponseMessageID string
	FirstTokenAt      *time.Time
	CompletedAt       time.Time
	Usage             *agentruntime.Usage
}

func (t agentRunTracker) metric(input agentRunMetricInput) domain.AgentRunMetric {
	completedAt := input.CompletedAt
	var completedAtPtr *time.Time
	if !completedAt.IsZero() {
		completedAtPtr = &completedAt
	}
	usage := input.Usage
	metric := domain.AgentRunMetric{
		RunID:             t.RunID,
		OrganizationID:    t.UserMessage.OrganizationID,
		ProjectID:         t.Scope.ProjectID,
		ChannelID:         t.Scope.ChannelID,
		ThreadID:          t.Scope.ThreadID,
		ConversationType:  t.UserMessage.ConversationType,
		ConversationID:    t.UserMessage.ConversationID,
		MessageID:         t.UserMessage.ID,
		ResponseMessageID: input.ResponseMessageID,
		AgentID:           t.Agent.ID,
		AgentName:         t.Agent.Name,
		Provider:          providerForAgent(t.Agent),
		Model:             strings.TrimSpace(t.Agent.Model),
		Status:            input.Status,
		StartedAt:         t.StartedAt,
		FirstTokenAt:      input.FirstTokenAt,
		CompletedAt:       completedAtPtr,
		CreatedAt:         time.Now().UTC(),
	}
	if usage != nil {
		if model := strings.TrimSpace(usage.Model); model != "" {
			metric.Model = model
		}
		metric.InputTokens = usage.InputTokens
		metric.CachedInputTokens = usage.CachedInputTokens
		metric.CacheCreationInputTokens = usage.CacheCreationInputTokens
		metric.CacheReadInputTokens = usage.CacheReadInputTokens
		metric.OutputTokens = usage.OutputTokens
		metric.ReasoningOutputTokens = usage.ReasoningOutputTokens
		metric.TotalTokens = usage.TotalTokens
		metric.TotalCostUSD = usage.TotalCostUSD
		metric.RawUsageJSON = rawUsageJSON(usage.Raw)
	}
	if metric.TotalTokens == nil && metric.InputTokens != nil && metric.OutputTokens != nil {
		total := *metric.InputTokens + *metric.OutputTokens
		metric.TotalTokens = &total
	}
	if completedAtPtr != nil {
		duration := completedAt.Sub(t.StartedAt).Milliseconds()
		if duration < 0 {
			duration = 0
		}
		metric.DurationMS = &duration
	}
	if input.FirstTokenAt != nil {
		ttft := input.FirstTokenAt.Sub(t.StartedAt).Milliseconds()
		if ttft < 0 {
			ttft = 0
		}
		metric.TTFTMS = &ttft
	}
	if completedAtPtr != nil && metric.OutputTokens != nil {
		seconds := metricTPSSeconds(t.StartedAt, completedAt, input.FirstTokenAt)
		if seconds > 0 {
			tps := float64(*metric.OutputTokens) / seconds
			metric.TPS = &tps
		}
	}
	metric.CacheHitRate = metricCacheHitRate(metric)
	return metric
}

func metricTPSSeconds(startedAt time.Time, completedAt time.Time, firstTokenAt *time.Time) float64 {
	totalSeconds := completedAt.Sub(startedAt).Seconds()
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	if firstTokenAt == nil {
		return totalSeconds
	}
	observedSeconds := completedAt.Sub(*firstTokenAt).Seconds()
	if observedSeconds < 0 {
		observedSeconds = 0
	}
	if observedSeconds < 0.25 && totalSeconds > observedSeconds {
		return totalSeconds
	}
	return observedSeconds
}

func metricCacheHitRate(metric domain.AgentRunMetric) *float64 {
	input := int64PtrValue(metric.InputTokens)
	cacheCreation := int64PtrValue(metric.CacheCreationInputTokens)
	cacheRead := int64PtrValue(metric.CacheReadInputTokens)
	cached := int64PtrValue(metric.CachedInputTokens)
	if cacheRead > cached {
		cached = cacheRead
	}
	if cached <= 0 {
		return nil
	}

	denominator := input
	if cacheRead > 0 || cacheCreation > 0 {
		denominator = input + cacheCreation + cacheRead
	} else if cached > input {
		denominator = input + cached
	}
	if denominator <= 0 {
		return nil
	}

	rate := float64(cached) / float64(denominator)
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	return &rate
}

func int64PtrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func runMetricScope(scope conversationScope) agentRunMetricScope {
	result := agentRunMetricScope{
		ProjectID: scope.project.ID,
		ChannelID: scope.channel.ID,
	}
	if scope.thread != nil {
		result.ThreadID = scope.thread.ID
	}
	return result
}

func providerForAgent(agent domain.Agent) string {
	switch strings.TrimSpace(agent.Kind) {
	case domain.AgentKindClaude, domain.AgentKindClaudePersistent:
		return domain.AgentKindClaude
	case domain.AgentKindCodex, domain.AgentKindCodexPersistent:
		return domain.AgentKindCodex
	case "", domain.AgentKindFake:
		return domain.AgentKindFake
	default:
		return strings.TrimSpace(agent.Kind)
	}
}

func rawUsageJSON(raw any) string {
	if raw == nil {
		return ""
	}
	content, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	return string(content)
}

func messageMetricsSummary(metric domain.AgentRunMetric) domain.MessageMetricsSummary {
	return domain.MessageMetricsSummary{
		RunID:        metric.RunID,
		Provider:     metric.Provider,
		TTFTMS:       metric.TTFTMS,
		TPS:          metric.TPS,
		DurationMS:   metric.DurationMS,
		InputTokens:  metric.InputTokens,
		OutputTokens: metric.OutputTokens,
		TotalTokens:  metric.TotalTokens,
		CacheHitRate: metric.CacheHitRate,
	}
}

func (a *App) recordAgentRunMetric(ctx context.Context, metric domain.AgentRunMetric) {
	if err := a.store.Metrics().Create(ctx, metric); err != nil {
		slog.Warn(
			"agent run metric persist failed",
			"run_id", metric.RunID,
			"agent_id", metric.AgentID,
			"provider", metric.Provider,
			"status", metric.Status,
			"error", err,
		)
	}
}

func (a *App) publishAgentRunFailed(message domain.Message, runID string, err error) {
	a.publishAgentRunFailedWithContext(message, runID, "", nil, err)
}

func (a *App) publishAgentRunFailedWithContext(message domain.Message, runID string, agentID string, team *domain.TeamMetadata, err error) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	slog.Error(
		"agent run failed",
		"run_id", runID,
		"organization_id", message.OrganizationID,
		"conversation_type", message.ConversationType,
		"conversation_id", message.ConversationID,
		"message_id", message.ID,
		"error", errText,
	)
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentRunFailed,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.AgentRunFailedPayload{RunID: runID, AgentID: agentID, Error: errText, Team: team},
	})
}

func (a *App) publishConversationEvent(evt domain.Event) {
	evt.ID = id.New("evt")
	evt.CreatedAt = time.Now().UTC()
	a.bus.Publish(evt)
}

func toAgentInputOptions(opts []agentruntime.InputRequestOption) []domain.AgentInputRequestOption {
	if len(opts) == 0 {
		return nil
	}
	result := make([]domain.AgentInputRequestOption, len(opts))
	for i, o := range opts {
		result[i] = domain.AgentInputRequestOption{Label: o.Label, Description: o.Description}
	}
	return result
}

func runtimeEventError(evt agentruntime.Event) error {
	if evt.Error != "" {
		return errors.New(evt.Error)
	}
	return errors.New("agent runtime failed")
}

func agentRunContextError(ctx context.Context) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return context.Canceled
}

func agentRunLogAttrs(runID string, message domain.Message, agent domain.Agent, workspace string, instructionWorkspace string) []any {
	return []any{
		"run_id", runID,
		"agent_id", agent.ID,
		"agent_kind", agent.Kind,
		"agent_name", agent.Name,
		"model", agent.Model,
		"effort", agent.Effort,
		"fast_mode", agent.FastMode,
		"yolo_mode", agent.YoloMode,
		"organization_id", message.OrganizationID,
		"conversation_type", message.ConversationType,
		"conversation_id", message.ConversationID,
		"message_id", message.ID,
		"workspace", workspace,
		"instruction_workspace", instructionWorkspace,
	}
}

func runtimeProcessItems(evt agentruntime.Event) []domain.ProcessItem {
	if len(evt.Process) == 0 {
		if evt.Thinking == "" {
			return nil
		}
		return []domain.ProcessItem{{
			Type: "thinking",
			Text: evt.Thinking,
		}}
	}
	items := make([]domain.ProcessItem, 0, len(evt.Process))
	for _, item := range evt.Process {
		items = append(items, domain.ProcessItem{
			Type:       item.Type,
			Text:       item.Text,
			ToolName:   item.ToolName,
			ToolCallID: item.ToolCallID,
			Status:     item.Status,
			Input:      item.Input,
			Output:     item.Output,
			Raw:        item.Raw,
			CreatedAt:  item.CreatedAt,
		})
	}
	return items
}
