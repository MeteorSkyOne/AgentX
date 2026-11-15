package runtime

import (
	"strings"
	"sync"
	"time"
)

// MinStreamGenerationWindow is the minimum intra-step streaming span required
// to use (stepEnd - stepFirstStream) as that step's generation duration. Shorter
// spans indicate batched or tiny single-chunk delivery where prefill + decode
// happened before the chunk arrived, so the full step duration from stepStart
// is used instead.
const MinStreamGenerationWindow = 250 * time.Millisecond

// TurnTimer tracks time-to-first-token (TTFT) and active LLM generation time
// across multi-step agent turns, excluding local tool execution, user input /
// permission waits, and inter-turn idle gaps.
type TurnTimer struct {
	mu                sync.Mutex
	startedAt         time.Time
	firstTokenAt      time.Time
	stepStartedAt     time.Time
	stepFirstOutputAt time.Time
	stepFirstStreamAt time.Time
	accumulated       time.Duration
	openTools         map[string]struct{}
	anonTools         int
	waitingInput      bool
	finished          bool
}

func NewTurnTimer(startedAt time.Time) *TurnTimer {
	startedAt = normalizeTimerInstant(startedAt)
	return &TurnTimer{
		startedAt:     startedAt,
		stepStartedAt: startedAt,
		openTools:     make(map[string]struct{}),
	}
}

func normalizeTimerInstant(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

// ObserveStream records arrival of a streaming token/chunk (thinking delta,
// message delta, or tool-call argument delta).
func (t *TurnTimer) ObserveStream() {
	t.ObserveStreamAt(time.Now().UTC())
}

func (t *TurnTimer) ObserveStreamAt(now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observeStreamLocked(now)
}

// ObserveOutput records arrival of a non-streaming or already-completed model
// output block (e.g. a complete assistant message or plan snapshot).
func (t *TurnTimer) ObserveOutput() {
	t.ObserveOutputAt(time.Now().UTC())
}

func (t *TurnTimer) ObserveOutputAt(now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observeOutputLocked(now)
}

// ToolStarted closes the active LLM step and pauses generation timing while the
// main-agent tool executes locally.
func (t *TurnTimer) ToolStarted(toolCallID string) {
	t.ToolStartedAt(toolCallID, time.Now().UTC())
}

func (t *TurnTimer) ToolStartedAt(toolCallID string, now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolStartedLocked(toolCallID, now)
}

// ToolCompleted marks a main-agent tool call finished and starts a fresh LLM
// step once all open tools and input waits have cleared.
func (t *TurnTimer) ToolCompleted(toolCallID string) {
	t.ToolCompletedAt(toolCallID, time.Now().UTC())
}

func (t *TurnTimer) ToolCompletedAt(toolCallID string, now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCompletedLocked(toolCallID, now)
}

// InputWaitStarted closes any active LLM step and pauses timing while waiting
// for user input or permission confirmation.
func (t *TurnTimer) InputWaitStarted() {
	t.InputWaitStartedAt(time.Now().UTC())
}

func (t *TurnTimer) InputWaitStartedAt(now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inputWaitStartedLocked(now)
}

// InputWaitCompleted resumes timing once the user answers a pending prompt,
// unless local tool calls are still executing.
func (t *TurnTimer) InputWaitCompleted() {
	t.InputWaitCompletedAt(time.Now().UTC())
}

func (t *TurnTimer) InputWaitCompletedAt(now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.waitingInput = false
	if !t.isPausedLocked() && t.stepStartedAt.IsZero() {
		t.stepStartedAt = now
	}
}

// PauseStep closes any active LLM step without opening a tool wait (used for
// inter-turn idle gaps in multi-turn autonomous runs).
func (t *TurnTimer) PauseStep() {
	t.PauseStepAt(time.Now().UTC())
}

func (t *TurnTimer) PauseStepAt(now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.closeStepLocked(now)
}

// ResumeStep starts a new LLM step if none is currently active and no tools or
// input prompts are waiting.
func (t *TurnTimer) ResumeStep() {
	t.ResumeStepAt(time.Now().UTC())
}

func (t *TurnTimer) ResumeStepAt(now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	if !t.isPausedLocked() && t.stepStartedAt.IsZero() {
		t.stepStartedAt = now
	}
}

// ObserveEvent updates the timer from a high-level runtime Event stream.
func (t *TurnTimer) ObserveEvent(evt Event) {
	t.ObserveEventAt(evt, time.Now().UTC())
}

func (t *TurnTimer) ObserveEventAt(evt Event, now time.Time) {
	if t == nil {
		return
	}
	now = normalizeTimerInstant(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}

	switch evt.Type {
	case EventDelta:
		subagentOnly := isSubagentOnlyProcess(evt.Process)
		if evt.Plan != nil {
			t.observeOutputLocked(now)
		}
		if !subagentOnly && !evt.ClearText && strings.TrimSpace(evt.Thinking) != "" {
			t.observeOutputLocked(now)
		}
		if strings.TrimSpace(evt.Text) != "" {
			t.observeStreamLocked(now)
		}
		for _, item := range evt.Process {
			if item.ParentToolCallID != "" {
				continue
			}
			switch item.Type {
			case "thinking":
				if !evt.ClearText && strings.TrimSpace(item.Text) != "" {
					t.observeOutputLocked(now)
				}
			case "tool_call":
				if isTerminalToolProcessStatus(item.Status) {
					t.toolCompletedLocked(item.ToolCallID, now)
				} else {
					t.toolStartedLocked(item.ToolCallID, now)
				}
			case "tool_result":
				t.toolCompletedLocked(item.ToolCallID, now)
			}
		}

	case EventInputRequest:
		if evt.InputRequest != nil {
			t.inputWaitStartedLocked(now)
		}

	case EventCompleted:
		if strings.TrimSpace(evt.Thinking) != "" || strings.TrimSpace(evt.Text) != "" || evt.Plan != nil {
			t.observeOutputLocked(now)
		}
		t.finishLocked(now)

	case EventCanceled, EventFailed:
		t.finishLocked(now)
	}
}

// FirstTokenAt returns a copy of the earliest model-output timestamp, or nil
// when no model output was observed.
func (t *TurnTimer) FirstTokenAt() *time.Time {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.firstTokenAt.IsZero() {
		return nil
	}
	ts := t.firstTokenAt
	return &ts
}

// Finish closes any active LLM step and returns the accumulated active
// generation duration.
func (t *TurnTimer) Finish() time.Duration {
	return t.FinishAt(time.Now().UTC())
}

func (t *TurnTimer) FinishAt(completedAt time.Time) time.Duration {
	if t == nil {
		return 0
	}
	completedAt = normalizeTimerInstant(completedAt)
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.finishLocked(completedAt)
}

// Apply finishes the timer at now and attaches FirstTokenAt and
// GenerationDuration onto usage (allocating a Usage if timing was recorded).
func (t *TurnTimer) Apply(usage *Usage) *Usage {
	return t.ApplyAt(usage, time.Now().UTC())
}

func (t *TurnTimer) ApplyAt(usage *Usage, completedAt time.Time) *Usage {
	if t == nil {
		return usage
	}
	genDuration := t.FinishAt(completedAt)
	firstTokenAt := t.FirstTokenAt()
	if usage == nil {
		if firstTokenAt == nil && genDuration <= 0 {
			return nil
		}
		return &Usage{
			FirstTokenAt:       firstTokenAt,
			GenerationDuration: genDuration,
		}
	}
	cloned := *usage
	if firstTokenAt != nil && (cloned.FirstTokenAt == nil || firstTokenAt.Before(*cloned.FirstTokenAt)) {
		cloned.FirstTokenAt = firstTokenAt
	}
	if genDuration > 0 {
		cloned.GenerationDuration = genDuration
	}
	return &cloned
}

func (t *TurnTimer) isPausedLocked() bool {
	return len(t.openTools) > 0 || t.anonTools > 0 || t.waitingInput
}

func (t *TurnTimer) ensureActiveStepForOutputLocked(now time.Time) {
	if t.isPausedLocked() {
		// New main-agent LLM output arrived while a tool or input wait appeared
		// open (e.g. a synthetic runtime that never emitted tool_result); clear
		// stale waits and begin a fresh step at now.
		clear(t.openTools)
		t.anonTools = 0
		t.waitingInput = false
	}
	if t.stepStartedAt.IsZero() {
		t.stepStartedAt = now
	}
}

func (t *TurnTimer) observeStreamLocked(now time.Time) {
	if t.finished {
		return
	}
	if t.firstTokenAt.IsZero() {
		t.firstTokenAt = now
	}
	t.ensureActiveStepForOutputLocked(now)
	if t.stepFirstOutputAt.IsZero() {
		t.stepFirstOutputAt = now
	}
	if t.stepFirstStreamAt.IsZero() {
		t.stepFirstStreamAt = now
	}
}

func (t *TurnTimer) observeOutputLocked(now time.Time) {
	if t.finished {
		return
	}
	if t.firstTokenAt.IsZero() {
		t.firstTokenAt = now
	}
	t.ensureActiveStepForOutputLocked(now)
	if t.stepFirstOutputAt.IsZero() {
		t.stepFirstOutputAt = now
	}
}

func (t *TurnTimer) toolStartedLocked(toolCallID string, now time.Time) {
	if t.finished {
		return
	}
	if t.firstTokenAt.IsZero() {
		t.firstTokenAt = now
	}
	if !t.stepStartedAt.IsZero() {
		if t.stepFirstOutputAt.IsZero() {
			t.stepFirstOutputAt = now
		}
		t.closeStepLocked(now)
	}
	if id := strings.TrimSpace(toolCallID); id != "" {
		t.openTools[id] = struct{}{}
	} else {
		t.anonTools++
	}
}

func (t *TurnTimer) toolCompletedLocked(toolCallID string, now time.Time) {
	if t.finished {
		return
	}
	id := strings.TrimSpace(toolCallID)
	var wasOpen bool
	if id != "" {
		_, wasOpen = t.openTools[id]
		delete(t.openTools, id)
	} else if t.anonTools > 0 {
		wasOpen = true
		t.anonTools--
	}
	if !wasOpen {
		if t.firstTokenAt.IsZero() {
			t.firstTokenAt = now
		}
		if !t.stepStartedAt.IsZero() {
			if t.stepFirstOutputAt.IsZero() {
				t.stepFirstOutputAt = now
			}
			t.closeStepLocked(now)
		}
	}
	if !t.isPausedLocked() && t.stepStartedAt.IsZero() {
		t.stepStartedAt = now
	}
}

func (t *TurnTimer) inputWaitStartedLocked(now time.Time) {
	if t.finished {
		return
	}
	if t.firstTokenAt.IsZero() {
		t.firstTokenAt = now
	}
	if !t.stepStartedAt.IsZero() {
		if t.stepFirstOutputAt.IsZero() {
			t.stepFirstOutputAt = now
		}
		t.closeStepLocked(now)
	}
	t.waitingInput = true
}

func (t *TurnTimer) closeStepLocked(endAt time.Time) {
	if t.stepStartedAt.IsZero() {
		return
	}
	if !t.stepFirstOutputAt.IsZero() {
		stepTotal := endAt.Sub(t.stepStartedAt)
		if stepTotal < 0 {
			stepTotal = 0
		}
		var streamed time.Duration
		if !t.stepFirstStreamAt.IsZero() {
			streamed = endAt.Sub(t.stepFirstStreamAt)
			if streamed < 0 {
				streamed = 0
			}
		}
		if streamed >= MinStreamGenerationWindow {
			t.accumulated += streamed
		} else {
			t.accumulated += stepTotal
		}
	}
	t.stepStartedAt = time.Time{}
	t.stepFirstOutputAt = time.Time{}
	t.stepFirstStreamAt = time.Time{}
}

func (t *TurnTimer) finishLocked(completedAt time.Time) time.Duration {
	if !t.finished {
		t.closeStepLocked(completedAt)
		t.finished = true
	}
	return t.accumulated
}

func isSubagentOnlyProcess(items []ProcessItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.ParentToolCallID == "" {
			return false
		}
	}
	return true
}

func isTerminalToolProcessStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "error", "canceled", "cancelled":
		return true
	default:
		return false
	}
}
