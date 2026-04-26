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

const (
	AgentKindFake   = "fake"
	AgentKindCodex  = "codex"
	AgentKindClaude = "claude"
)

type ChannelType string

const (
	ChannelTypeText   ChannelType = "text"
	ChannelTypeThread ChannelType = "thread"
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

type Project struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	WorkspaceID    string    `json:"workspace_id"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Channel struct {
	ID             string      `json:"id"`
	OrganizationID string      `json:"organization_id"`
	ProjectID      string      `json:"project_id"`
	Type           ChannelType `json:"type"`
	Name           string      `json:"name"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
	ArchivedAt     *time.Time  `json:"archived_at,omitempty"`
}

type Thread struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	ProjectID      string     `json:"project_id"`
	ChannelID      string     `json:"channel_id"`
	Title          string     `json:"title"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
}

type BotUser struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	DisplayName    string    `json:"display_name"`
	CreatedAt      time.Time `json:"created_at"`
}

type Agent struct {
	ID                 string            `json:"id"`
	OrganizationID     string            `json:"organization_id"`
	BotUserID          string            `json:"bot_user_id"`
	Kind               string            `json:"kind"`
	Name               string            `json:"name"`
	Handle             string            `json:"handle"`
	Model              string            `json:"model"`
	Effort             string            `json:"effort"`
	ConfigWorkspaceID  string            `json:"config_workspace_id"`
	DefaultWorkspaceID string            `json:"default_workspace_id,omitempty"`
	Enabled            bool              `json:"enabled"`
	YoloMode           bool              `json:"yolo_mode"`
	Env                map[string]string `json:"env,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

type AgentSession struct {
	AgentID           string           `json:"agent_id"`
	ConversationType  ConversationType `json:"conversation_type"`
	ConversationID    string           `json:"conversation_id"`
	ProviderSessionID string           `json:"provider_session_id"`
	Status            string           `json:"status"`
	ContextStartedAt  *time.Time       `json:"context_started_at,omitempty"`
	UpdatedAt         time.Time        `json:"updated_at"`
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

type ChannelAgent struct {
	ChannelID      string    `json:"channel_id"`
	AgentID        string    `json:"agent_id"`
	RunWorkspaceID string    `json:"run_workspace_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
	Metadata         map[string]any   `json:"metadata,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
}

type ProcessItem struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Status     string `json:"status,omitempty"`
	Input      any    `json:"input,omitempty"`
	Output     any    `json:"output,omitempty"`
	Raw        any    `json:"raw,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}
