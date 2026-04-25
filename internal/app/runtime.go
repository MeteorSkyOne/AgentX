package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/fake"
)

func (a *App) runAgentForMessage(ctx context.Context, userMessage domain.Message) {
	runID := id.New("run")
	defer func() {
		if recovered := recover(); recovered != nil {
			a.publishAgentRunFailed(userMessage, runID, fmt.Errorf("agent run panic: %v", recovered))
		}
	}()

	binding, err := a.store.Bindings().ByConversation(ctx, userMessage.ConversationType, userMessage.ConversationID)
	if err != nil {
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}
	agent, err := a.store.Agents().ByID(ctx, binding.AgentID)
	if err != nil {
		a.setFailedAgentSession(ctx, binding.AgentID, userMessage, "")
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}
	workspace, err := a.store.Workspaces().ByID(ctx, binding.WorkspaceID)
	if err != nil {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, "")
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}

	sessionKey := agent.ID + ":" + string(userMessage.ConversationType) + ":" + userMessage.ConversationID
	session, err := fake.New().StartSession(ctx, agentruntime.StartSessionRequest{
		AgentID:    agent.ID,
		Workspace:  workspace.Path,
		Model:      agent.Model,
		Env:        map[string]string{},
		SessionKey: sessionKey,
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

	if err := session.Send(ctx, agentruntime.Input{Prompt: userMessage.Body}); err != nil {
		a.setFailedAgentSession(ctx, agent.ID, userMessage, providerSessionID)
		a.publishAgentRunFailed(userMessage, runID, err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			a.setFailedAgentSession(context.WithoutCancel(ctx), agent.ID, userMessage, providerSessionID)
			a.publishAgentRunFailed(userMessage, runID, ctx.Err())
			return
		case evt, ok := <-session.Events():
			if !ok {
				return
			}
			switch evt.Type {
			case agentruntime.EventDelta:
				a.publishConversationEvent(domain.Event{
					Type:             domain.EventAgentOutputDelta,
					OrganizationID:   userMessage.OrganizationID,
					ConversationType: userMessage.ConversationType,
					ConversationID:   userMessage.ConversationID,
					Payload:          domain.AgentOutputDeltaPayload{RunID: runID, Text: evt.Text},
				})
			case agentruntime.EventCompleted:
				a.completeAgentRun(ctx, userMessage, agent, runID, providerSessionID, evt.Text)
				return
			case agentruntime.EventFailed:
				a.setFailedAgentSession(ctx, agent.ID, userMessage, providerSessionID)
				a.publishAgentRunFailed(userMessage, runID, runtimeEventError(evt))
				return
			}
		}
	}
}

func (a *App) completeAgentRun(ctx context.Context, userMessage domain.Message, agent domain.Agent, runID string, providerSessionID string, body string) {
	createdAt := time.Now().UTC()
	if !createdAt.After(userMessage.CreatedAt) {
		createdAt = userMessage.CreatedAt.Add(time.Nanosecond)
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
