package pi

import "testing"

func TestSubagentTrackerLifecycle(t *testing.T) {
	tr := newSubagentTracker()

	start := record{"type": "tool_execution_start", "toolCallId": "call_sub", "toolName": "subagent", "args": record{"agent": "scout", "task": "map the repo"}}
	items := tr.started(start)
	if len(items) != 1 || items[0].Type != "subagent_started" || items[0].ToolCallID != "call_sub" {
		t.Fatalf("started = %+v", items)
	}
	if again := tr.started(start); len(again) != 0 {
		t.Fatalf("duplicate start produced %+v", again)
	}
	if other := tr.started(record{"toolCallId": "call_bash", "toolName": "bash"}); len(other) != 0 {
		t.Fatalf("non-subagent start produced %+v", other)
	}
	if nested := tr.started(record{"toolCallId": "call_nested", "toolName": "subagent", "parentToolCallId": "call_sub"}); len(nested) != 0 {
		t.Fatalf("nested subagent start produced %+v", nested)
	}

	update := record{"type": "tool_execution_update", "toolCallId": "call_sub", "partialResult": record{"details": record{"progress": []any{
		record{"index": 0.0, "agent": "scout", "recentTools": []any{
			record{"tool": "read", "args": "README.md", "endMs": 200.0},
			record{"tool": "grep", "args": "TODO", "endMs": 100.0},
		}},
	}}}}
	items = tr.updated(update)
	if len(items) != 2 || items[0].ToolName != "grep" || items[1].ToolName != "read" {
		t.Fatalf("updated = %+v, want grep then read", items)
	}
	for _, item := range items {
		if item.Type != "tool_call" || item.ParentToolCallID != "call_sub" || item.Status != "completed" {
			t.Fatalf("child item = %+v", item)
		}
	}
	if repeat := tr.updated(update); len(repeat) != 0 {
		t.Fatalf("repeated update produced %+v", repeat)
	}
	later := record{"toolCallId": "call_sub", "partialResult": record{"details": record{"progress": []any{
		record{"index": 0.0, "recentTools": []any{
			record{"tool": "read", "args": "README.md", "endMs": 200.0},
			record{"tool": "bash", "args": "go test", "endMs": 300.0},
		}},
	}}}}
	if items = tr.updated(later); len(items) != 1 || items[0].ToolName != "bash" {
		t.Fatalf("later update = %+v, want only bash", items)
	}

	if unknown := tr.updated(record{"toolCallId": "call_bash"}); len(unknown) != 0 {
		t.Fatalf("unknown call update produced %+v", unknown)
	}

	end := record{"type": "tool_execution_end", "toolCallId": "call_sub", "isError": false, "result": record{"details": record{"results": []any{
		record{"agent": "scout", "exitCode": 0.0},
	}}}}
	items = tr.ended(end)
	if len(items) != 1 || items[0].Type != "subagent_completed" || items[0].Status != "completed" {
		t.Fatalf("ended = %+v", items)
	}
	if again := tr.ended(end); len(again) != 0 {
		t.Fatalf("second end produced %+v", again)
	}
}

func TestSubagentTrackerFailureStatuses(t *testing.T) {
	tr := newSubagentTracker()
	tr.started(record{"toolCallId": "a", "toolName": "subagent"})
	tr.started(record{"toolCallId": "b", "toolName": "subagent"})
	tr.started(record{"toolCallId": "c", "toolName": "subagent"})

	if items := tr.ended(record{"toolCallId": "a", "isError": true}); items[0].Status != "failed" {
		t.Fatalf("isError end = %+v", items)
	}
	if items := tr.ended(record{"toolCallId": "b", "result": record{"details": record{"results": []any{record{"exitCode": 1.0}}}}}); items[0].Status != "failed" {
		t.Fatalf("exitCode end = %+v", items)
	}
	if items := tr.ended(record{"toolCallId": "c", "result": record{"details": record{"results": []any{record{"exitCode": 0.0, "stopped": true}}}}}); items[0].Status != "stopped" {
		t.Fatalf("stopped end = %+v", items)
	}
}
