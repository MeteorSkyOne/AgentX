package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/cli"
)

type Options struct {
	Command            string
	PermissionMode     string
	AllowedTools       []string
	DisallowedTools    []string
	AppendSystemPrompt string
	ExtraArgs          []string
	Env                map[string]string
}

type Runtime struct {
	opts Options
}

func New(opts Options) runtime.Runtime {
	if strings.TrimSpace(opts.Command) == "" {
		opts.Command = "claude"
	}
	if strings.TrimSpace(opts.PermissionMode) == "" {
		opts.PermissionMode = "acceptEdits"
	}
	return Runtime{opts: opts}
}

func (r Runtime) StartSession(ctx context.Context, req runtime.StartSessionRequest) (runtime.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		workspace = "."
	}
	fallbackID := "claude:" + req.SessionKey
	if req.SessionKey == "" {
		fallbackID = "claude:" + req.AgentID
	}
	handler := newLineHandler(fallbackID)
	build := func(input runtime.Input) cli.Command {
		return cli.Command{
			Name: r.opts.Command,
			Args: r.buildArgs(req, input),
			Dir:  workspace,
			Env:  mergeMaps(r.opts.Env, req.Env),
		}
	}
	return cli.NewSession(fallbackID, build, handler), nil
}

func (r Runtime) buildArgs(req runtime.StartSessionRequest, input runtime.Input) []string {
	args := []string{"--print", "--verbose", "--output-format", "stream-json", "--input-format", "text"}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := strings.TrimSpace(req.Effort); effort != "" {
		args = append(args, "--effort", effort)
	}
	mode := strings.TrimSpace(r.opts.PermissionMode)
	if override := strings.TrimSpace(req.PermissionMode); override != "" {
		mode = override
	} else if req.YoloMode {
		mode = "bypassPermissions"
	}
	if mode != "" {
		args = append(args, "--permission-mode", mode)
	}
	if tools := strings.Join(r.opts.AllowedTools, ","); tools != "" {
		args = append(args, "--allowedTools", tools)
	}
	if tools := strings.Join(r.opts.DisallowedTools, ","); tools != "" {
		args = append(args, "--disallowedTools", tools)
	}
	if prompt := strings.TrimSpace(r.opts.AppendSystemPrompt); prompt != "" {
		args = append(args, "--append-system-prompt", prompt)
	}
	if previousSessionID := usablePreviousSessionID(req.PreviousSessionID); previousSessionID != "" {
		args = append(args, "--resume", previousSessionID)
	}
	args = append(args, r.opts.ExtraArgs...)
	return append(args, input.RenderedPrompt())
}

func usablePreviousSessionID(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "claude:") {
		return ""
	}
	return id
}

type lineHandler struct {
	mu        sync.Mutex
	sessionID string
	finalText strings.Builder
}

func newLineHandler(fallbackID string) *lineHandler {
	return &lineHandler{sessionID: fallbackID}
}

func (h *lineHandler) HandleLine(line []byte) ([]runtime.Event, error) {
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		text := strings.TrimSpace(string(line))
		if text == "" {
			return nil, nil
		}
		h.appendText(text)
		return []runtime.Event{{Type: runtime.EventDelta, Text: text}}, nil
	}

	h.setSessionID(stringValue(payload, "session_id"))
	switch stringValue(payload, "type") {
	case "assistant", "user":
		text, thinking, process := assistantContent(payload)
		if stringValue(payload, "type") != "assistant" {
			text = ""
		}
		if text == "" && thinking == "" && len(process) == 0 {
			return nil, nil
		}
		if text != "" {
			h.appendText(text)
		}
		return []runtime.Event{{Type: runtime.EventDelta, Text: text, Thinking: thinking, Process: process}}, nil
	case "result":
		if isErrorResult(payload) {
			return []runtime.Event{{Type: runtime.EventFailed, Error: resultError(payload)}}, nil
		}
		text := stringValue(payload, "result")
		if text == "" {
			text = h.text()
		}
		return []runtime.Event{{Type: runtime.EventCompleted, Text: text}}, nil
	default:
		return nil, nil
	}
}

func (h *lineHandler) Finish(stderr string, waitErr error) (runtime.Event, bool) {
	if waitErr != nil {
		return runtime.Event{Type: runtime.EventFailed, Error: commandError(stderr, waitErr)}, true
	}
	return runtime.Event{Type: runtime.EventCompleted, Text: h.text()}, true
}

func (h *lineHandler) CurrentSessionID() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessionID
}

func (h *lineHandler) setSessionID(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	h.mu.Lock()
	h.sessionID = id
	h.mu.Unlock()
}

func (h *lineHandler) appendText(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.finalText.Len() > 0 {
		h.finalText.WriteByte('\n')
	}
	h.finalText.WriteString(text)
}

func (h *lineHandler) text() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finalText.String()
}

func assistantText(payload map[string]any) string {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	var out strings.Builder
	for _, item := range content {
		part, _ := item.(map[string]any)
		if stringValue(part, "type") != "text" {
			continue
		}
		text := stringValue(part, "text")
		if text == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(text)
	}
	return out.String()
}

func assistantThinking(payload map[string]any) string {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	var out strings.Builder
	for _, item := range content {
		part, _ := item.(map[string]any)
		if stringValue(part, "type") != "thinking" {
			continue
		}
		text := stringValue(part, "thinking")
		if text == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(text)
	}
	return out.String()
}

func assistantContent(payload map[string]any) (string, string, []runtime.ProcessItem) {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	var textOut strings.Builder
	var thinkingOut strings.Builder
	var process []runtime.ProcessItem
	for _, item := range content {
		part, _ := item.(map[string]any)
		switch stringValue(part, "type") {
		case "text":
			text := stringValue(part, "text")
			if text == "" {
				continue
			}
			if textOut.Len() > 0 {
				textOut.WriteByte('\n')
			}
			textOut.WriteString(text)
		case "thinking":
			text := claudeThinkingText(part)
			if text == "" {
				continue
			}
			if thinkingOut.Len() > 0 {
				thinkingOut.WriteByte('\n')
			}
			thinkingOut.WriteString(text)
			process = append(process, runtime.ProcessItem{
				Type: "thinking",
				Text: text,
				Raw:  part,
			})
		case "tool_use":
			process = append(process, runtime.ProcessItem{
				Type:       "tool_call",
				Text:       stringValue(part, "text"),
				ToolName:   stringValue(part, "name"),
				ToolCallID: stringValue(part, "id"),
				Status:     stringValue(part, "status"),
				Input:      valueOrNil(part, "input"),
				Raw:        part,
			})
		case "tool_result":
			process = append(process, runtime.ProcessItem{
				Type:       "tool_result",
				Text:       stringValue(part, "text"),
				ToolCallID: claudeToolResultCallID(part),
				Status:     claudeToolResultStatus(part),
				Output:     claudeToolResultOutput(part),
				Raw:        part,
			})
		}
	}
	return textOut.String(), thinkingOut.String(), process
}

func claudeThinkingText(part map[string]any) string {
	if text := stringValue(part, "thinking"); text != "" {
		return text
	}
	return stringValue(part, "text")
}

func claudeToolResultCallID(part map[string]any) string {
	for _, key := range []string{"tool_use_id", "tool_call_id", "id"} {
		if text := stringValue(part, key); text != "" {
			return text
		}
	}
	return ""
}

func claudeToolResultStatus(part map[string]any) string {
	if isError, ok := part["is_error"].(bool); ok && isError {
		return "error"
	}
	return stringValue(part, "status")
}

func claudeToolResultOutput(part map[string]any) any {
	if output, ok := firstPresent(part, "content", "result", "output"); ok {
		return output
	}
	return nil
}

func firstPresent(values map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func valueOrNil(values map[string]any, key string) any {
	if value, ok := values[key]; ok {
		return value
	}
	return nil
}

func isErrorResult(payload map[string]any) bool {
	if isError, ok := payload["is_error"].(bool); ok && isError {
		return true
	}
	subtype := stringValue(payload, "subtype")
	return subtype != "" && subtype != "success"
}

func resultError(payload map[string]any) string {
	for _, key := range []string{"error", "message", "result", "subtype"} {
		if text := stringValue(payload, key); text != "" {
			return text
		}
	}
	return "claude runtime failed"
}

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func commandError(stderr string, waitErr error) string {
	if strings.TrimSpace(stderr) != "" {
		return stderr
	}
	if waitErr != nil {
		return waitErr.Error()
	}
	return "claude runtime failed"
}

func mergeMaps(first map[string]string, second map[string]string) map[string]string {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	merged := make(map[string]string, len(first)+len(second))
	for key, value := range first {
		merged[key] = value
	}
	for key, value := range second {
		merged[key] = value
	}
	return merged
}
