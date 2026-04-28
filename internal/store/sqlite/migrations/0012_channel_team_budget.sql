-- +goose Up
ALTER TABLE channels ADD COLUMN team_max_batches INTEGER NOT NULL DEFAULT 6;
ALTER TABLE channels ADD COLUMN team_max_runs INTEGER NOT NULL DEFAULT 12;

-- +goose Down
ALTER TABLE channels DROP COLUMN team_max_runs;
ALTER TABLE channels DROP COLUMN team_max_batches;
