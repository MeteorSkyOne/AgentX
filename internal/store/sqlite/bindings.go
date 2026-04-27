package sqlite

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type bindingRepo struct {
	q queryer
}

func (r bindingRepo) Upsert(ctx context.Context, binding domain.ConversationBinding) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO conversation_bindings (id, org_id, conversation_type, conversation_id, agent_id, workspace_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(conversation_type, conversation_id) DO UPDATE SET
  org_id = excluded.org_id,
  agent_id = excluded.agent_id,
  workspace_id = excluded.workspace_id,
  updated_at = excluded.updated_at`,
		binding.ID, binding.OrganizationID, string(binding.ConversationType), binding.ConversationID,
		binding.AgentID, binding.WorkspaceID, formatTime(binding.CreatedAt), formatTime(binding.UpdatedAt),
	)
	return err
}

func (r bindingRepo) ByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string) (domain.ConversationBinding, error) {
	return scanBinding(r.q.QueryRowContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, agent_id, workspace_id, created_at, updated_at
FROM conversation_bindings
WHERE conversation_type = ? AND conversation_id = ?`, string(conversationType), conversationID))
}
