-- +goose Up
CREATE TABLE scheduled_tasks (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  schedule TEXT NOT NULL,
  timezone TEXT NOT NULL,
  conversation_type TEXT,
  conversation_id TEXT,
  agent_id TEXT REFERENCES agents(id) ON DELETE SET NULL,
  workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
  prompt TEXT NOT NULL DEFAULT '',
  command TEXT NOT NULL DEFAULT '',
  timeout_seconds INTEGER NOT NULL DEFAULT 600,
  created_by TEXT NOT NULL REFERENCES users(id),
  last_run_id TEXT,
  last_run_status TEXT,
  last_run_at TEXT,
  last_finished_at TEXT,
  next_run_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX scheduled_tasks_project_idx
  ON scheduled_tasks(project_id, created_at);

CREATE INDEX scheduled_tasks_enabled_idx
  ON scheduled_tasks(enabled, next_run_at);

CREATE TABLE scheduled_task_runs (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  trigger TEXT NOT NULL,
  scheduled_for TEXT,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  exit_code INTEGER,
  stdout TEXT NOT NULL DEFAULT '',
  stderr TEXT NOT NULL DEFAULT '',
  output_truncated INTEGER NOT NULL DEFAULT 0,
  message_id TEXT REFERENCES messages(id) ON DELETE SET NULL
);

CREATE INDEX scheduled_task_runs_task_started_idx
  ON scheduled_task_runs(task_id, started_at);

-- +goose Down
DROP INDEX IF EXISTS scheduled_task_runs_task_started_idx;
DROP TABLE IF EXISTS scheduled_task_runs;
DROP INDEX IF EXISTS scheduled_tasks_enabled_idx;
DROP INDEX IF EXISTS scheduled_tasks_project_idx;
DROP TABLE IF EXISTS scheduled_tasks;
