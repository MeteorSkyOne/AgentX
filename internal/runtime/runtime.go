package runtime

import "context"

type StartSessionRequest struct {
	AgentID              string
	Workspace            string
	InstructionWorkspace string
	Model                string
	Effort               string
	PermissionMode       string
	FastMode             bool
	YoloMode             bool
	Env                  map[string]string
	SessionKey           string
	PreviousSessionID    string
}

type Input struct {
	Prompt  string
	Context string
}

func (i Input) RenderedPrompt() string {
	if i.Context == "" {
		return i.Prompt
	}
	return i.Context + "\n\nCurrent user message:\n" + i.Prompt
}

type EventType string

const (
	EventDelta     EventType = "delta"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
)

type Event struct {
	Type         EventType
	Text         string
	Thinking     string
	Process      []ProcessItem
	Error        string
	StaleSession bool
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

type Runtime interface {
	StartSession(ctx context.Context, req StartSessionRequest) (Session, error)
}

type Session interface {
	Send(ctx context.Context, input Input) error
	Events() <-chan Event
	CurrentSessionID() string
	Alive() bool
	Close(ctx context.Context) error
}
