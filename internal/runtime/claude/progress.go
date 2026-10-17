package claude

import (
	"regexp"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

const (
	goalSetPrefix       = "Goal set:"
	goalClearedPrefix   = "Goal cleared:"
	stopHookFeedbackTag = "Stop hook feedback:\n"
)

var taskCreatedPattern = regexp.MustCompile(`Task #(\S+) created successfully`)

// ProgressTracker derives goal and step-list snapshots from Claude Code's
// stream-json output.
//
// Claude Code's /goal is a session Stop hook: after each model turn a small
// evaluator judges the condition, and while it is unmet the CLI feeds the
// reason back as a synthetic "Stop hook feedback" user message and keeps
// going. stream-json carries no dedicated goal event, so the goal is inferred
// from the synthetic "Goal set:" reply of the local /goal command, the
// feedback messages (one per unmet evaluation) and the final result.
//
// Steps come from the TodoWrite tool (a full list per call) or the
// TaskCreate / TaskUpdate tools (one task per call).
type ProgressTracker struct {
	goal          *runtime.GoalState
	goalStartedAt time.Time

	todos        *runtime.PlanState
	tasks        []trackedTask
	pendingTasks map[string]trackedTask // TaskCreate tool_use id -> task awaiting its id
	usesTasks    bool

	now func() time.Time
}

type trackedTask struct {
	id      string
	subject string
	status  string
}

func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{pendingTasks: map[string]trackedTask{}, now: time.Now}
}

// Track inspects one stream-json payload and returns the progress events it
// produces. Subagent output is ignored: its steps belong to the subagent.
func (t *ProgressTracker) Track(payload map[string]any) []runtime.Event {
	if stringValue(payload, "parent_tool_use_id") != "" {
		return nil
	}
	var goalChanged, planChanged bool
	switch stringValue(payload, "type") {
	case "assistant":
		goalChanged = t.trackGoalCommand(payload)
		planChanged = t.trackToolUses(payload)
	case "user":
		goalChanged = t.trackGoalFeedback(payload)
		planChanged = t.trackToolResults(payload)
	}
	var events []runtime.Event
	if goalChanged || planChanged {
		evt := runtime.Event{Type: runtime.EventDelta}
		if goalChanged {
			evt.Goal = t.goalSnapshot()
		}
		if planChanged {
			evt.Plan = t.plan()
		}
		events = append(events, evt)
	}
	return events
}

// Finish concludes a goal that was running when the turn ended successfully:
// the Stop hook only lets the turn end once the evaluator judged the
// condition met. It returns nil when no goal was running.
func (t *ProgressTracker) Finish() *runtime.GoalState {
	if t.goal == nil || t.goal.Status != runtime.GoalStatusActive {
		return nil
	}
	t.goal.Status = runtime.GoalStatusComplete
	// The last recorded reason explained why the goal was not yet met; the
	// final, satisfied verdict is not on stdout.
	t.goal.LastReason = ""
	return t.goalSnapshot()
}

func (t *ProgressTracker) trackGoalCommand(payload map[string]any) bool {
	run, _ := payload["local_command_run"].(map[string]any)
	if stringValue(run, "command") != "goal" {
		return false
	}
	text := strings.TrimSpace(assistantText(payload))
	switch {
	case strings.HasPrefix(text, goalSetPrefix):
		objective := strings.TrimSpace(stringValue(run, "args"))
		if objective == "" {
			objective = strings.TrimSpace(strings.TrimPrefix(text, goalSetPrefix))
		}
		t.goal = &runtime.GoalState{Objective: objective, Status: runtime.GoalStatusActive}
		t.goalStartedAt = t.now()
		return true
	case strings.HasPrefix(text, goalClearedPrefix):
		if t.goal == nil {
			t.goal = &runtime.GoalState{Objective: strings.TrimSpace(strings.TrimPrefix(text, goalClearedPrefix))}
		}
		t.goal.Status = runtime.GoalStatusCleared
		return true
	}
	return false
}

func (t *ProgressTracker) trackGoalFeedback(payload map[string]any) bool {
	if t.goal == nil || t.goal.Status != runtime.GoalStatusActive {
		return false
	}
	if synthetic, _ := payload["isSynthetic"].(bool); !synthetic {
		return false
	}
	text := messageText(payload)
	if !strings.HasPrefix(text, stopHookFeedbackTag) {
		return false
	}
	// Format: "Stop hook feedback:\n[<condition>]: <reason>".
	body := strings.TrimPrefix(text, stopHookFeedbackTag)
	reason := body
	if prefix := "[" + t.goal.Objective + "]: "; strings.HasPrefix(body, prefix) {
		reason = strings.TrimPrefix(body, prefix)
	} else if _, after, ok := strings.Cut(body, "]: "); ok && strings.HasPrefix(body, "[") {
		reason = after
	}
	t.goal.Iterations++
	t.goal.LastReason = strings.TrimSpace(reason)
	return true
}

func (t *ProgressTracker) trackToolUses(payload map[string]any) bool {
	changed := false
	for _, part := range messageParts(payload) {
		if stringValue(part, "type") != "tool_use" {
			continue
		}
		input, _ := part["input"].(map[string]any)
		switch stringValue(part, "name") {
		case "TodoWrite":
			t.todos = todoWritePlan(input)
			changed = true
		case "TaskCreate":
			if id := stringValue(part, "id"); id != "" {
				t.pendingTasks[id] = trackedTask{subject: strings.TrimSpace(stringValue(input, "subject")), status: runtime.PlanStepPending}
			}
		case "TaskUpdate":
			if t.updateTask(input) {
				changed = true
			}
		}
	}
	return changed
}

func (t *ProgressTracker) trackToolResults(payload map[string]any) bool {
	changed := false
	for _, part := range messageParts(payload) {
		if stringValue(part, "type") != "tool_result" {
			continue
		}
		callID := claudeToolResultCallID(part)
		task, ok := t.pendingTasks[callID]
		if !ok {
			continue
		}
		delete(t.pendingTasks, callID)
		if claudeToolResultStatus(part) == "error" || task.subject == "" {
			continue
		}
		if match := taskCreatedPattern.FindStringSubmatch(toolResultText(part)); len(match) == 2 {
			task.id = match[1]
		}
		t.tasks = append(t.tasks, task)
		t.usesTasks = true
		changed = true
	}
	return changed
}

func (t *ProgressTracker) updateTask(input map[string]any) bool {
	id := strings.TrimPrefix(strings.TrimSpace(stringValue(input, "taskId")), "#")
	if id == "" {
		return false
	}
	for i := range t.tasks {
		if t.tasks[i].id != id {
			continue
		}
		status := stringValue(input, "status")
		if status == "deleted" {
			t.tasks = append(t.tasks[:i], t.tasks[i+1:]...)
			return true
		}
		if status != "" {
			t.tasks[i].status = runtime.NormalizePlanStepStatus(status)
		}
		if subject := strings.TrimSpace(stringValue(input, "subject")); subject != "" {
			t.tasks[i].subject = subject
		}
		return true
	}
	return false
}

func (t *ProgressTracker) goalSnapshot() *runtime.GoalState {
	if t.goal == nil {
		return nil
	}
	snapshot := *t.goal
	if !t.goalStartedAt.IsZero() {
		seconds := int64(t.now().Sub(t.goalStartedAt) / time.Second)
		snapshot.TimeUsedSeconds = &seconds
	}
	return &snapshot
}

// plan returns the current step list: the task list once the agent uses the
// task tools, otherwise the last TodoWrite list.
func (t *ProgressTracker) plan() *runtime.PlanState {
	if t.usesTasks {
		plan := &runtime.PlanState{Steps: make([]runtime.PlanStep, 0, len(t.tasks))}
		for _, task := range t.tasks {
			plan.Steps = append(plan.Steps, runtime.PlanStep{Step: task.subject, Status: task.status})
		}
		return plan
	}
	if t.todos == nil {
		return nil
	}
	plan := *t.todos
	plan.Steps = append([]runtime.PlanStep(nil), t.todos.Steps...)
	return &plan
}

// todoWritePlan parses TodoWrite input: {todos: [{content, status, activeForm}]}.
func todoWritePlan(input map[string]any) *runtime.PlanState {
	rawTodos, _ := input["todos"].([]any)
	plan := &runtime.PlanState{Steps: make([]runtime.PlanStep, 0, len(rawTodos))}
	for _, raw := range rawTodos {
		todo, _ := raw.(map[string]any)
		text := strings.TrimSpace(stringValue(todo, "content"))
		if text == "" {
			continue
		}
		plan.Steps = append(plan.Steps, runtime.PlanStep{Step: text, Status: runtime.NormalizePlanStepStatus(stringValue(todo, "status"))})
	}
	return plan
}

func messageParts(payload map[string]any) []map[string]any {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	parts := make([]map[string]any, 0, len(content))
	for _, item := range content {
		if part, ok := item.(map[string]any); ok {
			parts = append(parts, part)
		}
	}
	return parts
}

// messageText returns a message's text, whether its content is a plain string
// or a list of text parts.
func messageText(payload map[string]any) string {
	message, _ := payload["message"].(map[string]any)
	if text, ok := message["content"].(string); ok {
		return text
	}
	return assistantText(payload)
}

func toolResultText(part map[string]any) string {
	switch content := part["content"].(type) {
	case string:
		return content
	case []any:
		var out strings.Builder
		for _, item := range content {
			inner, _ := item.(map[string]any)
			out.WriteString(stringValue(inner, "text"))
		}
		return out.String()
	}
	return ""
}
