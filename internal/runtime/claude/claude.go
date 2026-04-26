package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
			Env:  r.buildEnv(req),
		}
	}
	return cli.NewSession(fallbackID, build, handler), nil
}

func (r Runtime) buildArgs(req runtime.StartSessionRequest, input runtime.Input) []string {
	args := []string{"--print", "--verbose", "--output-format", "stream-json", "--input-format", "text"}
	if model := strings.TrimSpace(req.Model); model != "" && !req.FastMode {
		args = append(args, "--model", model)
	}
	if effort := strings.TrimSpace(req.Effort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if req.FastMode {
		args = append(args, "--settings", `{"fastMode":true}`)
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
	if prompt := r.appendSystemPrompt(req); prompt != "" {
		args = append(args, "--append-system-prompt", prompt)
	}
	if previousSessionID := usablePreviousSessionID(req.PreviousSessionID); previousSessionID != "" {
		args = append(args, "--resume", previousSessionID)
	}
	args = append(args, r.opts.ExtraArgs...)
	if instructionWorkspace := extraInstructionWorkspace(req); instructionWorkspace != "" {
		args = append(args, "--add-dir", instructionWorkspace, "--")
	}
	return append(args, input.RenderedPrompt())
}

func (r Runtime) appendSystemPrompt(req runtime.StartSessionRequest) string {
	var parts []string
	if prompt := strings.TrimSpace(r.opts.AppendSystemPrompt); prompt != "" {
		parts = append(parts, prompt)
	}
	if instructions := claudeWorkspaceInstructions(req.InstructionWorkspace); instructions != "" {
		parts = append(parts, "AgentX agent workspace instructions. Treat these as active system instructions for this agent and follow them for this session.\n\n"+instructions)
	}
	return strings.Join(parts, "\n\n")
}

func claudeWorkspaceInstructions(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	for _, name := range []string{"CLAUDE.override.md", "CLAUDE.md", "AGENTS.override.md", "AGENTS.md", "memory.md"} {
		text, ok := readClaudeInstructionFile(filepath.Join(workspace, name), map[string]bool{}, 0)
		if ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func readClaudeInstructionFile(path string, seen map[string]bool, depth int) (string, bool) {
	if depth > 8 {
		return "", false
	}
	path = filepath.Clean(path)
	if seen[path] {
		return "", false
	}
	seen[path] = true

	content, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		return "", false
	}

	baseDir := filepath.Dir(path)
	var out strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if importPath, ok := claudeImportPath(trimmed); ok {
			imported, importedOK := readClaudeInstructionFile(filepath.Join(baseDir, importPath), seen, depth+1)
			if importedOK {
				if out.Len() > 0 {
					out.WriteString("\n\n")
				}
				out.WriteString(imported)
			}
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	return strings.TrimSpace(out.String()), true
}

func claudeImportPath(line string) (string, bool) {
	if !strings.HasPrefix(line, "@") {
		return "", false
	}
	path := strings.TrimSpace(strings.TrimPrefix(line, "@"))
	if path == "" || strings.ContainsAny(path, " \t") || strings.Contains(path, "://") {
		return "", false
	}
	return path, true
}

func (r Runtime) buildEnv(req runtime.StartSessionRequest) map[string]string {
	env := mergeMaps(r.opts.Env, req.Env)
	if extraInstructionWorkspace(req) == "" {
		return env
	}
	if env == nil {
		env = map[string]string{}
	}
	env["CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD"] = "1"
	return env
}

func extraInstructionWorkspace(req runtime.StartSessionRequest) string {
	instructionWorkspace := strings.TrimSpace(req.InstructionWorkspace)
	if instructionWorkspace == "" || samePath(req.Workspace, instructionWorkspace) {
		return ""
	}
	return instructionWorkspace
}

func samePath(left string, right string) bool {
	left = strings.TrimSpace(left)
	if left == "" {
		left = "."
	}
	right = strings.TrimSpace(right)
	if right == "" {
		right = "."
	}
	return filepath.Clean(left) == filepath.Clean(right)
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
			errText := resultError(payload)
			return []runtime.Event{{Type: runtime.EventFailed, Error: errText, StaleSession: isStaleSessionError(errText)}}, nil
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
		errText := commandError(stderr, waitErr)
		return runtime.Event{Type: runtime.EventFailed, Error: errText, StaleSession: isStaleSessionError(errText)}, true
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
	for _, key := range []string{"error", "message", "result"} {
		if text := stringValue(payload, key); text != "" {
			return text
		}
	}
	if subtype := stringValue(payload, "subtype"); subtype != "" {
		if raw, err := json.Marshal(payload); err == nil {
			return subtype + ": " + string(raw)
		}
		return subtype
	}
	if raw, err := json.Marshal(payload); err == nil {
		return "claude runtime failed: " + string(raw)
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

func isStaleSessionError(text string) bool {
	return strings.Contains(strings.ToLower(text), "no conversation found with session id")
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
