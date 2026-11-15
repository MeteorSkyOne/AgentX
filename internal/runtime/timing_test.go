package runtime

import (
	"testing"
	"time"
)

func TestTurnTimerStreamingSingleStep(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	timer := NewTurnTimer(t0)

	timer.ObserveStreamAt(t0.Add(1200 * time.Millisecond))
	timer.ObserveStreamAt(t0.Add(2200 * time.Millisecond))
	got := timer.FinishAt(t0.Add(3200 * time.Millisecond))

	first := timer.FirstTokenAt()
	if first == nil || !first.Equal(t0.Add(1200*time.Millisecond)) {
		t.Fatalf("FirstTokenAt = %v, want %v", first, t0.Add(1200*time.Millisecond))
	}
	if got != 2*time.Second {
		t.Fatalf("GenerationDuration = %v, want 2s", got)
	}
}

func TestTurnTimerBatchedSingleStepUsesFullStepDuration(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	timer := NewTurnTimer(t0)

	timer.ObserveStreamAt(t0.Add(1900 * time.Millisecond))
	got := timer.FinishAt(t0.Add(2000 * time.Millisecond))

	if got != 2*time.Second {
		t.Fatalf("GenerationDuration = %v, want 2s fallback for <250ms stream window", got)
	}
}

func TestTurnTimerMultiStepExcludesToolWaitAndIncludesThinking(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	timer := NewTurnTimer(t0)

	// Step 1: 3s of streamed thinking, then a 240s shell command (e.g. gh run watch).
	timer.ObserveStreamAt(t0.Add(1400 * time.Millisecond))
	timer.ObserveStreamAt(t0.Add(3000 * time.Millisecond))
	timer.ToolStartedAt("call_watch", t0.Add(4400*time.Millisecond))
	timer.ToolCompletedAt("call_watch", t0.Add(244400*time.Millisecond))

	// Step 2: 17s of streamed thinking + 300ms of final reply text.
	timer.ObserveStreamAt(t0.Add(245400 * time.Millisecond))
	timer.ObserveStreamAt(t0.Add(262400 * time.Millisecond))
	timer.ObserveStreamAt(t0.Add(262700 * time.Millisecond))
	got := timer.FinishAt(t0.Add(262700 * time.Millisecond))

	first := timer.FirstTokenAt()
	if first == nil || !first.Equal(t0.Add(1400*time.Millisecond)) {
		t.Fatalf("FirstTokenAt = %v, want %v (first thinking token, not final reply)", first, t0.Add(1400*time.Millisecond))
	}
	want := 3*time.Second + 17300*time.Millisecond
	if got != want {
		t.Fatalf("GenerationDuration = %v, want %v", got, want)
	}
}

func TestTurnTimerParallelToolsAndSubagentChildItems(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	timer := NewTurnTimer(t0)

	// Step 1: non-streamed assistant block emits two parallel tool calls at 2s.
	timer.ToolStartedAt("tc_a", t0.Add(2*time.Second))
	timer.ToolStartedAt("tc_b", t0.Add(2010*time.Millisecond))

	// Subagent child events while tc_a is running must not resume main-agent timing.
	timer.ObserveEventAt(Event{
		Type:     EventDelta,
		Thinking: "subagent thought",
		Process: []ProcessItem{{
			Type:             "tool_call",
			ToolCallID:       "sub_child_1",
			ParentToolCallID: "tc_a",
		}},
	}, t0.Add(5*time.Second))

	// tc_a finishes at 6s, but tc_b is still running until 12s.
	timer.ToolCompletedAt("tc_a", t0.Add(6*time.Second))
	timer.ToolCompletedAt("tc_b", t0.Add(12*time.Second))

	// Step 2 streams from 12.5s to 14.5s (2s).
	timer.ObserveStreamAt(t0.Add(12500 * time.Millisecond))
	got := timer.FinishAt(t0.Add(14500 * time.Millisecond))

	want := 4 * time.Second
	if got != want {
		t.Fatalf("GenerationDuration = %v, want %v", got, want)
	}
}

func TestTurnTimerExcludesInputWaitAndSelfHealsUnclosedTool(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	timer := NewTurnTimer(t0)

	// Model emits an input request at 1.5s; user takes 60s to respond.
	timer.ObserveEventAt(Event{
		Type:         EventInputRequest,
		InputRequest: &InputRequest{QuestionID: "q1", Question: "Proceed?"},
	}, t0.Add(1500*time.Millisecond))
	timer.InputWaitCompletedAt(t0.Add(61500 * time.Millisecond))

	// Model starts a tool call at 62.5s (1s step) that never emits tool_result,
	// then streams final text from 70s to 72s (2s step).
	timer.ObserveEventAt(Event{
		Type:    EventDelta,
		Process: []ProcessItem{{Type: "tool_call", ToolCallID: "dangling"}},
	}, t0.Add(62500*time.Millisecond))
	timer.ObserveEventAt(Event{
		Type: EventDelta,
		Text: "Final ",
	}, t0.Add(70*time.Second))
	timer.ObserveEventAt(Event{
		Type: EventCompleted,
		Text: "Final answer",
	}, t0.Add(72*time.Second))

	first := timer.FirstTokenAt()
	if first == nil || !first.Equal(t0.Add(1500*time.Millisecond)) {
		t.Fatalf("FirstTokenAt = %v, want %v", first, t0.Add(1500*time.Millisecond))
	}
	want := 1500*time.Millisecond + 1*time.Second + 2*time.Second
	if got := timer.FinishAt(t0.Add(72 * time.Second)); got != want {
		t.Fatalf("GenerationDuration = %v, want %v", got, want)
	}
}
