package store

import (
	"context"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

type Store interface {
	Tx(ctx context.Context, fn func(Tx) error) error
	Users() UserStore
	Organizations() OrganizationStore
	Projects() ProjectStore
	Channels() ChannelStore
	Threads() ThreadStore
	Messages() MessageStore
	BotUsers() BotUserStore
	Agents() AgentStore
	Workspaces() WorkspaceStore
	ChannelAgents() ChannelAgentStore
	Bindings() BindingStore
	Sessions() SessionStore
}

type Tx interface {
	Users() UserStore
	Organizations() OrganizationStore
	Projects() ProjectStore
	Channels() ChannelStore
	Threads() ThreadStore
	Messages() MessageStore
	BotUsers() BotUserStore
	Agents() AgentStore
	Workspaces() WorkspaceStore
	ChannelAgents() ChannelAgentStore
	Bindings() BindingStore
	Sessions() SessionStore
}

type UserStore interface {
	Create(ctx context.Context, user domain.User) error
	ByID(ctx context.Context, id string) (domain.User, error)
	First(ctx context.Context) (domain.User, error)
	CreateAPISession(ctx context.Context, token string, userID string) error
	UserIDByAPISession(ctx context.Context, token string) (string, error)
}

type OrganizationStore interface {
	Any(ctx context.Context) (bool, error)
	Create(ctx context.Context, org domain.Organization) error
	ListForUser(ctx context.Context, userID string) ([]domain.Organization, error)
	AddMember(ctx context.Context, orgID string, userID string, role domain.Role) error
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
