package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
)

func (a *App) ConversationMetrics(ctx context.Context, conversationType domain.ConversationType, conversationID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	var metrics []domain.AgentRunMetric
	var err error
	if filter.Group == "agent" {
		metrics, err = a.store.Metrics().ListAgentSummariesByConversation(ctx, conversationType, conversationID, filter)
	} else {
		metrics, err = a.store.Metrics().ListByConversation(ctx, conversationType, conversationID, filter)
	}
	if err != nil {
		return nil, err
	}
	return a.hydrateAgentRunMetricScopes(ctx, normalizeAgentRunMetrics(metrics))
}

func (a *App) ChannelMetrics(ctx context.Context, channelID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	var metrics []domain.AgentRunMetric
	var err error
	if filter.Group == "agent" {
		metrics, err = a.store.Metrics().ListAgentSummariesByChannel(ctx, channelID, filter)
	} else {
		metrics, err = a.store.Metrics().ListByChannel(ctx, channelID, filter)
	}
	if err != nil {
		return nil, err
	}
	return a.hydrateAgentRunMetricScopes(ctx, normalizeAgentRunMetrics(metrics))
}

func (a *App) ProjectMetrics(ctx context.Context, projectID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	var metrics []domain.AgentRunMetric
	var err error
	if filter.Group == "agent" {
		metrics, err = a.store.Metrics().ListAgentSummariesByProject(ctx, projectID, filter)
	} else {
		metrics, err = a.store.Metrics().ListByProject(ctx, projectID, filter)
	}
	if err != nil {
		return nil, err
	}
	return a.hydrateAgentRunMetricScopes(ctx, normalizeAgentRunMetrics(metrics))
}

func normalizeAgentRunMetrics(metrics []domain.AgentRunMetric) []domain.AgentRunMetric {
	if len(metrics) == 0 {
		return metrics
	}
	for i := range metrics {
		if rate := metricCacheHitRate(metrics[i]); rate != nil {
			metrics[i].CacheHitRate = rate
			continue
		}
		if metrics[i].CacheHitRate != nil {
			rate := *metrics[i].CacheHitRate
			if rate < 0 {
				rate = 0
			}
			if rate > 1 {
				rate = 1
			}
			metrics[i].CacheHitRate = &rate
		}
	}
	return metrics
}

func (a *App) hydrateAgentRunMetricScopes(ctx context.Context, metrics []domain.AgentRunMetric) ([]domain.AgentRunMetric, error) {
	projects := map[string]domain.Project{}
	channels := map[string]domain.Channel{}
	threads := map[string]domain.Thread{}
	for i := range metrics {
		if id := metrics[i].ProjectID; id != "" {
			project, ok, err := a.cachedMetricProject(ctx, projects, id)
			if err != nil {
				return nil, err
			}
			if ok {
				metrics[i].ProjectName = project.Name
			}
		}
		if id := metrics[i].ChannelID; id != "" {
			channel, ok, err := a.cachedMetricChannel(ctx, channels, id)
			if err != nil {
				return nil, err
			}
			if ok {
				metrics[i].ChannelName = channel.Name
			}
		}
		if id := metrics[i].ThreadID; id != "" {
			thread, ok, err := a.cachedMetricThread(ctx, threads, id)
			if err != nil {
				return nil, err
			}
			if ok {
				metrics[i].ThreadTitle = thread.Title
			}
		}
	}
	return metrics, nil
}

func (a *App) cachedMetricProject(ctx context.Context, cache map[string]domain.Project, id string) (domain.Project, bool, error) {
	if project, ok := cache[id]; ok {
		return project, true, nil
	}
	project, err := a.store.Projects().ByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, false, nil
		}
		return domain.Project{}, false, err
	}
	cache[id] = project
	return project, true, nil
}

func (a *App) cachedMetricChannel(ctx context.Context, cache map[string]domain.Channel, id string) (domain.Channel, bool, error) {
	if channel, ok := cache[id]; ok {
		return channel, true, nil
	}
	channel, err := a.store.Channels().ByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Channel{}, false, nil
		}
		return domain.Channel{}, false, err
	}
	cache[id] = channel
	return channel, true, nil
}

func (a *App) cachedMetricThread(ctx context.Context, cache map[string]domain.Thread, id string) (domain.Thread, bool, error) {
	if thread, ok := cache[id]; ok {
		return thread, true, nil
	}
	thread, err := a.store.Threads().ByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Thread{}, false, nil
		}
		return domain.Thread{}, false, err
	}
	cache[id] = thread
	return thread, true, nil
}
