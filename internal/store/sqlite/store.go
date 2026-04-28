package sqlite

import (
	"context"
	"database/sql"

	"github.com/meteorsky/agentx/internal/store"
)

var _ store.Store = (*Store)(nil)

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txStore struct {
	tx *sql.Tx
}

func (s *Store) Tx(ctx context.Context, fn func(store.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(&txStore{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) Users() store.UserStore {
	return userRepo{q: s.db}
}

func (s *Store) UserPreferences() store.UserPreferencesStore {
	return userPreferencesRepo{q: s.db}
}

func (s *Store) Organizations() store.OrganizationStore {
	return organizationRepo{q: s.db}
}

func (s *Store) NotificationSettings() store.NotificationSettingsStore {
	return notificationSettingsRepo{q: s.db}
}

func (s *Store) Projects() store.ProjectStore {
	return projectRepo{q: s.db}
}

func (s *Store) Channels() store.ChannelStore {
	return channelRepo{q: s.db}
}

func (s *Store) Threads() store.ThreadStore {
	return threadRepo{q: s.db}
}

func (s *Store) Messages() store.MessageStore {
	return messageRepo{q: s.db}
}

func (s *Store) MessageAttachments() store.MessageAttachmentStore {
	return messageAttachmentRepo{q: s.db}
}

func (s *Store) BotUsers() store.BotUserStore {
	return botUserRepo{q: s.db}
}

func (s *Store) Agents() store.AgentStore {
	return agentRepo{q: s.db}
}

func (s *Store) Workspaces() store.WorkspaceStore {
	return workspaceRepo{q: s.db}
}

func (s *Store) ChannelAgents() store.ChannelAgentStore {
	return channelAgentRepo{q: s.db}
}

func (s *Store) Bindings() store.BindingStore {
	return bindingRepo{q: s.db}
}

func (s *Store) Sessions() store.SessionStore {
	return sessionRepo{q: s.db}
}

func (s *Store) Metrics() store.MetricsStore {
	return metricsRepo{q: s.db}
}

func (t *txStore) Users() store.UserStore {
	return userRepo{q: t.tx}
}

func (t *txStore) UserPreferences() store.UserPreferencesStore {
	return userPreferencesRepo{q: t.tx}
}

func (t *txStore) Organizations() store.OrganizationStore {
	return organizationRepo{q: t.tx}
}

func (t *txStore) NotificationSettings() store.NotificationSettingsStore {
	return notificationSettingsRepo{q: t.tx}
}

func (t *txStore) Projects() store.ProjectStore {
	return projectRepo{q: t.tx}
}

func (t *txStore) Channels() store.ChannelStore {
	return channelRepo{q: t.tx}
}

func (t *txStore) Threads() store.ThreadStore {
	return threadRepo{q: t.tx}
}

func (t *txStore) Messages() store.MessageStore {
	return messageRepo{q: t.tx}
}

func (t *txStore) MessageAttachments() store.MessageAttachmentStore {
	return messageAttachmentRepo{q: t.tx}
}

func (t *txStore) BotUsers() store.BotUserStore {
	return botUserRepo{q: t.tx}
}

func (t *txStore) Agents() store.AgentStore {
	return agentRepo{q: t.tx}
}

func (t *txStore) Workspaces() store.WorkspaceStore {
	return workspaceRepo{q: t.tx}
}

func (t *txStore) ChannelAgents() store.ChannelAgentStore {
	return channelAgentRepo{q: t.tx}
}

func (t *txStore) Bindings() store.BindingStore {
	return bindingRepo{q: t.tx}
}

func (t *txStore) Sessions() store.SessionStore {
	return sessionRepo{q: t.tx}
}

func (t *txStore) Metrics() store.MetricsStore {
	return metricsRepo{q: t.tx}
}
