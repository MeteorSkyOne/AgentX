package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
)

type metricsRepo struct {
	q queryer
}

func (r metricsRepo) Create(ctx context.Context, metric domain.AgentRunMetric) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_run_metrics (
  run_id, org_id, project_id, channel_id, thread_id, conversation_type, conversation_id,
  message_id, response_message_id, agent_id, agent_name, provider, model, status,
  started_at, first_token_at, completed_at, ttft_ms, duration_ms, tps,
  input_tokens, cached_input_tokens, cache_creation_input_tokens, cache_read_input_tokens,
  output_tokens, reasoning_output_tokens, total_tokens, cache_hit_rate, total_cost_usd,
  raw_usage_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO UPDATE SET
  response_message_id = excluded.response_message_id,
  status = excluded.status,
  first_token_at = excluded.first_token_at,
  completed_at = excluded.completed_at,
  ttft_ms = excluded.ttft_ms,
  duration_ms = excluded.duration_ms,
  tps = excluded.tps,
  input_tokens = excluded.input_tokens,
  cached_input_tokens = excluded.cached_input_tokens,
  cache_creation_input_tokens = excluded.cache_creation_input_tokens,
  cache_read_input_tokens = excluded.cache_read_input_tokens,
  output_tokens = excluded.output_tokens,
  reasoning_output_tokens = excluded.reasoning_output_tokens,
  total_tokens = excluded.total_tokens,
  cache_hit_rate = excluded.cache_hit_rate,
  total_cost_usd = excluded.total_cost_usd,
  raw_usage_json = excluded.raw_usage_json,
  created_at = excluded.created_at`,
		metric.RunID,
		metric.OrganizationID,
		metric.ProjectID,
		metric.ChannelID,
		metric.ThreadID,
		string(metric.ConversationType),
		metric.ConversationID,
		metric.MessageID,
		metric.ResponseMessageID,
		metric.AgentID,
		metric.AgentName,
		metric.Provider,
		metric.Model,
		metric.Status,
		formatTime(metric.StartedAt),
		nullableTime(metric.FirstTokenAt),
		nullableTime(metric.CompletedAt),
		nullableInt64(metric.TTFTMS),
		nullableInt64(metric.DurationMS),
		nullableFloat64(metric.TPS),
		nullableInt64(metric.InputTokens),
		nullableInt64(metric.CachedInputTokens),
		nullableInt64(metric.CacheCreationInputTokens),
		nullableInt64(metric.CacheReadInputTokens),
		nullableInt64(metric.OutputTokens),
		nullableInt64(metric.ReasoningOutputTokens),
		nullableInt64(metric.TotalTokens),
		nullableFloat64(metric.CacheHitRate),
		nullableFloat64(metric.TotalCostUSD),
		metric.RawUsageJSON,
		formatTime(metric.CreatedAt),
	)
	return err
}

func (r metricsRepo) ListByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	rows, err := r.q.QueryContext(ctx, metricsSelectSQL(`
WHERE conversation_type = ? AND conversation_id = ? AND (? = '' OR provider = ?)
ORDER BY started_at DESC
LIMIT ?`),
		string(conversationType), conversationID, metricProvider(filter), metricProvider(filter), metricLimit(filter),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentRunMetrics(rows)
}

func (r metricsRepo) ListByChannel(ctx context.Context, channelID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	rows, err := r.q.QueryContext(ctx, metricsSelectSQL(`
WHERE channel_id = ? AND (? = '' OR provider = ?)
ORDER BY started_at DESC
LIMIT ?`),
		channelID, metricProvider(filter), metricProvider(filter), metricLimit(filter),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentRunMetrics(rows)
}

func (r metricsRepo) ListByProject(ctx context.Context, projectID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	rows, err := r.q.QueryContext(ctx, metricsSelectSQL(`
WHERE project_id = ? AND (? = '' OR provider = ?)
ORDER BY started_at DESC
LIMIT ?`),
		projectID, metricProvider(filter), metricProvider(filter), metricLimit(filter),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentRunMetrics(rows)
}

func (r metricsRepo) ListAgentSummariesByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	rows, err := r.q.QueryContext(ctx, metricsAgentSummarySelectSQL(
		"project_id", "channel_id", "thread_id", "conversation_type", "conversation_id",
		`WHERE conversation_type = ? AND conversation_id = ? AND (? = '' OR provider = ?)
GROUP BY org_id, project_id, channel_id, thread_id, conversation_type, conversation_id, agent_id, provider
ORDER BY MAX(started_at) DESC
LIMIT ?`,
	),
		string(conversationType), conversationID, metricProvider(filter), metricProvider(filter), metricLimit(filter),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentMetricSummaries(rows)
}

func (r metricsRepo) ListAgentSummariesByChannel(ctx context.Context, channelID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	rows, err := r.q.QueryContext(ctx, metricsAgentSummarySelectSQL(
		"project_id", "channel_id", "''", "''", "channel_id",
		`WHERE channel_id = ? AND (? = '' OR provider = ?)
GROUP BY org_id, project_id, channel_id, agent_id, provider
ORDER BY MAX(started_at) DESC
LIMIT ?`,
	),
		channelID, metricProvider(filter), metricProvider(filter), metricLimit(filter),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentMetricSummaries(rows)
}

func (r metricsRepo) ListAgentSummariesByProject(ctx context.Context, projectID string, filter store.MetricsFilter) ([]domain.AgentRunMetric, error) {
	rows, err := r.q.QueryContext(ctx, metricsAgentSummarySelectSQL(
		"project_id", "''", "''", "''", "project_id",
		`WHERE project_id = ? AND (? = '' OR provider = ?)
GROUP BY org_id, project_id, agent_id, provider
ORDER BY MAX(started_at) DESC
LIMIT ?`,
	),
		projectID, metricProvider(filter), metricProvider(filter), metricLimit(filter),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentMetricSummaries(rows)
}

func metricsSelectSQL(where string) string {
	return `
SELECT run_id, org_id, project_id, channel_id, thread_id, conversation_type, conversation_id,
  message_id, response_message_id, agent_id, agent_name, provider, model, status,
  started_at, first_token_at, completed_at, ttft_ms, duration_ms, tps,
  input_tokens, cached_input_tokens, cache_creation_input_tokens, cache_read_input_tokens,
  output_tokens, reasoning_output_tokens, total_tokens, cache_hit_rate, total_cost_usd,
  raw_usage_json, created_at
FROM agent_run_metrics
` + where
}

func metricsAgentSummarySelectSQL(projectExpr string, channelExpr string, threadExpr string, conversationTypeExpr string, conversationIDExpr string, where string) string {
	return `
SELECT '' AS run_id, org_id,
  ` + projectExpr + ` AS project_id,
  ` + channelExpr + ` AS channel_id,
  ` + threadExpr + ` AS thread_id,
  ` + conversationTypeExpr + ` AS conversation_type,
  ` + conversationIDExpr + ` AS conversation_id,
  '' AS message_id, '' AS response_message_id, agent_id,
  COALESCE(NULLIF(MAX(agent_name), ''), agent_id) AS agent_name,
  provider,
  CASE
    WHEN COUNT(DISTINCT COALESCE(NULLIF(model, ''), 'default')) <= 1 THEN COALESCE(NULLIF(MAX(model), ''), 'default')
    ELSE 'mixed'
  END AS model,
  CASE
    WHEN SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) = 0 THEN 'completed'
    WHEN SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) = 0 THEN 'failed'
    ELSE 'mixed'
  END AS status,
  MAX(started_at) AS started_at,
  NULL AS first_token_at,
  MAX(completed_at) AS completed_at,
  CAST(AVG(ttft_ms) AS INTEGER) AS ttft_ms,
  CAST(AVG(duration_ms) AS INTEGER) AS duration_ms,
  CASE
    WHEN SUM(COALESCE(output_tokens, 0)) > 0 AND SUM(
      CASE
        WHEN duration_ms IS NULL THEN 0
        WHEN ttft_ms IS NOT NULL AND duration_ms - ttft_ms >= 250 THEN (duration_ms - ttft_ms) / 1000.0
        ELSE duration_ms / 1000.0
      END
    ) > 0
    THEN SUM(COALESCE(output_tokens, 0)) / SUM(
      CASE
        WHEN duration_ms IS NULL THEN 0
        WHEN ttft_ms IS NOT NULL AND duration_ms - ttft_ms >= 250 THEN (duration_ms - ttft_ms) / 1000.0
        ELSE duration_ms / 1000.0
      END
    )
    ELSE NULL
  END AS tps,
  NULLIF(SUM(COALESCE(input_tokens, 0)), 0) AS input_tokens,
  NULLIF(SUM(CASE
    WHEN COALESCE(cache_read_input_tokens, 0) > COALESCE(cached_input_tokens, 0) THEN COALESCE(cache_read_input_tokens, 0)
    ELSE COALESCE(cached_input_tokens, 0)
  END), 0) AS cached_input_tokens,
  NULLIF(SUM(COALESCE(cache_creation_input_tokens, 0)), 0) AS cache_creation_input_tokens,
  NULLIF(SUM(COALESCE(cache_read_input_tokens, 0)), 0) AS cache_read_input_tokens,
  NULLIF(SUM(COALESCE(output_tokens, 0)), 0) AS output_tokens,
  NULLIF(SUM(COALESCE(reasoning_output_tokens, 0)), 0) AS reasoning_output_tokens,
  NULLIF(SUM(COALESCE(total_tokens, 0)), 0) AS total_tokens,
  NULL AS cache_hit_rate,
  NULLIF(SUM(COALESCE(total_cost_usd, 0)), 0) AS total_cost_usd,
  '' AS raw_usage_json,
  MAX(created_at) AS created_at,
  COUNT(*) AS run_count,
  SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed_runs,
  SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_runs
FROM agent_run_metrics
` + where
}

func metricProvider(filter store.MetricsFilter) string {
	return strings.TrimSpace(filter.Provider)
}

func metricLimit(filter store.MetricsFilter) int {
	if filter.Limit <= 0 {
		return 100
	}
	if filter.Limit > 500 {
		return 500
	}
	return filter.Limit
}

func scanAgentRunMetrics(rows *sql.Rows) ([]domain.AgentRunMetric, error) {
	var metrics []domain.AgentRunMetric
	for rows.Next() {
		metric, err := scanAgentRunMetric(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return metrics, nil
}

func scanAgentMetricSummaries(rows *sql.Rows) ([]domain.AgentRunMetric, error) {
	var metrics []domain.AgentRunMetric
	for rows.Next() {
		metric, err := scanAgentMetricSummary(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return metrics, nil
}

func scanAgentRunMetric(scanner interface {
	Scan(dest ...any) error
}) (domain.AgentRunMetric, error) {
	var metric domain.AgentRunMetric
	var conversationType string
	var startedAt, createdAt string
	var firstTokenAt, completedAt sql.NullString
	var ttftMS, durationMS sql.NullInt64
	var tps, cacheHitRate, totalCostUSD sql.NullFloat64
	var inputTokens, cachedInputTokens, cacheCreationInputTokens, cacheReadInputTokens sql.NullInt64
	var outputTokens, reasoningOutputTokens, totalTokens sql.NullInt64
	if err := scanner.Scan(
		&metric.RunID, &metric.OrganizationID, &metric.ProjectID, &metric.ChannelID, &metric.ThreadID,
		&conversationType, &metric.ConversationID, &metric.MessageID, &metric.ResponseMessageID,
		&metric.AgentID, &metric.AgentName, &metric.Provider, &metric.Model, &metric.Status,
		&startedAt, &firstTokenAt, &completedAt, &ttftMS, &durationMS, &tps,
		&inputTokens, &cachedInputTokens, &cacheCreationInputTokens, &cacheReadInputTokens,
		&outputTokens, &reasoningOutputTokens, &totalTokens, &cacheHitRate, &totalCostUSD,
		&metric.RawUsageJSON, &createdAt,
	); err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.ConversationType = domain.ConversationType(conversationType)
	var err error
	metric.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.FirstTokenAt, err = parseNullableTime(firstTokenAt)
	if err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.CompletedAt, err = parseNullableTime(completedAt)
	if err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.TTFTMS = ptrInt64(ttftMS)
	metric.DurationMS = ptrInt64(durationMS)
	metric.TPS = ptrFloat64(tps)
	metric.InputTokens = ptrInt64(inputTokens)
	metric.CachedInputTokens = ptrInt64(cachedInputTokens)
	metric.CacheCreationInputTokens = ptrInt64(cacheCreationInputTokens)
	metric.CacheReadInputTokens = ptrInt64(cacheReadInputTokens)
	metric.OutputTokens = ptrInt64(outputTokens)
	metric.ReasoningOutputTokens = ptrInt64(reasoningOutputTokens)
	metric.TotalTokens = ptrInt64(totalTokens)
	metric.CacheHitRate = ptrFloat64(cacheHitRate)
	metric.TotalCostUSD = ptrFloat64(totalCostUSD)
	return metric, nil
}

func scanAgentMetricSummary(scanner interface {
	Scan(dest ...any) error
}) (domain.AgentRunMetric, error) {
	var metric domain.AgentRunMetric
	var conversationType string
	var startedAt, createdAt string
	var firstTokenAt, completedAt sql.NullString
	var ttftMS, durationMS sql.NullInt64
	var tps, cacheHitRate, totalCostUSD sql.NullFloat64
	var inputTokens, cachedInputTokens, cacheCreationInputTokens, cacheReadInputTokens sql.NullInt64
	var outputTokens, reasoningOutputTokens, totalTokens sql.NullInt64
	if err := scanner.Scan(
		&metric.RunID, &metric.OrganizationID, &metric.ProjectID, &metric.ChannelID, &metric.ThreadID,
		&conversationType, &metric.ConversationID, &metric.MessageID, &metric.ResponseMessageID,
		&metric.AgentID, &metric.AgentName, &metric.Provider, &metric.Model, &metric.Status,
		&startedAt, &firstTokenAt, &completedAt, &ttftMS, &durationMS, &tps,
		&inputTokens, &cachedInputTokens, &cacheCreationInputTokens, &cacheReadInputTokens,
		&outputTokens, &reasoningOutputTokens, &totalTokens, &cacheHitRate, &totalCostUSD,
		&metric.RawUsageJSON, &createdAt, &metric.RunCount, &metric.CompletedRuns, &metric.FailedRuns,
	); err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.ConversationType = domain.ConversationType(conversationType)
	var err error
	metric.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.FirstTokenAt, err = parseNullableTime(firstTokenAt)
	if err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.CompletedAt, err = parseNullableTime(completedAt)
	if err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.AgentRunMetric{}, err
	}
	metric.TTFTMS = ptrInt64(ttftMS)
	metric.DurationMS = ptrInt64(durationMS)
	metric.TPS = ptrFloat64(tps)
	metric.InputTokens = ptrInt64(inputTokens)
	metric.CachedInputTokens = ptrInt64(cachedInputTokens)
	metric.CacheCreationInputTokens = ptrInt64(cacheCreationInputTokens)
	metric.CacheReadInputTokens = ptrInt64(cacheReadInputTokens)
	metric.OutputTokens = ptrInt64(outputTokens)
	metric.ReasoningOutputTokens = ptrInt64(reasoningOutputTokens)
	metric.TotalTokens = ptrInt64(totalTokens)
	metric.CacheHitRate = ptrFloat64(cacheHitRate)
	metric.TotalCostUSD = ptrFloat64(totalCostUSD)
	return metric, nil
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func ptrInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func ptrFloat64(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}
