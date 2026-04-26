package codex

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
	Command          string
	FullAuto         bool
	BypassSandbox    bool
	SkipGitRepoCheck bool
	ExtraArgs        []string
	Env              map[string]string
}

type Runtime struct {
	opts Options
}

func New(opts Options) runtime.Runtime {
	if strings.TrimSpace(opts.Command) == "" {
		opts.Command = "codex"
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
	fallbackID := "codex:" + req.SessionKey
	if req.SessionKey == "" {
		fallbackID = "codex:" + req.AgentID
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
	args := []string{"exec"}
	previousSessionID := usablePreviousSessionID(req.PreviousSessionID)
	if previousSessionID != "" {
		args = append(args, "resume")
	}

	args = append(args, "--json")
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if r.opts.BypassSandbox || req.YoloMode {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else if r.opts.FullAuto {
		args = append(args, "--full-auto")
	}
	if r.opts.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	args = append(args, r.opts.ExtraArgs...)
	if previousSessionID != "" {
		args = append(args, previousSessionID)
	}
	return append(args, input.RenderedPrompt())
}

func usablePreviousSessionID(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "codex:") {
		return ""
	}
	return id
}

type lineHandler struct {
	mu        sync.Mutex
	sessionID string
	finalText strings.Builder
	pending   []string
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

	switch stringValue(payload, "type") {
	case "thread.started":
		h.setSessionID(stringValue(payload, "thread_id"))
	case "turn.started":
		h.clearPending()
	case "item.started":
		item, _ := payload["item"].(map[string]any)
		return h.handleItemStarted(item)
	case "item.completed":
		item, _ := payload["item"].(map[string]any)
		return h.handleItemCompleted(item)
	case "response_item":
		item, _ := payload["payload"].(map[string]any)
		return h.handleItem(item)
	case "event_msg":
		item, _ := payload["payload"].(map[string]any)
		return h.handleEventMessage(item)
	case "turn.completed":
		h.flushPendingAsText()
		return []runtime.Event{{Type: runtime.EventCompleted, Text: h.text()}}, nil
	case "turn.failed":
		return []runtime.Event{{Type: runtime.EventFailed, Error: errorText(payload)}}, nil
	case "error":
		return []runtime.Event{{Type: runtime.EventFailed, Error: errorText(payload)}}, nil
	}
	return nil, nil
}

func (h *lineHandler) handleItemStarted(item map[string]any) ([]runtime.Event, error) {
	itemType := normalizedType(stringValue(item, "type"))
	switch itemType {
	case "", "agent_message", "message", "reasoning", "user_message", "plan", "hook_prompt", "context_compaction":
		return nil, nil
	}

	events := h.flushPendingAsThinking()
	if process := codexStartedToolProcessItems(item); len(process) > 0 {
		events = append(events, runtime.Event{Type: runtime.EventDelta, Process: process})
	}
	return events, nil
}

func (h *lineHandler) handleItemCompleted(item map[string]any) ([]runtime.Event, error) {
	switch normalizedType(stringValue(item, "type")) {
	case "agent_message", "message":
		text := codexMessageText(item)
		if text != "" {
			h.appendPending(text)
		}
		return nil, nil
	default:
		return h.handleItem(item)
	}
}

func (h *lineHandler) handleItem(item map[string]any) ([]runtime.Event, error) {
	switch normalizedType(stringValue(item, "type")) {
	case "agent_message":
		text := codexMessageText(item)
		if text != "" {
			h.appendText(text)
			return []runtime.Event{{Type: runtime.EventDelta, Text: text}}, nil
		}
	case "message":
		if role := stringValue(item, "role"); role != "" && role != "assistant" {
			return nil, nil
		}
		text := codexMessageText(item)
		if text != "" {
			h.appendText(text)
			return []runtime.Event{{Type: runtime.EventDelta, Text: text}}, nil
		}
	case "reasoning":
		text := reasoningSummaryText(item)
		if text != "" {
			return []runtime.Event{{Type: runtime.EventDelta, Thinking: text, Process: []runtime.ProcessItem{{
				Type: "thinking",
				Text: text,
				Raw:  item,
			}}}}, nil
		}
	default:
		if process := codexToolProcessItems(item); len(process) > 0 {
			return []runtime.Event{{Type: runtime.EventDelta, Process: process}}, nil
		}
	}
	return nil, nil
}

func (h *lineHandler) handleEventMessage(item map[string]any) ([]runtime.Event, error) {
	switch normalizedType(stringValue(item, "type")) {
	case "agent_message", "agent_message_delta":
		text := firstText(item, "message", "text", "delta")
		if text != "" {
			h.appendText(text)
			return []runtime.Event{{Type: runtime.EventDelta, Text: text}}, nil
		}
	case "agent_reasoning", "agent_reasoning_delta":
		text := firstText(item, "text", "message", "delta")
		if text != "" {
			return []runtime.Event{{Type: runtime.EventDelta, Thinking: text, Process: []runtime.ProcessItem{{
				Type: "thinking",
				Text: text,
				Raw:  item,
			}}}}, nil
		}
	default:
		if process := codexToolProcessItems(item); len(process) > 0 {
			return []runtime.Event{{Type: runtime.EventDelta, Process: process}}, nil
		}
	}
	return nil, nil
}

func (h *lineHandler) Finish(stderr string, waitErr error) (runtime.Event, bool) {
	if waitErr != nil {
		return runtime.Event{Type: runtime.EventFailed, Error: commandError(stderr, waitErr)}, true
	}
	h.flushPendingAsText()
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

func (h *lineHandler) appendPending(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending = append(h.pending, text)
}

func (h *lineHandler) clearPending() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending = h.pending[:0]
}

func (h *lineHandler) flushPendingAsThinking() []runtime.Event {
	h.mu.Lock()
	pending := append([]string(nil), h.pending...)
	h.pending = h.pending[:0]
	h.mu.Unlock()

	events := make([]runtime.Event, 0, len(pending))
	for _, text := range pending {
		if strings.TrimSpace(text) == "" {
			continue
		}
		events = append(events, runtime.Event{Type: runtime.EventDelta, Thinking: text, Process: []runtime.ProcessItem{{
			Type: "thinking",
			Text: text,
		}}})
	}
	return events
}

func (h *lineHandler) flushPendingAsText() {
	h.mu.Lock()
	pending := append([]string(nil), h.pending...)
	h.pending = h.pending[:0]
	h.mu.Unlock()

	for _, text := range pending {
		if strings.TrimSpace(text) != "" {
			h.appendText(text)
		}
	}
}

func (h *lineHandler) text() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finalText.String()
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

func firstText(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := stringValue(values, key); text != "" {
			return text
		}
	}
	return ""
}

func reasoningSummaryText(item map[string]any) string {
	arr, _ := item["summary"].([]any)
	var out strings.Builder
	for _, elem := range arr {
		if text, ok := elem.(string); ok {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(text)
			continue
		}
		part, _ := elem.(map[string]any)
		if stringValue(part, "type") != "summary_text" {
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
	if out.Len() > 0 {
		return out.String()
	}
	return stringValue(item, "text")
}

func codexMessageText(item map[string]any) string {
	content, _ := item["content"].([]any)
	var out strings.Builder
	for _, elem := range content {
		part, _ := elem.(map[string]any)
		switch normalizedType(stringValue(part, "type")) {
		case "output_text", "text":
			text := stringValue(part, "text")
			if text == "" {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(text)
		}
	}
	if out.Len() > 0 {
		return out.String()
	}
	return firstText(item, "text", "message")
}

func codexStartedToolProcessItems(item map[string]any) []runtime.ProcessItem {
	itemType := normalizedType(stringValue(item, "type"))
	switch itemType {
	case "command_execution", "commandexecution", "function_call", "functioncall", "web_search", "websearch", "file_search", "filesearch", "mcp_tool_call", "mcptoolcall", "dynamic_tool_call", "dynamictoolcall", "file_change", "filechange":
		return []runtime.ProcessItem{codexToolCallItem(item)}
	default:
		return nil
	}
}

func codexToolProcessItems(item map[string]any) []runtime.ProcessItem {
	itemType := normalizedType(stringValue(item, "type"))
	if isCodexToolResultType(itemType) {
		return []runtime.ProcessItem{codexToolResultItem(item)}
	}
	if !isCodexToolCallType(itemType) {
		return nil
	}

	call := codexToolCallItem(item)
	items := []runtime.ProcessItem{call}
	if output, ok := firstPresent(item, "output", "result", "content", "contentItems", "aggregated_output", "aggregatedOutput", "formatted_output", "formattedOutput", "stdout", "stderr"); ok {
		result := codexToolResultItem(item)
		result.Output = output
		items = append(items, result)
	}
	return items
}

func codexToolCallItem(item map[string]any) runtime.ProcessItem {
	return runtime.ProcessItem{
		Type:       "tool_call",
		Text:       codexToolDisplayText(item),
		ToolName:   codexToolName(item),
		ToolCallID: codexToolCallID(item),
		Status:     stringValue(item, "status"),
		Input:      codexToolInput(item),
		Raw:        item,
	}
}

func codexToolResultItem(item map[string]any) runtime.ProcessItem {
	output, _ := firstPresent(item, "output", "result", "content", "contentItems", "aggregated_output", "aggregatedOutput", "formatted_output", "formattedOutput", "stdout", "stderr")
	return runtime.ProcessItem{
		Type:       "tool_result",
		Text:       codexToolDisplayText(item),
		ToolName:   codexToolName(item),
		ToolCallID: codexToolCallID(item),
		Status:     codexToolResultStatus(item),
		Input:      codexToolInput(item),
		Output:     output,
		Raw:        item,
	}
}

func isCodexToolCallType(itemType string) bool {
	switch itemType {
	case "command_execution", "commandexecution", "dynamic_tool_call", "dynamictoolcall", "file_change", "filechange", "web_search", "websearch", "file_search", "filesearch", "mcptoolcall", "mcp_tool_call", "mcp_tool_call_begin", "collabtoolcall", "collab_tool_call", "collab_tool_call_begin", "toolcall", "tool_call", "tool_call_begin", "functioncall", "function_call", "exec_command_begin", "patch_apply_begin":
		return true
	default:
		return false
	}
}

func isCodexToolResultType(itemType string) bool {
	switch itemType {
	case "command_execution", "commandexecution", "dynamic_tool_call", "dynamictoolcall", "file_change", "filechange", "web_search", "websearch", "file_search", "filesearch", "mcptoolresult", "mcp_tool_result", "mcp_tool_call_end", "mcp_tool_call_output", "collabtoolresult", "collab_tool_result", "collab_tool_call_end", "toolresult", "tool_result", "tool_call_end", "toolcalloutput", "tool_call_output", "functioncalloutput", "function_call_output", "custom_tool_call_output", "exec_command_end", "patch_apply_end":
		return true
	default:
		return false
	}
}

func normalizedType(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "-", "_")
	return strings.ToLower(value)
}

func codexToolName(item map[string]any) string {
	switch normalizedType(stringValue(item, "type")) {
	case "command_execution", "commandexecution", "exec_command_begin", "exec_command_end":
		return "Bash"
	case "web_search", "websearch":
		return "WebSearch"
	case "file_search", "filesearch":
		return "FileSearch"
	case "file_change", "filechange", "patch_apply_begin", "patch_apply_end":
		return "Patch"
	case "mcp_tool_call", "mcptoolcall", "mcp_tool_call_begin", "mcp_tool_call_end", "mcp_tool_call_output":
		if tool := stringValue(item, "tool"); tool != "" {
			if server := stringValue(item, "server"); server != "" {
				return server + "." + tool
			}
			return tool
		}
		if name := stringValue(item, "name"); name != "" {
			return name
		}
		return "MCP"
	case "dynamic_tool_call", "dynamictoolcall":
		if tool := stringValue(item, "tool"); tool != "" {
			return tool
		}
		return "Tool"
	}
	switch normalizedType(stringValue(item, "type")) {
	case "exec_command_begin", "exec_command_end":
		return "exec_command"
	case "patch_apply_begin", "patch_apply_end":
		return "apply_patch"
	}
	for _, key := range []string{"tool_name", "toolName", "name", "tool", "command"} {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			if name := stringValue(nested, "name"); name != "" {
				return name
			}
			continue
		}
		if text := stringValue(item, key); text != "" {
			if server := stringValue(item, "server"); server != "" && key == "tool" {
				return server + "." + text
			}
			return text
		}
	}
	return ""
}

func codexToolCallID(item map[string]any) string {
	for _, key := range []string{"tool_call_id", "toolCallId", "call_id", "callId", "id", "item_id", "itemId"} {
		if text := stringValue(item, key); text != "" {
			return text
		}
	}
	return ""
}

func codexToolInput(item map[string]any) any {
	if input, ok := firstPresent(item, "input", "arguments", "args", "params", "parameters", "changes"); ok {
		return decodeJSONValue(input)
	}
	if command, ok := firstPresent(item, "command", "cmd"); ok {
		values := map[string]any{"command": command}
		if cwd := stringValue(item, "cwd"); cwd != "" {
			values["cwd"] = cwd
		}
		return values
	}
	if query, ok := firstPresent(item, "query"); ok {
		return map[string]any{"query": query}
	}
	if action, ok := item["action"].(map[string]any); ok {
		return action
	}
	return nil
}

func codexToolResultStatus(item map[string]any) string {
	if status := stringValue(item, "status"); status != "" {
		return status
	}
	if isError, ok := item["is_error"].(bool); ok && isError {
		return "error"
	}
	if isError, ok := item["error"].(bool); ok && isError {
		return "error"
	}
	return ""
}

func codexToolDisplayText(item map[string]any) string {
	for _, key := range []string{"text", "summary", "message", "formatted_output", "formattedOutput"} {
		if text := stringValue(item, key); text != "" {
			return text
		}
	}
	return ""
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

func decodeJSONValue(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return value
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return value
	}
	return decoded
}

func errorText(payload map[string]any) string {
	for _, key := range []string{"message", "error"} {
		if text := stringValue(payload, key); text != "" {
			return text
		}
	}
	return "codex runtime failed"
}

func commandError(stderr string, waitErr error) string {
	if strings.TrimSpace(stderr) != "" {
		return stderr
	}
	if waitErr != nil {
		return waitErr.Error()
	}
	return "codex runtime failed"
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
