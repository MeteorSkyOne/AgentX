package runtime

import "context"

type StartSessionRequest struct {
	AgentID    string
	Workspace  string
	Model      string
	Env        map[string]string
	SessionKey string
}

type Input struct {
	Prompt string
}

type EventType string

const (
	EventDelta     EventType = "delta"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
)

type Event struct {
	Type  EventType
	Text  string
	Error string
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
