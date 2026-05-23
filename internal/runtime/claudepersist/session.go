package claudepersist

import (
	"context"
	"encoding/json"
	"errors"
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
	claudeResultSettleDelay = 750 * time.Millisecond
	claudeResultMaxWait     = 10 * time.Minute
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
}

func newPersistentSession(proc *procpool.ManagedProcess, key string, rt *Runtime) *persistentSession {
	fallbackID := "claude:" + key
	return &persistentSession{
		process:      proc,
		key:          key,
		rt:           rt,
		events:       make(chan runtime.Event, 64),
		sessionID:    fallbackID,
		alive:        true,
		done:         make(chan struct{}),
		pendingInput: make(chan inputAnswer, 1),
	}
}

func (s *persistentSession) waitForSystemEvent(ctx context.Context) error {
	timeout := time.NewTimer(10 * time.Second)
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

func (s *persistentSession) ContextUsage(ctx context.Context) (*runtime.ContextUsage, error) {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return nil, procpool.ErrProcessDead
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
		return false, nil

	case "assistant", "user":
		payloadType := claude.StringValue(payload, "type")
		text, thinking, process := claude.AssistantContent(payload)
		if payloadType != "assistant" {
			text = ""
		}
		if text == "" && thinking == "" && len(process) == 0 {
			return false, nil
		}
		if text != "" {
			state.appendText(text)
		}
		var clearText bool
		if thinking != "" || len(process) > 0 {
			if promoted := state.promotePendingImmediate(); promoted != "" {
				process = append([]runtime.ProcessItem{{Type: "thinking", Text: promoted}}, process...)
				clearText = true
			}
		}
		if text != "" {
			state.appendPendingText(text)
		}
		state.trackProcess(process)
		s.emit(runtime.Event{Type: runtime.EventDelta, Text: text, Thinking: thinking, Process: process, ClearText: clearText})
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
		if stage := state.stageThinkingForResult(text); stage != "" {
			evt.Thinking = stage
			evt.Process = []runtime.ProcessItem{{Type: "thinking", Text: stage}}
		}
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

type claudeTurnState struct {
	textBuf        strings.Builder
	pendingTextBuf strings.Builder
	stageTextBuf   strings.Builder

	openTools         map[string]struct{}
	pendingCompletion *runtime.Event
	completionText    string
	settleTimer       *time.Timer
	maxWaitTimer      *time.Timer
}

func newClaudeTurnState() *claudeTurnState {
	return &claudeTurnState{openTools: map[string]struct{}{}}
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

func (s *claudeTurnState) appendPendingText(text string) {
	appendStageText(&s.pendingTextBuf, text)
}

func (s *claudeTurnState) promotePendingImmediate() string {
	text := strings.TrimSpace(s.pendingTextBuf.String())
	if text == "" {
		return ""
	}
	s.pendingTextBuf.Reset()
	return text
}

func (s *claudeTurnState) stageThinkingForResult(result string) string {
	if strings.TrimSpace(result) != "" && strings.TrimSpace(s.pendingTextBuf.String()) != "" && !sameNormalizedText(s.pendingTextBuf.String(), result) {
		s.promotePendingStageText()
	}
	stage := strings.TrimSpace(s.stageTextBuf.String())
	if stage == "" {
		return ""
	}
	if strings.TrimSpace(result) == "" {
		return ""
	}
	return stage
}

func (s *claudeTurnState) promotePendingStageText() {
	text := strings.TrimSpace(s.pendingTextBuf.String())
	if text == "" {
		return
	}
	appendStageText(&s.stageTextBuf, text)
	s.pendingTextBuf.Reset()
}

func (s *claudeTurnState) trackProcess(process []runtime.ProcessItem) {
	for _, item := range process {
		switch item.Type {
		case "tool_call":
			if item.ToolCallID != "" {
				s.openTools[item.ToolCallID] = struct{}{}
			}
		case "tool_result":
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
	s.pendingCompletion = &evt
	s.completionText = s.text()
	if s.maxWaitTimer == nil {
		s.maxWaitTimer = time.NewTimer(claudeResultMaxWait)
	}
}

func (s *claudeTurnState) updateCompletionTimers() {
	if s.pendingCompletion == nil || s.hasOpenTools() {
		s.stopSettleTimer()
		return
	}
	if s.settleTimer == nil {
		s.settleTimer = time.NewTimer(claudeResultSettleDelay)
		return
	}
	if !s.settleTimer.Stop() {
		select {
		case <-s.settleTimer.C:
		default:
		}
	}
	s.settleTimer.Reset(claudeResultSettleDelay)
}

func (s *claudeTurnState) settleC() <-chan time.Time {
	if s.settleTimer == nil {
		return nil
	}
	return s.settleTimer.C
}

func (s *claudeTurnState) maxWaitC() <-chan time.Time {
	if s.maxWaitTimer == nil {
		return nil
	}
	return s.maxWaitTimer.C
}

func (s *claudeTurnState) completionEvent() runtime.Event {
	evt := runtime.Event{Type: runtime.EventCompleted, Text: s.text()}
	if s.pendingCompletion != nil {
		evt = *s.pendingCompletion
		if text := strings.TrimSpace(s.text()); text != "" && !sameNormalizedText(text, s.completionText) {
			evt.Text = s.text()
		}
	}
	s.stopSettleTimer()
	if s.maxWaitTimer != nil {
		s.maxWaitTimer.Stop()
	}
	return evt
}

func (s *claudeTurnState) stopSettleTimer() {
	if s.settleTimer == nil {
		return
	}
	if !s.settleTimer.Stop() {
		select {
		case <-s.settleTimer.C:
		default:
		}
	}
}

func appendStageText(buf *strings.Builder, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if buf.Len() > 0 {
		buf.WriteByte('\n')
	}
	buf.WriteString(text)
}

func sameNormalizedText(left string, right string) bool {
	return strings.Join(strings.Fields(left), " ") == strings.Join(strings.Fields(right), " ")
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

	// When in a plan-mode override session, auto-approve ExitPlanMode and
	// restore the base permission mode so the process exits plan cleanly.
	if request != nil && (toolName == "ExitPlanMode" || toolName == "ExitPlanModeV2") && s.modeOverride != "" {
		response := map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "success",
				"request_id": requestID,
				"response": map[string]any{
					"behavior":     "allow",
					"updatedInput": map[string]any{},
					"updatedPermissions": []any{
						map[string]any{
							"type":        "setMode",
							"mode":        s.baseMode,
							"destination": "session",
						},
					},
				},
			},
		}
		if err := s.process.WriteJSON(response); err != nil {
			slog.Warn("claudepersist: failed to send ExitPlanMode response", "key", s.key, "error", err)
		}
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
