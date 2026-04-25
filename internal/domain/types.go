package domain

import "time"

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type ConversationType string

const (
	ConversationChannel ConversationType = "channel"
	ConversationThread  ConversationType = "thread"
	ConversationDM      ConversationType = "dm"
)

type SenderType string

const (
	SenderUser   SenderType = "user"
	SenderBot    SenderType = "bot"
	SenderSystem SenderType = "system"
)

type MessageKind string

const (
	MessageText  MessageKind = "text"
	MessageEvent MessageKind = "event"
)

type User struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Channel struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
}

type BotUser struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	DisplayName    string    `json:"display_name"`
	CreatedAt      time.Time `json:"created_at"`
}

type Agent struct {
	ID                 string    `json:"id"`
	OrganizationID     string    `json:"organization_id"`
	BotUserID          string    `json:"bot_user_id"`
	Kind               string    `json:"kind"`
	Name               string    `json:"name"`
	Model              string    `json:"model"`
	DefaultWorkspaceID string    `json:"default_workspace_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Workspace struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Type           string    `json:"type"`
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ConversationBinding struct {
	ID               string           `json:"id"`
	OrganizationID   string           `json:"organization_id"`
	ConversationType ConversationType `json:"conversation_type"`
	ConversationID   string           `json:"conversation_id"`
	AgentID          string           `json:"agent_id"`
	WorkspaceID      string           `json:"workspace_id"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type Message struct {
	ID               string           `json:"id"`
	OrganizationID   string           `json:"organization_id"`
	ConversationType ConversationType `json:"conversation_type"`
	ConversationID   string           `json:"conversation_id"`
	SenderType       SenderType       `json:"sender_type"`
	SenderID         string           `json:"sender_id"`
	Kind             MessageKind      `json:"kind"`
	Body             string           `json:"body"`
	CreatedAt        time.Time        `json:"created_at"`
}
