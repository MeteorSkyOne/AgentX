package claude

import (
	"strings"
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func handleLines(t *testing.T, handler *lineHandler, lines ...string) []runtime.Event {
	t.Helper()
	var all []runtime.Event
	for _, line := range lines {
		events, err := handler.HandleLine([]byte(line))
		if err != nil {
			t.Fatalf("HandleLine(%s): %v", line, err)
		}
		all = append(all, events...)
	}
	return all
}

func countCompleted(events []runtime.Event) int {
	var n int
	for _, evt := range events {
		if evt.Type == runtime.EventCompleted {
			n++
		}
	}
	return n
}

// A background subagent produces two results — one when it is launched, one
// when the agent reports back — and the ephemeral CLI flushes both as it exits.
// The run must complete exactly once, carrying both replies.
func TestLineHandlerCompletesOnceAcrossBackgroundSubagentResults(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_task1","name":"Agent","input":{"run_in_background":true}}]}}`,
		`{"type":"system","subtype":"task_started","task_id":"task-1","tool_use_id":"toolu_task1"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"launched"}]}}`,
		`{"type":"assistant","parent_tool_use_id":"toolu_task1","message":{"content":[{"type":"text","text":"SUBAGENT CHATTER"}]}}`,
		`{"type":"system","subtype":"task_notification","task_id":"task-1","status":"completed"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"the subagent reported back"}]}}`,
		`{"type":"result","subtype":"success","result":"launched","usage":{"input_tokens":10,"output_tokens":5}}`,
		`{"type":"result","subtype":"success","result":"the subagent reported back","usage":{"input_tokens":3,"output_tokens":7}}`,
	)
	if n := countCompleted(events); n != 0 {
		t.Fatalf("expected results to be held until exit, got %d completed events", n)
	}

	completed, ok := handler.Finish("", nil)
	if !ok || completed.Type != runtime.EventCompleted {
		t.Fatalf("Finish returned %#v (ok=%v)", completed, ok)
	}
	if !strings.Contains(completed.Text, "launched") {
		t.Errorf("completed text lost the pre-subagent reply: %q", completed.Text)
	}
	if !strings.Contains(completed.Text, "the subagent reported back") {
		t.Errorf("completed text lost the post-subagent reply: %q", completed.Text)
	}
	if strings.Contains(completed.Text, "SUBAGENT CHATTER") {
		t.Errorf("subagent chatter leaked into the main reply: %q", completed.Text)
	}
	if completed.Usage == nil || completed.Usage.InputTokens == nil || *completed.Usage.InputTokens != 13 {
		t.Errorf("expected usage summed across both results, got %#v", completed.Usage)
	}
	if completed.Usage.OutputTokens == nil || *completed.Usage.OutputTokens != 12 {
		t.Errorf("expected output tokens summed across both results, got %#v", completed.Usage)
	}
}

// The persistent CLI emits results live, so the first result can arrive before
// the task's terminal notification. Its usage must still survive into the
// merged completion instead of being discarded with the stale result.
func TestLineHandlerSumsUsageWhenResultPrecedesTaskDrain(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"system","subtype":"task_started","task_id":"task-1","tool_use_id":"toolu_task1"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"launched"}]}}`,
		`{"type":"result","subtype":"success","result":"launched","usage":{"input_tokens":10,"output_tokens":5}}`,
		`{"type":"system","subtype":"task_notification","task_id":"task-1","status":"completed"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}`,
		`{"type":"result","subtype":"success","result":"done","usage":{"input_tokens":3,"output_tokens":7}}`,
	)
	if n := countCompleted(events); n != 0 {
		t.Fatalf("expected results to be held until exit, got %d completed events", n)
	}

	completed, ok := handler.Finish("", nil)
	if !ok || completed.Type != runtime.EventCompleted {
		t.Fatalf("Finish returned %#v (ok=%v)", completed, ok)
	}
	if completed.Usage == nil || completed.Usage.InputTokens == nil || *completed.Usage.InputTokens != 13 {
		t.Errorf("expected usage summed across both results, got %#v", completed.Usage)
	}
	if completed.Usage.OutputTokens == nil || *completed.Usage.OutputTokens != 12 {
		t.Errorf("expected output tokens summed across both results, got %#v", completed.Usage)
	}
}

// background_tasks_changed carries the full current task set, so a tracked task
// missing from the list — or listed as terminal — has finished even if its
// task_notification never arrived.
func TestBackgroundTasksChangedReconcilesFullSet(t *testing.T) {
	tracker := NewBackgroundTracker()
	started := tracker.TrackSystemMessage(map[string]any{
		"subtype": "task_started", "task_id": "task-1", "tool_use_id": "toolu_1",
	})
	if len(started.Started) != 1 || started.Started[0].ToolUseID != "toolu_1" {
		t.Fatalf("expected a started signal attributed to toolu_1, got %#v", started)
	}
	tracker.TrackSystemMessage(map[string]any{
		"subtype": "background_tasks_changed",
		"tasks": []any{
			map[string]any{"task_id": "task-1", "status": "running"},
			map[string]any{"task_id": "task-2", "status": "running", "tool_use_id": "toolu_2"},
		},
	})
	if n := tracker.PendingCount(); n != 2 {
		t.Fatalf("expected 2 tracked tasks after list update, got %d", n)
	}

	outcome := tracker.TrackSystemMessage(map[string]any{
		"subtype": "background_tasks_changed",
		"tasks": []any{
			map[string]any{"task_id": "task-2", "status": "completed", "tool_use_id": "toolu_2"},
		},
	})
	if !outcome.Drained {
		t.Error("expected reconciling an empty live set to report the last task drained")
	}
	if len(outcome.Finished) != 2 {
		t.Errorf("expected both tasks reported finished, got %#v", outcome.Finished)
	}
	if n := tracker.PendingCount(); n != 0 {
		t.Errorf("expected no tracked tasks after reconcile, got %d", n)
	}
	if !tracker.Waiting() {
		t.Error("expected tracker to wait for the agent's final result after draining")
	}
}

// A task's terminal notification is attributed back to the tool call recorded
// when the task started, so the UI can flip that subagent to done.
func TestTaskNotificationCarriesSpawningToolCall(t *testing.T) {
	tracker := NewBackgroundTracker()
	tracker.TrackSystemMessage(map[string]any{
		"subtype": "task_started", "task_id": "task-1", "tool_use_id": "toolu_task1",
	})

	outcome := tracker.TrackSystemMessage(map[string]any{
		"subtype": "task_notification", "task_id": "task-1", "status": "completed",
	})
	if len(outcome.Finished) != 1 {
		t.Fatalf("expected 1 finished signal, got %#v", outcome.Finished)
	}
	if got := outcome.Finished[0]; got.ToolUseID != "toolu_task1" || got.Status != "completed" {
		t.Errorf("unexpected finished signal: %#v", got)
	}

	items := SubagentSignalItems(outcome)
	if len(items) != 1 || items[0].Type != "subagent_completed" || items[0].ToolCallID != "toolu_task1" {
		t.Errorf("unexpected signal items: %#v", items)
	}
}

// A turn with no subagent at all keeps completing on the result itself.
func TestLineHandlerCompletesOnResultWithoutSubagents(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"there are 40 files"}]}}`,
		`{"type":"result","subtype":"success","result":"there are 40 files"}`,
	)
	if n := countCompleted(events); n != 1 {
		t.Fatalf("expected the result to complete the run directly, got %d completed events", n)
	}
}

// If the CLI exits without the follow-up result, the held result must still be
// delivered rather than lost.
func TestLineHandlerFinishEmitsHeldResult(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"system","subtype":"task_started","task_id":"task-stuck","tool_use_id":"toolu_stuck"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"launched"}]}}`,
		`{"type":"result","subtype":"success","result":"launched","usage":{"input_tokens":10,"output_tokens":5}}`,
	)
	if n := countCompleted(events); n != 0 {
		t.Fatalf("expected the result to be held back, got %d completed events", n)
	}

	evt, ok := handler.Finish("", nil)
	if !ok || evt.Type != runtime.EventCompleted {
		t.Fatalf("Finish returned %#v (ok=%v)", evt, ok)
	}
	if !strings.Contains(evt.Text, "launched") {
		t.Errorf("held result lost its text: %q", evt.Text)
	}
	if evt.Usage == nil {
		t.Error("held result lost its usage")
	}
}

// Subagent process items carry their parent tool call so the UI can group them.
func TestLineHandlerTagsSubagentProcessItems(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"assistant","parent_tool_use_id":"toolu_parent","message":{"content":[{"type":"tool_use","id":"toolu_child","name":"Bash","input":{}}]}}`,
	)
	var tagged int
	for _, evt := range events {
		for _, item := range evt.Process {
			if item.ParentToolCallID == "toolu_parent" {
				tagged++
			}
		}
	}
	if tagged != 1 {
		t.Fatalf("expected 1 tagged subagent process item, got %d (events=%#v)", tagged, events)
	}
}

func subagentSignals(events []runtime.Event) []runtime.ProcessItem {
	var items []runtime.ProcessItem
	for _, evt := range events {
		for _, item := range evt.Process {
			if item.Type == "subagent_started" || item.Type == "subagent_completed" {
				items = append(items, item)
			}
		}
	}
	return items
}

// Recorded from Claude Code 2.1.302: a background agent joins
// background_tasks_changed (no tool_use_id) before its task_started. The
// started signal must still go out once task_started names the tool call —
// otherwise the UI only sees the spawning call's "launched" result and shows
// the subagent as done while it runs.
func TestBackgroundAgentListedBeforeTaskStartedIsAnnounced(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"ad5a","task_type":"local_agent","subagent_type":"general-purpose","description":"Sleep then reply BETA"}]}`,
		`{"type":"system","subtype":"task_started","task_id":"ad5a","tool_use_id":"toolu_bg","description":"Sleep then reply BETA","is_backgrounded":true,"task_type":"local_agent"}`,
		`{"type":"system","subtype":"task_started","task_id":"ad5a","tool_use_id":"toolu_bg","is_backgrounded":true,"task_type":"local_agent"}`,
	)
	signals := subagentSignals(events)
	if len(signals) != 1 || signals[0].Type != "subagent_started" || signals[0].ToolCallID != "toolu_bg" {
		t.Fatalf("signals = %#v, want one subagent_started for toolu_bg", signals)
	}

	events = handleLines(t, handler,
		`{"type":"system","subtype":"task_updated","task_id":"ad5a","patch":{"status":"completed"}}`,
		`{"type":"system","subtype":"task_notification","task_id":"ad5a","tool_use_id":"toolu_bg","status":"completed"}`,
		`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`,
	)
	signals = subagentSignals(events)
	if len(signals) != 1 || signals[0].Type != "subagent_completed" || signals[0].ToolCallID != "toolu_bg" {
		t.Fatalf("signals = %#v, want one subagent_completed for toolu_bg", signals)
	}
}

// background_tasks_changed lists background tasks only, so a running
// foreground subagent missing from it has not finished.
func TestForegroundAgentSurvivesBackgroundTasksChanged(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"system","subtype":"task_started","task_id":"fg","tool_use_id":"toolu_fg","is_backgrounded":false,"task_type":"local_agent"}`,
		`{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"sh","task_type":"local_bash","description":"sleep"}]}`,
		`{"type":"system","subtype":"task_started","task_id":"sh","tool_use_id":"toolu_sh","is_backgrounded":true,"task_type":"local_bash"}`,
		`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`,
	)
	for _, signal := range subagentSignals(events) {
		if signal.Type == "subagent_completed" && signal.ToolCallID == "toolu_fg" {
			t.Fatalf("foreground subagent finished by background_tasks_changed: %#v", subagentSignals(events))
		}
	}

	events = handleLines(t, handler,
		`{"type":"system","subtype":"task_notification","task_id":"fg","tool_use_id":"toolu_fg","status":"completed"}`,
	)
	signals := subagentSignals(events)
	if len(signals) != 1 || signals[0].Type != "subagent_completed" || signals[0].ToolCallID != "toolu_fg" {
		t.Fatalf("signals = %#v, want subagent_completed for toolu_fg", signals)
	}
}

// A foreground agent moved to the background is then subject to the
// background level signal.
func TestBackgroundedForegroundAgentFinishesWhenDroppedFromLevel(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"system","subtype":"task_started","task_id":"fg","tool_use_id":"toolu_fg","is_backgrounded":false,"task_type":"local_agent"}`,
		`{"type":"system","subtype":"task_updated","task_id":"fg","patch":{"is_backgrounded":true}}`,
		`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`,
	)
	signals := subagentSignals(events)
	if len(signals) != 2 || signals[1].Type != "subagent_completed" || signals[1].ToolCallID != "toolu_fg" {
		t.Fatalf("signals = %#v, want started then completed for toolu_fg", signals)
	}
}

func TestStoppedAndKilledTasksFinish(t *testing.T) {
	handler := newLineHandler("fallback")

	events := handleLines(t, handler,
		`{"type":"system","subtype":"task_started","task_id":"a","tool_use_id":"toolu_a","is_backgrounded":true}`,
		`{"type":"system","subtype":"task_started","task_id":"b","tool_use_id":"toolu_b","is_backgrounded":true}`,
		`{"type":"system","subtype":"task_notification","task_id":"a","tool_use_id":"toolu_a","status":"stopped"}`,
		`{"type":"system","subtype":"task_updated","task_id":"b","patch":{"status":"killed"}}`,
	)
	var finished []string
	for _, signal := range subagentSignals(events) {
		if signal.Type == "subagent_completed" {
			finished = append(finished, signal.ToolCallID+":"+signal.Status)
		}
	}
	if strings.Join(finished, ",") != "toolu_a:stopped,toolu_b:killed" {
		t.Fatalf("finished = %v", finished)
	}
}

// A background shell command is silent while it runs, so it does not hold the
// turn open and its tool call is not rendered as a subagent.
func TestBackgroundShellDoesNotHoldTheTurn(t *testing.T) {
	tracker := NewBackgroundTracker()
	started := tracker.TrackSystemMessage(map[string]any{
		"subtype": "background_tasks_changed",
		"tasks":   []any{map[string]any{"task_id": "b1", "task_type": "local_bash"}},
	})
	started.Started = append(started.Started, tracker.TrackSystemMessage(map[string]any{
		"subtype": "task_started", "task_id": "b1", "task_type": "local_bash",
		"tool_use_id": "toolu_bash", "is_backgrounded": true,
	}).Started...)
	if len(started.Started) != 0 {
		t.Fatalf("background shell must not announce a subagent, got %#v", started.Started)
	}
	if tracker.Waiting() {
		t.Fatal("background shell must not hold the turn open")
	}
	if !tracker.Seen() {
		t.Fatal("background shell still counts as a task for the ephemeral runtime")
	}
	finished := tracker.TrackSystemMessage(map[string]any{
		"subtype": "task_notification", "task_id": "b1", "status": "completed", "tool_use_id": "toolu_bash",
	})
	if items := SubagentSignalItems(finished); len(items) != 0 {
		t.Fatalf("background shell must not emit subagent items, got %#v", items)
	}
}

// A subagent finishing while a background shell is still running is the last
// task holding the turn, so the agent's report on it is still awaited.
func TestSubagentDrainsWhileBackgroundShellRuns(t *testing.T) {
	tracker := NewBackgroundTracker()
	tracker.TrackSystemMessage(map[string]any{
		"subtype": "task_started", "task_id": "a1", "task_type": "local_agent", "tool_use_id": "toolu_agent", "is_backgrounded": true,
	})
	tracker.TrackSystemMessage(map[string]any{
		"subtype": "task_started", "task_id": "b1", "task_type": "local_bash", "tool_use_id": "toolu_bash", "is_backgrounded": true,
	})
	tracker.NoteResult()
	if !tracker.Waiting() || tracker.PendingCount() != 1 {
		t.Fatalf("subagent must hold the turn: waiting=%v pending=%d", tracker.Waiting(), tracker.PendingCount())
	}
	outcome := tracker.TrackSystemMessage(map[string]any{
		"subtype": "task_notification", "task_id": "a1", "status": "completed",
	})
	if !outcome.Drained {
		t.Fatal("finishing the subagent must drain the turn despite the running shell")
	}
	tracker.NoteResult()
	if tracker.Waiting() {
		t.Fatal("turn must be over once the agent reported on the subagent")
	}
}
