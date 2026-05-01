package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type scheduledTaskRepo struct {
	q queryer
}

func (r scheduledTaskRepo) Create(ctx context.Context, task domain.ScheduledTask) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO scheduled_tasks (
  id, org_id, project_id, name, kind, enabled, schedule, timezone,
  conversation_type, conversation_id, agent_id, workspace_id, prompt, command,
  timeout_seconds, created_by, last_run_id, last_run_status, last_run_at,
  last_finished_at, next_run_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.OrganizationID, task.ProjectID, task.Name, string(task.Kind), boolToInt(task.Enabled),
		task.Schedule, task.Timezone, nullableString(string(task.ConversationType)), nullableString(task.ConversationID),
		nullableString(task.AgentID), nullableString(task.WorkspaceID), task.Prompt, task.Command, task.TimeoutSeconds,
		task.CreatedBy, nullableString(task.LastRunID), nullableString(task.LastRunStatus), nullableTime(task.LastRunAt),
		nullableTime(task.LastFinishedAt), nullableTime(task.NextRunAt), formatTime(task.CreatedAt), formatTime(task.UpdatedAt),
	)
	return err
}

func (r scheduledTaskRepo) Update(ctx context.Context, task domain.ScheduledTask) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE scheduled_tasks
SET name = ?, kind = ?, enabled = ?, schedule = ?, timezone = ?,
    conversation_type = ?, conversation_id = ?, agent_id = ?, workspace_id = ?,
    prompt = ?, command = ?, timeout_seconds = ?, next_run_at = ?, updated_at = ?
WHERE id = ?`,
		task.Name, string(task.Kind), boolToInt(task.Enabled), task.Schedule, task.Timezone,
		nullableString(string(task.ConversationType)), nullableString(task.ConversationID), nullableString(task.AgentID),
		nullableString(task.WorkspaceID), task.Prompt, task.Command, task.TimeoutSeconds, nullableTime(task.NextRunAt),
		formatTime(task.UpdatedAt), task.ID,
	)
	return err
}

func (r scheduledTaskRepo) Delete(ctx context.Context, id string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = ?`, id)
	return err
}

func (r scheduledTaskRepo) ByID(ctx context.Context, id string) (domain.ScheduledTask, error) {
	return scanScheduledTask(r.q.QueryRowContext(ctx, scheduledTaskSelectSQL()+` WHERE id = ?`, id))
}

func (r scheduledTaskRepo) ListByProject(ctx context.Context, projectID string) ([]domain.ScheduledTask, error) {
	rows, err := r.q.QueryContext(ctx, scheduledTaskSelectSQL()+`
WHERE project_id = ?
ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledTasks(rows)
}

func (r scheduledTaskRepo) ListEnabled(ctx context.Context) ([]domain.ScheduledTask, error) {
	rows, err := r.q.QueryContext(ctx, scheduledTaskSelectSQL()+`
WHERE enabled = 1
ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledTasks(rows)
}

func (r scheduledTaskRepo) UpdateScheduleState(ctx context.Context, taskID string, lastRunID string, lastRunStatus string, lastRunAt *time.Time, lastFinishedAt *time.Time, nextRunAt *time.Time, updatedAt time.Time) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE scheduled_tasks
SET last_run_id = ?, last_run_status = ?, last_run_at = ?, last_finished_at = ?, next_run_at = ?, updated_at = ?
WHERE id = ?`,
		nullableString(lastRunID), nullableString(lastRunStatus), nullableTime(lastRunAt),
		nullableTime(lastFinishedAt), nullableTime(nextRunAt), formatTime(updatedAt), taskID,
	)
	return err
}

func (r scheduledTaskRepo) CreateRun(ctx context.Context, run domain.ScheduledTaskRun) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO scheduled_task_runs (
  id, task_id, org_id, project_id, kind, trigger, scheduled_for, started_at,
  finished_at, status, error, exit_code, stdout, stderr, output_truncated, message_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.TaskID, run.OrganizationID, run.ProjectID, string(run.Kind), string(run.Trigger),
		nullableTime(run.ScheduledFor), formatTime(run.StartedAt), nullableTime(run.FinishedAt), string(run.Status),
		run.Error, nullableInt(run.ExitCode), run.Stdout, run.Stderr, boolToInt(run.OutputTruncated), nullableString(run.MessageID),
	)
	return err
}

func (r scheduledTaskRepo) UpdateRun(ctx context.Context, run domain.ScheduledTaskRun) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE scheduled_task_runs
SET finished_at = ?, status = ?, error = ?, exit_code = ?, stdout = ?, stderr = ?, output_truncated = ?, message_id = ?
WHERE id = ?`,
		nullableTime(run.FinishedAt), string(run.Status), run.Error, nullableInt(run.ExitCode),
		run.Stdout, run.Stderr, boolToInt(run.OutputTruncated), nullableString(run.MessageID), run.ID,
	)
	return err
}

func (r scheduledTaskRepo) ListRunsByTask(ctx context.Context, taskID string, limit int) ([]domain.ScheduledTaskRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT id, task_id, org_id, project_id, kind, trigger, scheduled_for, started_at,
  finished_at, status, error, exit_code, stdout, stderr, output_truncated, message_id
FROM scheduled_task_runs
WHERE task_id = ?
ORDER BY started_at DESC
LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledTaskRuns(rows)
}

func scheduledTaskSelectSQL() string {
	return `SELECT id, org_id, project_id, name, kind, enabled, schedule, timezone,
  conversation_type, conversation_id, agent_id, workspace_id, prompt, command,
  timeout_seconds, created_by, last_run_id, last_run_status, last_run_at,
  last_finished_at, next_run_at, created_at, updated_at
FROM scheduled_tasks`
}

func scanScheduledTasks(rows *sql.Rows) ([]domain.ScheduledTask, error) {
	var tasks []domain.ScheduledTask
	for rows.Next() {
		task, err := scanScheduledTask(rows)
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

func scanScheduledTask(scanner interface {
	Scan(dest ...any) error
}) (domain.ScheduledTask, error) {
	var task domain.ScheduledTask
	var kind string
	var enabled int
	var conversationType, conversationID, agentID, workspaceID sql.NullString
	var lastRunID, lastRunStatus, lastRunAt, lastFinishedAt, nextRunAt sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&task.ID, &task.OrganizationID, &task.ProjectID, &task.Name, &kind, &enabled,
		&task.Schedule, &task.Timezone, &conversationType, &conversationID, &agentID, &workspaceID,
		&task.Prompt, &task.Command, &task.TimeoutSeconds, &task.CreatedBy, &lastRunID, &lastRunStatus,
		&lastRunAt, &lastFinishedAt, &nextRunAt, &createdAt, &updatedAt,
	); err != nil {
		return domain.ScheduledTask{}, err
	}
	task.Kind = domain.ScheduledTaskKind(kind)
	task.Enabled = enabled != 0
	task.ConversationType = domain.ConversationType(conversationType.String)
	task.ConversationID = conversationID.String
	task.AgentID = agentID.String
	task.WorkspaceID = workspaceID.String
	task.LastRunID = lastRunID.String
	task.LastRunStatus = lastRunStatus.String
	var err error
	task.LastRunAt, err = parseNullableTime(lastRunAt)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	task.LastFinishedAt, err = parseNullableTime(lastFinishedAt)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	task.NextRunAt, err = parseNullableTime(nextRunAt)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	task.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	task.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	return task, nil
}

func scanScheduledTaskRuns(rows *sql.Rows) ([]domain.ScheduledTaskRun, error) {
	var runs []domain.ScheduledTaskRun
	for rows.Next() {
		run, err := scanScheduledTaskRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func scanScheduledTaskRun(scanner interface {
	Scan(dest ...any) error
}) (domain.ScheduledTaskRun, error) {
	var run domain.ScheduledTaskRun
	var kind, trigger, status string
	var scheduledFor, finishedAt sql.NullString
	var exitCode sql.NullInt64
	var outputTruncated int
	var messageID sql.NullString
	var startedAt string
	if err := scanner.Scan(
		&run.ID, &run.TaskID, &run.OrganizationID, &run.ProjectID, &kind, &trigger,
		&scheduledFor, &startedAt, &finishedAt, &status, &run.Error, &exitCode,
		&run.Stdout, &run.Stderr, &outputTruncated, &messageID,
	); err != nil {
		return domain.ScheduledTaskRun{}, err
	}
	run.Kind = domain.ScheduledTaskKind(kind)
	run.Trigger = domain.ScheduledTaskTrigger(trigger)
	run.Status = domain.ScheduledTaskRunStatus(status)
	run.OutputTruncated = outputTruncated != 0
	run.MessageID = messageID.String
	if exitCode.Valid {
		value := int(exitCode.Int64)
		run.ExitCode = &value
	}
	var err error
	run.ScheduledFor, err = parseNullableTime(scheduledFor)
	if err != nil {
		return domain.ScheduledTaskRun{}, err
	}
	run.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return domain.ScheduledTaskRun{}, err
	}
	run.FinishedAt, err = parseNullableTime(finishedAt)
	if err != nil {
		return domain.ScheduledTaskRun{}, err
	}
	return run, nil
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
