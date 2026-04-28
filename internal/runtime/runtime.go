package runtime

import (
	"context"
	"strconv"
	"strings"
)

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
	Prompt      string
	Context     string
	Attachments []Attachment
}

type Attachment struct {
	ID          string
	Filename    string
	ContentType string
	Kind        string
	SizeBytes   int64
	LocalPath   string
}

func (i Input) RenderedPrompt() string {
	prompt := i.promptWithAttachmentReferences()
	if i.Context == "" {
		return prompt
	}
	return i.Context + "\n\nCurrent user message:\n" + prompt
}

func (i Input) promptWithAttachmentReferences() string {
	prompt := i.Prompt
	if len(i.Attachments) == 0 {
		return prompt
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "Please review the attached file(s)."
	}
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\nAttachments available to this agent:\n")
	for _, attachment := range i.Attachments {
		b.WriteString("- ")
		if attachment.Filename != "" {
			b.WriteString(attachment.Filename)
		} else {
			b.WriteString(attachment.ID)
		}
		if attachment.Kind != "" || attachment.ContentType != "" || attachment.SizeBytes > 0 {
			b.WriteString(" (")
			var wrote bool
			if attachment.Kind != "" {
				b.WriteString(attachment.Kind)
				wrote = true
			}
			if attachment.ContentType != "" {
				if wrote {
					b.WriteString(", ")
				}
				b.WriteString(attachment.ContentType)
				wrote = true
			}
			if attachment.SizeBytes > 0 {
				if wrote {
					b.WriteString(", ")
				}
				b.WriteString(strconv.FormatInt(attachment.SizeBytes, 10))
				b.WriteString(" bytes")
			}
			b.WriteString(")")
		}
		if attachment.LocalPath != "" {
			b.WriteString(": ")
			b.WriteString(attachment.LocalPath)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
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
	Usage        *Usage
	Error        string
	StaleSession bool
}

type Usage struct {
	Model                    string
	InputTokens              *int64
	CachedInputTokens        *int64
	CacheCreationInputTokens *int64
	CacheReadInputTokens     *int64
	OutputTokens             *int64
	ReasoningOutputTokens    *int64
	TotalTokens              *int64
	TotalCostUSD             *float64
	DurationMS               *int64
	DurationAPIMS            *int64
	Raw                      any
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
