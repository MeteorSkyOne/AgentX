package codexpersist

import (
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestTurnPlanUpdatedEmitsPlanSnapshot(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 1)}

	terminal := s.handleNotification(jsonRPCMessage{
		Method: "turn/plan/updated",
		Params: map[string]any{
			"threadId":    "thr",
			"turnId":      "turn",
			"explanation": "Fix the bug",
			"plan": []any{
				map[string]any{"step": "Inspect", "status": "completed"},
				map[string]any{"step": "Patch", "status": "inProgress"},
				map[string]any{"step": "Test", "status": "pending"},
				map[string]any{"step": "  ", "status": "pending"},
			},
		},
	}, newNotificationState())

	if terminal {
		t.Fatalf("plan update should not be terminal")
	}
	evt := <-s.events
	if evt.Type != runtime.EventDelta || evt.Plan == nil {
		t.Fatalf("event = %#v, want plan delta", evt)
	}
	want := []runtime.PlanStep{
		{Step: "Inspect", Status: runtime.PlanStepCompleted},
		{Step: "Patch", Status: runtime.PlanStepInProgress},
		{Step: "Test", Status: runtime.PlanStepPending},
	}
	if evt.Plan.Explanation != "Fix the bug" || len(evt.Plan.Steps) != len(want) {
		t.Fatalf("plan = %#v", evt.Plan)
	}
	for i, step := range want {
		if evt.Plan.Steps[i] != step {
			t.Fatalf("step %d = %#v, want %#v", i, evt.Plan.Steps[i], step)
		}
	}
}

func TestThreadGoalUpdatedAndClearedEmitGoalSnapshots(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 2)}
	state := newNotificationState()

	s.handleNotification(jsonRPCMessage{
		Method: "thread/goal/updated",
		Params: map[string]any{
			"threadId": "thr",
			"goal": map[string]any{
				"threadId":        "thr",
				"objective":       "Ship it",
				"status":          "budgetLimited",
				"tokenBudget":     float64(50000),
				"tokensUsed":      float64(63900),
				"timeUsedSeconds": float64(120),
			},
		},
	}, state)
	evt := <-s.events
	goal := evt.Goal
	if goal == nil || goal.Objective != "Ship it" || goal.Status != runtime.GoalStatusBudgetLimited {
		t.Fatalf("goal = %#v", goal)
	}
	if ptrValue(goal.TokenBudget) != 50000 || ptrValue(goal.TokensUsed) != 63900 || ptrValue(goal.TimeUsedSeconds) != 120 {
		t.Fatalf("goal usage = %#v", goal)
	}

	s.handleNotification(jsonRPCMessage{Method: "thread/goal/cleared", Params: map[string]any{"threadId": "thr"}}, state)
	evt = <-s.events
	if evt.Goal == nil || evt.Goal.Objective != "Ship it" || evt.Goal.Status != runtime.GoalStatusCleared {
		t.Fatalf("cleared goal = %#v", evt.Goal)
	}
}

func TestThreadGoalClearedWithoutKnownGoalEmitsNothing(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 1)}

	s.handleNotification(jsonRPCMessage{Method: "thread/goal/cleared", Params: map[string]any{"threadId": "thr"}}, newNotificationState())

	select {
	case evt := <-s.events:
		t.Fatalf("unexpected event %#v", evt)
	default:
	}
}
