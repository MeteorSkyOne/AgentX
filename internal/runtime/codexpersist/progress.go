package codexpersist

import (
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

// goalProgressEvent turns a thread/goal/updated or thread/goal/cleared
// notification into a goal snapshot event, remembering the latest goal so a
// later clear can still name its objective. It returns nil for other methods.
func (s *persistentSession) goalProgressEvent(msg jsonRPCMessage) *runtime.Event {
	var goal *runtime.GoalState
	switch msg.Method {
	case "thread/goal/updated":
		raw, _ := notificationParams(msg)["goal"].(map[string]any)
		goal = goalStateFromThreadGoal(raw)
	case "thread/goal/cleared":
		s.mu.Lock()
		last := s.lastGoal
		s.mu.Unlock()
		if last == nil {
			return nil
		}
		cleared := *last
		cleared.Status = runtime.GoalStatusCleared
		goal = &cleared
	}
	if goal == nil {
		return nil
	}
	s.mu.Lock()
	s.lastGoal = goal
	s.mu.Unlock()
	return &runtime.Event{Type: runtime.EventDelta, Goal: goal}
}

// goalStateFromThreadGoal parses a codex app-server ThreadGoal:
// {threadId, objective, status, tokenBudget, tokensUsed, timeUsedSeconds, ...}.
func goalStateFromThreadGoal(goal map[string]any) *runtime.GoalState {
	if goal == nil {
		return nil
	}
	objective := strings.TrimSpace(stringVal(goal, "objective"))
	if objective == "" {
		return nil
	}
	return &runtime.GoalState{
		Objective:       objective,
		Status:          runtime.NormalizeGoalStatus(stringVal(goal, "status")),
		TokensUsed:      int64Ptr(goal, "tokensUsed"),
		TokenBudget:     int64Ptr(goal, "tokenBudget"),
		TimeUsedSeconds: int64Ptr(goal, "timeUsedSeconds"),
	}
}

// planStateFromNotification parses turn/plan/updated:
// {threadId, turnId, explanation, plan: [{step, status}]}.
func planStateFromNotification(params map[string]any) *runtime.PlanState {
	rawSteps, _ := params["plan"].([]any)
	plan := &runtime.PlanState{
		Explanation: strings.TrimSpace(stringVal(params, "explanation")),
		Steps:       make([]runtime.PlanStep, 0, len(rawSteps)),
	}
	for _, raw := range rawSteps {
		step, _ := raw.(map[string]any)
		text := strings.TrimSpace(stringVal(step, "step"))
		if text == "" {
			continue
		}
		plan.Steps = append(plan.Steps, runtime.PlanStep{
			Step:   text,
			Status: runtime.NormalizePlanStepStatus(stringVal(step, "status")),
		})
	}
	return plan
}
