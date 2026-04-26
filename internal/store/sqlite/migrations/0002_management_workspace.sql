-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

ALTER TABLE channels ADD COLUMN project_id TEXT;
ALTER TABLE channels ADD COLUMN type TEXT NOT NULL DEFAULT 'text';
ALTER TABLE channels ADD COLUMN updated_at TEXT;
ALTER TABLE channels ADD COLUMN archived_at TEXT;

ALTER TABLE agents ADD COLUMN handle TEXT;
ALTER TABLE agents ADD COLUMN config_workspace_id TEXT;
ALTER TABLE agents ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1;

INSERT OR IGNORE INTO workspaces (id, org_id, type, name, path, created_by, created_at, updated_at)
SELECT
  'wks_project_' || organizations.id,
  organizations.id,
  'project',
  'Default Workspace',
  '.agentx/projects/' || organizations.id,
  (
    SELECT memberships.user_id
    FROM memberships
    WHERE memberships.org_id = organizations.id
    ORDER BY memberships.created_at ASC, memberships.user_id ASC
    LIMIT 1
  ),
  organizations.created_at,
  organizations.created_at
FROM organizations
WHERE EXISTS (
  SELECT 1 FROM memberships WHERE memberships.org_id = organizations.id
);

INSERT OR IGNORE INTO projects (id, org_id, name, workspace_id, created_by, created_at, updated_at)
SELECT
  'prj_default_' || organizations.id,
  organizations.id,
  'Default',
  'wks_project_' || organizations.id,
  (
    SELECT memberships.user_id
    FROM memberships
    WHERE memberships.org_id = organizations.id
    ORDER BY memberships.created_at ASC, memberships.user_id ASC
    LIMIT 1
  ),
  organizations.created_at,
  organizations.created_at
FROM organizations
WHERE EXISTS (
  SELECT 1 FROM memberships WHERE memberships.org_id = organizations.id
);

UPDATE channels
SET
  project_id = COALESCE(project_id, 'prj_default_' || org_id),
  updated_at = COALESCE(updated_at, created_at),
  type = COALESCE(NULLIF(type, ''), 'text');

UPDATE agents
SET
  handle = COALESCE(handle, 'agent_' || lower(substr(id, 5))),
  config_workspace_id = COALESCE(config_workspace_id, default_workspace_id),
  enabled = COALESCE(enabled, 1);

CREATE UNIQUE INDEX agents_org_handle_idx
  ON agents(org_id, handle);

CREATE TABLE threads (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT
);

CREATE INDEX threads_channel_updated_idx
  ON threads(channel_id, archived_at, updated_at);

CREATE TABLE channel_agents (
  channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
  agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  run_workspace_id TEXT REFERENCES workspaces(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (channel_id, agent_id)
);

INSERT OR IGNORE INTO channel_agents (channel_id, agent_id, run_workspace_id, created_at, updated_at)
SELECT conversation_id, agent_id, workspace_id, created_at, updated_at
FROM conversation_bindings
WHERE conversation_type = 'channel';

CREATE INDEX channel_agents_agent_idx
  ON channel_agents(agent_id);

CREATE INDEX channels_project_archived_idx
  ON channels(project_id, archived_at, created_at);

CREATE INDEX messages_org_conversation_created_idx
  ON messages(org_id, conversation_type, conversation_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS messages_org_conversation_created_idx;
DROP INDEX IF EXISTS channels_project_archived_idx;
DROP INDEX IF EXISTS channel_agents_agent_idx;
DROP TABLE IF EXISTS channel_agents;
DROP INDEX IF EXISTS threads_channel_updated_idx;
DROP TABLE IF EXISTS threads;
DROP INDEX IF EXISTS agents_org_handle_idx;
DROP TABLE IF EXISTS projects;
