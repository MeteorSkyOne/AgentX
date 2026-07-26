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
