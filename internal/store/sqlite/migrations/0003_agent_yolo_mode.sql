-- +goose Up
ALTER TABLE agents ADD COLUMN yolo_mode INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE agents DROP COLUMN yolo_mode;
