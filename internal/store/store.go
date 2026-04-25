package store

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type Store interface {
	Tx(ctx context.Context, fn func(Tx) error) error
	Users() UserStore
	Organizations() OrganizationStore
	Channels() ChannelStore
	Messages() MessageStore
	BotUsers() BotUserStore
	Agents() AgentStore
	Workspaces() WorkspaceStore
	Bindings() BindingStore
	Sessions() SessionStore
}

type Tx interface {
	Users() UserStore
	Organizations() OrganizationStore
	Channels() ChannelStore
	Messages() MessageStore
	BotUsers() BotUserStore
	Agents() AgentStore
	Workspaces() WorkspaceStore
	Bindings() BindingStore
	Sessions() SessionStore
}

type UserStore interface {
	Create(ctx context.Context, user domain.User) error
	ByID(ctx context.Context, id string) (domain.User, error)
	CreateAPISession(ctx context.Context, token string, userID string) error
	UserIDByAPISession(ctx context.Context, token string) (string, error)
}

type OrganizationStore interface {
	Create(ctx context.Context, org domain.Organization) error
	ListForUser(ctx context.Context, userID string) ([]domain.Organization, error)
	AddMember(ctx context.Context, orgID string, userID string, role domain.Role) error
}

type ChannelStore interface {
	Create(ctx context.Context, channel domain.Channel) error
	ListByOrganization(ctx context.Context, orgID string) ([]domain.Channel, error)
	ByID(ctx context.Context, id string) (domain.Channel, error)
}

type MessageStore interface {
	Create(ctx context.Context, message domain.Message) error
	List(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error)
}

type BotUserStore interface {
	Create(ctx context.Context, bot domain.BotUser) error
}

type AgentStore interface {
	Create(ctx context.Context, agent domain.Agent) error
	ByID(ctx context.Context, id string) (domain.Agent, error)
	DefaultForOrganization(ctx context.Context, orgID string) (domain.Agent, error)
}

type WorkspaceStore interface {
	Create(ctx context.Context, workspace domain.Workspace) error
	ByID(ctx context.Context, id string) (domain.Workspace, error)
}

type BindingStore interface {
	Upsert(ctx context.Context, binding domain.ConversationBinding) error
	ByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string) (domain.ConversationBinding, error)
}

type SessionStore interface {
	SetAgentSession(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, providerSessionID string, status string) error
}
