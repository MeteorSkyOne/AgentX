package sqlite

import (
	"context"
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
		`INSERT INTO users (id, display_name, created_at) VALUES (?, ?, ?)`,
		user.ID, user.DisplayName, formatTime(user.CreatedAt),
	)
	return err
}

func (r userRepo) ByID(ctx context.Context, id string) (domain.User, error) {
	var user domain.User
	var createdAt string
	err := r.q.QueryRowContext(ctx,
		`SELECT id, display_name, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.DisplayName, &createdAt)
	if err != nil {
		return domain.User{}, err
	}
	user.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r userRepo) First(ctx context.Context) (domain.User, error) {
	var user domain.User
	var createdAt string
	err := r.q.QueryRowContext(ctx,
		`SELECT id, display_name, created_at FROM users ORDER BY created_at ASC, id ASC LIMIT 1`,
	).Scan(&user.ID, &user.DisplayName, &createdAt)
	if err != nil {
		return domain.User{}, err
	}
	user.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r userRepo) CreateAPISession(ctx context.Context, token string, userID string) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO api_sessions (token, user_id, created_at) VALUES (?, ?, ?)`,
		token, userID, formatTime(time.Now().UTC()),
	)
	return err
}

func (r userRepo) UserIDByAPISession(ctx context.Context, token string) (string, error) {
	var userID string
	err := r.q.QueryRowContext(ctx,
		`SELECT user_id FROM api_sessions WHERE token = ?`,
		token,
	).Scan(&userID)
	return userID, err
}

func (r botUserRepo) Create(ctx context.Context, bot domain.BotUser) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO bot_users (id, org_id, display_name, created_at) VALUES (?, ?, ?, ?)`,
		bot.ID, bot.OrganizationID, bot.DisplayName, formatTime(bot.CreatedAt),
	)
	return err
}
