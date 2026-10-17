package claude

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

func trackLine(t *testing.T, tracker *ProgressTracker, line string) []runtime.Event {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("unmarshal %s: %v", line, err)
	}
	return tracker.Track(payload)
}

func TestProgressTrackerFollowsStopHookGoal(t *testing.T) {
	tracker := NewProgressTracker()
	now := time.Unix(1000, 0)
	tracker.now = func() time.Time { return now }

	events := trackLine(t, tracker, `{"type":"assistant","message":{"model":"<synthetic>","role":"assistant","content":[{"type":"text","text":"Goal set: counter.txt contains 2"}]},"parent_tool_use_id":null,"local_command_run":{"command":"goal","args":"counter.txt contains 2"}}`)
	if len(events) != 1 || events[0].Goal == nil {
		t.Fatalf("events = %#v, want goal snapshot", events)
	}
	if goal := events[0].Goal; goal.Objective != "counter.txt contains 2" || goal.Status != runtime.GoalStatusActive || goal.Iterations != 0 {
		t.Fatalf("goal = %#v", goal)
	}

	now = now.Add(90 * time.Second)
	events = trackLine(t, tracker, `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Stop hook feedback:\n[counter.txt contains 2]: counter.txt still contains 1"}]},"parent_tool_use_id":null,"isSynthetic":true}`)
	if len(events) != 1 || events[0].Goal == nil {
		t.Fatalf("events = %#v, want goal snapshot", events)
	}
	goal := events[0].Goal
	if goal.Iterations != 1 || goal.LastReason != "counter.txt still contains 1" || goal.TimeUsedSeconds == nil || *goal.TimeUsedSeconds != 90 {
		t.Fatalf("goal = %#v", goal)
	}

	// A real user message that happens to look like feedback is not counted.
	if events := trackLine(t, tracker, `{"type":"user","message":{"role":"user","content":"Stop hook feedback:\n[x]: y"}}`); len(events) != 0 {
		t.Fatalf("non-synthetic feedback produced %#v", events)
	}

	final := tracker.Finish()
	if final == nil || final.Status != runtime.GoalStatusComplete || final.Iterations != 1 || final.LastReason != "" {
		t.Fatalf("final goal = %#v", final)
	}
	if again := tracker.Finish(); again != nil {
		t.Fatalf("Finish after completion = %#v, want nil", again)
	}
}

func TestProgressTrackerGoalCleared(t *testing.T) {
	tracker := NewProgressTracker()

	events := trackLine(t, tracker, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Goal cleared: ship it"}]},"local_command_run":{"command":"goal","args":"clear"}}`)
	if len(events) != 1 || events[0].Goal == nil || events[0].Goal.Objective != "ship it" || events[0].Goal.Status != runtime.GoalStatusCleared {
		t.Fatalf("events = %#v, want cleared goal", events)
	}
	if final := tracker.Finish(); final != nil {
		t.Fatalf("Finish for cleared goal = %#v, want nil", final)
	}
}

func TestProgressTrackerWithoutGoalFinishesNothing(t *testing.T) {
	tracker := NewProgressTracker()
	trackLine(t, tracker, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Goal set: not a command reply"}]}}`)
	if final := tracker.Finish(); final != nil {
		t.Fatalf("Finish = %#v, want nil", final)
	}
}

func TestProgressTrackerTodoWrite(t *testing.T) {
	tracker := NewProgressTracker()

	events := trackLine(t, tracker, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"TodoWrite","input":{"todos":[{"content":"Inspect","status":"completed","activeForm":"Inspecting"},{"content":"Patch","status":"in_progress","activeForm":"Patching"},{"content":"Test","status":"pending","activeForm":"Testing"}]}}]}}`)
	if len(events) != 1 || events[0].Plan == nil || events[0].Goal != nil {
		t.Fatalf("events = %#v, want plan snapshot", events)
	}
	want := []runtime.PlanStep{
		{Step: "Inspect", Status: runtime.PlanStepCompleted},
		{Step: "Patch", Status: runtime.PlanStepInProgress},
		{Step: "Test", Status: runtime.PlanStepPending},
	}
	assertPlanSteps(t, events[0].Plan, want)

	// Subagent todos belong to the subagent.
	if events := trackLine(t, tracker, `{"type":"assistant","parent_tool_use_id":"agent1","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu2","name":"TodoWrite","input":{"todos":[]}}]}}`); len(events) != 0 {
		t.Fatalf("subagent todos produced %#v", events)
	}
}

func TestProgressTrackerTaskTools(t *testing.T) {
	tracker := NewProgressTracker()

	if events := trackLine(t, tracker, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"TaskCreate","input":{"subject":"Inspect","description":"look"}},{"type":"tool_use","id":"c2","name":"TaskCreate","input":{"subject":"Patch","description":"fix"}}]}}`); len(events) != 0 {
		t.Fatalf("TaskCreate before its result produced %#v", events)
	}
	events := trackLine(t, tracker, `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":"Task #1 created successfully: Inspect"},{"type":"tool_result","tool_use_id":"c2","content":[{"type":"text","text":"Task #2 created successfully: Patch"}]}]}}`)
	if len(events) != 1 || events[0].Plan == nil {
		t.Fatalf("events = %#v, want plan snapshot", events)
	}
	assertPlanSteps(t, events[0].Plan, []runtime.PlanStep{
		{Step: "Inspect", Status: runtime.PlanStepPending},
		{Step: "Patch", Status: runtime.PlanStepPending},
	})

	events = trackLine(t, tracker, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"u1","name":"TaskUpdate","input":{"taskId":"1","status":"completed"}},{"type":"tool_use","id":"u2","name":"TaskUpdate","input":{"taskId":"2","status":"in_progress"}}]}}`)
	if len(events) != 1 {
		t.Fatalf("events = %#v, want plan snapshot", events)
	}
	assertPlanSteps(t, events[0].Plan, []runtime.PlanStep{
		{Step: "Inspect", Status: runtime.PlanStepCompleted},
		{Step: "Patch", Status: runtime.PlanStepInProgress},
	})

	events = trackLine(t, tracker, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"u3","name":"TaskUpdate","input":{"taskId":"1","status":"deleted"}}]}}`)
	assertPlanSteps(t, events[0].Plan, []runtime.PlanStep{{Step: "Patch", Status: runtime.PlanStepInProgress}})
}

func TestLineHandlerEmitsGoalBeforeCompletion(t *testing.T) {
	handler := newLineHandler("fallback")
	if _, err := handler.HandleLine([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Goal set: done"}]},"local_command_run":{"command":"goal","args":"done"}}`)); err != nil {
		t.Fatalf("HandleLine: %v", err)
	}
	events, err := handler.HandleLine([]byte(`{"type":"result","subtype":"success","result":"All done"}`))
	if err != nil {
		t.Fatalf("HandleLine: %v", err)
	}
	if len(events) != 2 || events[0].Goal == nil || events[0].Goal.Status != runtime.GoalStatusComplete || events[1].Type != runtime.EventCompleted {
		t.Fatalf("events = %#v, want completed goal then completion", events)
	}
}

func assertPlanSteps(t *testing.T, plan *runtime.PlanState, want []runtime.PlanStep) {
	t.Helper()
	if plan == nil || len(plan.Steps) != len(want) {
		t.Fatalf("plan = %#v, want %d steps", plan, len(want))
	}
	for i, step := range want {
		if plan.Steps[i] != step {
			t.Fatalf("step %d = %#v, want %#v", i, plan.Steps[i], step)
		}
	}
}
