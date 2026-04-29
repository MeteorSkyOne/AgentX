package codexpersist

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

type persistentSession struct {
	process  *procpool.ManagedProcess
	rpc      *rpcClient
	key      string
	rt       *Runtime
	req      runtime.StartSessionRequest
	events   chan runtime.Event
	threadID string

	mu        sync.Mutex
	sessionID string
	alive     bool
	started   bool
	turnHeld  bool
	done      chan struct{}
	closeOnce sync.Once
}

func newPersistentSession(proc *procpool.ManagedProcess, rpc *rpcClient, key string, rt *Runtime, req runtime.StartSessionRequest) *persistentSession {
	fallbackID := "codex:" + key
	threadID := usablePreviousSessionID(req.PreviousSessionID)
	return &persistentSession{
		process:   proc,
		rpc:       rpc,
		key:       key,
		rt:        rt,
		req:       req,
		events:    make(chan runtime.Event, 64),
		threadID:  threadID,
		sessionID: fallbackID,
		alive:     true,
		done:      make(chan struct{}),
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

	go s.runTurn(ctx, input)
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

func (s *persistentSession) runTurn(ctx context.Context, input runtime.Input) {
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

	if s.threadID == "" {
		threadID, err := s.startThread(ctx)
		if err != nil {
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: err.Error()})
			return
		}
		s.threadID = threadID
		s.mu.Lock()
		s.sessionID = threadID
		s.mu.Unlock()
	}

	userInput := buildUserInput(input)
	turnParams := map[string]any{
		"threadId": s.threadID,
		"input":    userInput,
	}
	if model := strings.TrimSpace(s.req.Model); model != "" {
		turnParams["model"] = model
	}
	if effort := strings.TrimSpace(s.req.Effort); effort != "" {
		turnParams["effort"] = effort
	}

	result, err := s.rpc.Call(ctx, "turn/start", turnParams)
	if err != nil {
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: fmt.Sprintf("turn/start failed: %v", err)})
		return
	}
	_ = result

	s.processNotifications(ctx)
}

func (s *persistentSession) startThread(ctx context.Context) (string, error) {
	workspace := strings.TrimSpace(s.req.Workspace)
	if workspace == "" {
		workspace = "."
	}

	params := map[string]any{
		"cwd": workspace,
	}
	if model := strings.TrimSpace(s.req.Model); model != "" {
		params["model"] = model
	}
	if s.req.YoloMode {
		params["approvalPolicy"] = "never"
	}

	result, err := s.rpc.Call(ctx, "thread/start", params)
	if err != nil {
		return "", fmt.Errorf("thread/start: %w", err)
	}

	thread, _ := result["thread"].(map[string]any)
	if thread == nil {
		return "", fmt.Errorf("thread/start: missing thread in response")
	}
	threadID, _ := thread["id"].(string)
	if threadID == "" {
		return "", fmt.Errorf("thread/start: missing thread id")
	}
	return threadID, nil
}

func (s *persistentSession) processNotifications(ctx context.Context) {
	var textBuf strings.Builder
	for {
		select {
		case <-ctx.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: ctx.Err().Error()})
			return
		case <-s.process.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: "codex app-server exited"})
			return
		case msg, ok := <-s.rpc.Notifications():
			if !ok {
				if text := textBuf.String(); text != "" {
					s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text})
				} else {
					s.emit(runtime.Event{Type: runtime.EventFailed, Error: "notification channel closed"})
				}
				return
			}

			if msg.ID != nil {
				s.handleServerRequest(msg)
				continue
			}

			terminal := s.handleNotification(msg, &textBuf)
			if terminal {
				return
			}
		}
	}
}

func (s *persistentSession) handleNotification(msg jsonRPCMessage, textBuf *strings.Builder) bool {
	params := notificationParams(msg)

	switch msg.Method {
	case "item/agentMessage/delta":
		delta, _ := params["delta"].(string)
		if delta != "" {
			textBuf.WriteString(delta)
			s.emit(runtime.Event{Type: runtime.EventDelta, Text: delta})
		}

	case "item/reasoning/textDelta":
		delta, _ := params["delta"].(string)
		if delta != "" {
			s.emit(runtime.Event{Type: runtime.EventDelta, Thinking: delta})
		}

	case "item/started":
		item, _ := params["item"].(map[string]any)
		if pi := itemToProcessItem(item, "started"); pi != nil {
			s.emit(runtime.Event{Type: runtime.EventDelta, Process: []runtime.ProcessItem{*pi}})
		}

	case "item/completed":
		item, _ := params["item"].(map[string]any)
		if pi := itemToProcessItem(item, "completed"); pi != nil {
			s.emit(runtime.Event{Type: runtime.EventDelta, Process: []runtime.ProcessItem{*pi}})
		}

	case "turn/completed":
		text := textBuf.String()
		usage := turnCompletedUsage(params)
		s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text, Usage: usage})
		return true

	case "thread/tokenUsage/updated":
		// token usage updates are captured in turn/completed

	case "error":
		errMsg, _ := params["message"].(string)
		if errMsg == "" {
			errMsg = "codex app-server error"
		}
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: errMsg})
		return true

	case "thread/closed":
		text := textBuf.String()
		if text != "" {
			s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text})
		} else {
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: "thread closed"})
		}
		return true
	}
	return false
}

func (s *persistentSession) handleServerRequest(msg jsonRPCMessage) {
	switch msg.Method {
	case "item/commandExecution/requestApproval",
		"item/fileChange/requestApproval",
		"item/permissions/requestApproval",
		"execCommandApproval",
		"applyPatchApproval":
		if err := s.rpc.RespondToRequest(msg.ID, map[string]any{"decision": "accept"}); err != nil {
			slog.Warn("codexpersist: failed to auto-approve", "method", msg.Method, "error", err)
		}
	case "item/tool/requestUserInput":
		if err := s.rpc.RespondToRequest(msg.ID, map[string]any{"input": ""}); err != nil {
			slog.Warn("codexpersist: failed to respond to user input request", "error", err)
		}
	default:
		if err := s.rpc.RespondToRequest(msg.ID, map[string]any{}); err != nil {
			slog.Warn("codexpersist: failed to respond to server request", "method", msg.Method, "error", err)
		}
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

func buildUserInput(input runtime.Input) []map[string]any {
	items := []map[string]any{
		{"type": "text", "text": input.RenderedPrompt()},
	}
	for _, att := range input.Attachments {
		if att.Kind == "image" || strings.HasPrefix(strings.ToLower(att.ContentType), "image/") {
			if att.LocalPath != "" {
				items = append(items, map[string]any{"type": "localImage", "path": att.LocalPath})
			}
		}
	}
	return items
}

func usablePreviousSessionID(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "codex:") {
		return ""
	}
	return id
}

func notificationParams(msg jsonRPCMessage) map[string]any {
	if msg.Params == nil {
		return nil
	}
	switch p := msg.Params.(type) {
	case map[string]any:
		return p
	default:
		data, err := json.Marshal(msg.Params)
		if err != nil {
			return nil
		}
		var result map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			return nil
		}
		return result
	}
}

func itemToProcessItem(item map[string]any, status string) *runtime.ProcessItem {
	if item == nil {
		return nil
	}
	itemType, _ := item["type"].(string)
	switch itemType {
	case "function_call", "command_execution", "file_change", "mcp_tool_call", "web_search":
		pi := &runtime.ProcessItem{
			Type:       "tool_call",
			ToolName:   itemToolName(item),
			ToolCallID: stringVal(item, "id"),
			Status:     status,
			Raw:        item,
		}
		if input, ok := item["input"]; ok {
			pi.Input = input
		}
		if output, ok := item["output"]; ok && status == "completed" {
			pi.Output = output
		}
		return pi
	case "message", "agent_message":
		return nil
	case "reasoning":
		text := stringVal(item, "text")
		if text == "" {
			if summary, ok := item["summary"].([]any); ok && len(summary) > 0 {
				if first, ok := summary[0].(map[string]any); ok {
					text = stringVal(first, "text")
				}
			}
		}
		if text == "" {
			return nil
		}
		return &runtime.ProcessItem{Type: "thinking", Text: text, Raw: item}
	}
	return nil
}

func itemToolName(item map[string]any) string {
	if name := stringVal(item, "name"); name != "" {
		return name
	}
	if name := stringVal(item, "toolName"); name != "" {
		return name
	}
	return stringVal(item, "type")
}

func stringVal(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func turnCompletedUsage(params map[string]any) *runtime.Usage {
	turn, _ := params["turn"].(map[string]any)
	if turn == nil {
		return nil
	}
	usage, _ := turn["tokenUsage"].(map[string]any)
	if usage == nil {
		return nil
	}
	last, _ := usage["last"].(map[string]any)
	if last == nil {
		return nil
	}
	u := &runtime.Usage{
		Model:       stringVal(turn, "model"),
		InputTokens: int64Ptr(last, "inputTokens"),
		OutputTokens: int64Ptr(last, "outputTokens"),
		CachedInputTokens:     int64Ptr(last, "cachedInputTokens"),
		ReasoningOutputTokens: int64Ptr(last, "reasoningOutputTokens"),
		TotalTokens:           int64Ptr(last, "totalTokens"),
	}
	return u
}

func int64Ptr(m map[string]any, key string) *int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		i := int64(n)
		return &i
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return nil
		}
		return &i
	}
	return nil
}
