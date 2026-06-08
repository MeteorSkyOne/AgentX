package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/meteorsky/agentx/internal/domain"
)

type roadmapStageRepo struct {
	q queryer
}

func (r roadmapStageRepo) Create(ctx context.Context, stage domain.RoadmapStage) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO roadmap_stages (id, org_id, project_id, name, description, status, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		stage.ID, stage.OrganizationID, stage.ProjectID, stage.Name, stage.Description,
		string(stage.Status), stage.Position, formatTime(stage.CreatedAt), formatTime(stage.UpdatedAt),
	)
	return err
}

func (r roadmapStageRepo) ByID(ctx context.Context, id string) (domain.RoadmapStage, error) {
	return scanRoadmapStage(r.q.QueryRowContext(ctx, roadmapStageSelectSQL()+` WHERE id = ?`, id))
}

func (r roadmapStageRepo) Update(ctx context.Context, stage domain.RoadmapStage) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE roadmap_stages SET name = ?, description = ?, status = ?, position = ?, updated_at = ?
WHERE id = ?`,
		stage.Name, stage.Description, string(stage.Status), stage.Position,
		formatTime(stage.UpdatedAt), stage.ID,
	)
	return err
}

func (r roadmapStageRepo) Delete(ctx context.Context, id string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM roadmap_stages WHERE id = ?`, id)
	return err
}

func (r roadmapStageRepo) ListByProject(ctx context.Context, projectID string) ([]domain.RoadmapStage, error) {
	rows, err := r.q.QueryContext(ctx, roadmapStageSelectSQL()+`
WHERE project_id = ?
ORDER BY position ASC, created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoadmapStages(rows)
}

func (r roadmapStageRepo) MaxPositionByProject(ctx context.Context, projectID string) (int, error) {
	var pos sql.NullInt64
	err := r.q.QueryRowContext(ctx, `SELECT MAX(position) FROM roadmap_stages WHERE project_id = ?`, projectID).Scan(&pos)
	if err != nil {
		return -1, err
	}
	if !pos.Valid {
		return -1, nil
	}
	return int(pos.Int64), nil
}

func (r roadmapStageRepo) ReorderByProject(ctx context.Context, projectID string, ids []string) error {
	for i, stageID := range ids {
		_, err := r.q.ExecContext(ctx, `UPDATE roadmap_stages SET position = ? WHERE id = ? AND project_id = ?`, i, stageID, projectID)
		if err != nil {
			return fmt.Errorf("reorder stage %s: %w", stageID, err)
		}
	}
	return nil
}

type roadmapTaskRepo struct {
	q queryer
}

func (r roadmapTaskRepo) Create(ctx context.Context, task domain.RoadmapTask) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO roadmap_tasks (id, org_id, stage_id, title, description, completed, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.OrganizationID, task.StageID, task.Title, task.Description,
		boolToInt(task.Completed), task.Position, formatTime(task.CreatedAt), formatTime(task.UpdatedAt),
	)
	return err
}

func (r roadmapTaskRepo) ByID(ctx context.Context, id string) (domain.RoadmapTask, error) {
	return scanRoadmapTask(r.q.QueryRowContext(ctx, roadmapTaskSelectSQL()+` WHERE id = ?`, id))
}

func (r roadmapTaskRepo) Update(ctx context.Context, task domain.RoadmapTask) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE roadmap_tasks SET title = ?, description = ?, completed = ?, position = ?, updated_at = ?
WHERE id = ?`,
		task.Title, task.Description, boolToInt(task.Completed), task.Position,
		formatTime(task.UpdatedAt), task.ID,
	)
	return err
}

func (r roadmapTaskRepo) Delete(ctx context.Context, id string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM roadmap_tasks WHERE id = ?`, id)
	return err
}

func (r roadmapTaskRepo) ListByStage(ctx context.Context, stageID string) ([]domain.RoadmapTask, error) {
	rows, err := r.q.QueryContext(ctx, roadmapTaskSelectSQL()+`
WHERE stage_id = ?
ORDER BY position ASC, created_at ASC`, stageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoadmapTasks(rows)
}

func (r roadmapTaskRepo) ListByProject(ctx context.Context, projectID string) ([]domain.RoadmapTask, error) {
	rows, err := r.q.QueryContext(ctx, roadmapTaskSelectSQL()+`
WHERE stage_id IN (SELECT id FROM roadmap_stages WHERE project_id = ?)
ORDER BY position ASC, created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoadmapTasks(rows)
}

func (r roadmapTaskRepo) MaxPositionByStage(ctx context.Context, stageID string) (int, error) {
	var pos sql.NullInt64
	err := r.q.QueryRowContext(ctx, `SELECT MAX(position) FROM roadmap_tasks WHERE stage_id = ?`, stageID).Scan(&pos)
	if err != nil {
		return -1, err
	}
	if !pos.Valid {
		return -1, nil
	}
	return int(pos.Int64), nil
}

func (r roadmapTaskRepo) ReorderByStage(ctx context.Context, stageID string, ids []string) error {
	for i, taskID := range ids {
		_, err := r.q.ExecContext(ctx, `UPDATE roadmap_tasks SET position = ? WHERE id = ? AND stage_id = ?`, i, taskID, stageID)
		if err != nil {
			return fmt.Errorf("reorder task %s: %w", taskID, err)
		}
	}
	return nil
}

func roadmapStageSelectSQL() string {
	return `SELECT id, org_id, project_id, name, description, status, position, created_at, updated_at
FROM roadmap_stages`
}

func scanRoadmapStages(rows *sql.Rows) ([]domain.RoadmapStage, error) {
	var stages []domain.RoadmapStage
	for rows.Next() {
		stage, err := scanRoadmapStage(rows)
		if err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stages, nil
}

func scanRoadmapStage(scanner interface {
	Scan(dest ...any) error
}) (domain.RoadmapStage, error) {
	var stage domain.RoadmapStage
	var status string
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&stage.ID, &stage.OrganizationID, &stage.ProjectID, &stage.Name, &stage.Description,
		&status, &stage.Position, &createdAt, &updatedAt,
	); err != nil {
		return domain.RoadmapStage{}, err
	}
	stage.Status = domain.RoadmapStageStatus(status)
	var err error
	stage.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.RoadmapStage{}, err
	}
	stage.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.RoadmapStage{}, err
	}
	return stage, nil
}

func roadmapTaskSelectSQL() string {
	return `SELECT id, org_id, stage_id, title, description, completed, position, created_at, updated_at
FROM roadmap_tasks`
}

func scanRoadmapTasks(rows *sql.Rows) ([]domain.RoadmapTask, error) {
	var tasks []domain.RoadmapTask
	for rows.Next() {
		task, err := scanRoadmapTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func scanRoadmapTask(scanner interface {
	Scan(dest ...any) error
}) (domain.RoadmapTask, error) {
	var task domain.RoadmapTask
	var completed int
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&task.ID, &task.OrganizationID, &task.StageID, &task.Title, &task.Description,
		&completed, &task.Position, &createdAt, &updatedAt,
	); err != nil {
		return domain.RoadmapTask{}, err
	}
	task.Completed = completed != 0
	var err error
	task.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.RoadmapTask{}, err
	}
	task.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.RoadmapTask{}, err
	}
	return task, nil
}
