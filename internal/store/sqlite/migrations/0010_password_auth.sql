-- +goose Up
PRAGMA foreign_keys = ON;

ALTER TABLE users ADD COLUMN username TEXT;
ALTER TABLE users ADD COLUMN password_hash TEXT;
ALTER TABLE users ADD COLUMN password_updated_at TEXT;

CREATE UNIQUE INDEX users_username_unique_idx
  ON users(username)
  WHERE username IS NOT NULL;

DROP TABLE api_sessions;

CREATE TABLE api_sessions (
  token_hash TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE INDEX api_sessions_user_expires_idx
  ON api_sessions(user_id, expires_at);

-- +goose Down
DROP INDEX IF EXISTS api_sessions_user_expires_idx;
DROP TABLE IF EXISTS api_sessions;

CREATE TABLE api_sessions (
  token TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL
);

DROP INDEX IF EXISTS users_username_unique_idx;

CREATE TABLE users_legacy (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

INSERT INTO users_legacy (id, display_name, created_at)
SELECT id, display_name, created_at FROM users;

DROP TABLE users;
ALTER TABLE users_legacy RENAME TO users;
