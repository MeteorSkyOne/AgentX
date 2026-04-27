package sqlite

import (
	"context"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type threadRepo struct {
	q queryer
}

func (r threadRepo) Create(ctx context.Context, thread domain.Thread) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO threads (id, org_id, project_id, channel_id, title, created_by, created_at, updated_at, archived_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		thread.ID, thread.OrganizationID, thread.ProjectID, thread.ChannelID, thread.Title, thread.CreatedBy,
		formatTime(thread.CreatedAt), formatTime(thread.UpdatedAt), nullableTime(thread.ArchivedAt),
	)
	return err
}

func (r threadRepo) ListByChannel(ctx context.Context, channelID string) ([]domain.Thread, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, project_id, channel_id, title, created_by, created_at, updated_at, archived_at
FROM threads
WHERE channel_id = ? AND archived_at IS NULL
ORDER BY updated_at DESC, created_at DESC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []domain.Thread
	for rows.Next() {
		thread, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return threads, nil
}

func (r threadRepo) ByID(ctx context.Context, id string) (domain.Thread, error) {
	return scanThread(r.q.QueryRowContext(ctx, `
SELECT id, org_id, project_id, channel_id, title, created_by, created_at, updated_at, archived_at
FROM threads
WHERE id = ?`, id))
}

func (r threadRepo) Update(ctx context.Context, thread domain.Thread) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE threads
SET title = ?
WHERE id = ?`,
		thread.Title, thread.ID,
	)
	return err
}

func (r threadRepo) Archive(ctx context.Context, id string, archivedAt time.Time) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE threads
SET archived_at = ?, updated_at = ?
WHERE id = ?`,
		formatTime(archivedAt), formatTime(archivedAt), id,
	)
	return err
}
