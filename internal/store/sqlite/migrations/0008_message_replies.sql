-- +goose Up
ALTER TABLE messages ADD COLUMN reply_to_message_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE messages DROP COLUMN reply_to_message_id;
