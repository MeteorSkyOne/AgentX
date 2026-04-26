-- +goose Up
ALTER TABLE agents ADD COLUMN fast_mode INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE agents DROP COLUMN fast_mode;
