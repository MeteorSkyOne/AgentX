package sqlite

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type projectRepo struct {
	q queryer
}

func (r projectRepo) Create(ctx context.Context, project domain.Project) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO projects (id, org_id, name, workspace_id, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		project.ID, project.OrganizationID, project.Name, project.WorkspaceID, project.CreatedBy,
		formatTime(project.CreatedAt), formatTime(project.UpdatedAt),
	)
	return err
}

func (r projectRepo) ListByOrganization(ctx context.Context, orgID string) ([]domain.Project, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, name, workspace_id, created_by, created_at, updated_at
FROM projects
WHERE org_id = ?
ORDER BY created_at ASC, id ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

func (r projectRepo) ByID(ctx context.Context, id string) (domain.Project, error) {
	return scanProject(r.q.QueryRowContext(ctx, `
SELECT id, org_id, name, workspace_id, created_by, created_at, updated_at
FROM projects
WHERE id = ?`, id))
}

func (r projectRepo) Update(ctx context.Context, project domain.Project) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE projects
SET name = ?, workspace_id = ?, updated_at = ?
WHERE id = ?`,
		project.Name, project.WorkspaceID, formatTime(project.UpdatedAt), project.ID,
	)
	return err
}

func (r projectRepo) Delete(ctx context.Context, id string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	return err
}
