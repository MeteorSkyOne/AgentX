package app

import (
	"context"
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

var errAgentRunStopped = errors.New("agent run stopped")
var errAgentRunCanceled = errors.New("agent run canceled")

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
	terminalStatus := "failed"
	defer func() {
		a.removeActiveAgentRun(activeKey, runID)
		a.handleAgentRunTerminated(context.WithoutCancel(ctx), activeKey, terminalStatus)
	}()

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
		terminalStatus = "failed"
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
		a.setActiveAgentRunSession(ctx, activeKey, runID, session)

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
					activeRun.setContextUsage(evt.Usage.Context)
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
					if evt.Text == "" && evt.Thinking == "" && len(process) == 0 && !evt.ClearText {
						continue
					}
					activeRun.appendDelta(evt.Text, evt.Thinking, process, evt.ClearText, opts.Team)
					a.publishConversationEvent(domain.Event{
						Type:             domain.EventAgentOutputDelta,
						OrganizationID:   userMessage.OrganizationID,
						ConversationType: userMessage.ConversationType,
						ConversationID:   userMessage.ConversationID,
						Payload:          domain.AgentOutputDeltaPayload{RunID: runID, AgentID: agent.ID, Text: evt.Text, Thinking: evt.Thinking, Process: process, ClearText: evt.ClearText, Team: opts.Team},
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
					}), usage, team)
					if err != nil {
						sendResult(domain.Message{}, err)
						_ = session.Close(context.WithoutCancel(ctx))
						return
					}
					if opts.OnCompleted != nil {
						if err := opts.OnCompleted(ctx); err != nil {
							terminalStatus = "failed"
							a.publishAgentRunFailedWithContext(userMessage, runID, agent.ID, opts.Team, err)
							sendResult(domain.Message{}, err)
							_ = session.Close(context.WithoutCancel(ctx))
							return
						}
					}
					terminalStatus = "completed"
					sendResult(botMessage, nil)
					_ = session.Close(context.WithoutCancel(ctx))
					return
				case agentruntime.EventCanceled:
					terminalStatus = "canceled"
					a.removePendingQuestions(userMessage.ConversationType, userMessage.ConversationID)
					completedAt := time.Now().UTC()
					a.recordAgentRunMetric(ctx, runTracker.metric(agentRunMetricInput{
						Status:            "canceled",
						ProviderSessionID: session.CurrentSessionID(),
						CompletedAt:       completedAt,
						FirstTokenAt:      firstTokenAt,
						Usage:             usage,
					}))
					a.setCanceledAgentSession(ctx, agent.ID, userMessage, session.CurrentSessionID())
					a.persistAgentSessionContextUsage(ctx, agent.ID, userMessage.ConversationType, userMessage.ConversationID, usage)
					a.publishAgentRunCanceledWithContext(userMessage, runID, agent.ID, opts.Team)
					sendResult(domain.Message{}, errAgentRunCanceled)
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
						a.notifyAgentInputRequest(context.WithoutCancel(ctx), agent.Name, userMessage.OrganizationID, userMessage.ConversationType, userMessage.ConversationID, payload)
					}
				case agentruntime.EventFailed:
					terminalStatus = "failed"
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
					a.persistAgentSessionContextUsage(ctx, agent.ID, userMessage.ConversationType, userMessage.ConversationID, usage)
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
