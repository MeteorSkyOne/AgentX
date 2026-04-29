-- +goose Up
ALTER TABLE user_preferences ADD COLUMN hide_avatars INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_preferences DROP COLUMN hide_avatars;
