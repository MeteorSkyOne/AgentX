package claude

import "github.com/meteorsky/agentx/internal/runtime"

// BackgroundTracker follows the lifecycle of subagents that Claude Code runs in
// the background.
//
// Background subagents outlive the result message that ends the main agent's
// reply: the CLI emits a result as soon as the subagent is launched, keeps the
// subagent running, and then wakes the main agent to report once it finishes —
// producing a second result. A turn is only over when no background task is
// outstanding and the main agent has had its final say.
type BackgroundTracker struct {
	// tasks maps each outstanding task to the tool call that spawned it, so a
	// task reaching a terminal state can be attributed back to that call.
	tasks               map[string]string
	awaitingFinalResult bool
	seen                bool
}

// TaskSignal describes one subagent task changing state, attributed to the
// tool call that spawned it. ToolUseID may be empty when the CLI never told us
// which call the task belongs to.
type TaskSignal struct {
	TaskID    string
	ToolUseID string
	Status    string
}

// TrackOutcome is what a single system message did to the tracked task set.
type TrackOutcome struct {
	// Drained means this message finished the last outstanding task: the main
	// agent is about to be woken up, so any result already in hand is stale.
	Drained bool
	// Started and Finished list the tasks this message opened or closed, for
	// surfacing subagent state to the UI.
	Started  []TaskSignal
	Finished []TaskSignal
}

func NewBackgroundTracker() *BackgroundTracker {
	return &BackgroundTracker{tasks: map[string]string{}}
}

// TrackSystemMessage folds a system message into the tracked task set.
func (t *BackgroundTracker) TrackSystemMessage(payload map[string]any) TrackOutcome {
	var outcome TrackOutcome
	switch StringValue(payload, "subtype") {
	case "task_started":
		if signal, added := t.add(StringValue(payload, "task_id"), StringValue(payload, "tool_use_id")); added {
			outcome.Started = append(outcome.Started, signal)
		}
	case "background_tasks_changed":
		// The message carries the full current task set, so reconcile against
		// it: unknown live tasks are picked up, and a tracked task that is
		// listed as terminal — or no longer listed at all — has finished, even
		// if its task_notification was never seen.
		tasks, _ := payload["tasks"].([]any)
		live := map[string]struct{}{}
		for _, item := range tasks {
			task, _ := item.(map[string]any)
			taskID := StringValue(task, "task_id")
			if taskID == "" || isTerminalTaskStatus(StringValue(task, "status")) {
				continue
			}
			live[taskID] = struct{}{}
			if signal, added := t.add(taskID, StringValue(task, "tool_use_id")); added {
				outcome.Started = append(outcome.Started, signal)
			}
		}
		for taskID := range t.tasks {
			if _, stillLive := live[taskID]; stillLive {
				continue
			}
			signal, drained := t.finish(taskID, "completed")
			outcome.Finished = append(outcome.Finished, signal)
			if drained {
				outcome.Drained = true
			}
		}
	case "task_notification":
		if status := StringValue(payload, "status"); isTerminalTaskStatus(status) {
			taskID := StringValue(payload, "task_id")
			if _, tracked := t.tasks[taskID]; tracked {
				signal, drained := t.finish(taskID, status)
				outcome.Finished = append(outcome.Finished, signal)
				outcome.Drained = drained
			}
		}
	case "task_updated":
		patch, _ := payload["patch"].(map[string]any)
		if status := StringValue(patch, "status"); isTerminalTaskStatus(status) {
			taskID := StringValue(payload, "task_id")
			if _, tracked := t.tasks[taskID]; tracked {
				signal, drained := t.finish(taskID, status)
				outcome.Finished = append(outcome.Finished, signal)
				outcome.Drained = drained
			}
		}
	}
	return outcome
}

// add tracks a task, reporting whether it was new. An already-tracked task
// keeps its known tool call unless this message supplies one.
func (t *BackgroundTracker) add(taskID string, toolUseID string) (TaskSignal, bool) {
	if taskID == "" {
		return TaskSignal{}, false
	}
	t.seen = true
	if existing, tracked := t.tasks[taskID]; tracked {
		if existing == "" && toolUseID != "" {
			t.tasks[taskID] = toolUseID
		}
		return TaskSignal{}, false
	}
	t.tasks[taskID] = toolUseID
	return TaskSignal{TaskID: taskID, ToolUseID: toolUseID}, true
}

// Seen reports whether this turn ever ran a task. Callers that see results only
// after the fact — the ephemeral CLI buffers every result until it exits —
// cannot use Waiting to decide when a turn is over, and instead defer to the
// process exiting once a task has appeared.
func (t *BackgroundTracker) Seen() bool {
	return t.seen
}

// finish closes a tracked task, reporting whether it was the last one.
func (t *BackgroundTracker) finish(taskID string, status string) (TaskSignal, bool) {
	toolUseID := t.tasks[taskID]
	delete(t.tasks, taskID)
	signal := TaskSignal{TaskID: taskID, ToolUseID: toolUseID, Status: status}
	if len(t.tasks) > 0 {
		return signal, false
	}
	t.awaitingFinalResult = true
	return signal, true
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

// SubagentSignalItems converts an outcome's task transitions into process
// items the UI uses to show a subagent's real running state — the spawning
// tool call's own result arrives when the task launches, not when it ends, so
// it cannot serve as the completion signal. Tasks with no known tool call are
// skipped: there is nothing to attach them to.
func SubagentSignalItems(outcome TrackOutcome) []runtime.ProcessItem {
	items := make([]runtime.ProcessItem, 0, len(outcome.Started)+len(outcome.Finished))
	for _, signal := range outcome.Started {
		if signal.ToolUseID == "" {
			continue
		}
		items = append(items, runtime.ProcessItem{Type: "subagent_started", ToolCallID: signal.ToolUseID})
	}
	for _, signal := range outcome.Finished {
		if signal.ToolUseID == "" {
			continue
		}
		items = append(items, runtime.ProcessItem{Type: "subagent_completed", ToolCallID: signal.ToolUseID, Status: signal.Status})
	}
	return items
}

func isTerminalTaskStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "canceled", "error", "timeout":
		return true
	}
	return false
}
