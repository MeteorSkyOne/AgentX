package runtime

import "strings"

// Goal statuses, normalized across providers. Codex reports its native goal
// status (camelCase on the wire); Claude Code's Stop-hook goal is inferred from
// its stream-json output and only ever reports active, complete or cleared.
const (
	GoalStatusActive        = "active"
	GoalStatusPaused        = "paused"
	GoalStatusBlocked       = "blocked"
	GoalStatusUsageLimited  = "usage_limited"
	GoalStatusBudgetLimited = "budget_limited"
	GoalStatusComplete      = "complete"
	GoalStatusCleared       = "cleared"
)

// Plan step statuses, normalized across providers.
const (
	PlanStepPending    = "pending"
	PlanStepInProgress = "in_progress"
	PlanStepCompleted  = "completed"
)

// GoalState is a snapshot of an agent's autonomous /goal run. Events carry the
// whole snapshot, so the latest one always replaces the previous.
type GoalState struct {
	Objective       string `json:"objective"`
	Status          string `json:"status"`
	TokensUsed      *int64 `json:"tokens_used,omitempty"`
	TokenBudget     *int64 `json:"token_budget,omitempty"`
	TimeUsedSeconds *int64 `json:"time_used_seconds,omitempty"`
	// Iterations counts the goal evaluations that found the goal not yet met.
	Iterations int    `json:"iterations,omitempty"`
	LastReason string `json:"last_reason,omitempty"`
}

// PlanState is a snapshot of the agent's step list (Codex update_plan, Claude
// TodoWrite / TaskCreate / TaskUpdate). The latest snapshot replaces the
// previous one.
type PlanState struct {
	Explanation string     `json:"explanation,omitempty"`
	Steps       []PlanStep `json:"steps"`
}

type PlanStep struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

// NormalizePlanStepStatus maps provider step statuses (in_progress,
// inProgress, completed, ...) onto the PlanStep* constants.
func NormalizePlanStepStatus(status string) string {
	switch normalizeStatusKey(status) {
	case "inprogress", "active", "running":
		return PlanStepInProgress
	case "completed", "complete", "done":
		return PlanStepCompleted
	default:
		return PlanStepPending
	}
}

// NormalizeGoalStatus maps provider goal statuses onto the GoalStatus*
// constants; unknown statuses are passed through lowercased.
func NormalizeGoalStatus(status string) string {
	switch key := normalizeStatusKey(status); key {
	case "", "active", "inprogress", "pending", "running":
		return GoalStatusActive
	case "usagelimited":
		return GoalStatusUsageLimited
	case "budgetlimited":
		return GoalStatusBudgetLimited
	case "complete", "completed", "achieved", "done":
		return GoalStatusComplete
	default:
		return key
	}
}

var statusKeyReplacer = strings.NewReplacer("_", "", "-", "", " ", "")

func normalizeStatusKey(status string) string {
	return statusKeyReplacer.Replace(strings.ToLower(strings.TrimSpace(status)))
}
