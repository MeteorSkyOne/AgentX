package sqlite

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type notificationSettingsRepo struct {
	q queryer
}

func (r notificationSettingsRepo) ByOrganization(ctx context.Context, orgID string) (domain.NotificationSettings, error) {
	return scanNotificationSettings(r.q.QueryRowContext(ctx, `
SELECT org_id, webhook_enabled, webhook_url, webhook_secret, created_at, updated_at
FROM notification_settings
WHERE org_id = ?`, orgID))
}

func (r notificationSettingsRepo) Upsert(ctx context.Context, settings domain.NotificationSettings) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO notification_settings (org_id, webhook_enabled, webhook_url, webhook_secret, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(org_id) DO UPDATE SET
  webhook_enabled = excluded.webhook_enabled,
  webhook_url = excluded.webhook_url,
  webhook_secret = excluded.webhook_secret,
  updated_at = excluded.updated_at`,
		settings.OrganizationID, boolToInt(settings.WebhookEnabled), settings.WebhookURL, settings.WebhookSecret,
		formatTime(settings.CreatedAt), formatTime(settings.UpdatedAt),
	)
	return err
}
