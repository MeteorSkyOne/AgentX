package runtime

import (
	"context"
	"time"
)

// Background task kinds.
const (
	// BackgroundTaskShell is a shell command left running in the background.
	BackgroundTaskShell = "shell"
	// BackgroundTaskMonitor watches a command and wakes the agent on each line
	// it prints.
	BackgroundTaskMonitor = "monitor"
	// BackgroundTaskAgent is a subagent running in the background.
	BackgroundTaskAgent = "agent"
)

// BackgroundTaskRunning is the status of a task that has not ended. Ended
// tasks carry the provider's terminal status (completed, failed, stopped, …).
const BackgroundTaskRunning = "running"

// BackgroundTask is work an agent left running outside its reply.
type BackgroundTask struct {
	ID          string
	Kind        string
	Description string
	// ToolCallID is the tool call that started the task.
	ToolCallID string
	Status     string
	// Summary is the provider's closing note on an ended task.
	Summary string
	// OutputFile is where the task's output is written, when known.
	OutputFile string
	StartedAt  time.Time
	// EndedAt is zero while the task runs.
	EndedAt time.Time
}

// BackgroundTaskReporter is implemented by runtimes that can list the
// background tasks of their sessions and stop them.
type BackgroundTaskReporter interface {
	// SetBackgroundTaskHandler installs the handler that receives a session's
	// full task list whenever it changes. It must not block.
	SetBackgroundTaskHandler(handler func(sessionKey string, tasks []BackgroundTask))
	StopBackgroundTask(ctx context.Context, sessionKey string, taskID string) error
}
