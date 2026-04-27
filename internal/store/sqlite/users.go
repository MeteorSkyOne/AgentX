package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type userRepo struct {
	q queryer
}

type botUserRepo struct {
	q queryer
}

func (r userRepo) Create(ctx context.Context, user domain.User) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, password_hash, password_updated_at, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID, nullableString(user.Username), user.DisplayName, nullableString(user.PasswordHash),
		nullableTime(user.PasswordUpdatedAt), formatTime(user.CreatedAt),
	)
	return err
}

func (r userRepo) ByID(ctx context.Context, id string) (domain.User, error) {
	return scanUser(r.q.QueryRowContext(ctx,
		`SELECT id, username, display_name, password_hash, password_updated_at, created_at FROM users WHERE id = ?`,
		id,
	))
}

func (r userRepo) ByUsername(ctx context.Context, username string) (domain.User, error) {
	return scanUser(r.q.QueryRowContext(ctx,
		`SELECT id, username, display_name, password_hash, password_updated_at, created_at FROM users WHERE username = ?`,
		username,
	))
}

func (r userRepo) First(ctx context.Context) (domain.User, error) {
	return scanUser(r.q.QueryRowContext(ctx,
		`SELECT id, username, display_name, password_hash, password_updated_at, created_at
FROM users
ORDER BY created_at ASC, id ASC
LIMIT 1`,
	))
}

func (r userRepo) HasPassword(ctx context.Context) (bool, error) {
	var exists int
	err := r.q.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE password_hash IS NOT NULL AND password_hash != '')`,
	).Scan(&exists)
	return exists != 0, err
}

func (r userRepo) SetCredentials(ctx context.Context, userID string, username string, displayName string, passwordHash string, passwordUpdatedAt time.Time) error {
	_, err := r.q.ExecContext(ctx,
		`UPDATE users
SET username = ?, display_name = ?, password_hash = ?, password_updated_at = ?
WHERE id = ?`,
		username, displayName, passwordHash, formatTime(passwordUpdatedAt), userID,
	)
	return err
}

func (r userRepo) CreateAPISession(ctx context.Context, tokenHash string, userID string, createdAt time.Time, expiresAt time.Time) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO api_sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, userID, formatTime(createdAt), formatTime(expiresAt),
	)
	return err
}

func (r userRepo) UserIDByAPISessionHash(ctx context.Context, tokenHash string, now time.Time) (string, error) {
	var userID string
	err := r.q.QueryRowContext(ctx,
		`SELECT user_id FROM api_sessions WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, formatTime(now),
	).Scan(&userID)
	return userID, err
}

func (r userRepo) DeleteAPISession(ctx context.Context, tokenHash string) error {
	_, err := r.q.ExecContext(ctx,
		`DELETE FROM api_sessions WHERE token_hash = ?`,
		tokenHash,
	)
	return err
}

func (r userRepo) DeleteAllAPISessions(ctx context.Context) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM api_sessions`)
	return err
}

func (r botUserRepo) Create(ctx context.Context, bot domain.BotUser) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO bot_users (id, org_id, display_name, created_at) VALUES (?, ?, ?, ?)`,
		bot.ID, bot.OrganizationID, bot.DisplayName, formatTime(bot.CreatedAt),
	)
	return err
}

func scanUser(scanner interface {
	Scan(dest ...any) error
}) (domain.User, error) {
	var user domain.User
	var username sql.NullString
	var passwordHash sql.NullString
	var passwordUpdatedAt sql.NullString
	var createdAt string
	if err := scanner.Scan(
		&user.ID, &username, &user.DisplayName, &passwordHash, &passwordUpdatedAt, &createdAt,
	); err != nil {
		return domain.User{}, err
	}
	user.Username = username.String
	user.PasswordHash = passwordHash.String
	var err error
	user.PasswordUpdatedAt, err = parseNullableTime(passwordUpdatedAt)
	if err != nil {
		return domain.User{}, err
	}
	user.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}
