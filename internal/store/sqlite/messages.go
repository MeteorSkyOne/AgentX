package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type messageRepo struct {
	q queryer
}

func (r messageRepo) Create(ctx context.Context, message domain.Message) error {
	metadataJSON, err := json.Marshal(emptyMapIfNil(message.Metadata))
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `
INSERT INTO messages (id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, reply_to_message_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		message.ID, message.OrganizationID, string(message.ConversationType), message.ConversationID,
		string(message.SenderType), message.SenderID, string(message.Kind), message.Body, string(metadataJSON), message.ReplyToMessageID, formatTime(message.CreatedAt),
	)
	return err
}

func (r messageRepo) ByID(ctx context.Context, id string) (domain.Message, error) {
	return scanMessage(r.q.QueryRowContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, reply_to_message_id, created_at
FROM messages
WHERE id = ?`, id))
}

func (r messageRepo) Update(ctx context.Context, message domain.Message) error {
	metadataJSON, err := json.Marshal(emptyMapIfNil(message.Metadata))
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `
UPDATE messages
SET body = ?, metadata_json = ?
WHERE id = ?`,
		message.Body, string(metadataJSON), message.ID,
	)
	return err
}

func (r messageRepo) Delete(ctx context.Context, id string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, id)
	return err
}

func (r messageRepo) List(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, reply_to_message_id, created_at
FROM messages
WHERE conversation_type = ? AND conversation_id = ?
ORDER BY created_at ASC
LIMIT ?`, string(conversationType), conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows)
}

func (r messageRepo) ListRecent(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, reply_to_message_id, created_at
FROM messages
WHERE conversation_type = ? AND conversation_id = ?
ORDER BY created_at DESC
LIMIT ?`, string(conversationType), conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (r messageRepo) ListRecentBefore(ctx context.Context, conversationType domain.ConversationType, conversationID string, before time.Time, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, reply_to_message_id, created_at
FROM messages
WHERE conversation_type = ? AND conversation_id = ? AND created_at < ?
ORDER BY created_at DESC
LIMIT ?`, string(conversationType), conversationID, formatTime(before), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}
