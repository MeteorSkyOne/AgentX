package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type sessionRepo struct {
	q queryer
}

func (r sessionRepo) SetAgentSession(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, providerSessionID string, status string) error {
	id := agentID + ":" + string(conversationType) + ":" + conversationID
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_sessions (id, agent_id, conversation_type, conversation_id, provider_session_id, status, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_id, conversation_type, conversation_id) DO UPDATE SET
  provider_session_id = excluded.provider_session_id,
  status = excluded.status,
  updated_at = excluded.updated_at`,
		id, agentID, string(conversationType), conversationID, providerSessionID, status, formatTime(time.Now().UTC()),
	)
	return err
}

func (r sessionRepo) ResetAgentSessionContext(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, contextStartedAt time.Time) error {
	id := agentID + ":" + string(conversationType) + ":" + conversationID
	now := time.Now().UTC()
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_sessions (id, agent_id, conversation_type, conversation_id, provider_session_id, status, context_started_at, context_usage_json, context_usage_updated_at, updated_at)
VALUES (?, ?, ?, ?, '', 'reset', ?, NULL, NULL, ?)
ON CONFLICT(agent_id, conversation_type, conversation_id) DO UPDATE SET
  provider_session_id = '',
  status = 'reset',
  context_started_at = excluded.context_started_at,
  context_usage_json = NULL,
  context_usage_updated_at = NULL,
  updated_at = excluded.updated_at`,
		id, agentID, string(conversationType), conversationID, formatTime(contextStartedAt), formatTime(now),
	)
	return err
}

func (r sessionRepo) SetAgentSessionContextStartedAt(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, contextStartedAt time.Time) error {
	id := agentID + ":" + string(conversationType) + ":" + conversationID
	now := time.Now().UTC()
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_sessions (id, agent_id, conversation_type, conversation_id, provider_session_id, status, context_started_at, context_usage_json, context_usage_updated_at, updated_at)
VALUES (?, ?, ?, ?, '', 'completed', ?, NULL, NULL, ?)
ON CONFLICT(agent_id, conversation_type, conversation_id) DO UPDATE SET
  context_started_at = excluded.context_started_at,
  context_usage_json = NULL,
  context_usage_updated_at = NULL,
  updated_at = excluded.updated_at`,
		id, agentID, string(conversationType), conversationID, formatTime(contextStartedAt), formatTime(now),
	)
	return err
}

func (r sessionRepo) SetAgentSessionContextUsage(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, usage *domain.ContextUsage) error {
	id := agentID + ":" + string(conversationType) + ":" + conversationID
	now := time.Now().UTC()
	var usageJSON sql.NullString
	var usageUpdatedAt sql.NullString
	if usage != nil {
		content, err := json.Marshal(usage)
		if err != nil {
			return err
		}
		usageJSON = sql.NullString{String: string(content), Valid: true}
		usageUpdatedAt = sql.NullString{String: formatTime(now), Valid: true}
	}
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_sessions (id, agent_id, conversation_type, conversation_id, provider_session_id, status, context_usage_json, context_usage_updated_at, updated_at)
VALUES (?, ?, ?, ?, '', 'completed', ?, ?, ?)
ON CONFLICT(agent_id, conversation_type, conversation_id) DO UPDATE SET
  context_usage_json = excluded.context_usage_json,
  context_usage_updated_at = excluded.context_usage_updated_at,
  updated_at = excluded.updated_at`,
		id, agentID, string(conversationType), conversationID, usageJSON, usageUpdatedAt, formatTime(now),
	)
	return err
}

func (r sessionRepo) ByConversation(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string) (domain.AgentSession, error) {
	return scanAgentSession(r.q.QueryRowContext(ctx, `
SELECT agent_id, conversation_type, conversation_id, provider_session_id, status, context_started_at, context_usage_json, context_usage_updated_at, updated_at
FROM agent_sessions
WHERE agent_id = ? AND conversation_type = ? AND conversation_id = ?`,
		agentID, string(conversationType), conversationID,
	))
}
