package codex

import (
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestTodoListItemsEmitPlanSnapshots(t *testing.T) {
	handler := newLineHandler("fallback")

	for _, line := range []string{
		`{"type":"item.started","item":{"id":"todo","type":"todo_list","items":[{"text":"Inspect","completed":false},{"text":"Patch","completed":false}]}}`,
		`{"type":"item.updated","item":{"id":"todo","type":"todo_list","items":[{"text":"Inspect","completed":true},{"text":"Patch","completed":false}]}}`,
	} {
		events, err := handler.HandleLine([]byte(line))
		if err != nil {
			t.Fatalf("HandleLine: %v", err)
		}
		if len(events) != 1 || events[0].Plan == nil || len(events[0].Process) != 0 {
			t.Fatalf("events = %#v, want a single plan delta", events)
		}
	}

	events, _ := handler.HandleLine([]byte(`{"type":"item.completed","item":{"id":"todo","type":"todo_list","items":[{"text":"Inspect","completed":true},{"text":"Patch","completed":true}]}}`))
	if len(events) != 1 || events[0].Plan == nil {
		t.Fatalf("events = %#v, want plan delta", events)
	}
	want := []runtime.PlanStep{
		{Step: "Inspect", Status: runtime.PlanStepCompleted},
		{Step: "Patch", Status: runtime.PlanStepCompleted},
	}
	for i, step := range want {
		if events[0].Plan.Steps[i] != step {
			t.Fatalf("step %d = %#v, want %#v", i, events[0].Plan.Steps[i], step)
		}
	}
}
