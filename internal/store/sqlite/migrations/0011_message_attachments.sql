-- +goose Up
CREATE TABLE message_attachments (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  conversation_type TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  filename TEXT NOT NULL,
  content_type TEXT NOT NULL,
  kind TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  storage_path TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX message_attachments_message_idx
  ON message_attachments(message_id, created_at);

CREATE INDEX message_attachments_conversation_idx
  ON message_attachments(conversation_type, conversation_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS message_attachments;
