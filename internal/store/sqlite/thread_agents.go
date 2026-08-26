package sqlite

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type threadAgentRepo struct {
	q queryer
}

func (r threadAgentRepo) ReplaceForThread(ctx context.Context, threadID string, agents []domain.ThreadAgent) error {
	if _, err := r.q.ExecContext(ctx, `DELETE FROM thread_agents WHERE thread_id = ?`, threadID); err != nil {
		return err
	}
	for _, agent := range agents {
		if agent.ThreadID == "" {
			agent.ThreadID = threadID
		}
		if _, err := r.q.ExecContext(ctx, `
INSERT INTO thread_agents (thread_id, agent_id, created_at)
VALUES (?, ?, ?)`,
			agent.ThreadID, agent.AgentID, formatTime(agent.CreatedAt),
		); err != nil {
			return err
		}
	}
	return nil
}

func (r threadAgentRepo) ListByThread(ctx context.Context, threadID string) ([]domain.ThreadAgent, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT thread_id, agent_id, created_at
FROM thread_agents
WHERE thread_id = ?
ORDER BY created_at ASC, agent_id ASC`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.ThreadAgent
	for rows.Next() {
		agent, err := scanThreadAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

func (r threadAgentRepo) DeleteForAgent(ctx context.Context, agentID string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM thread_agents WHERE agent_id = ?`, agentID)
	return err
}
