package domain

import "time"

type EventType string

const (
	EventMessageCreated             EventType = "MessageCreated"
	EventMessageUpdated             EventType = "MessageUpdated"
	EventMessageDeleted             EventType = "MessageDeleted"
	EventConversationBindingUpdated EventType = "ConversationBindingUpdated"
	EventAgentRunStarted            EventType = "AgentRunStarted"
	EventAgentOutputDelta           EventType = "AgentOutputDelta"
	EventAgentRunCompleted          EventType = "AgentRunCompleted"
	EventAgentRunFailed             EventType = "AgentRunFailed"
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

type AgentOutputDeltaPayload struct {
	RunID    string        `json:"run_id"`
	AgentID  string        `json:"agent_id"`
	Text     string        `json:"text"`
	Thinking string        `json:"thinking,omitempty"`
	Process  []ProcessItem `json:"process,omitempty"`
}

type AgentRunPayload struct {
	RunID   string `json:"run_id"`
	AgentID string `json:"agent_id"`
}

type AgentRunFailedPayload struct {
	RunID string `json:"run_id"`
	Error string `json:"error"`
}
