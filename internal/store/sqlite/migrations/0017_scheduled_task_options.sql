-- +goose Up
ALTER TABLE scheduled_tasks ADD COLUMN fresh_context INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN post_title TEXT NOT NULL DEFAULT '';
ALTER TABLE scheduled_tasks ADD COLUMN notify INTEGER NOT NULL DEFAULT 1;
ALTER TABLE scheduled_task_runs ADD COLUMN thread_id TEXT REFERENCES threads(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE scheduled_task_runs DROP COLUMN thread_id;
ALTER TABLE scheduled_tasks DROP COLUMN notify;
ALTER TABLE scheduled_tasks DROP COLUMN post_title;
ALTER TABLE scheduled_tasks DROP COLUMN fresh_context;
