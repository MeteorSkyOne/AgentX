package codexpersist

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

// goalContinuationGracePeriod bounds how long the goal loop waits for codex to
// auto-continue with a fresh turn after one finishes. A goal is autonomous and
// spans many server-side turns; codex emits the next turn/started promptly after
// a turn finishes (and reports a terminal thread/goal/updated status when it
// stops), so this is only a backstop for when codex goes silent without a
// terminal signal. It is generous to avoid completing while codex is merely slow
// to continue; even if it does fire early, the goal is cleared on exit so codex
// stops auto-continuing (see clearGoalBestEffort). It is a var so tests can
// shrink it.
var goalContinuationGracePeriod = 90 * time.Second

// goalCtrl tells the goal notification loop how to react after the tracker has
// consumed a control notification.
type goalCtrl int

const (
	goalCtrlNone    goalCtrl = iota // keep looping, leave the idle timer as-is
	goalCtrlArmIdle                 // a turn finished; wait for codex to continue
	goalCtrlStop                    // terminal: the emitted events conclude the run
)

// goalTurnTracker accumulates the output of a /goal run across the many codex
// turns it spans and decides when the run is actually complete. It is pure (no
// session/process/rpc dependencies) so the completion logic can be unit tested.
type goalTurnTracker struct {
	state      *notificationState
	transcript strings.Builder
	turnActive bool
	goalDone   bool
	usage      *runtime.Usage // token usage accumulated across the goal's turns
}

func newGoalTurnTracker() *goalTurnTracker {
	// A turn is already in flight when the tracker is created: handleGoalSet
	// starts the first turn via turn/start before entering the loop.
	return &goalTurnTracker{state: newNotificationState(), turnActive: true}
}

// flush moves the current turn's final agent text into the goal transcript and
// returns a delta event that streams it live (nil when there is nothing to
// stream). Each turn's answer becomes its own block separated by a process-break
// marker so the frontend interleaves it with the tool calls that ran during the
// turn — surfacing per-turn intermediate text the same way the Claude runtime
// does. processCount is the number of process items delivered so far, used as the
// marker offset. The streamed bytes equal what is appended to the transcript so
// the live view stays consistent with the final completed body.
func (t *goalTurnTracker) flush(usage *runtime.Usage, processCount int) *runtime.Event {
	t.usage = mergeGoalUsage(t.usage, usage)
	text := strings.TrimSpace(t.state.textString())
	t.state = newNotificationState()
	if text == "" {
		if usage != nil {
			return &runtime.Event{Type: runtime.EventDelta, Usage: usage}
		}
		return nil
	}
	chunk := fmt.Sprintf("\n\n<!-- process-break:%d -->\n\n%s", processCount, text)
	t.transcript.WriteString(chunk)
	return &runtime.Event{Type: runtime.EventDelta, Text: chunk, Usage: usage}
}

// completedEvent carries the full goal transcript and the token usage summed
// across every turn, so the final run metric reflects the whole goal rather than
// only the last turn's tokens.
func (t *goalTurnTracker) completedEvent() runtime.Event {
	return runtime.Event{Type: runtime.EventCompleted, Text: t.transcript.String(), Usage: t.usage}
}

// handleControl consumes the goal/turn lifecycle notifications. It returns
// handled=false for streaming notifications (deltas, items, token usage), which
// the caller forwards to the normal handleNotification path.
func (t *goalTurnTracker) handleControl(msg jsonRPCMessage, processCount int) (events []runtime.Event, ctrl goalCtrl, handled bool) {
	params := notificationParams(msg)
	switch msg.Method {
	case "turn/completed":
		t.turnActive = false
		usage := turnCompletedUsage(params)
		if turnStatus(params) == "interrupted" {
			if e := t.flush(usage, processCount); e != nil {
				events = append(events, *e)
			}
			events = append(events, runtime.Event{Type: runtime.EventCanceled, Text: t.transcript.String()})
			return events, goalCtrlStop, true
		}
		if e := t.flush(usage, processCount); e != nil {
			events = append(events, *e)
		}
		if t.goalDone {
			events = append(events, t.completedEvent())
			return events, goalCtrlStop, true
		}
		return events, goalCtrlArmIdle, true

	case "thread/goal/cleared":
		t.goalDone = true
		return t.finishIfBetweenTurns(processCount)

	case "thread/goal/updated":
		if isTerminalGoalStatus(goalUpdateStatus(params)) {
			t.goalDone = true
			return t.finishIfBetweenTurns(processCount)
		}
		return nil, goalCtrlNone, true

	case "thread/closed":
		if e := t.flush(nil, processCount); e != nil {
			events = append(events, *e)
		}
		if t.transcript.Len() > 0 {
			events = append(events, t.completedEvent())
		} else {
			events = append(events, runtime.Event{Type: runtime.EventFailed, Error: "thread closed"})
		}
		return events, goalCtrlStop, true
	}
	return nil, goalCtrlNone, false
}

// finishIfBetweenTurns completes the run immediately when the goal reached a
// terminal status while no turn is in flight. When a turn is still active the run
// waits for that turn/completed so the agent's final summary is not cut off — but
// it also arms the idle backstop so the run cannot hang forever if codex abandons
// the in-flight turn without ever emitting turn/completed. Genuine turn output
// (item/* notifications) re-disarms the backstop, so a still-producing turn is
// never cut short.
func (t *goalTurnTracker) finishIfBetweenTurns(processCount int) ([]runtime.Event, goalCtrl, bool) {
	if t.turnActive {
		return nil, goalCtrlArmIdle, true
	}
	var events []runtime.Event
	if e := t.flush(nil, processCount); e != nil {
		events = append(events, *e)
	}
	return append(events, t.completedEvent()), goalCtrlStop, true
}

// finishGoal concludes the run when the grace window elapses with no continuation.
func (t *goalTurnTracker) finishGoal(processCount int) []runtime.Event {
	var events []runtime.Event
	if e := t.flush(nil, processCount); e != nil {
		events = append(events, *e)
	}
	return append(events, t.completedEvent())
}

// finishOnChannelClose concludes the run when the notification channel closes.
func (t *goalTurnTracker) finishOnChannelClose(processCount int) []runtime.Event {
	var events []runtime.Event
	if e := t.flush(nil, processCount); e != nil {
		events = append(events, *e)
	}
	if t.transcript.Len() > 0 {
		return append(events, t.completedEvent())
	}
	return append(events, runtime.Event{Type: runtime.EventFailed, Error: "notification channel closed"})
}

// processGoalNotifications consumes notifications for a /goal run. Unlike a
// normal turn, intermediate turn/completed events are NOT terminal: codex keeps
// the goal going with fresh turns until it reaches a terminal status
// (thread/goal/updated) or is cleared. The whole goal is surfaced to AgentX as a
// single turn whose body is the concatenation of every turn's answer.
func (s *persistentSession) processGoalNotifications(ctx context.Context) {
	tracker := newGoalTurnTracker()
	defer func() {
		// If the goal did not reach a terminal status on its own (interrupt,
		// idle-grace timeout, error, ctx/stream end), clear it so codex stops
		// auto-continuing. Otherwise the still-active goal keeps driving turns on
		// the pooled process and their notifications leak into the next run.
		if !tracker.goalDone {
			s.clearGoalBestEffort()
		}
	}()
	var idleC <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: ctx.Err().Error()})
			return
		case <-s.process.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: codexAppServerExitedError(s.process.Stderr())})
			return
		case <-idleC:
			for _, e := range tracker.finishGoal(s.deliveredProcessCount()) {
				s.emit(e)
			}
			return
		case msg, ok := <-s.rpc.Notifications():
			if !ok {
				for _, e := range tracker.finishOnChannelClose(s.deliveredProcessCount()) {
					s.emit(e)
				}
				return
			}
			if msg.ID != nil {
				s.handleServerRequest(msg)
				continue
			}
			if isGoalActivitySignal(msg.Method) {
				// A turn is actively producing output, so codex is still working
				// on the goal: cancel any pending "stopped" grace timer.
				tracker.turnActive = true
				idleC = nil
			}
			switch {
			case msg.Method == "error":
				// Reuse the normal error handling (message extraction + stale
				// session detection); it emits EventFailed and is terminal.
				s.handleNotification(msg, tracker.state)
				return
			case msg.Method == "turn/started":
				s.handleNotification(msg, tracker.state) // registers the active turn id
			default:
				events, ctrl, handled := tracker.handleControl(msg, s.deliveredProcessCount())
				if !handled {
					s.handleNotification(msg, tracker.state)
					continue
				}
				if msg.Method == "turn/completed" {
					// The turn is over; drop the active turn id so an Interrupt or
					// Steer during the gap before the next turn falls back to the
					// pending-request path instead of targeting a dead turn.
					s.clearActiveTurn()
				}
				for _, e := range events {
					s.emit(e)
				}
				switch ctrl {
				case goalCtrlArmIdle:
					idleC = time.After(goalContinuationGracePeriod)
				case goalCtrlStop:
					s.clearActiveTurn()
					return
				}
			}
		}
	}
}

// isGoalActivitySignal reports whether a notification means a turn is actively
// producing output (so the goal is still running). Used to cancel the
// continuation grace timer; token-usage and goal-status updates are deliberately
// excluded since they can fire while no turn is in flight.
func isGoalActivitySignal(method string) bool {
	return method == "turn/started" || strings.HasPrefix(method, "item/")
}

// clearGoalBestEffort tells codex to drop the active goal so it stops driving
// further turns. It runs on every non-clean exit of the goal loop (interrupt,
// idle timeout, error, ctx/stream end); a dead process just makes the call fail
// harmlessly. A fresh context is used because the run's context may already be
// cancelled by the time we get here.
func (s *persistentSession) clearGoalBestEffort() {
	if strings.TrimSpace(s.threadID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.rpc.Call(ctx, "thread/goal/clear", map[string]any{"threadId": s.threadID}); err != nil {
		slog.Warn("codexpersist: clear goal on goal-run exit failed", "error", err)
	}
}

// mergeGoalUsage sums the per-turn token counts of a goal while keeping the most
// recent context-window snapshot and model, so the run reports the whole goal's
// consumption rather than only its last turn.
func mergeGoalUsage(acc *runtime.Usage, next *runtime.Usage) *runtime.Usage {
	if next == nil {
		return acc
	}
	if acc == nil {
		clone := *next
		return &clone
	}
	acc.InputTokens = addInt64Ptr(acc.InputTokens, next.InputTokens)
	acc.OutputTokens = addInt64Ptr(acc.OutputTokens, next.OutputTokens)
	acc.CachedInputTokens = addInt64Ptr(acc.CachedInputTokens, next.CachedInputTokens)
	acc.ReasoningOutputTokens = addInt64Ptr(acc.ReasoningOutputTokens, next.ReasoningOutputTokens)
	acc.TotalTokens = addInt64Ptr(acc.TotalTokens, next.TotalTokens)
	if next.Context != nil {
		acc.Context = next.Context
	}
	if next.Model != "" {
		acc.Model = next.Model
	}
	return acc
}

func addInt64Ptr(a *int64, b *int64) *int64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	sum := *a + *b
	return &sum
}

// goalUpdateStatus extracts the goal status from a thread/goal/updated
// notification, tolerating the shapes codex may use: a top-level status string,
// a nested status object, or a status on the goal object.
func goalUpdateStatus(params map[string]any) string {
	if params == nil {
		return ""
	}
	for _, key := range []string{"status", "state"} {
		switch v := params[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case map[string]any:
			for _, inner := range []string{"state", "status", "type", "kind"} {
				if s := stringVal(v, inner); s != "" {
					return s
				}
			}
		}
	}
	for _, key := range []string{"goal", "threadGoal", "thread_goal"} {
		if goal, ok := params[key].(map[string]any); ok {
			if s := goalUpdateStatus(goal); s != "" {
				return s
			}
		}
	}
	return ""
}

// isTerminalGoalStatus reports whether a goal status means the autonomous run has
// stopped (achieved, failed, abandoned, paused, hit a usage/budget limit, etc.).
// Only explicit terminal statuses match: unknown or in-progress statuses are
// treated as non-terminal so the run never finishes early on an unrecognized
// progress update — the grace timer is the backstop in that case.
func isTerminalGoalStatus(status string) bool {
	switch normalizeGoalStatus(status) {
	case "completed", "complete", "achieved", "done", "succeeded", "success",
		"failed", "failure", "errored", "abandoned", "cancelled", "canceled",
		"stopped", "paused", "blocked", "usagelimited", "budgetlimited",
		"expired", "timedout":
		return true
	default:
		return false
	}
}

func normalizeGoalStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	status = strings.ReplaceAll(status, "_", "")
	status = strings.ReplaceAll(status, "-", "")
	status = strings.ReplaceAll(status, " ", "")
	return status
}
