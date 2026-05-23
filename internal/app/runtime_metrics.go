package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

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
		StartedAt:    &metric.StartedAt,
		CompletedAt:  metric.CompletedAt,
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
