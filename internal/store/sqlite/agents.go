package sqlite

import (
	"context"
	"encoding/json"

	"github.com/meteorsky/agentx/internal/domain"
)

type agentRepo struct {
	q queryer
}

func (r agentRepo) Create(ctx context.Context, agent domain.Agent) error {
	if !agent.Enabled {
		agent.Enabled = true
	}
	agent = normalizeAgent(agent)
	envJSON, err := json.Marshal(emptyMapIfNil(agent.Env))
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `
	INSERT INTO agents (id, org_id, bot_user_id, kind, name, handle, description, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.ID, agent.OrganizationID, agent.BotUserID, agent.Kind, agent.Name, agent.Handle, agent.Description, agent.Model, agent.Effort,
		agent.DefaultWorkspaceID, agent.ConfigWorkspaceID, boolToInt(agent.Enabled), boolToInt(agent.FastMode), boolToInt(agent.YoloMode), string(envJSON),
		formatTime(agent.CreatedAt), formatTime(agent.UpdatedAt),
	)
	return err
}

func (r agentRepo) ByID(ctx context.Context, id string) (domain.Agent, error) {
	return scanAgent(r.q.QueryRowContext(ctx, `
	SELECT id, org_id, bot_user_id, kind, name, handle, description, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at
	FROM agents
	WHERE id = ?`, id))
}

func (r agentRepo) DefaultForOrganization(ctx context.Context, orgID string) (domain.Agent, error) {
	return scanAgent(r.q.QueryRowContext(ctx, `
	SELECT id, org_id, bot_user_id, kind, name, handle, description, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at
	FROM agents
	WHERE org_id = ?
	ORDER BY created_at ASC
LIMIT 1`, orgID))
}

func (r agentRepo) ListByOrganization(ctx context.Context, orgID string) ([]domain.Agent, error) {
	rows, err := r.q.QueryContext(ctx, `
	SELECT id, org_id, bot_user_id, kind, name, handle, description, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at
	FROM agents
	WHERE org_id = ?
	ORDER BY created_at ASC, id ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
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

func (r agentRepo) ByHandle(ctx context.Context, orgID string, handle string) (domain.Agent, error) {
	return scanAgent(r.q.QueryRowContext(ctx, `
	SELECT id, org_id, bot_user_id, kind, name, handle, description, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at
	FROM agents
	WHERE org_id = ? AND handle = ?`, orgID, handle))
}

func (r agentRepo) Update(ctx context.Context, agent domain.Agent) error {
	agent = normalizeAgent(agent)
	envJSON, err := json.Marshal(emptyMapIfNil(agent.Env))
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `
UPDATE agents
	SET kind = ?, name = ?, handle = ?, description = ?, model = ?, effort = ?, default_workspace_id = ?, config_workspace_id = ?, enabled = ?, fast_mode = ?, yolo_mode = ?, env_json = ?, updated_at = ?
	WHERE id = ?`,
		agent.Kind, agent.Name, agent.Handle, agent.Description, agent.Model, agent.Effort, agent.DefaultWorkspaceID, agent.ConfigWorkspaceID,
		boolToInt(agent.Enabled), boolToInt(agent.FastMode), boolToInt(agent.YoloMode), string(envJSON), formatTime(agent.UpdatedAt), agent.ID,
	)
	return err
}
