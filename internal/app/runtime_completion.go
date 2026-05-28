package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

func (a *App) completeAgentRun(ctx context.Context, userMessage domain.Message, agent domain.Agent, runID string, providerSessionID string, body string, thinking string, process []domain.ProcessItem, metric domain.AgentRunMetric, usage *agentruntime.Usage, team *domain.TeamMetadata) (domain.Message, error) {
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
	if err := a.setAgentSessionContextUsage(ctx, agent.ID, userMessage.ConversationType, userMessage.ConversationID, usage); err != nil {
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
	if team == nil {
		a.dispatchBotMentionedAgents(context.WithoutCancel(ctx), botMessage, agent.ID)
	}
	return botMessage, nil
}

func (a *App) dispatchBotMentionedAgents(ctx context.Context, botMessage domain.Message, senderAgentID string) {
	mentions := agentMentions(botMessage.Body)
	if len(mentions) == 0 {
		return
	}
	scope, err := a.conversationScope(ctx, botMessage.ConversationType, botMessage.ConversationID)
	if err != nil {
		slog.Warn("failed to resolve scope for bot mention dispatch", "error", err)
		return
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		slog.Warn("failed to resolve agents for bot mention dispatch", "error", err)
		return
	}
	targets := mentionedAgentsForBody(agents, botMessage.Body)
	for _, target := range targets {
		if target.Agent.ID == senderAgentID {
			continue
		}
		a.dispatchAgentRunOrQueue(ctx, botMessage, target)
	}
}

func (a *App) setFailedAgentSession(ctx context.Context, agentID string, message domain.Message, providerSessionID string) {
	_ = a.store.Sessions().SetAgentSession(ctx, agentID, message.ConversationType, message.ConversationID, providerSessionID, "failed")
}

func (a *App) setCanceledAgentSession(ctx context.Context, agentID string, message domain.Message, providerSessionID string) {
	_ = a.store.Sessions().SetAgentSession(ctx, agentID, message.ConversationType, message.ConversationID, providerSessionID, "canceled")
}

func (a *App) persistAgentSessionContextUsage(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, usage *agentruntime.Usage) {
	_ = a.setAgentSessionContextUsage(ctx, agentID, conversationType, conversationID, usage)
}

func (a *App) setAgentSessionContextUsage(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, usage *agentruntime.Usage) error {
	if usage == nil || usage.Context == nil {
		return nil
	}
	return a.store.Sessions().SetAgentSessionContextUsage(ctx, agentID, conversationType, conversationID, contextUsageToDomain(usage.Context))
}
