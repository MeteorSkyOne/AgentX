package sqlite

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type messageAttachmentRepo struct {
	q queryer
}

func (r messageAttachmentRepo) Create(ctx context.Context, attachment domain.MessageAttachment) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO message_attachments (
  id, message_id, org_id, conversation_type, conversation_id, filename,
  content_type, kind, size_bytes, storage_path, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attachment.ID, attachment.MessageID, attachment.OrganizationID,
		string(attachment.ConversationType), attachment.ConversationID, attachment.Filename,
		attachment.ContentType, string(attachment.Kind), attachment.SizeBytes,
		attachment.StoragePath, formatTime(attachment.CreatedAt),
	)
	return err
}

func (r messageAttachmentRepo) ByID(ctx context.Context, id string) (domain.MessageAttachment, error) {
	return scanMessageAttachment(r.q.QueryRowContext(ctx, `
SELECT id, message_id, org_id, conversation_type, conversation_id, filename, content_type, kind, size_bytes, storage_path, created_at
FROM message_attachments
WHERE id = ?`, id))
}

func (r messageAttachmentRepo) ListByMessage(ctx context.Context, messageID string) ([]domain.MessageAttachment, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, message_id, org_id, conversation_type, conversation_id, filename, content_type, kind, size_bytes, storage_path, created_at
FROM message_attachments
WHERE message_id = ?
ORDER BY created_at ASC, id ASC`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessageAttachments(rows)
}

func (r messageAttachmentRepo) DeleteByMessage(ctx context.Context, messageID string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM message_attachments WHERE message_id = ?`, messageID)
	return err
}
