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
//
// Background shell commands are the exception: they are silent for as long as
// they run (their output goes to a file, not the stream) and can run for an
// hour, so they do not hold the turn open. The agent's report once one finishes
// arrives as a turn of its own, outside any reply.
type BackgroundTracker struct {
	// tasks holds each outstanding task, keyed by task id.
	tasks               map[string]*backgroundTask
	awaitingFinalResult bool
	seen                bool
}

type backgroundTask struct {
	// toolUseID is the tool call that spawned the task, so its state changes
	// can be attributed back to that call. It may only become known later:
	// a task launched in the background joins background_tasks_changed (which
	// carries no tool_use_id) before its task_started.
	toolUseID string
	// announced records that a started signal went out for toolUseID.
	announced bool
	// background marks tasks running in the background. Only those are listed
	// by background_tasks_changed, so only they may be finished by dropping
	// out of it; a foreground task ends with its own task_notification.
	background bool
	// detached marks a task that does not hold the turn open; see
	// BackgroundTracker.
	detached bool
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
	return &BackgroundTracker{tasks: map[string]*backgroundTask{}}
}

// TrackSystemMessage folds a system message into the tracked task set.
func (t *BackgroundTracker) TrackSystemMessage(payload map[string]any) TrackOutcome {
	var outcome TrackOutcome
	switch StringValue(payload, "subtype") {
	case "task_started":
		background, _ := payload["is_backgrounded"].(bool)
		if signal, ok := t.add(StringValue(payload, "task_id"), StringValue(payload, "tool_use_id"), StringValue(payload, "task_type"), background); ok {
			outcome.Started = append(outcome.Started, signal)
		}
	case "background_tasks_changed":
		// The message carries the full set of live background tasks, so
		// reconcile against it: unknown live tasks are picked up, and a tracked
		// background task that is listed as terminal — or no longer listed at
		// all — has finished, even if its task_notification was never seen.
		// Foreground tasks are never listed and are left alone.
		tasks, _ := payload["tasks"].([]any)
		live := map[string]struct{}{}
		for _, item := range tasks {
			task, _ := item.(map[string]any)
			taskID := StringValue(task, "task_id")
			if taskID == "" || IsTerminalTaskStatus(StringValue(task, "status")) {
				continue
			}
			live[taskID] = struct{}{}
			if signal, ok := t.add(taskID, StringValue(task, "tool_use_id"), StringValue(task, "task_type"), true); ok {
				outcome.Started = append(outcome.Started, signal)
			}
		}
		for taskID, task := range t.tasks {
			if _, stillLive := live[taskID]; stillLive || !task.background {
				continue
			}
			signal, drained := t.finish(taskID, "completed", "")
			outcome.Finished = append(outcome.Finished, signal)
			if drained {
				outcome.Drained = true
			}
		}
	case "task_notification":
		if status := StringValue(payload, "status"); IsTerminalTaskStatus(status) {
			taskID := StringValue(payload, "task_id")
			if _, tracked := t.tasks[taskID]; tracked {
				signal, drained := t.finish(taskID, status, StringValue(payload, "tool_use_id"))
				outcome.Finished = append(outcome.Finished, signal)
				outcome.Drained = drained
			}
		}
	case "task_updated":
		patch, _ := payload["patch"].(map[string]any)
		taskID := StringValue(payload, "task_id")
		if backgrounded, _ := patch["is_backgrounded"].(bool); backgrounded {
			if task, tracked := t.tasks[taskID]; tracked {
				task.background = true
			}
		}
		if status := StringValue(patch, "status"); IsTerminalTaskStatus(status) {
			if _, tracked := t.tasks[taskID]; tracked {
				signal, drained := t.finish(taskID, status, "")
				outcome.Finished = append(outcome.Finished, signal)
				outcome.Drained = drained
			}
		}
	}
	return outcome
}

// add tracks a task and reports a started signal the first time the task's
// spawning tool call is known — which may be on a later message than the one
// that first mentioned the task.
func (t *BackgroundTracker) add(taskID string, toolUseID string, taskType string, background bool) (TaskSignal, bool) {
	if taskID == "" {
		return TaskSignal{}, false
	}
	t.seen = true
	task, tracked := t.tasks[taskID]
	if !tracked {
		task = &backgroundTask{}
		t.tasks[taskID] = task
	}
	if taskType == "local_bash" {
		task.detached = true
	}
	if background {
		task.background = true
	}
	if task.toolUseID == "" {
		task.toolUseID = toolUseID
	}
	// A detached task's tool call is not a subagent: it returns at once, and
	// the task ends after the reply that made it is over.
	if task.toolUseID == "" || task.announced || task.detached {
		return TaskSignal{}, false
	}
	task.announced = true
	return TaskSignal{TaskID: taskID, ToolUseID: task.toolUseID}, true
}

// Seen reports whether this turn ever ran a task. Callers that see results only
// after the fact — the ephemeral CLI buffers every result until it exits —
// cannot use Waiting to decide when a turn is over, and instead defer to the
// process exiting once a task has appeared.
func (t *BackgroundTracker) Seen() bool {
	return t.seen
}

// finish closes a tracked task, reporting whether it was the last one holding
// the turn open. toolUseID fills in the spawning call when the task never
// reported it.
func (t *BackgroundTracker) finish(taskID string, status string, toolUseID string) (TaskSignal, bool) {
	if task := t.tasks[taskID]; task != nil && task.detached {
		toolUseID = ""
	} else if task != nil && task.toolUseID != "" {
		toolUseID = task.toolUseID
	}
	delete(t.tasks, taskID)
	signal := TaskSignal{TaskID: taskID, ToolUseID: toolUseID, Status: status}
	if t.holding() > 0 {
		return signal, false
	}
	t.awaitingFinalResult = true
	return signal, true
}

// holding counts the outstanding tasks that keep the turn open.
func (t *BackgroundTracker) holding() int {
	n := 0
	for _, task := range t.tasks {
		if !task.detached {
			n++
		}
	}
	return n
}

// Waiting reports whether the turn must stay open regardless of any result
// message already received.
func (t *BackgroundTracker) Waiting() bool {
	return t.holding() > 0 || t.awaitingFinalResult
}

// NoteResult records that a result arrived, satisfying the wait for the main
// agent's final word.
func (t *BackgroundTracker) NoteResult() {
	t.awaitingFinalResult = false
}

func (t *BackgroundTracker) PendingCount() int {
	return t.holding()
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

// IsTerminalTaskStatus reports whether a Claude Code task status means the
// task has ended.
func IsTerminalTaskStatus(status string) bool {
	switch status {
	case "completed", "failed", "stopped", "killed", "cancelled", "canceled", "error", "timeout":
		return true
	}
	return false
}
