package domain

import "time"

type EventType string

const (
	EventMessageCreated             EventType = "MessageCreated"
	EventMessageUpdated             EventType = "MessageUpdated"
	EventMessageDeleted             EventType = "MessageDeleted"
	EventMessageHistoryStarted      EventType = "MessageHistoryStarted"
	EventMessageHistoryChunk        EventType = "MessageHistoryChunk"
	EventMessageHistoryCompleted    EventType = "MessageHistoryCompleted"
	EventConversationBindingUpdated EventType = "ConversationBindingUpdated"
	EventAgentRunStarted            EventType = "AgentRunStarted"
	EventAgentOutputDelta           EventType = "AgentOutputDelta"
	EventAgentRunCompleted          EventType = "AgentRunCompleted"
	EventAgentRunCanceled           EventType = "AgentRunCanceled"
	EventAgentRunFailed             EventType = "AgentRunFailed"
	EventAgentInputRequest          EventType = "AgentInputRequest"
	EventAgentRunProgress           EventType = "AgentRunProgress"
	EventAgentPromptQueued          EventType = "AgentPromptQueued"
	EventAgentPromptQueueRemoved    EventType = "AgentPromptQueueRemoved"
	// EventAgentBackgroundTasksUpdated carries an agent's full background task
	// list in a conversation whenever it changes.
	EventAgentBackgroundTasksUpdated EventType = "AgentBackgroundTasksUpdated"
)

type Event struct {
	ID               string           `json:"id"`
	Type             EventType        `json:"type"`
	OrganizationID   string           `json:"organization_id"`
	ConversationType ConversationType `json:"conversation_type,omitempty"`
	ConversationID   string           `json:"conversation_id,omitempty"`
	Payload          any              `json:"payload"`
	CreatedAt        time.Time        `json:"created_at"`
}

type MessageCreatedPayload struct {
	Message Message `json:"message"`
}

type MessageUpdatedPayload struct {
	Message Message `json:"message"`
}

type MessageDeletedPayload struct {
	MessageID string `json:"message_id"`
}

type MessageHistoryStartedPayload struct {
	Before string `json:"before,omitempty"`
}

type MessageHistoryChunkPayload struct {
	Messages []Message `json:"messages"`
}

type MessageHistoryCompletedPayload struct {
	HasMore bool   `json:"has_more"`
	Before  string `json:"before,omitempty"`
}

type AgentOutputDeltaPayload struct {
	RunID     string        `json:"run_id"`
	AgentID   string        `json:"agent_id"`
	Text      string        `json:"text"`
	Thinking  string        `json:"thinking,omitempty"`
	Process   []ProcessItem `json:"process,omitempty"`
	ClearText bool          `json:"clear_text,omitempty"`
	Team      *TeamMetadata `json:"team,omitempty"`
}

// AgentRunProgressPayload carries the run's latest goal and step-list
// snapshots; each field is present once the agent has reported it.
type AgentRunProgressPayload struct {
	RunID   string        `json:"run_id"`
	AgentID string        `json:"agent_id"`
	Goal    *AgentGoal    `json:"goal,omitempty"`
	Plan    *AgentPlan    `json:"plan,omitempty"`
	Team    *TeamMetadata `json:"team,omitempty"`
}

// BackgroundTask is work an agent left running outside its reply: a shell
// command, a monitor, or a subagent. Kind is "shell", "monitor" or "agent";
// Status is "running" until the task ends.
type BackgroundTask struct {
	ID          string     `json:"id"`
	AgentID     string     `json:"agent_id"`
	Kind        string     `json:"kind"`
	Description string     `json:"description,omitempty"`
	ToolCallID  string     `json:"tool_call_id,omitempty"`
	Status      string     `json:"status"`
	Summary     string     `json:"summary,omitempty"`
	HasOutput   bool       `json:"has_output"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
}

type AgentBackgroundTasksPayload struct {
	AgentID string           `json:"agent_id"`
	Tasks   []BackgroundTask `json:"tasks"`
}

type AgentRunPayload struct {
	RunID   string        `json:"run_id"`
	AgentID string        `json:"agent_id"`
	Team    *TeamMetadata `json:"team,omitempty"`
}

type AgentRunFailedPayload struct {
	RunID   string        `json:"run_id"`
	AgentID string        `json:"agent_id,omitempty"`
	Error   string        `json:"error"`
	Team    *TeamMetadata `json:"team,omitempty"`
	// Persisted reports whether the failure was also stored as a chat message
	// (so it survives a page refresh). When true, clients can drop the
	// ephemeral streaming entry and rely on the persisted message instead.
	Persisted bool `json:"persisted,omitempty"`
}

type AgentInputRequestPayload struct {
	RunID      string                    `json:"run_id"`
	AgentID    string                    `json:"agent_id"`
	QuestionID string                    `json:"question_id"`
	Question   string                    `json:"question"`
	Options    []AgentInputRequestOption `json:"options,omitempty"`
	Team       *TeamMetadata             `json:"team,omitempty"`
}

type AgentInputRequestOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type AgentPromptQueuedPayload struct {
	QueueID   string    `json:"queue_id"`
	MessageID string    `json:"message_id"`
	AgentID   string    `json:"agent_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	CanSteer  bool      `json:"can_steer"`
}

type AgentPromptQueueRemovedPayload struct {
	QueueID string `json:"queue_id"`
	Status  string `json:"status"`
}

type TeamMetadata struct {
	SessionID       string `json:"session_id"`
	RootMessageID   string `json:"root_message_id"`
	LeaderAgentID   string `json:"leader_agent_id"`
	Phase           string `json:"phase"`
	Turn            int    `json:"turn"`
	SourceMessageID string `json:"source_message_id,omitempty"`
}
