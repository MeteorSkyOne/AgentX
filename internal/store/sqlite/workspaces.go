package sqlite

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type workspaceRepo struct {
	q queryer
}

func (r workspaceRepo) Create(ctx context.Context, workspace domain.Workspace) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO workspaces (id, org_id, type, name, path, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		workspace.ID, workspace.OrganizationID, workspace.Type, workspace.Name, workspace.Path,
		workspace.CreatedBy, formatTime(workspace.CreatedAt), formatTime(workspace.UpdatedAt),
	)
	return err
}

func (r workspaceRepo) ByID(ctx context.Context, id string) (domain.Workspace, error) {
	return scanWorkspace(r.q.QueryRowContext(ctx, `
SELECT id, org_id, type, name, path, created_by, created_at, updated_at
FROM workspaces
WHERE id = ?`, id))
}

func (r workspaceRepo) Update(ctx context.Context, workspace domain.Workspace) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE workspaces SET name = ?, path = ?, updated_at = ? WHERE id = ?`,
		workspace.Name, workspace.Path, formatTime(workspace.UpdatedAt), workspace.ID,
	)
	return err
}
