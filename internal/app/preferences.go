package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

func (a *App) UserPreferences(ctx context.Context, userID string) (domain.UserPreferences, error) {
	preferences, err := a.store.UserPreferences().ByUser(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return defaultUserPreferences(userID), nil
		}
		return domain.UserPreferences{}, err
	}
	return preferences, nil
}

func (a *App) UpdateUserPreferences(ctx context.Context, preferences domain.UserPreferences) (domain.UserPreferences, error) {
	now := time.Now().UTC()
	current, err := a.store.UserPreferences().ByUser(ctx, preferences.UserID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return domain.UserPreferences{}, err
		}
		current = defaultUserPreferences(preferences.UserID)
		current.CreatedAt = now
	}
	current.ShowTTFT = preferences.ShowTTFT
	current.ShowTPS = preferences.ShowTPS
	current.HideAvatars = preferences.HideAvatars
	current.UpdatedAt = now
	if current.CreatedAt.IsZero() {
		current.CreatedAt = now
	}
	if err := a.store.UserPreferences().Upsert(ctx, current); err != nil {
		return domain.UserPreferences{}, err
	}
	return current, nil
}

func defaultUserPreferences(userID string) domain.UserPreferences {
	now := time.Now().UTC()
	return domain.UserPreferences{
		UserID:      userID,
		ShowTTFT:    true,
		ShowTPS:     true,
		HideAvatars: false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
