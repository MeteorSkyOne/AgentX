package claude

// BackgroundTracker follows the lifecycle of subagents that Claude Code runs in
// the background.
//
// Background subagents outlive the result message that ends the main agent's
// reply: the CLI emits a result as soon as the subagent is launched, keeps the
// subagent running, and then wakes the main agent to report once it finishes —
// producing a second result. A turn is only over when no background task is
// outstanding and the main agent has had its final say.
type BackgroundTracker struct {
	tasks               map[string]struct{}
	awaitingFinalResult bool
	seen                bool
}

func NewBackgroundTracker() *BackgroundTracker {
	return &BackgroundTracker{tasks: map[string]struct{}{}}
}

// TrackSystemMessage folds a system message into the tracked task set. It
// reports whether this message drained the last outstanding task, which means
// the main agent is about to be woken up and any result already in hand is
// stale.
func (t *BackgroundTracker) TrackSystemMessage(payload map[string]any) bool {
	switch StringValue(payload, "subtype") {
	case "task_started":
		t.add(StringValue(payload, "task_id"))
	case "background_tasks_changed":
		tasks, _ := payload["tasks"].([]any)
		for _, item := range tasks {
			task, _ := item.(map[string]any)
			t.add(StringValue(task, "task_id"))
		}
	case "task_notification":
		if isTerminalTaskStatus(StringValue(payload, "status")) {
			return t.finish(StringValue(payload, "task_id"))
		}
	case "task_updated":
		patch, _ := payload["patch"].(map[string]any)
		if isTerminalTaskStatus(StringValue(patch, "status")) {
			return t.finish(StringValue(payload, "task_id"))
		}
	}
	return false
}

func (t *BackgroundTracker) add(taskID string) {
	if taskID == "" {
		return
	}
	t.seen = true
	t.tasks[taskID] = struct{}{}
}

// Seen reports whether this turn ever ran a task. Callers that see results only
// after the fact — the ephemeral CLI buffers every result until it exits —
// cannot use Waiting to decide when a turn is over, and instead defer to the
// process exiting once a task has appeared.
func (t *BackgroundTracker) Seen() bool {
	return t.seen
}

func (t *BackgroundTracker) finish(taskID string) bool {
	if taskID == "" {
		return false
	}
	if _, tracked := t.tasks[taskID]; !tracked {
		return false
	}
	delete(t.tasks, taskID)
	if len(t.tasks) > 0 {
		return false
	}
	t.awaitingFinalResult = true
	return true
}

// Waiting reports whether the turn must stay open regardless of any result
// message already received.
func (t *BackgroundTracker) Waiting() bool {
	return len(t.tasks) > 0 || t.awaitingFinalResult
}

// NoteResult records that a result arrived, satisfying the wait for the main
// agent's final word.
func (t *BackgroundTracker) NoteResult() {
	t.awaitingFinalResult = false
}

func (t *BackgroundTracker) PendingCount() int {
	return len(t.tasks)
}

func isTerminalTaskStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "canceled", "error", "timeout":
		return true
	}
	return false
}
