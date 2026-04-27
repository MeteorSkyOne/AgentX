-- +goose Up
CREATE TABLE user_preferences (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  show_ttft INTEGER NOT NULL DEFAULT 1,
  show_tps INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE agent_run_metrics (
  run_id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL DEFAULT '',
  channel_id TEXT NOT NULL DEFAULT '',
  thread_id TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  message_id TEXT NOT NULL,
  response_message_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  agent_name TEXT NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  first_token_at TEXT,
  completed_at TEXT,
  ttft_ms INTEGER,
  duration_ms INTEGER,
  tps REAL,
  input_tokens INTEGER,
  cached_input_tokens INTEGER,
  cache_creation_input_tokens INTEGER,
  cache_read_input_tokens INTEGER,
  output_tokens INTEGER,
  reasoning_output_tokens INTEGER,
  total_tokens INTEGER,
  cache_hit_rate REAL,
  total_cost_usd REAL,
  raw_usage_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX agent_run_metrics_conversation_idx
  ON agent_run_metrics(conversation_type, conversation_id, started_at);

CREATE INDEX agent_run_metrics_channel_idx
  ON agent_run_metrics(channel_id, started_at);

CREATE INDEX agent_run_metrics_project_idx
  ON agent_run_metrics(project_id, started_at);

CREATE INDEX agent_run_metrics_provider_idx
  ON agent_run_metrics(provider, started_at);

-- +goose Down
DROP INDEX IF EXISTS agent_run_metrics_provider_idx;
DROP INDEX IF EXISTS agent_run_metrics_project_idx;
DROP INDEX IF EXISTS agent_run_metrics_channel_idx;
DROP INDEX IF EXISTS agent_run_metrics_conversation_idx;
DROP TABLE IF EXISTS agent_run_metrics;
DROP TABLE IF EXISTS user_preferences;
