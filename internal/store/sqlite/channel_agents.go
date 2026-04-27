package sqlite

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type channelAgentRepo struct {
	q queryer
}

func (r channelAgentRepo) ReplaceForChannel(ctx context.Context, channelID string, agents []domain.ChannelAgent) error {
	if _, err := r.q.ExecContext(ctx, `DELETE FROM channel_agents WHERE channel_id = ?`, channelID); err != nil {
		return err
	}
	for _, agent := range agents {
		if agent.ChannelID == "" {
			agent.ChannelID = channelID
		}
		if _, err := r.q.ExecContext(ctx, `
INSERT INTO channel_agents (channel_id, agent_id, run_workspace_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`,
			agent.ChannelID, agent.AgentID, nullableString(agent.RunWorkspaceID),
			formatTime(agent.CreatedAt), formatTime(agent.UpdatedAt),
		); err != nil {
			return err
		}
	}
	return nil
}

func (r channelAgentRepo) ListByChannel(ctx context.Context, channelID string) ([]domain.ChannelAgent, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT channel_id, agent_id, run_workspace_id, created_at, updated_at
FROM channel_agents
WHERE channel_id = ?
ORDER BY created_at ASC, agent_id ASC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.ChannelAgent
	for rows.Next() {
		agent, err := scanChannelAgent(rows)
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

func (r channelAgentRepo) DeleteForAgent(ctx context.Context, agentID string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM channel_agents WHERE agent_id = ?`, agentID)
	return err
}
