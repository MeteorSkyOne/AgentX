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

type NotificationSettings struct {
	OrganizationID          string    `json:"organization_id"`
	WebhookEnabled          bool      `json:"webhook_enabled"`
	WebhookURL              string    `json:"webhook_url"`
	WebhookSecret           string    `json:"-"`
	WebhookSecretConfigured bool      `json:"webhook_secret_configured"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type UserPreferences struct {
	UserID    string    `json:"-"`
	ShowTTFT  bool      `json:"show_ttft"`
	ShowTPS   bool      `json:"show_tps"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
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
	Description        string            `json:"description"`
	Model              string            `json:"model"`
	Effort             string            `json:"effort"`
	ConfigWorkspaceID  string            `json:"config_workspace_id"`
	DefaultWorkspaceID string            `json:"default_workspace_id,omitempty"`
	Enabled            bool              `json:"enabled"`
	FastMode           bool              `json:"fast_mode"`
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
	ID               string            `json:"id"`
	OrganizationID   string            `json:"organization_id"`
	ConversationType ConversationType  `json:"conversation_type"`
	ConversationID   string            `json:"conversation_id"`
	SenderType       SenderType        `json:"sender_type"`
	SenderID         string            `json:"sender_id"`
	Kind             MessageKind       `json:"kind"`
	Body             string            `json:"body"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
	ReplyToMessageID string            `json:"reply_to_message_id,omitempty"`
	ReplyTo          *MessageReference `json:"reply_to,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
}

type MetricsUsage struct {
	Model                    string
	InputTokens              *int64
	CachedInputTokens        *int64
	CacheCreationInputTokens *int64
	CacheReadInputTokens     *int64
	OutputTokens             *int64
	ReasoningOutputTokens    *int64
	TotalTokens              *int64
	TotalCostUSD             *float64
	Raw                      any
}

type MessageMetricsSummary struct {
	RunID        string   `json:"run_id"`
	Provider     string   `json:"provider"`
	TTFTMS       *int64   `json:"ttft_ms"`
	TPS          *float64 `json:"tps"`
	DurationMS   *int64   `json:"duration_ms"`
	InputTokens  *int64   `json:"input_tokens"`
	OutputTokens *int64   `json:"output_tokens"`
	TotalTokens  *int64   `json:"total_tokens"`
	CacheHitRate *float64 `json:"cache_hit_rate"`
}

type AgentRunMetric struct {
	RunID                    string           `json:"run_id"`
	OrganizationID           string           `json:"organization_id"`
	ProjectID                string           `json:"project_id,omitempty"`
	ProjectName              string           `json:"project_name,omitempty"`
	ChannelID                string           `json:"channel_id,omitempty"`
	ChannelName              string           `json:"channel_name,omitempty"`
	ThreadID                 string           `json:"thread_id,omitempty"`
	ThreadTitle              string           `json:"thread_title,omitempty"`
	ConversationType         ConversationType `json:"conversation_type"`
	ConversationID           string           `json:"conversation_id"`
	MessageID                string           `json:"message_id"`
	ResponseMessageID        string           `json:"response_message_id,omitempty"`
	AgentID                  string           `json:"agent_id"`
	AgentName                string           `json:"agent_name"`
	Provider                 string           `json:"provider"`
	Model                    string           `json:"model"`
	Status                   string           `json:"status"`
	RunCount                 int64            `json:"run_count,omitempty"`
	CompletedRuns            int64            `json:"completed_runs,omitempty"`
	FailedRuns               int64            `json:"failed_runs,omitempty"`
	StartedAt                time.Time        `json:"started_at"`
	FirstTokenAt             *time.Time       `json:"first_token_at"`
	CompletedAt              *time.Time       `json:"completed_at"`
	TTFTMS                   *int64           `json:"ttft_ms"`
	DurationMS               *int64           `json:"duration_ms"`
	TPS                      *float64         `json:"tps"`
	InputTokens              *int64           `json:"input_tokens"`
	CachedInputTokens        *int64           `json:"cached_input_tokens"`
	CacheCreationInputTokens *int64           `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int64           `json:"cache_read_input_tokens"`
	OutputTokens             *int64           `json:"output_tokens"`
	ReasoningOutputTokens    *int64           `json:"reasoning_output_tokens"`
	TotalTokens              *int64           `json:"total_tokens"`
	CacheHitRate             *float64         `json:"cache_hit_rate"`
	TotalCostUSD             *float64         `json:"total_cost_usd"`
	RawUsageJSON             string           `json:"-"`
	CreatedAt                time.Time        `json:"created_at"`
}

type MessageReference struct {
	MessageID  string     `json:"message_id"`
	Deleted    bool       `json:"deleted,omitempty"`
	SenderType SenderType `json:"sender_type,omitempty"`
	SenderID   string     `json:"sender_id,omitempty"`
	Body       string     `json:"body,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
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
