-- +goose Up
CREATE TABLE roadmap_stages (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  position INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX roadmap_stages_project_idx ON roadmap_stages(project_id, position);

CREATE TABLE roadmap_tasks (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  stage_id TEXT NOT NULL REFERENCES roadmap_stages(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  completed INTEGER NOT NULL DEFAULT 0,
  position INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX roadmap_tasks_stage_idx ON roadmap_tasks(stage_id, position);

-- +goose Down
DROP INDEX IF EXISTS roadmap_tasks_stage_idx;
DROP TABLE IF EXISTS roadmap_tasks;
DROP INDEX IF EXISTS roadmap_stages_project_idx;
DROP TABLE IF EXISTS roadmap_stages;
