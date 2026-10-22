-- +goose Up
ALTER TABLE agents ADD COLUMN system_prompt_mode TEXT NOT NULL DEFAULT 'append';

-- +goose Down
ALTER TABLE agents DROP COLUMN system_prompt_mode;
