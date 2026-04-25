package sqlite

import (
	"context"
	"database/sql"
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

type channelRepo struct {
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

func (s *Store) Channels() store.ChannelStore {
	return channelRepo{q: s.db}
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

func (t *txStore) Channels() store.ChannelStore {
	return channelRepo{q: t.tx}
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

func (r channelRepo) Create(ctx context.Context, channel domain.Channel) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO channels (id, org_id, name, created_at) VALUES (?, ?, ?, ?)`,
		channel.ID, channel.OrganizationID, channel.Name, formatTime(channel.CreatedAt),
	)
	return err
}

func (r channelRepo) ListByOrganization(ctx context.Context, orgID string) ([]domain.Channel, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT id, org_id, name, created_at FROM channels WHERE org_id = ? ORDER BY created_at ASC`,
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

func (r channelRepo) ByID(ctx context.Context, id string) (domain.Channel, error) {
	return scanChannel(r.q.QueryRowContext(ctx,
		`SELECT id, org_id, name, created_at FROM channels WHERE id = ?`,
		id,
	))
}

func (r messageRepo) Create(ctx context.Context, message domain.Message) error {
	_, err := r.q.ExecContext(ctx, `
INSERT INTO messages (id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		message.ID, message.OrganizationID, string(message.ConversationType), message.ConversationID,
		string(message.SenderType), message.SenderID, string(message.Kind), message.Body, formatTime(message.CreatedAt),
	)
	return err
}

func (r messageRepo) List(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, created_at
FROM messages
WHERE conversation_type = ? AND conversation_id = ?
ORDER BY created_at ASC
LIMIT ?`, string(conversationType), conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
	_, err := r.q.ExecContext(ctx, `
INSERT INTO agents (id, org_id, bot_user_id, kind, name, model, default_workspace_id, env_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, '{}', ?, ?)`,
		agent.ID, agent.OrganizationID, agent.BotUserID, agent.Kind, agent.Name, agent.Model,
		agent.DefaultWorkspaceID, formatTime(agent.CreatedAt), formatTime(agent.UpdatedAt),
	)
	return err
}

func (r agentRepo) ByID(ctx context.Context, id string) (domain.Agent, error) {
	return scanAgent(r.q.QueryRowContext(ctx, `
SELECT id, org_id, bot_user_id, kind, name, model, default_workspace_id, created_at, updated_at
FROM agents
WHERE id = ?`, id))
}

func (r agentRepo) DefaultForOrganization(ctx context.Context, orgID string) (domain.Agent, error) {
	return scanAgent(r.q.QueryRowContext(ctx, `
SELECT id, org_id, bot_user_id, kind, name, model, default_workspace_id, created_at, updated_at
FROM agents
WHERE org_id = ?
ORDER BY created_at ASC
LIMIT 1`, orgID))
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

func scanChannel(scanner interface {
	Scan(dest ...any) error
}) (domain.Channel, error) {
	var channel domain.Channel
	var createdAt string
	if err := scanner.Scan(&channel.ID, &channel.OrganizationID, &channel.Name, &createdAt); err != nil {
		return domain.Channel{}, err
	}
	var err error
	channel.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Channel{}, err
	}
	return channel, nil
}

func scanMessage(scanner interface {
	Scan(dest ...any) error
}) (domain.Message, error) {
	var message domain.Message
	var conversationType, senderType, kind, createdAt string
	if err := scanner.Scan(
		&message.ID, &message.OrganizationID, &conversationType, &message.ConversationID,
		&senderType, &message.SenderID, &kind, &message.Body, &createdAt,
	); err != nil {
		return domain.Message{}, err
	}
	message.ConversationType = domain.ConversationType(conversationType)
	message.SenderType = domain.SenderType(senderType)
	message.Kind = domain.MessageKind(kind)
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
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&agent.ID, &agent.OrganizationID, &agent.BotUserID, &agent.Kind, &agent.Name, &agent.Model,
		&agent.DefaultWorkspaceID, &createdAt, &updatedAt,
	); err != nil {
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
	return agent, nil
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
