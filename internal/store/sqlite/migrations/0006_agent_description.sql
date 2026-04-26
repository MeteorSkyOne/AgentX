-- +goose Up
ALTER TABLE agents ADD COLUMN description TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agents DROP COLUMN description;
