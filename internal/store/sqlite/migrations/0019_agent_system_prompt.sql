-- +goose Up
ALTER TABLE agents ADD COLUMN system_prompt TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agents DROP COLUMN system_prompt;
