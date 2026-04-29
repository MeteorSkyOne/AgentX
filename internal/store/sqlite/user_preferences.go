package sqlite

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type userPreferencesRepo struct {
	q queryer
}

func (r userPreferencesRepo) ByUser(ctx context.Context, userID string) (domain.UserPreferences, error) {
	return scanUserPreferences(r.q.QueryRowContext(ctx, `
SELECT user_id, show_ttft, show_tps, hide_avatars, created_at, updated_at
FROM user_preferences
WHERE user_id = ?`, userID))
}

func (r userPreferencesRepo) Upsert(ctx context.Context, preferences domain.UserPreferences) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO user_preferences (user_id, show_ttft, show_tps, hide_avatars, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
  show_ttft = excluded.show_ttft,
  show_tps = excluded.show_tps,
  hide_avatars = excluded.hide_avatars,
  updated_at = excluded.updated_at`,
		preferences.UserID,
		boolInt(preferences.ShowTTFT),
		boolInt(preferences.ShowTPS),
		boolInt(preferences.HideAvatars),
		formatTime(preferences.CreatedAt),
		formatTime(preferences.UpdatedAt),
	)
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
