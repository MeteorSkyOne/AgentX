package store

import (
	"context"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type Store interface {
	Tx(ctx context.Context, fn func(Tx) error) error
	Users() UserStore
	UserPreferences() UserPreferencesStore
	Organizations() OrganizationStore
	NotificationSettings() NotificationSettingsStore
	Projects() ProjectStore
	Channels() ChannelStore
	Threads() ThreadStore
	Messages() MessageStore
	MessageAttachments() MessageAttachmentStore
	BotUsers() BotUserStore
	Agents() AgentStore
	Workspaces() WorkspaceStore
	ChannelAgents() ChannelAgentStore
	Bindings() BindingStore
	Sessions() SessionStore
	Metrics() MetricsStore
}

type Tx interface {
	Users() UserStore
	UserPreferences() UserPreferencesStore
	Organizations() OrganizationStore
	NotificationSettings() NotificationSettingsStore
	Projects() ProjectStore
	Channels() ChannelStore
	Threads() ThreadStore
	Messages() MessageStore
	MessageAttachments() MessageAttachmentStore
	BotUsers() BotUserStore
	Agents() AgentStore
	Workspaces() WorkspaceStore
	ChannelAgents() ChannelAgentStore
	Bindings() BindingStore
	Sessions() SessionStore
	Metrics() MetricsStore
}

type UserStore interface {
	Create(ctx context.Context, user domain.User) error
	ByID(ctx context.Context, id string) (domain.User, error)
	ByUsername(ctx context.Context, username string) (domain.User, error)
	First(ctx context.Context) (domain.User, error)
	HasPassword(ctx context.Context) (bool, error)
	SetCredentials(ctx context.Context, userID string, username string, displayName string, passwordHash string, passwordUpdatedAt time.Time) error
	CreateAPISession(ctx context.Context, tokenHash string, userID string, createdAt time.Time, expiresAt time.Time) error
	UserIDByAPISessionHash(ctx context.Context, tokenHash string, now time.Time) (string, error)
	DeleteAPISession(ctx context.Context, tokenHash string) error
	DeleteAllAPISessions(ctx context.Context) error
}

type UserPreferencesStore interface {
	ByUser(ctx context.Context, userID string) (domain.UserPreferences, error)
	Upsert(ctx context.Context, preferences domain.UserPreferences) error
}

type OrganizationStore interface {
	Any(ctx context.Context) (bool, error)
	Create(ctx context.Context, org domain.Organization) error
	ListForUser(ctx context.Context, userID string) ([]domain.Organization, error)
	MemberRole(ctx context.Context, orgID string, userID string) (domain.Role, error)
	AddMember(ctx context.Context, orgID string, userID string, role domain.Role) error
}

type NotificationSettingsStore interface {
	ByOrganization(ctx context.Context, orgID string) (domain.NotificationSettings, error)
	Upsert(ctx context.Context, settings domain.NotificationSettings) error
}

type ProjectStore interface {
	Create(ctx context.Context, project domain.Project) error
	ListByOrganization(ctx context.Context, orgID string) ([]domain.Project, error)
	ByID(ctx context.Context, id string) (domain.Project, error)
	Update(ctx context.Context, project domain.Project) error
	Delete(ctx context.Context, id string) error
}

type ChannelStore interface {
	Create(ctx context.Context, channel domain.Channel) error
	ListByOrganization(ctx context.Context, orgID string) ([]domain.Channel, error)
	ListByProject(ctx context.Context, projectID string) ([]domain.Channel, error)
	ByID(ctx context.Context, id string) (domain.Channel, error)
	Update(ctx context.Context, channel domain.Channel) error
	Archive(ctx context.Context, id string, archivedAt time.Time) error
}

type ThreadStore interface {
	Create(ctx context.Context, thread domain.Thread) error
	ListByChannel(ctx context.Context, channelID string) ([]domain.Thread, error)
	ByID(ctx context.Context, id string) (domain.Thread, error)
	Update(ctx context.Context, thread domain.Thread) error
	Archive(ctx context.Context, id string, archivedAt time.Time) error
}

type MessageStore interface {
	Create(ctx context.Context, message domain.Message) error
	ByID(ctx context.Context, id string) (domain.Message, error)
	Update(ctx context.Context, message domain.Message) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error)
	ListRecent(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error)
	ListRecentBefore(ctx context.Context, conversationType domain.ConversationType, conversationID string, before time.Time, limit int) ([]domain.Message, error)
}

type MessageAttachmentStore interface {
	Create(ctx context.Context, attachment domain.MessageAttachment) error
	ByID(ctx context.Context, id string) (domain.MessageAttachment, error)
	ListByMessage(ctx context.Context, messageID string) ([]domain.MessageAttachment, error)
	DeleteByMessage(ctx context.Context, messageID string) error
}

type BotUserStore interface {
	Create(ctx context.Context, bot domain.BotUser) error
}

type AgentStore interface {
	Create(ctx context.Context, agent domain.Agent) error
	ByID(ctx context.Context, id string) (domain.Agent, error)
	DefaultForOrganization(ctx context.Context, orgID string) (domain.Agent, error)
	ListByOrganization(ctx context.Context, orgID string) ([]domain.Agent, error)
	ByHandle(ctx context.Context, orgID string, handle string) (domain.Agent, error)
	Update(ctx context.Context, agent domain.Agent) error
}

type WorkspaceStore interface {
	Create(ctx context.Context, workspace domain.Workspace) error
	ByID(ctx context.Context, id string) (domain.Workspace, error)
	Update(ctx context.Context, workspace domain.Workspace) error
}

type ChannelAgentStore interface {
	ReplaceForChannel(ctx context.Context, channelID string, agents []domain.ChannelAgent) error
	ListByChannel(ctx context.Context, channelID string) ([]domain.ChannelAgent, error)
	ListByAgent(ctx context.Context, agentID string) ([]domain.ChannelAgent, error)
	DeleteForAgent(ctx context.Context, agentID string) error
}

type BindingStore interface {
	Upsert(ctx context.Context, binding domain.ConversationBinding) error
	ByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string) (domain.ConversationBinding, error)
}

type SessionStore interface {
	SetAgentSession(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, providerSessionID string, status string) error
	ResetAgentSessionContext(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, contextStartedAt time.Time) error
	SetAgentSessionContextStartedAt(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, contextStartedAt time.Time) error
	ByConversation(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string) (domain.AgentSession, error)
}

type MetricsFilter struct {
	Limit    int
	Provider string
	Group    string
}

type MetricsStore interface {
	Create(ctx context.Context, metric domain.AgentRunMetric) error
	ListByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string, filter MetricsFilter) ([]domain.AgentRunMetric, error)
	ListByChannel(ctx context.Context, channelID string, filter MetricsFilter) ([]domain.AgentRunMetric, error)
	ListByProject(ctx context.Context, projectID string, filter MetricsFilter) ([]domain.AgentRunMetric, error)
	ListAgentSummariesByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string, filter MetricsFilter) ([]domain.AgentRunMetric, error)
	ListAgentSummariesByChannel(ctx context.Context, channelID string, filter MetricsFilter) ([]domain.AgentRunMetric, error)
	ListAgentSummariesByProject(ctx context.Context, projectID string, filter MetricsFilter) ([]domain.AgentRunMetric, error)
}
