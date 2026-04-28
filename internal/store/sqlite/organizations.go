package sqlite

import (
	"context"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type organizationRepo struct {
	q queryer
}

func (r organizationRepo) Create(ctx context.Context, org domain.Organization) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO organizations (id, name, created_at) VALUES (?, ?, ?)`,
		org.ID, org.Name, formatTime(org.CreatedAt),
	)
	return err
}

func (r organizationRepo) Any(ctx context.Context) (bool, error) {
	var exists bool
	err := r.q.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM organizations LIMIT 1)`,
	).Scan(&exists)
	return exists, err
}

func (r organizationRepo) ListForUser(ctx context.Context, userID string) ([]domain.Organization, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT organizations.id, organizations.name, organizations.created_at
FROM organizations
JOIN memberships ON memberships.org_id = organizations.id
WHERE memberships.user_id = ?
ORDER BY organizations.created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []domain.Organization
	for rows.Next() {
		org, err := scanOrganization(rows)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r organizationRepo) MemberRole(ctx context.Context, orgID string, userID string) (domain.Role, error) {
	var role string
	err := r.q.QueryRowContext(ctx,
		`SELECT role FROM memberships WHERE org_id = ? AND user_id = ?`,
		orgID, userID,
	).Scan(&role)
	return domain.Role(role), err
}

func (r organizationRepo) AddMember(ctx context.Context, orgID string, userID string, role domain.Role) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO memberships (org_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		orgID, userID, string(role), formatTime(time.Now().UTC()),
	)
	return err
}
