-- +goose Up
ALTER TABLE agents ADD COLUMN effort TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_sessions ADD COLUMN context_started_at TEXT;

-- +goose Down
ALTER TABLE agent_sessions DROP COLUMN context_started_at;
ALTER TABLE agents DROP COLUMN effort;
