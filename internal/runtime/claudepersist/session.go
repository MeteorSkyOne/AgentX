package claudepersist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/claude"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

var (
	// Current Claude Code versions do not emit their init event until the first
	// stream-json input arrives. Keep a short grace period for older versions
	// and immediate startup failures without delaying every new session.
	claudeStartupInitGrace  = 250 * time.Millisecond
	claudeResultSettleDelay = 100 * time.Millisecond
	claudeResultMaxWait     = 30 * time.Second
	// claudeBackgroundIdleTimeout bounds a turn that is being held open for a
	// background subagent. It is refreshed by any output, so it only trips when
	// the process has genuinely gone silent.
	claudeBackgroundIdleTimeout = 5 * time.Minute
)

type inputAnswer struct {
	questionID string
	answer     string
}

type persistentSession struct {
	process *procpool.ManagedProcess
	key     string
	rt      *Runtime
	events  chan runtime.Event

	mu                  sync.Mutex
	eventMu             sync.Mutex
	sessionID           string
	alive               bool
	started             bool
	turnHeld            bool
	done                chan struct{}
	closeOnce           sync.Once
	pendingInput        chan inputAnswer
	pendingControlInput map[string]any
	modeOverride        string
	baseMode            string
	// taskByToolUse maps a spawning tool call to its running background task,
	// so StopSubagent can be invoked from outside the turn goroutine.
	taskByToolUse map[string]string
}

func newPersistentSession(proc *procpool.ManagedProcess, key string, rt *Runtime) *persistentSession {
	fallbackID := "claude:" + key
	return &persistentSession{
		process:       proc,
		key:           key,
		rt:            rt,
		events:        make(chan runtime.Event, 64),
		sessionID:     fallbackID,
		alive:         true,
		done:          make(chan struct{}),
		pendingInput:  make(chan inputAnswer, 1),
		taskByToolUse: map[string]string{},
	}
}

func (s *persistentSession) waitForSystemEvent(ctx context.Context) error {
	s.process.AttachReader()
	defer s.process.DetachReader()

	timeout := time.NewTimer(claudeStartupInitGrace)
	defer timeout.Stop()
	for {
		select {
		case line, ok := <-s.process.StdoutLines():
			if !ok {
				return s.processExitError("persistent process exited before initialization")
			}
			var payload map[string]any
			if err := json.Unmarshal(line, &payload); err != nil {
				continue
			}
			if claude.StringValue(payload, "type") == "system" {
				if id := claude.StringValue(payload, "session_id"); id != "" {
					s.mu.Lock()
					s.sessionID = id
					s.mu.Unlock()
				}
				return nil
			}
		case <-timeout.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-s.process.Done():
			return s.processExitError("persistent process exited before initialization")
		}
	}
}

func (s *persistentSession) Send(ctx context.Context, input runtime.Input) error {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return procpool.ErrProcessDead
	}
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	modeOverride := s.modeOverride
	s.mu.Unlock()

	if err := s.process.AcquireTurn(ctx); err != nil {
		s.emitError(err)
		return nil
	}
	s.mu.Lock()
	s.turnHeld = true
	s.mu.Unlock()
	s.process.AttachReader()

	if modeOverride != "" {
		s.sendSetPermissionMode(modeOverride)
	}

	msg, err := buildUserMessage(input)
	if err != nil {
		s.releaseTurn()
		s.emitFailed(err.Error())
		return nil
	}

	if err := s.process.WriteJSON(msg); err != nil {
		s.releaseTurn()
		s.emitError(err)
		return nil
	}

	go s.readEvents(ctx)
	return nil
}

func (s *persistentSession) Events() <-chan runtime.Event {
	return s.events
}

func (s *persistentSession) CurrentSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

func (s *persistentSession) Alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive
}

func (s *persistentSession) RespondToInputRequest(questionID string, answer string) error {
	select {
	case s.pendingInput <- inputAnswer{questionID: questionID, answer: answer}:
		return nil
	default:
		return errors.New("no pending input request")
	}
}

// recordTaskSignals mirrors the turn's task lifecycle into a session-level map
// so StopSubagent, called from outside the turn goroutine, can resolve which
// task a tool call spawned.
func (s *persistentSession) recordTaskSignals(outcome claude.TrackOutcome) {
	if len(outcome.Started) == 0 && len(outcome.Finished) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, signal := range outcome.Started {
		if signal.ToolUseID != "" {
			s.taskByToolUse[signal.ToolUseID] = signal.TaskID
		}
	}
	for _, signal := range outcome.Finished {
		if signal.ToolUseID != "" {
			delete(s.taskByToolUse, signal.ToolUseID)
		}
	}
}

// StopSubagent asks Claude Code to stop the background task spawned by the
// given tool call. The CLI acknowledges by emitting the task's terminal
// lifecycle messages, which flow through the normal turn handling — the
// subagent flips to done in the UI and the turn drains as usual.
func (s *persistentSession) StopSubagent(_ context.Context, toolCallID string) error {
	s.mu.Lock()
	taskID, ok := s.taskByToolUse[toolCallID]
	alive := s.alive
	s.mu.Unlock()
	if !alive {
		return procpool.ErrProcessDead
	}
	if !ok {
		return errors.New("no running subagent for this tool call")
	}
	msg := map[string]any{
		"type":       "control_request",
		"request_id": id.New("ctrl"),
		"request": map[string]any{
			"subtype": "stop_task",
			"task_id": taskID,
		},
	}
	if err := s.process.WriteJSON(msg); err != nil {
		return err
	}
	slog.Info("claudepersist: requested subagent stop", "key", s.key, "task_id", taskID, "tool_call_id", toolCallID)
	return nil
}

func (s *persistentSession) ContextUsage(ctx context.Context) (*runtime.ContextUsage, error) {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return nil, procpool.ErrProcessDead
	}
	// Claude Code initializes stream-json sessions on the first user input.
	// A control request cannot be that first input, so let the caller fall back
	// to the synthetic /context command until this process has a real session ID.
	if strings.HasPrefix(s.sessionID, "claude:") {
		s.mu.Unlock()
		return nil, nil
	}
	if s.started {
		s.mu.Unlock()
		return nil, errors.New("cannot read context usage while session is running")
	}
	s.started = true
	s.mu.Unlock()

	if err := s.process.AcquireTurn(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.turnHeld = true
	s.mu.Unlock()
	s.process.AttachReader()
	defer s.releaseTurn()

	requestID := id.New("ctx")
	msg := map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request": map[string]any{
			"subtype": "get_context_usage",
		},
	}
	if err := s.process.WriteJSON(msg); err != nil {
		return nil, err
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.process.Done():
			return nil, s.processExitError("persistent process exited while reading context usage")
		case line, ok := <-s.process.StdoutLines():
			if !ok {
				return nil, s.processExitError("stdout closed while reading context usage")
			}
			usage, matched, err := contextUsageFromControlResponse(line, requestID)
			if !matched {
				continue
			}
			if err == nil && usage == nil {
				err = errors.New("context usage response did not include usage")
			}
			return usage, err
		}
	}
}

func (s *persistentSession) Close(ctx context.Context) error {
	s.mu.Lock()
	s.alive = false
	turnHeld := s.turnHeld
	s.turnHeld = false
	modeOverride := s.modeOverride
	s.mu.Unlock()

	if modeOverride != "" {
		s.sendSetPermissionMode(s.baseMode)
	}
	if turnHeld {
		s.process.DetachReader()
		s.process.ReleaseTurn()
	}
	s.closeEventStream()
	return nil
}

func contextUsageFromControlResponse(line []byte, requestID string) (*runtime.ContextUsage, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		return nil, false, nil
	}
	if claude.StringValue(payload, "type") != "control_response" {
		return nil, false, nil
	}
	response, _ := payload["response"].(map[string]any)
	if response == nil || claude.StringValue(response, "request_id") != requestID {
		return nil, false, nil
	}
	if subtype := claude.StringValue(response, "subtype"); subtype != "" && subtype != "success" {
		if message := firstStringValue(response, "error", "message"); message != "" {
			return nil, true, errors.New(message)
		}
		return nil, true, errors.New("context usage request failed")
	}
	data, _ := response["response"].(map[string]any)
	if data == nil {
		return nil, true, nil
	}
	usage := &runtime.ContextUsage{
		TotalTokens:         firstInt64Ptr(data, "totalTokens", "total_tokens"),
		ContextWindowTokens: firstInt64Ptr(data, "rawMaxTokens", "raw_max_tokens", "maxTokens", "max_tokens"),
		UsedPercent:         firstFloat64Ptr(data, "percentage", "usedPercent", "used_percent"),
		Model:               firstStringValue(data, "model"),
		Source:              "claude_get_context_usage",
	}
	if usage.TotalTokens == nil && usage.ContextWindowTokens == nil && usage.UsedPercent == nil && usage.Model == "" {
		return nil, true, nil
	}
	return usage, true, nil
}

func firstStringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := claude.StringValue(values, key); text != "" {
			return text
		}
	}
	return ""
}

func firstInt64Ptr(values map[string]any, keys ...string) *int64 {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		if parsed, ok := numberInt64(value); ok {
			return &parsed
		}
	}
	return nil
}

func firstFloat64Ptr(values map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		if parsed, ok := numberFloat64(value); ok {
			return &parsed
		}
	}
	return nil
}

func numberInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
		asFloat, err := typed.Float64()
		if err == nil {
			return int64(asFloat), true
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return int64(parsed), true
		}
	}
	return 0, false
}

func numberFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	}
	return 0, false
}

func (s *persistentSession) Stop(ctx context.Context) error {
	s.InitiateStop()

	done := make(chan struct{})
	go func() {
		s.process.Kill()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *persistentSession) InitiateStop() {
	s.mu.Lock()
	s.alive = false
	turnHeld := s.turnHeld
	s.turnHeld = false
	s.mu.Unlock()

	if turnHeld {
		s.process.DetachReader()
		s.process.ReleaseTurn()
	}
	s.rt.pool.Detach(s.process)
	s.closeEventStream()
}

func (s *persistentSession) readEvents(ctx context.Context) {
	defer func() {
		s.releaseTurn()
		s.mu.Lock()
		s.alive = false
		s.mu.Unlock()
		s.closeEventStream()
	}()

	state := newClaudeTurnState()
	for {
		settleC := state.settleC()
		maxWaitC := state.maxWaitC()
		backgroundC := state.backgroundC()
		select {
		case <-ctx.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: ctx.Err().Error()})
			return
		case <-settleC:
			s.emit(state.completionEvent())
			return
		case <-maxWaitC:
			s.emit(state.completionEvent())
			return
		case <-backgroundC:
			slog.Warn("claudepersist: background subagent went silent, completing turn",
				"key", s.key, "pending_tasks", state.background.PendingCount())
			s.emit(state.completionEvent())
			return
		case <-s.process.Done():
			errText := s.processExitText("persistent process exited")
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText, StaleSession: claude.IsStaleSessionError(errText)})
			return
		case line, ok := <-s.process.StdoutLines():
			if !ok {
				if state.pendingCompletion != nil {
					s.emit(state.completionEvent())
				} else if text := state.text(); text != "" {
					s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text})
				} else {
					s.emit(runtime.Event{Type: runtime.EventFailed, Error: "stdout closed"})
				}
				return
			}
			terminal, inputReq := s.handleLine(line, state)
			if inputReq != nil {
				if s.waitForInputResponse(ctx, inputReq) {
					return
				}
				continue
			}
			if terminal {
				return
			}
			state.updateCompletionTimers()
		}
	}
}

func (s *persistentSession) handleLine(line []byte, state *claudeTurnState) (bool, *runtime.InputRequest) {
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		text := strings.TrimSpace(string(line))
		if text != "" {
			state.appendText(text)
			s.emit(runtime.Event{Type: runtime.EventDelta, Text: text})
		}
		return false, nil
	}

	if sid := claude.StringValue(payload, "session_id"); sid != "" {
		s.mu.Lock()
		s.sessionID = sid
		s.mu.Unlock()
	}

	switch claude.StringValue(payload, "type") {
	case "system":
		outcome := state.trackSystemMessage(payload)
		s.recordTaskSignals(outcome)
		if items := claude.SubagentSignalItems(outcome); len(items) > 0 {
			state.processItemCount += len(items)
			s.emit(runtime.Event{Type: runtime.EventDelta, Process: items})
		}
		return false, nil

	case "assistant", "user":
		payloadType := claude.StringValue(payload, "type")
		text, thinking, process := claude.AssistantContent(payload)
		contextUsage := claude.ClaudeContextUsage(payload)
		if payloadType != "assistant" {
			text = ""
			contextUsage = nil
		}
		if text == "" && thinking == "" && len(process) == 0 && contextUsage == nil {
			return false, nil
		}

		// Output produced by a subagent is streamed for visibility but kept out
		// of the main agent's reply: its text belongs to the subagent, and its
		// tool calls must not gate the main turn's completion.
		if parent := claude.StringValue(payload, "parent_tool_use_id"); parent != "" {
			for i := range process {
				process[i].ParentToolCallID = parent
			}
			state.processItemCount += len(process)
			s.emit(runtime.Event{Type: runtime.EventDelta, Thinking: thinking, Process: process})
			return false, nil
		}
		if thinking != "" && thinking == state.lastThinkingText {
			thinking = ""
			filtered := process[:0]
			for _, item := range process {
				if item.Type != "thinking" {
					filtered = append(filtered, item)
				}
			}
			process = filtered
		}
		if thinking != "" {
			state.lastThinkingText = thinking
		}
		state.processItemCount += len(process)
		// Whitespace-only text carries no content; folding it in would emit an
		// empty process break and leave a stray marker in the stored message.
		if strings.TrimSpace(text) != "" {
			needsBreak := state.sawToolsSinceText && state.textBuf.Len() > 0
			if needsBreak {
				// Prefix the marker onto text so the delta carries it too, then
				// write once — writing it separately would duplicate it.
				marker := fmt.Sprintf("\n\n<!-- process-break:%d -->\n\n", state.processItemCount)
				text = marker + text
			} else if state.textBuf.Len() > 0 {
				state.textBuf.WriteByte('\n')
			}
			state.textBuf.WriteString(text)
			state.sawToolsSinceText = false
		}
		state.trackProcess(process)
		s.emit(runtime.Event{Type: runtime.EventDelta, Text: text, Thinking: thinking, Process: process, Usage: contextEventUsage(contextUsage)})
		return false, nil

	case "result":
		if claude.IsErrorResult(payload) {
			errText := claude.ResultError(payload)
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText, StaleSession: claude.IsStaleSessionError(errText)})
			return true, nil
		}
		text := claude.StringValue(payload, "result")
		if text == "" {
			text = state.text()
		}
		evt := runtime.Event{Type: runtime.EventCompleted, Text: text, Usage: claude.ClaudeUsage(payload)}
		state.deferCompletion(evt)
		return false, nil

	case "control_request":
		if inputReq := s.handleControlRequest(payload); inputReq != nil {
			return false, inputReq
		}
		return false, nil

	case "control_response":
		return false, nil

	default:
		return false, nil
	}
}

func contextEventUsage(contextUsage *runtime.ContextUsage) *runtime.Usage {
	if contextUsage == nil {
		return nil
	}
	return &runtime.Usage{Model: contextUsage.Model, Context: contextUsage}
}

func sameNormalizedText(left string, right string) bool {
	return strings.Join(strings.Fields(left), " ") == strings.Join(strings.Fields(right), " ")
}

type claudeTurnState struct {
	textBuf strings.Builder

	openTools         map[string]struct{}
	sawToolsSinceText bool
	lastThinkingText  string
	processItemCount  int
	pendingCompletion *runtime.Event
	// usageAcc holds token usage from results that are no longer the pending
	// completion — a turn spanning background subagents sees several results,
	// and each one's usage must survive into the final tally.
	usageAcc     *runtime.Usage
	settleTimer  *time.Timer
	maxWaitTimer *time.Timer

	background      *claude.BackgroundTracker
	backgroundTimer *time.Timer
}

func newClaudeTurnState() *claudeTurnState {
	return &claudeTurnState{
		openTools:  map[string]struct{}{},
		background: claude.NewBackgroundTracker(),
	}
}

// trackSystemMessage maintains the background task set from Claude Code's task
// lifecycle messages.
func (s *claudeTurnState) trackSystemMessage(payload map[string]any) claude.TrackOutcome {
	outcome := s.background.TrackSystemMessage(payload)
	if outcome.Drained {
		// The main agent is about to be woken up to report on the finished
		// task, so whatever result we are holding is not the last word — but
		// its usage still counts toward the turn.
		s.retirePendingCompletion()
	}
	return outcome
}

// retirePendingCompletion discards a held result as stale while folding its
// usage into the turn's tally.
func (s *claudeTurnState) retirePendingCompletion() {
	if s.pendingCompletion == nil {
		return
	}
	s.usageAcc = claude.MergeUsage(s.usageAcc, s.pendingCompletion.Usage)
	s.pendingCompletion = nil
}

// waitingOnBackground reports whether the turn must stay open regardless of any
// result message already received.
func (s *claudeTurnState) waitingOnBackground() bool {
	return s.background.Waiting()
}

func (s *claudeTurnState) text() string {
	return s.textBuf.String()
}

func (s *claudeTurnState) appendText(text string) {
	if s.textBuf.Len() > 0 {
		s.textBuf.WriteByte('\n')
	}
	s.textBuf.WriteString(text)
}

func (s *claudeTurnState) trackProcess(process []runtime.ProcessItem) {
	for _, item := range process {
		switch item.Type {
		case "tool_call":
			s.sawToolsSinceText = true
			if item.ToolCallID != "" {
				s.openTools[item.ToolCallID] = struct{}{}
			}
		case "tool_result":
			s.sawToolsSinceText = true
			if item.ToolCallID != "" {
				delete(s.openTools, item.ToolCallID)
			}
		}
	}
}

func (s *claudeTurnState) hasOpenTools() bool {
	return len(s.openTools) > 0
}

func (s *claudeTurnState) deferCompletion(evt runtime.Event) {
	s.retirePendingCompletion()
	s.pendingCompletion = &evt
	s.background.NoteResult()
	// Reset rather than create-once: a turn that spans background subagents
	// sees several results, and the cap applies to the most recent one.
	if s.maxWaitTimer == nil {
		s.maxWaitTimer = time.NewTimer(claudeResultMaxWait)
		return
	}
	resetTimer(s.maxWaitTimer, claudeResultMaxWait)
}

func (s *claudeTurnState) updateCompletionTimers() {
	if s.waitingOnBackground() {
		s.stopSettleTimer()
		s.touchBackgroundTimer()
		return
	}
	s.stopBackgroundTimer()
	if s.pendingCompletion == nil || s.hasOpenTools() {
		s.stopSettleTimer()
		return
	}
	if s.settleTimer == nil {
		s.settleTimer = time.NewTimer(claudeResultSettleDelay)
		return
	}
	resetTimer(s.settleTimer, claudeResultSettleDelay)
}

func (s *claudeTurnState) settleC() <-chan time.Time {
	if s.settleTimer == nil {
		return nil
	}
	return s.settleTimer.C
}

// maxWaitC is disabled while background work is outstanding: that cap exists to
// bound the wait for stragglers after a result, not to cut a subagent short.
func (s *claudeTurnState) maxWaitC() <-chan time.Time {
	if s.maxWaitTimer == nil || s.waitingOnBackground() {
		return nil
	}
	return s.maxWaitTimer.C
}

// backgroundC fires when a turn held open for background work has gone quiet
// for too long, so a stuck subagent cannot hang the turn forever.
func (s *claudeTurnState) backgroundC() <-chan time.Time {
	if s.backgroundTimer == nil {
		return nil
	}
	return s.backgroundTimer.C
}

func (s *claudeTurnState) touchBackgroundTimer() {
	if s.backgroundTimer == nil {
		s.backgroundTimer = time.NewTimer(claudeBackgroundIdleTimeout)
		return
	}
	resetTimer(s.backgroundTimer, claudeBackgroundIdleTimeout)
}

func (s *claudeTurnState) stopBackgroundTimer() {
	if s.backgroundTimer == nil {
		return
	}
	drainTimer(s.backgroundTimer)
	s.backgroundTimer = nil
}

func resetTimer(timer *time.Timer, d time.Duration) {
	drainTimer(timer)
	timer.Reset(d)
}

func drainTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func (s *claudeTurnState) completionEvent() runtime.Event {
	evt := runtime.Event{Type: runtime.EventCompleted, Text: s.text()}
	if s.pendingCompletion != nil {
		evt = *s.pendingCompletion
		accumulated := strings.TrimSpace(s.text())
		result := strings.TrimSpace(evt.Text)
		switch {
		case accumulated != "" && result != "" && !sameNormalizedText(accumulated, result):
			if strings.Contains(accumulated, result) {
				evt.Text = accumulated
			} else {
				marker := fmt.Sprintf("\n\n<!-- process-break:%d -->\n\n", s.processItemCount)
				evt.Text = accumulated + marker + result
			}
		case accumulated != "" && result == "":
			evt.Text = accumulated
		case accumulated != "" && sameNormalizedText(accumulated, result):
			evt.Text = accumulated
		}
	}
	evt.Usage = claude.MergeUsage(s.usageAcc, evt.Usage)
	s.stopSettleTimer()
	s.stopBackgroundTimer()
	if s.maxWaitTimer != nil {
		s.maxWaitTimer.Stop()
	}
	return evt
}

func (s *claudeTurnState) stopSettleTimer() {
	if s.settleTimer == nil {
		return
	}
	drainTimer(s.settleTimer)
	s.settleTimer = nil
}

func (s *persistentSession) waitForInputResponse(ctx context.Context, inputReq *runtime.InputRequest) bool {
	s.emit(runtime.Event{
		Type:         runtime.EventInputRequest,
		InputRequest: inputReq,
	})

	select {
	case <-ctx.Done():
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: ctx.Err().Error()})
		return true
	case <-s.process.Done():
		errText := s.processExitText("persistent process exited")
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText})
		return true
	case answer := <-s.pendingInput:
		requestID, _ := inputReq.RequestID.(string)
		s.mu.Lock()
		rawInput := s.pendingControlInput
		s.pendingControlInput = nil
		s.mu.Unlock()
		// Build updatedInput: echo original input and add answers
		updatedInput := map[string]any{}
		for k, v := range rawInput {
			updatedInput[k] = v
		}
		updatedInput["answers"] = map[string]any{
			inputReq.Question: answer.answer,
		}
		response := map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "success",
				"request_id": requestID,
				"response": map[string]any{
					"behavior":     "allow",
					"updatedInput": updatedInput,
				},
			},
		}
		if err := s.process.WriteJSON(response); err != nil {
			slog.Warn("claudepersist: failed to send input response", "key", s.key, "error", err)
		}
		return false
	}
}

func (s *persistentSession) emit(evt runtime.Event) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.events == nil {
		return
	}
	if s.done != nil {
		select {
		case <-s.done:
			return
		default:
		}
	}
	select {
	case s.events <- evt:
	default:
	}
}

func (s *persistentSession) emitFailed(errText string) {
	s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText})
	s.mu.Lock()
	s.alive = false
	s.mu.Unlock()
	s.closeEventStream()
}

func (s *persistentSession) emitError(err error) {
	errText := err.Error()
	if errors.Is(err, procpool.ErrProcessDead) {
		errText = s.processExitText("persistent process exited")
	}
	s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText, StaleSession: claude.IsStaleSessionError(errText)})
	s.mu.Lock()
	s.alive = false
	s.mu.Unlock()
	s.closeEventStream()
}

func (s *persistentSession) processExitError(fallback string) error {
	return errors.New(s.processExitText(fallback))
}

func (s *persistentSession) processExitText(fallback string) string {
	if stderr := strings.TrimSpace(s.process.Stderr()); stderr != "" {
		return stderr
	}
	return fallback
}

func (s *persistentSession) closeEventStream() {
	s.closeOnce.Do(func() {
		s.eventMu.Lock()
		defer s.eventMu.Unlock()
		if s.done != nil {
			close(s.done)
		}
		if s.events != nil {
			close(s.events)
		}
	})
}

func (s *persistentSession) releaseTurn() {
	s.mu.Lock()
	held := s.turnHeld
	s.turnHeld = false
	s.mu.Unlock()
	if held {
		s.process.DetachReader()
		s.process.ReleaseTurn()
	}
}

func buildUserMessage(input runtime.Input) (map[string]any, error) {
	if claude.HasImageAttachments(input) {
		stdinBytes, err := claude.StreamJSONInput(input)
		if err != nil {
			return nil, err
		}
		var msg map[string]any
		if err := json.Unmarshal(stdinBytes, &msg); err != nil {
			return nil, err
		}
		return msg, nil
	}
	return map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"type": "text",
					"text": input.RenderedPrompt(),
				},
			},
		},
	}, nil
}

func (s *persistentSession) handleControlRequest(payload map[string]any) *runtime.InputRequest {
	requestID := claude.StringValue(payload, "request_id")
	if requestID == "" {
		return nil
	}

	request, _ := payload["request"].(map[string]any)

	toolName := claude.StringValue(request, "tool_name")

	// When in a plan-mode override session, reject the plan and restore
	// the base permission mode so the agent continues in normal mode.
	if request != nil && (toolName == "ExitPlanMode" || toolName == "ExitPlanModeV2") && s.modeOverride != "" {
		response := map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "success",
				"request_id": requestID,
				"response": map[string]any{
					"behavior": "deny",
					"message":  "Plan mode is managed externally. Continue working in normal mode.",
				},
			},
		}
		if err := s.process.WriteJSON(response); err != nil {
			slog.Warn("claudepersist: failed to send ExitPlanMode deny response", "key", s.key, "error", err)
		}
		s.sendSetPermissionMode(s.baseMode)
		s.modeOverride = ""
		return nil
	}

	if request != nil && claude.StringValue(request, "tool_name") == "AskUserQuestion" {
		input, _ := request["input"].(map[string]any)
		if input != nil {
			question, options, _ := claude.ParseAskUserQuestion(map[string]any{
				"input": input,
				"id":    claude.StringValue(request, "tool_use_id"),
			})
			if question == "" {
				question = "The agent is requesting input"
			}
			s.mu.Lock()
			s.pendingControlInput = input
			s.mu.Unlock()
			return &runtime.InputRequest{
				QuestionID: id.New("qst"),
				Question:   question,
				ToolCallID: claude.StringValue(request, "tool_use_id"),
				RequestID:  requestID,
				Options:    options,
			}
		}
	}

	response := map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response": map[string]any{
				"behavior": "allow",
			},
		},
	}
	if err := s.process.WriteJSON(response); err != nil {
		slog.Warn("claudepersist: failed to send permission response", "key", s.key, "error", err)
	}
	return nil
}

func (s *persistentSession) sendSetPermissionMode(mode string) {
	msg := map[string]any{
		"type":       "control_request",
		"request_id": id.New("ctrl"),
		"request": map[string]any{
			"subtype": "set_permission_mode",
			"mode":    mode,
		},
	}
	if err := s.process.WriteJSON(msg); err != nil {
		slog.Warn("claudepersist: failed to send set_permission_mode", "key", s.key, "mode", mode, "error", err)
	}
}
