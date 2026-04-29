package claudepersist

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/claude"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
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
	sessionID           string
	alive               bool
	started             bool
	turnHeld            bool
	done                chan struct{}
	closeOnce           sync.Once
	pendingInput        chan inputAnswer
	pendingControlInput map[string]any
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

func (s *persistentSession) waitForSystemEvent(ctx context.Context) {
	timeout := time.After(10 * time.Second)
	for {
		select {
		case line, ok := <-s.process.StdoutLines():
			if !ok {
				return
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
				return
			}
		case <-timeout:
			return
		case <-ctx.Done():
			return
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
	s.mu.Unlock()

	if err := s.process.AcquireTurn(ctx); err != nil {
		s.emitFailed(err.Error())
		return nil
	}
	s.mu.Lock()
	s.turnHeld = true
	s.mu.Unlock()

	msg, err := buildUserMessage(input)
	if err != nil {
		s.releaseTurn()
		s.emitFailed(err.Error())
		return nil
	}

	if err := s.process.WriteJSON(msg); err != nil {
		s.releaseTurn()
		s.emitFailed(err.Error())
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

func (s *persistentSession) Close(ctx context.Context) error {
	s.mu.Lock()
	s.alive = false
	turnHeld := s.turnHeld
	s.turnHeld = false
	s.mu.Unlock()

	if turnHeld {
		s.process.ReleaseTurn()
	}
	s.closeOnce.Do(func() {
		close(s.done)
		close(s.events)
	})
	return nil
}

func (s *persistentSession) readEvents(ctx context.Context) {
	defer func() {
		s.releaseTurn()
		s.mu.Lock()
		s.alive = false
		s.mu.Unlock()
		s.closeOnce.Do(func() {
			close(s.done)
			close(s.events)
		})
	}()

	var textBuf strings.Builder
	for {
		select {
		case <-ctx.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: ctx.Err().Error()})
			return
		case <-s.process.Done():
			stderr := s.process.Stderr()
			errText := "persistent process exited"
			if stderr != "" {
				errText = stderr
			}
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText, StaleSession: claude.IsStaleSessionError(errText)})
			return
		case line, ok := <-s.process.StdoutLines():
			if !ok {
				if text := textBuf.String(); text != "" {
					s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text})
				} else {
					s.emit(runtime.Event{Type: runtime.EventFailed, Error: "stdout closed"})
				}
				return
			}
			terminal, inputReq := s.handleLine(line, &textBuf)
			if inputReq != nil {
				if s.waitForInputResponse(ctx, inputReq) {
					return
				}
				continue
			}
			if terminal {
				return
			}
		}
	}
}

func (s *persistentSession) handleLine(line []byte, textBuf *strings.Builder) (bool, *runtime.InputRequest) {
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		text := strings.TrimSpace(string(line))
		if text != "" {
			textBuf.WriteString(text)
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

	case "assistant":
		text, thinking, process := claude.AssistantContent(payload)
		if text == "" && thinking == "" && len(process) == 0 {
			return false, nil
		}
		if text != "" {
			textBuf.WriteString(text)
		}
		s.emit(runtime.Event{Type: runtime.EventDelta, Text: text, Thinking: thinking, Process: process})
		return false, nil

	case "result":
		if claude.IsErrorResult(payload) {
			errText := claude.ResultError(payload)
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText, StaleSession: claude.IsStaleSessionError(errText)})
			return true, nil
		}
		text := claude.StringValue(payload, "result")
		if text == "" {
			text = textBuf.String()
		}
		s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text, Usage: claude.ClaudeUsage(payload)})
		return true, nil

	case "control_request":
		if inputReq := s.handleControlRequest(payload); inputReq != nil {
			return false, inputReq
		}
		return false, nil

	default:
		return false, nil
	}
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
		stderr := s.process.Stderr()
		errText := "persistent process exited"
		if stderr != "" {
			errText = stderr
		}
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
	select {
	case <-s.done:
	case s.events <- evt:
	}
}

func (s *persistentSession) emitFailed(errText string) {
	s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText})
	s.mu.Lock()
	s.alive = false
	s.mu.Unlock()
	s.closeOnce.Do(func() {
		close(s.done)
		close(s.events)
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

	// Check if this is an AskUserQuestion permission request
	request, _ := payload["request"].(map[string]any)
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
