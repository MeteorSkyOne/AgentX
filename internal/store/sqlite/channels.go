package sqlite

import (
	"context"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type channelRepo struct {
	q queryer
}

func (r channelRepo) Create(ctx context.Context, channel domain.Channel) error {
	channel = normalizeChannel(channel)
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO channels (id, org_id, project_id, type, name, team_max_batches, team_max_runs, created_at, updated_at, archived_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		channel.ID, channel.OrganizationID, channel.ProjectID, string(channel.Type), channel.Name,
		channel.TeamMaxBatches, channel.TeamMaxRuns,
		formatTime(channel.CreatedAt), formatTime(channel.UpdatedAt), nullableTime(channel.ArchivedAt),
	)
	return err
}

func (r channelRepo) ListByOrganization(ctx context.Context, orgID string) ([]domain.Channel, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT id, org_id, project_id, type, name, team_max_batches, team_max_runs, created_at, updated_at, archived_at
	FROM channels
	WHERE org_id = ? AND archived_at IS NULL
	ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func (r channelRepo) ListByProject(ctx context.Context, projectID string) ([]domain.Channel, error) {
	rows, err := r.q.QueryContext(ctx, `
	SELECT id, org_id, project_id, type, name, team_max_batches, team_max_runs, created_at, updated_at, archived_at
	FROM channels
	WHERE project_id = ? AND archived_at IS NULL
ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func (r channelRepo) ByID(ctx context.Context, id string) (domain.Channel, error) {
	return scanChannel(r.q.QueryRowContext(ctx,
		`SELECT id, org_id, project_id, type, name, team_max_batches, team_max_runs, created_at, updated_at, archived_at FROM channels WHERE id = ?`,
		id,
	))
}

func (r channelRepo) Update(ctx context.Context, channel domain.Channel) error {
	channel = normalizeChannel(channel)
	_, err := r.q.ExecContext(ctx, `
	UPDATE channels
	SET name = ?, type = ?, team_max_batches = ?, team_max_runs = ?, updated_at = ?
	WHERE id = ?`,
		channel.Name, string(channel.Type), channel.TeamMaxBatches, channel.TeamMaxRuns, formatTime(channel.UpdatedAt), channel.ID,
	)
	return err
}

func (r channelRepo) Archive(ctx context.Context, id string, archivedAt time.Time) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE channels
SET archived_at = ?, updated_at = ?
WHERE id = ?`,
		formatTime(archivedAt), formatTime(archivedAt), id,
	)
	return err
}
