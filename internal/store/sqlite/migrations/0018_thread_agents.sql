-- +goose Up
CREATE TABLE thread_agents (
  thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (thread_id, agent_id)
);

CREATE INDEX thread_agents_agent_idx ON thread_agents(agent_id);

-- +goose Down
DROP INDEX IF EXISTS thread_agents_agent_idx;
DROP TABLE IF EXISTS thread_agents;
