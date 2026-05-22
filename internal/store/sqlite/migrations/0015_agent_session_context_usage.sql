-- +goose Up
ALTER TABLE agent_sessions ADD COLUMN context_usage_json TEXT;
ALTER TABLE agent_sessions ADD COLUMN context_usage_updated_at TEXT;

-- +goose Down
ALTER TABLE agent_sessions DROP COLUMN context_usage_updated_at;
ALTER TABLE agent_sessions DROP COLUMN context_usage_json;
