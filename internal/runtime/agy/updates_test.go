package agy

import (
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestTurnStateCoalescesThoughtsAndPromotesPreToolText(t *testing.T) {
	st := newTurnState("check file")

	// Consecutive thought chunks should buffer without emitting per-token items.
	if evts := st.handleUpdate(record{
		"sessionUpdate": "agent_thought_chunk",
		"content":       record{"type": "text", "text": "First "},
	}); len(evts) != 0 {
		t.Fatalf("expected buffered thought chunk, got %+v", evts)
	}
	if evts := st.handleUpdate(record{
		"sessionUpdate": "agent_thought_chunk",
		"content":       record{"type": "text", "text": "second."},
	}); len(evts) != 0 {
		t.Fatalf("expected buffered thought chunk, got %+v", evts)
	}

	// Streaming assistant text flushes the buffered thought as one item, then streams text.
	evts := st.handleUpdate(record{
		"sessionUpdate": "agent_message_chunk",
		"content":       record{"type": "text", "text": "Let me read that file."},
	})
	if len(evts) != 2 {
		t.Fatalf("expected 2 events (flushed thought + text delta), got %d: %+v", len(evts), evts)
	}
	if evts[0].Thinking != "First second." || len(evts[0].Process) != 1 || evts[0].Process[0].Type != "thinking" {
		t.Fatalf("unexpected flushed thought event: %+v", evts[0])
	}
	if evts[1].Text != "Let me read that file." {
		t.Fatalf("unexpected text delta: %+v", evts[1])
	}

	// A tool call promotes the pre-tool text into thinking with ClearText=true.
	evts = st.handleUpdate(record{
		"sessionUpdate": "tool_call",
		"toolCallId":    "call-1",
		"name":          "read_file",
		"status":        "in_progress",
		"rawInput":      record{"Path": "/tmp/a.txt"},
	})
	if len(evts) != 2 {
		t.Fatalf("expected 2 events (promoted pre-tool text + tool_call), got %d: %+v", len(evts), evts)
	}
	if !evts[0].ClearText || evts[0].Process[0].Text != "Let me read that file." {
		t.Fatalf("expected ClearText promotion, got %+v", evts[0])
	}
	if len(evts[1].Process) != 1 || evts[1].Process[0].Type != "tool_call" || evts[1].Process[0].ToolName != "read_file" {
		t.Fatalf("unexpected tool_call event: %+v", evts[1])
	}

	// Terminal tool_call_update emits tool_result once.
	evts = st.handleUpdate(record{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    "call-1",
		"name":          "read_file",
		"status":        "completed",
		"content":       []any{record{"type": "text", "text": "hello"}},
	})
	if len(evts) != 1 || len(evts[0].Process) != 1 || evts[0].Process[0].Type != "tool_result" || evts[0].Process[0].Output != "hello" {
		t.Fatalf("unexpected tool_result event: %+v", evts)
	}

	// Final reply text after tool call stays in st.text.
	_ = st.handleUpdate(record{
		"sessionUpdate": "agent_message_chunk",
		"content":       record{"type": "text", "text": "Done!"},
	})
	if got := st.text.String(); got != "Done!" {
		t.Fatalf("final text = %q, want Done!", got)
	}
}

func TestTurnStateSubagentPlanGoalAndUsage(t *testing.T) {
	st := newTurnState("/goal ship the feature")
	initEvts := st.initialEvents()
	if len(initEvts) != 1 || initEvts[0].Goal == nil || initEvts[0].Goal.Objective != "ship the feature" || initEvts[0].Goal.Status != runtime.GoalStatusActive {
		t.Fatalf("initialEvents = %+v", initEvts)
	}

	// Subagent start + completion signals.
	evts := st.handleUpdate(record{
		"sessionUpdate": "tool_call",
		"toolCallId":    "sub-1",
		"name":          "invoke_subagent",
		"status":        "in_progress",
		"args":          record{"TypeName": "research", "Role": "Code Researcher"},
	})
	if len(evts) != 1 || len(evts[0].Process) != 2 || evts[0].Process[1].Type != "subagent_started" {
		t.Fatalf("expected tool_call + subagent_started, got %+v", evts)
	}
	evts = st.handleUpdate(record{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    "sub-1",
		"name":          "invoke_subagent",
		"status":        "completed",
	})
	if len(evts) != 1 || len(evts[0].Process) != 2 || evts[0].Process[1].Type != "subagent_completed" {
		t.Fatalf("expected tool_result + subagent_completed, got %+v", evts)
	}

	// Plan update.
	evts = st.handleUpdate(record{
		"sessionUpdate": "plan",
		"entries": []any{
			record{"content": "Step A", "status": "completed"},
			record{"content": "Step B", "status": "in_progress"},
		},
	})
	if len(evts) != 1 || evts[0].Plan == nil || len(evts[0].Plan.Steps) != 2 || evts[0].Plan.Steps[0].Status != runtime.PlanStepCompleted || evts[0].Plan.Steps[1].Status != runtime.PlanStepInProgress {
		t.Fatalf("unexpected plan event: %+v", evts)
	}

	// Context usage update + turn usage mapping.
	_ = st.handleUpdate(record{
		"sessionUpdate": "usage_update",
		"used":          float64(262144),
		"size":          float64(1048576),
	})
	if st.contextUsage == nil || *st.contextUsage.TotalTokens != 262144 || *st.contextUsage.ContextWindowTokens != 1048576 || *st.contextUsage.UsedPercent != 25 {
		t.Fatalf("unexpected contextUsage: %+v", st.contextUsage)
	}

	usage := buildUsage(record{
		"stopReason": "end_turn",
		"usage": record{
			"inputTokens":       float64(2000),
			"outputTokens":      float64(300),
			"cachedReadTokens":  float64(1000),
			"cachedWriteTokens": float64(100),
			"thoughtTokens":     float64(50),
			"totalTokens":       float64(3400),
		},
	}, "gemini-4-carbon", 1200, st.contextUsage)

	if usage == nil || *usage.InputTokens != 2000 || *usage.OutputTokens != 300 || *usage.CachedInputTokens != 1000 || *usage.CacheReadInputTokens != 1000 || *usage.CacheCreationInputTokens != 100 || *usage.ReasoningOutputTokens != 50 || *usage.TotalTokens != 3400 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	if usage.Context == nil || usage.Context.Model != "gemini-4-carbon" {
		t.Fatalf("unexpected usage.Context: %+v", usage.Context)
	}

	goal := st.finishGoal("end_turn", usage.TotalTokens)
	if goal == nil || goal.Status != runtime.GoalStatusComplete || *goal.TokensUsed != 3400 {
		t.Fatalf("unexpected finished goal: %+v", goal)
	}
}

func TestPermissionOptionSelection(t *testing.T) {
	q, callID, choices := permissionQuestionFromParams(record{
		"toolCall": record{
			"toolCallId": "tc-9",
			"name":       "run_command",
			"title":      "Terminal",
			"args":       record{"CommandLine": "rm -f temp.txt"},
		},
		"options": []any{
			record{"optionId": "allow_once", "name": "Allow", "kind": "allow_once"},
			record{"optionId": "deny_once", "name": "Deny", "kind": "reject_once"},
		},
	})
	if callID != "tc-9" || q == "" || len(choices) != 2 {
		t.Fatalf("unexpected permission parse: q=%q callID=%q choices=%+v", q, callID, choices)
	}
	if got := selectPermissionOption(choices, "Allow"); got != "allow_once" {
		t.Fatalf("selectPermissionOption(Allow) = %q, want allow_once", got)
	}
	if got := selectPermissionOption(choices, "Deny"); got != "deny_once" {
		t.Fatalf("selectPermissionOption(Deny) = %q, want deny_once", got)
	}
	if got := selectPermissionOption(choices, "something unknown"); got != "deny_once" {
		t.Fatalf("selectPermissionOption(unknown) = %q, want conservative deny_once", got)
	}
}
