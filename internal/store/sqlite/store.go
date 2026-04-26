package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
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

type userRepo struct {
	q queryer
}

type organizationRepo struct {
	q queryer
}

type notificationSettingsRepo struct {
	q queryer
}

type projectRepo struct {
	q queryer
}

type channelRepo struct {
	q queryer
}

type threadRepo struct {
	q queryer
}

type messageRepo struct {
	q queryer
}

type botUserRepo struct {
	q queryer
}

type agentRepo struct {
	q queryer
}

type workspaceRepo struct {
	q queryer
}

type channelAgentRepo struct {
	q queryer
}

type bindingRepo struct {
	q queryer
}

type sessionRepo struct {
	q queryer
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

func (t *txStore) Users() store.UserStore {
	return userRepo{q: t.tx}
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

func (r organizationRepo) Create(ctx context.Context, org domain.Organization) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO organizations (id, name, created_at) VALUES (?, ?, ?)`,
		org.ID, org.Name, formatTime(org.CreatedAt),
	)
	return err
}

func (r organizationRepo) Any(ctx context.Context) (bool, error) {
	var exists bool
	err := r.q.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM organizations LIMIT 1)`,
	).Scan(&exists)
	return exists, err
}

func (r organizationRepo) ListForUser(ctx context.Context, userID string) ([]domain.Organization, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT organizations.id, organizations.name, organizations.created_at
FROM organizations
JOIN memberships ON memberships.org_id = organizations.id
WHERE memberships.user_id = ?
ORDER BY organizations.created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []domain.Organization
	for rows.Next() {
		org, err := scanOrganization(rows)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r organizationRepo) AddMember(ctx context.Context, orgID string, userID string, role domain.Role) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO memberships (org_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		orgID, userID, string(role), formatTime(time.Now().UTC()),
	)
	return err
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

func (r projectRepo) Create(ctx context.Context, project domain.Project) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO projects (id, org_id, name, workspace_id, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		project.ID, project.OrganizationID, project.Name, project.WorkspaceID, project.CreatedBy,
		formatTime(project.CreatedAt), formatTime(project.UpdatedAt),
	)
	return err
}

func (r projectRepo) ListByOrganization(ctx context.Context, orgID string) ([]domain.Project, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, name, workspace_id, created_by, created_at, updated_at
FROM projects
WHERE org_id = ?
ORDER BY created_at ASC, id ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

func (r projectRepo) ByID(ctx context.Context, id string) (domain.Project, error) {
	return scanProject(r.q.QueryRowContext(ctx, `
SELECT id, org_id, name, workspace_id, created_by, created_at, updated_at
FROM projects
WHERE id = ?`, id))
}

func (r projectRepo) Update(ctx context.Context, project domain.Project) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE projects
SET name = ?, workspace_id = ?, updated_at = ?
WHERE id = ?`,
		project.Name, project.WorkspaceID, formatTime(project.UpdatedAt), project.ID,
	)
	return err
}

func (r projectRepo) Delete(ctx context.Context, id string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	return err
}

func (r channelRepo) Create(ctx context.Context, channel domain.Channel) error {
	channel = normalizeChannel(channel)
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO channels (id, org_id, project_id, type, name, created_at, updated_at, archived_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		channel.ID, channel.OrganizationID, channel.ProjectID, string(channel.Type), channel.Name,
		formatTime(channel.CreatedAt), formatTime(channel.UpdatedAt), nullableTime(channel.ArchivedAt),
	)
	return err
}

func (r channelRepo) ListByOrganization(ctx context.Context, orgID string) ([]domain.Channel, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT id, org_id, project_id, type, name, created_at, updated_at, archived_at
FROM channels
WHERE org_id = ? AND archived_at IS NULL
ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func (r channelRepo) ListByProject(ctx context.Context, projectID string) ([]domain.Channel, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, project_id, type, name, created_at, updated_at, archived_at
FROM channels
WHERE project_id = ? AND archived_at IS NULL
ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func (r channelRepo) ByID(ctx context.Context, id string) (domain.Channel, error) {
	return scanChannel(r.q.QueryRowContext(ctx,
		`SELECT id, org_id, project_id, type, name, created_at, updated_at, archived_at FROM channels WHERE id = ?`,
		id,
	))
}

func (r channelRepo) Update(ctx context.Context, channel domain.Channel) error {
	channel = normalizeChannel(channel)
	_, err := r.q.ExecContext(ctx, `
UPDATE channels
SET name = ?, type = ?, updated_at = ?
WHERE id = ?`,
		channel.Name, string(channel.Type), formatTime(channel.UpdatedAt), channel.ID,
	)
	return err
}

func (r channelRepo) Archive(ctx context.Context, id string, archivedAt time.Time) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE channels
SET archived_at = ?, updated_at = ?
WHERE id = ?`,
		formatTime(archivedAt), formatTime(archivedAt), id,
	)
	return err
}

func (r threadRepo) Create(ctx context.Context, thread domain.Thread) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO threads (id, org_id, project_id, channel_id, title, created_by, created_at, updated_at, archived_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		thread.ID, thread.OrganizationID, thread.ProjectID, thread.ChannelID, thread.Title, thread.CreatedBy,
		formatTime(thread.CreatedAt), formatTime(thread.UpdatedAt), nullableTime(thread.ArchivedAt),
	)
	return err
}

func (r threadRepo) ListByChannel(ctx context.Context, channelID string) ([]domain.Thread, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, project_id, channel_id, title, created_by, created_at, updated_at, archived_at
FROM threads
WHERE channel_id = ? AND archived_at IS NULL
ORDER BY updated_at DESC, created_at DESC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []domain.Thread
	for rows.Next() {
		thread, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return threads, nil
}

func (r threadRepo) ByID(ctx context.Context, id string) (domain.Thread, error) {
	return scanThread(r.q.QueryRowContext(ctx, `
SELECT id, org_id, project_id, channel_id, title, created_by, created_at, updated_at, archived_at
FROM threads
WHERE id = ?`, id))
}

func (r threadRepo) Update(ctx context.Context, thread domain.Thread) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE threads
SET title = ?
WHERE id = ?`,
		thread.Title, thread.ID,
	)
	return err
}

func (r threadRepo) Archive(ctx context.Context, id string, archivedAt time.Time) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE threads
SET archived_at = ?, updated_at = ?
WHERE id = ?`,
		formatTime(archivedAt), formatTime(archivedAt), id,
	)
	return err
}

func (r messageRepo) Create(ctx context.Context, message domain.Message) error {
	metadataJSON, err := json.Marshal(emptyMapIfNil(message.Metadata))
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `
INSERT INTO messages (id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		message.ID, message.OrganizationID, string(message.ConversationType), message.ConversationID,
		string(message.SenderType), message.SenderID, string(message.Kind), message.Body, string(metadataJSON), formatTime(message.CreatedAt),
	)
	return err
}

func (r messageRepo) ByID(ctx context.Context, id string) (domain.Message, error) {
	return scanMessage(r.q.QueryRowContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, created_at
FROM messages
WHERE id = ?`, id))
}

func (r messageRepo) Update(ctx context.Context, message domain.Message) error {
	metadataJSON, err := json.Marshal(emptyMapIfNil(message.Metadata))
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `
UPDATE messages
SET body = ?, metadata_json = ?
WHERE id = ?`,
		message.Body, string(metadataJSON), message.ID,
	)
	return err
}

func (r messageRepo) Delete(ctx context.Context, id string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, id)
	return err
}

func (r messageRepo) List(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, created_at
FROM messages
WHERE conversation_type = ? AND conversation_id = ?
ORDER BY created_at ASC
LIMIT ?`, string(conversationType), conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows)
}

func (r messageRepo) ListRecent(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, created_at
FROM messages
WHERE conversation_type = ? AND conversation_id = ?
ORDER BY created_at DESC
LIMIT ?`, string(conversationType), conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (r messageRepo) ListRecentBefore(ctx context.Context, conversationType domain.ConversationType, conversationID string, before time.Time, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, metadata_json, created_at
FROM messages
WHERE conversation_type = ? AND conversation_id = ? AND created_at < ?
ORDER BY created_at DESC
LIMIT ?`, string(conversationType), conversationID, formatTime(before), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func scanMessages(rows *sql.Rows) ([]domain.Message, error) {
	var messages []domain.Message
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (r botUserRepo) Create(ctx context.Context, bot domain.BotUser) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO bot_users (id, org_id, display_name, created_at) VALUES (?, ?, ?, ?)`,
		bot.ID, bot.OrganizationID, bot.DisplayName, formatTime(bot.CreatedAt),
	)
	return err
}

func (r agentRepo) Create(ctx context.Context, agent domain.Agent) error {
	if !agent.Enabled {
		agent.Enabled = true
	}
	agent = normalizeAgent(agent)
	envJSON, err := json.Marshal(emptyMapIfNil(agent.Env))
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `
INSERT INTO agents (id, org_id, bot_user_id, kind, name, handle, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.ID, agent.OrganizationID, agent.BotUserID, agent.Kind, agent.Name, agent.Handle, agent.Model, agent.Effort,
		agent.DefaultWorkspaceID, agent.ConfigWorkspaceID, boolToInt(agent.Enabled), boolToInt(agent.FastMode), boolToInt(agent.YoloMode), string(envJSON),
		formatTime(agent.CreatedAt), formatTime(agent.UpdatedAt),
	)
	return err
}

func (r agentRepo) ByID(ctx context.Context, id string) (domain.Agent, error) {
	return scanAgent(r.q.QueryRowContext(ctx, `
SELECT id, org_id, bot_user_id, kind, name, handle, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at
FROM agents
WHERE id = ?`, id))
}

func (r agentRepo) DefaultForOrganization(ctx context.Context, orgID string) (domain.Agent, error) {
	return scanAgent(r.q.QueryRowContext(ctx, `
SELECT id, org_id, bot_user_id, kind, name, handle, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at
FROM agents
WHERE org_id = ?
ORDER BY created_at ASC
LIMIT 1`, orgID))
}

func (r agentRepo) ListByOrganization(ctx context.Context, orgID string) ([]domain.Agent, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, bot_user_id, kind, name, handle, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at
FROM agents
WHERE org_id = ?
ORDER BY created_at ASC, id ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

func (r agentRepo) ByHandle(ctx context.Context, orgID string, handle string) (domain.Agent, error) {
	return scanAgent(r.q.QueryRowContext(ctx, `
SELECT id, org_id, bot_user_id, kind, name, handle, model, effort, default_workspace_id, config_workspace_id, enabled, fast_mode, yolo_mode, env_json, created_at, updated_at
FROM agents
WHERE org_id = ? AND handle = ?`, orgID, handle))
}

func (r agentRepo) Update(ctx context.Context, agent domain.Agent) error {
	agent = normalizeAgent(agent)
	envJSON, err := json.Marshal(emptyMapIfNil(agent.Env))
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `
UPDATE agents
SET kind = ?, name = ?, handle = ?, model = ?, effort = ?, default_workspace_id = ?, config_workspace_id = ?, enabled = ?, fast_mode = ?, yolo_mode = ?, env_json = ?, updated_at = ?
WHERE id = ?`,
		agent.Kind, agent.Name, agent.Handle, agent.Model, agent.Effort, agent.DefaultWorkspaceID, agent.ConfigWorkspaceID,
		boolToInt(agent.Enabled), boolToInt(agent.FastMode), boolToInt(agent.YoloMode), string(envJSON), formatTime(agent.UpdatedAt), agent.ID,
	)
	return err
}

func (r workspaceRepo) Create(ctx context.Context, workspace domain.Workspace) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO workspaces (id, org_id, type, name, path, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		workspace.ID, workspace.OrganizationID, workspace.Type, workspace.Name, workspace.Path,
		workspace.CreatedBy, formatTime(workspace.CreatedAt), formatTime(workspace.UpdatedAt),
	)
	return err
}

func (r workspaceRepo) ByID(ctx context.Context, id string) (domain.Workspace, error) {
	return scanWorkspace(r.q.QueryRowContext(ctx, `
SELECT id, org_id, type, name, path, created_by, created_at, updated_at
FROM workspaces
WHERE id = ?`, id))
}

func (r workspaceRepo) Update(ctx context.Context, workspace domain.Workspace) error {
	_, err := r.q.ExecContext(ctx, `
UPDATE workspaces SET name = ?, path = ?, updated_at = ? WHERE id = ?`,
		workspace.Name, workspace.Path, formatTime(workspace.UpdatedAt), workspace.ID,
	)
	return err
}

func (r channelAgentRepo) ReplaceForChannel(ctx context.Context, channelID string, agents []domain.ChannelAgent) error {
	if _, err := r.q.ExecContext(ctx, `DELETE FROM channel_agents WHERE channel_id = ?`, channelID); err != nil {
		return err
	}
	for _, agent := range agents {
		if agent.ChannelID == "" {
			agent.ChannelID = channelID
		}
		if _, err := r.q.ExecContext(ctx, `
INSERT INTO channel_agents (channel_id, agent_id, run_workspace_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`,
			agent.ChannelID, agent.AgentID, nullableString(agent.RunWorkspaceID),
			formatTime(agent.CreatedAt), formatTime(agent.UpdatedAt),
		); err != nil {
			return err
		}
	}
	return nil
}

func (r channelAgentRepo) ListByChannel(ctx context.Context, channelID string) ([]domain.ChannelAgent, error) {
	rows, err := r.q.QueryContext(ctx, `
SELECT channel_id, agent_id, run_workspace_id, created_at, updated_at
FROM channel_agents
WHERE channel_id = ?
ORDER BY created_at ASC, agent_id ASC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.ChannelAgent
	for rows.Next() {
		agent, err := scanChannelAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

func (r bindingRepo) Upsert(ctx context.Context, binding domain.ConversationBinding) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO conversation_bindings (id, org_id, conversation_type, conversation_id, agent_id, workspace_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(conversation_type, conversation_id) DO UPDATE SET
  org_id = excluded.org_id,
  agent_id = excluded.agent_id,
  workspace_id = excluded.workspace_id,
  updated_at = excluded.updated_at`,
		binding.ID, binding.OrganizationID, string(binding.ConversationType), binding.ConversationID,
		binding.AgentID, binding.WorkspaceID, formatTime(binding.CreatedAt), formatTime(binding.UpdatedAt),
	)
	return err
}

func (r bindingRepo) ByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string) (domain.ConversationBinding, error) {
	return scanBinding(r.q.QueryRowContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, agent_id, workspace_id, created_at, updated_at
FROM conversation_bindings
WHERE conversation_type = ? AND conversation_id = ?`, string(conversationType), conversationID))
}

func (r sessionRepo) SetAgentSession(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, providerSessionID string, status string) error {
	id := agentID + ":" + string(conversationType) + ":" + conversationID
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_sessions (id, agent_id, conversation_type, conversation_id, provider_session_id, status, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_id, conversation_type, conversation_id) DO UPDATE SET
  provider_session_id = excluded.provider_session_id,
  status = excluded.status,
  updated_at = excluded.updated_at`,
		id, agentID, string(conversationType), conversationID, providerSessionID, status, formatTime(time.Now().UTC()),
	)
	return err
}

func (r sessionRepo) ResetAgentSessionContext(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, contextStartedAt time.Time) error {
	id := agentID + ":" + string(conversationType) + ":" + conversationID
	now := time.Now().UTC()
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_sessions (id, agent_id, conversation_type, conversation_id, provider_session_id, status, context_started_at, updated_at)
VALUES (?, ?, ?, ?, '', 'reset', ?, ?)
ON CONFLICT(agent_id, conversation_type, conversation_id) DO UPDATE SET
  provider_session_id = '',
  status = 'reset',
  context_started_at = excluded.context_started_at,
  updated_at = excluded.updated_at`,
		id, agentID, string(conversationType), conversationID, formatTime(contextStartedAt), formatTime(now),
	)
	return err
}

func (r sessionRepo) SetAgentSessionContextStartedAt(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, contextStartedAt time.Time) error {
	id := agentID + ":" + string(conversationType) + ":" + conversationID
	now := time.Now().UTC()
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agent_sessions (id, agent_id, conversation_type, conversation_id, provider_session_id, status, context_started_at, updated_at)
VALUES (?, ?, ?, ?, '', 'completed', ?, ?)
ON CONFLICT(agent_id, conversation_type, conversation_id) DO UPDATE SET
  context_started_at = excluded.context_started_at,
  updated_at = excluded.updated_at`,
		id, agentID, string(conversationType), conversationID, formatTime(contextStartedAt), formatTime(now),
	)
	return err
}

func (r sessionRepo) ByConversation(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string) (domain.AgentSession, error) {
	return scanAgentSession(r.q.QueryRowContext(ctx, `
SELECT agent_id, conversation_type, conversation_id, provider_session_id, status, context_started_at, updated_at
FROM agent_sessions
WHERE agent_id = ? AND conversation_type = ? AND conversation_id = ?`,
		agentID, string(conversationType), conversationID,
	))
}

func scanOrganization(scanner interface {
	Scan(dest ...any) error
}) (domain.Organization, error) {
	var org domain.Organization
	var createdAt string
	if err := scanner.Scan(&org.ID, &org.Name, &createdAt); err != nil {
		return domain.Organization{}, err
	}
	var err error
	org.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Organization{}, err
	}
	return org, nil
}

func scanNotificationSettings(scanner interface {
	Scan(dest ...any) error
}) (domain.NotificationSettings, error) {
	var settings domain.NotificationSettings
	var createdAt, updatedAt string
	var webhookEnabled int
	if err := scanner.Scan(
		&settings.OrganizationID, &webhookEnabled, &settings.WebhookURL, &settings.WebhookSecret,
		&createdAt, &updatedAt,
	); err != nil {
		return domain.NotificationSettings{}, err
	}
	var err error
	settings.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	settings.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	settings.WebhookEnabled = webhookEnabled != 0
	settings.WebhookSecretConfigured = settings.WebhookSecret != ""
	return settings, nil
}

func scanProject(scanner interface {
	Scan(dest ...any) error
}) (domain.Project, error) {
	var project domain.Project
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&project.ID, &project.OrganizationID, &project.Name, &project.WorkspaceID,
		&project.CreatedBy, &createdAt, &updatedAt,
	); err != nil {
		return domain.Project{}, err
	}
	var err error
	project.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Project{}, err
	}
	project.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func scanChannel(scanner interface {
	Scan(dest ...any) error
}) (domain.Channel, error) {
	var channel domain.Channel
	var channelType, createdAt, updatedAt string
	var archivedAt sql.NullString
	if err := scanner.Scan(
		&channel.ID, &channel.OrganizationID, &channel.ProjectID, &channelType, &channel.Name,
		&createdAt, &updatedAt, &archivedAt,
	); err != nil {
		return domain.Channel{}, err
	}
	channel.Type = domain.ChannelType(channelType)
	var err error
	channel.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Channel{}, err
	}
	if updatedAt == "" {
		channel.UpdatedAt = channel.CreatedAt
	} else {
		channel.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return domain.Channel{}, err
		}
	}
	channel.ArchivedAt, err = parseNullableTime(archivedAt)
	if err != nil {
		return domain.Channel{}, err
	}
	return channel, nil
}

func scanThread(scanner interface {
	Scan(dest ...any) error
}) (domain.Thread, error) {
	var thread domain.Thread
	var createdAt, updatedAt string
	var archivedAt sql.NullString
	if err := scanner.Scan(
		&thread.ID, &thread.OrganizationID, &thread.ProjectID, &thread.ChannelID, &thread.Title,
		&thread.CreatedBy, &createdAt, &updatedAt, &archivedAt,
	); err != nil {
		return domain.Thread{}, err
	}
	var err error
	thread.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Thread{}, err
	}
	thread.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Thread{}, err
	}
	thread.ArchivedAt, err = parseNullableTime(archivedAt)
	if err != nil {
		return domain.Thread{}, err
	}
	return thread, nil
}

func scanMessage(scanner interface {
	Scan(dest ...any) error
}) (domain.Message, error) {
	var message domain.Message
	var conversationType, senderType, kind, metadataJSON, createdAt string
	if err := scanner.Scan(
		&message.ID, &message.OrganizationID, &conversationType, &message.ConversationID,
		&senderType, &message.SenderID, &kind, &message.Body, &metadataJSON, &createdAt,
	); err != nil {
		return domain.Message{}, err
	}
	message.ConversationType = domain.ConversationType(conversationType)
	message.SenderType = domain.SenderType(senderType)
	message.Kind = domain.MessageKind(kind)
	if metadataJSON != "" && metadataJSON != "{}" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(metadataJSON), &meta); err == nil && len(meta) > 0 {
			message.Metadata = meta
		}
	}
	var err error
	message.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func scanAgent(scanner interface {
	Scan(dest ...any) error
}) (domain.Agent, error) {
	var agent domain.Agent
	var envJSON, createdAt, updatedAt string
	var enabled, fastMode, yoloMode int
	if err := scanner.Scan(
		&agent.ID, &agent.OrganizationID, &agent.BotUserID, &agent.Kind, &agent.Name, &agent.Handle,
		&agent.Model, &agent.Effort, &agent.DefaultWorkspaceID, &agent.ConfigWorkspaceID, &enabled, &fastMode, &yoloMode, &envJSON, &createdAt, &updatedAt,
	); err != nil {
		return domain.Agent{}, err
	}
	if err := json.Unmarshal([]byte(envJSON), &agent.Env); err != nil {
		return domain.Agent{}, err
	}
	var err error
	agent.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.Enabled = enabled != 0
	agent.FastMode = fastMode != 0
	agent.YoloMode = yoloMode != 0
	if agent.ConfigWorkspaceID == "" {
		agent.ConfigWorkspaceID = agent.DefaultWorkspaceID
	}
	if agent.DefaultWorkspaceID == "" {
		agent.DefaultWorkspaceID = agent.ConfigWorkspaceID
	}
	return agent, nil
}

func scanAgentSession(scanner interface {
	Scan(dest ...any) error
}) (domain.AgentSession, error) {
	var session domain.AgentSession
	var conversationType, updatedAt string
	var contextStartedAt sql.NullString
	if err := scanner.Scan(
		&session.AgentID, &conversationType, &session.ConversationID, &session.ProviderSessionID,
		&session.Status, &contextStartedAt, &updatedAt,
	); err != nil {
		return domain.AgentSession{}, err
	}
	session.ConversationType = domain.ConversationType(conversationType)
	var err error
	session.ContextStartedAt, err = parseNullableTime(contextStartedAt)
	if err != nil {
		return domain.AgentSession{}, err
	}
	session.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.AgentSession{}, err
	}
	return session, nil
}

func emptyMapIfNil[T any](values map[string]T) map[string]T {
	if values == nil {
		return map[string]T{}
	}
	return values
}

func scanWorkspace(scanner interface {
	Scan(dest ...any) error
}) (domain.Workspace, error) {
	var workspace domain.Workspace
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&workspace.ID, &workspace.OrganizationID, &workspace.Type, &workspace.Name, &workspace.Path,
		&workspace.CreatedBy, &createdAt, &updatedAt,
	); err != nil {
		return domain.Workspace{}, err
	}
	var err error
	workspace.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Workspace{}, err
	}
	workspace.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Workspace{}, err
	}
	return workspace, nil
}

func scanChannelAgent(scanner interface {
	Scan(dest ...any) error
}) (domain.ChannelAgent, error) {
	var agent domain.ChannelAgent
	var runWorkspaceID sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&agent.ChannelID, &agent.AgentID, &runWorkspaceID, &createdAt, &updatedAt,
	); err != nil {
		return domain.ChannelAgent{}, err
	}
	if runWorkspaceID.Valid {
		agent.RunWorkspaceID = runWorkspaceID.String
	}
	var err error
	agent.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ChannelAgent{}, err
	}
	agent.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.ChannelAgent{}, err
	}
	return agent, nil
}

func scanBinding(scanner interface {
	Scan(dest ...any) error
}) (domain.ConversationBinding, error) {
	var binding domain.ConversationBinding
	var conversationType, createdAt, updatedAt string
	if err := scanner.Scan(
		&binding.ID, &binding.OrganizationID, &conversationType, &binding.ConversationID,
		&binding.AgentID, &binding.WorkspaceID, &createdAt, &updatedAt,
	); err != nil {
		return domain.ConversationBinding{}, err
	}
	binding.ConversationType = domain.ConversationType(conversationType)
	var err error
	binding.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ConversationBinding{}, err
	}
	binding.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.ConversationBinding{}, err
	}
	return binding, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeChannel(channel domain.Channel) domain.Channel {
	if channel.Type == "" {
		channel.Type = domain.ChannelTypeText
	}
	if channel.UpdatedAt.IsZero() {
		channel.UpdatedAt = channel.CreatedAt
	}
	return channel
}

func normalizeAgent(agent domain.Agent) domain.Agent {
	if agent.ConfigWorkspaceID == "" {
		agent.ConfigWorkspaceID = agent.DefaultWorkspaceID
	}
	if agent.DefaultWorkspaceID == "" {
		agent.DefaultWorkspaceID = agent.ConfigWorkspaceID
	}
	if agent.Handle == "" {
		agent.Handle = agent.ID
	}
	if !agent.Enabled && agent.CreatedAt.IsZero() {
		agent.Enabled = true
	}
	return agent
}
