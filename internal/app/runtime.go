package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

const runtimeContextMessageLimit = 40
const runtimeContextBodyLimit = 4000

type agentRunOptions struct {
	Prompt         string
	PermissionMode string
	OnCompleted    func(context.Context) error
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
	defer func() {
		if recovered := recover(); recovered != nil {
			a.publishAgentRunFailed(userMessage, runID, fmt.Errorf("agent run panic: %v", recovered))
		}
	}()

	agent := target.Agent
	workspace := target.RunWorkspace
	if err := os.MkdirAll(workspace.Path, 0o755); err != nil {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, "")
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}
	rt, ok := a.runtimeForAgent(agent)
	if !ok {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, "")
		a.publishAgentRunFailed(userMessage, runID, fmt.Errorf("runtime for agent kind %q is not configured", agent.Kind))
		return
	}

	previousSessionID, err := a.previousProviderSessionID(ctx, agent.ID, userMessage)
	if err != nil {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, "")
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}

	runtimeContext, err := a.runtimeContextForMessage(ctx, agent.ID, userMessage)
	if err != nil {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, "")
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}

	sessionKey := agent.ID + ":" + string(userMessage.ConversationType) + ":" + userMessage.ConversationID
	session, err := rt.StartSession(ctx, agentruntime.StartSessionRequest{
		AgentID:           agent.ID,
		Workspace:         workspace.Path,
		Model:             agent.Model,
		Effort:            agent.Effort,
		PermissionMode:    opts.PermissionMode,
		FastMode:          agent.FastMode,
		YoloMode:          agent.YoloMode,
		Env:               agent.Env,
		SessionKey:        sessionKey,
		PreviousSessionID: previousSessionID,
	})
	if err != nil {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, "")
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}
	defer func() {
		_ = session.Close(context.WithoutCancel(ctx))
	}()

	providerSessionID := session.CurrentSessionID()
	if err := a.store.Sessions().SetAgentSession(ctx, agent.ID, userMessage.ConversationType, userMessage.ConversationID, providerSessionID, "running"); err != nil {
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentRunStarted,
		OrganizationID:   userMessage.OrganizationID,
		ConversationType: userMessage.ConversationType,
		ConversationID:   userMessage.ConversationID,
		Payload:          domain.AgentRunPayload{RunID: runID, AgentID: agent.ID},
	})

	prompt := opts.Prompt
	if prompt == "" {
		prompt = userMessage.Body
	}
	if err := session.Send(ctx, agentruntime.Input{Prompt: prompt, Context: runtimeContext}); err != nil {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, session.CurrentSessionID())
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}

	var thinkingBuf strings.Builder
	var processBuf []domain.ProcessItem
	for {
		select {
		case <-ctx.Done():
			a.setFailedAgentSession(context.WithoutCancel(ctx), agent.ID, userMessage, session.CurrentSessionID())
			a.publishAgentRunFailed(userMessage, runID, ctx.Err())
			return
		case evt, ok := <-session.Events():
			if !ok {
				return
			}
			switch evt.Type {
			case agentruntime.EventDelta:
				if evt.Thinking != "" {
					thinkingBuf.WriteString(evt.Thinking)
				}
				process := runtimeProcessItems(evt)
				processBuf = append(processBuf, process...)
				a.publishConversationEvent(domain.Event{
					Type:             domain.EventAgentOutputDelta,
					OrganizationID:   userMessage.OrganizationID,
					ConversationType: userMessage.ConversationType,
					ConversationID:   userMessage.ConversationID,
					Payload:          domain.AgentOutputDeltaPayload{RunID: runID, AgentID: agent.ID, Text: evt.Text, Thinking: evt.Thinking, Process: process},
				})
			case agentruntime.EventCompleted:
				if evt.Thinking != "" {
					thinkingBuf.WriteString(evt.Thinking)
				}
				processBuf = append(processBuf, runtimeProcessItems(evt)...)
				a.completeAgentRun(ctx, userMessage, agent, runID, session.CurrentSessionID(), evt.Text, thinkingBuf.String(), processBuf)
				if opts.OnCompleted != nil {
					if err := opts.OnCompleted(ctx); err != nil {
						a.publishAgentRunFailed(userMessage, runID, err)
					}
				}
				return
			case agentruntime.EventFailed:
				a.setFailedAgentSession(ctx, agent.ID, userMessage, session.CurrentSessionID())
				a.publishAgentRunFailed(userMessage, runID, runtimeEventError(evt))
				return
			}
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

func (a *App) completeAgentRun(ctx context.Context, userMessage domain.Message, agent domain.Agent, runID string, providerSessionID string, body string, thinking string, process []domain.ProcessItem) {
	createdAt := time.Now().UTC()
	if !createdAt.After(userMessage.CreatedAt) {
		createdAt = userMessage.CreatedAt.Add(time.Nanosecond)
	}
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
	botMessage := domain.Message{
		ID:               id.New("msg"),
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
		a.publishAgentRunFailed(userMessage, runID, err)
		return
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
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentRunCompleted,
		OrganizationID:   userMessage.OrganizationID,
		ConversationType: userMessage.ConversationType,
		ConversationID:   userMessage.ConversationID,
		Payload:          domain.AgentRunPayload{RunID: runID, AgentID: agent.ID},
	})
}

func (a *App) setFailedAgentSession(ctx context.Context, agentID string, message domain.Message, providerSessionID string) {
	_ = a.store.Sessions().SetAgentSession(ctx, agentID, message.ConversationType, message.ConversationID, providerSessionID, "failed")
}

func (a *App) publishAgentRunFailed(message domain.Message, runID string, err error) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentRunFailed,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.AgentRunFailedPayload{RunID: runID, Error: errText},
	})
}

func (a *App) publishConversationEvent(evt domain.Event) {
	evt.ID = id.New("evt")
	evt.CreatedAt = time.Now().UTC()
	a.bus.Publish(evt)
}

func runtimeEventError(evt agentruntime.Event) error {
	if evt.Error != "" {
		return errors.New(evt.Error)
	}
	return errors.New("agent runtime failed")
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
