package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestBuildArgsStartsAndResumesClaudePrint(t *testing.T) {
	rt := Runtime{opts: Options{
		PermissionMode:     "acceptEdits",
		AllowedTools:       []string{"Read", "Bash"},
		DisallowedTools:    []string{"WebSearch"},
		AppendSystemPrompt: "be brief",
	}}
	req := runtime.StartSessionRequest{Model: "sonnet"}
	args := rt.buildArgs(req, runtime.Input{Prompt: "hello"})
	want := []string{
		"--print", "--verbose", "--output-format", "stream-json", "--input-format", "text",
		"--model", "sonnet", "--permission-mode", "acceptEdits",
		"--allowedTools", "Read,Bash", "--disallowedTools", "WebSearch",
		"--append-system-prompt", "be brief", "hello",
	}
	assertArgs(t, args, want)

	req.PreviousSessionID = "123e4567-e89b-12d3-a456-426614174000"
	args = rt.buildArgs(req, runtime.Input{Prompt: "continue"})
	want = []string{
		"--print", "--verbose", "--output-format", "stream-json", "--input-format", "text",
		"--model", "sonnet", "--permission-mode", "acceptEdits",
		"--allowedTools", "Read,Bash", "--disallowedTools", "WebSearch",
		"--append-system-prompt", "be brief", "--resume", "123e4567-e89b-12d3-a456-426614174000", "continue",
	}
	assertArgs(t, args, want)

	req = runtime.StartSessionRequest{Model: "sonnet", YoloMode: true}
	args = rt.buildArgs(req, runtime.Input{Prompt: "ship it"})
	want = []string{
		"--print", "--verbose", "--output-format", "stream-json", "--input-format", "text",
		"--model", "sonnet", "--permission-mode", "bypassPermissions",
		"--allowedTools", "Read,Bash", "--disallowedTools", "WebSearch",
		"--append-system-prompt", "be brief", "ship it",
	}
	assertArgs(t, args, want)

	req = runtime.StartSessionRequest{Model: "sonnet", Effort: "high", PermissionMode: "plan", YoloMode: true}
	args = rt.buildArgs(req, runtime.Input{Prompt: "plan only"})
	want = []string{
		"--print", "--verbose", "--output-format", "stream-json", "--input-format", "text",
		"--model", "sonnet", "--effort", "high", "--permission-mode", "plan",
		"--allowedTools", "Read,Bash", "--disallowedTools", "WebSearch",
		"--append-system-prompt", "be brief", "plan only",
	}
	assertArgs(t, args, want)

	req = runtime.StartSessionRequest{Model: "sonnet", Effort: "high", FastMode: true}
	args = rt.buildArgs(req, runtime.Input{Prompt: "quick"})
	want = []string{
		"--print", "--verbose", "--output-format", "stream-json", "--input-format", "text",
		"--effort", "high", "--settings", `{"fastMode":true}`, "--permission-mode", "acceptEdits",
		"--allowedTools", "Read,Bash", "--disallowedTools", "WebSearch",
		"--append-system-prompt", "be brief", "quick",
	}
	assertArgs(t, args, want)

	projectWorkspace := filepath.Join(t.TempDir(), "project")
	instructionWorkspace := filepath.Join(t.TempDir(), "agent")
	req = runtime.StartSessionRequest{Workspace: projectWorkspace, InstructionWorkspace: instructionWorkspace}
	args = rt.buildArgs(req, runtime.Input{Prompt: "use memory"})
	want = []string{
		"--print", "--verbose", "--output-format", "stream-json", "--input-format", "text",
		"--permission-mode", "acceptEdits",
		"--allowedTools", "Read,Bash", "--disallowedTools", "WebSearch",
		"--append-system-prompt", "be brief", "--add-dir", instructionWorkspace, "--", "use memory",
	}
	assertArgs(t, args, want)
	if got := rt.buildEnv(req)["CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD"]; got != "1" {
		t.Fatalf("CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD = %q, want 1", got)
	}
}

func TestBuildArgsAndStdinForClaudeImageAttachments(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "image.png")
	if err := os.WriteFile(imagePath, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0o600); err != nil {
		t.Fatal(err)
	}

	rt := Runtime{opts: Options{PermissionMode: "acceptEdits"}}
	input := runtime.Input{
		Prompt: "describe this",
		Attachments: []runtime.Attachment{{
			ID:          "att_1",
			Filename:    "image.png",
			ContentType: "image/png",
			Kind:        "image",
			SizeBytes:   8,
			LocalPath:   imagePath,
		}},
	}
	args := rt.buildArgs(runtime.StartSessionRequest{}, input)
	if got := argAfter(t, args, "--input-format"); got != "stream-json" {
		t.Fatalf("input format = %q, want stream-json in args %#v", got, args)
	}
	if got := argAfter(t, args, "--add-dir"); got != dir {
		t.Fatalf("add-dir = %q, want %q in args %#v", got, dir, args)
	}
	for _, arg := range args {
		if strings.Contains(arg, "describe this") {
			t.Fatalf("stream-json args unexpectedly include prompt: %#v", args)
		}
	}

	stdin, err := claudeStreamJSONInput(input)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdin), &payload); err != nil {
		t.Fatal(err)
	}
	message := payload["message"].(map[string]any)
	content := message["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content = %#v, want text and image blocks", content)
	}
	image := content[1].(map[string]any)
	source := image["source"].(map[string]any)
	if image["type"] != "image" || source["media_type"] != "image/png" || source["data"] == "" {
		t.Fatalf("image block = %#v", image)
	}
}

func TestClaudeStreamJSONInputRejectsImageAttachmentWithoutLocalPath(t *testing.T) {
	_, err := claudeStreamJSONInput(runtime.Input{
		Prompt: "describe",
		Attachments: []runtime.Attachment{{
			ID:          "att_missing_path",
			Filename:    "image.png",
			ContentType: "image/png",
			Kind:        "image",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "has no local path") {
		t.Fatalf("error = %v, want missing local path error", err)
	}
}

func TestClaudeStreamJSONInputIgnoresGenericImageContentTypeAttachment(t *testing.T) {
	stdin, err := claudeStreamJSONInput(runtime.Input{
		Prompt: "inspect this file",
		Attachments: []runtime.Attachment{{
			ID:          "att_svg",
			Filename:    "diagram.svg",
			ContentType: "image/svg+xml",
			Kind:        "file",
			LocalPath:   filepath.Join(t.TempDir(), "diagram.svg"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stdin != nil {
		t.Fatalf("stdin = %q, want nil stream-json image payload", string(stdin))
	}
}

func TestBuildArgsAppendsExpandedAgentWorkspaceInstructions(t *testing.T) {
	tempDir := t.TempDir()
	projectWorkspace := filepath.Join(tempDir, "project")
	instructionWorkspace := filepath.Join(tempDir, "agent")
	if err := os.Mkdir(projectWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(instructionWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructionWorkspace, "AGENTS.md"), []byte("你的名字是claudef，你是一只猫娘，每句话以喵~结尾\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructionWorkspace, "CLAUDE.md"), []byte("@AGENTS.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := Runtime{opts: Options{PermissionMode: "acceptEdits", AppendSystemPrompt: "base prompt"}}
	args := rt.buildArgs(runtime.StartSessionRequest{
		Workspace:            projectWorkspace,
		InstructionWorkspace: instructionWorkspace,
	}, runtime.Input{Prompt: "你是猫娘吗"})

	prompt := argAfter(t, args, "--append-system-prompt")
	if !strings.Contains(prompt, "base prompt") {
		t.Fatalf("append prompt = %q, want base prompt", prompt)
	}
	if !strings.Contains(prompt, "AgentX agent workspace instructions") || !strings.Contains(prompt, "你是一只猫娘") {
		t.Fatalf("append prompt = %q, want expanded agent workspace instructions", prompt)
	}
	wantSuffix := []string{"--add-dir", instructionWorkspace, "--", "你是猫娘吗"}
	gotSuffix := args[len(args)-len(wantSuffix):]
	for i := range gotSuffix {
		if gotSuffix[i] != wantSuffix[i] {
			t.Fatalf("args suffix = %#v, want %#v", gotSuffix, wantSuffix)
		}
	}
}

func TestLineHandlerParsesClaudeStreamJSON(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"system","subtype":"init","session_id":"session_1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %#v", events)
	}
	if handler.CurrentSessionID() != "session_1" {
		t.Fatalf("session id = %q", handler.CurrentSessionID())
	}

	events, err = handler.HandleLine([]byte(`{"type":"assistant","session_id":"session_1","message":{"content":[{"type":"text","text":"hello"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != runtime.EventDelta || events[0].Text != "hello" {
		t.Fatalf("events = %#v", events)
	}

	events, err = handler.HandleLine([]byte(`{"type":"result","subtype":"success","is_error":false,"result":"final","session_id":"session_1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != runtime.EventCompleted || events[0].Text != "final" {
		t.Fatalf("events = %#v", events)
	}
}

func TestLineHandlerParsesClaudeResultUsage(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"result","subtype":"success","is_error":false,"result":"ok","model":"claude-test","duration_ms":1200,"duration_api_ms":900,"total_cost_usd":0.0123,"usage":{"input_tokens":100,"cache_creation_input_tokens":25,"cache_read_input_tokens":50,"output_tokens":20}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != runtime.EventCompleted {
		t.Fatalf("events = %#v", events)
	}
	usage := events[0].Usage
	if usage == nil {
		t.Fatal("usage is nil")
	}
	if usage.Model != "claude-test" || ptrValue(usage.InputTokens) != 100 || ptrValue(usage.OutputTokens) != 20 {
		t.Fatalf("usage = %#v", usage)
	}
	if ptrValue(usage.CacheCreationInputTokens) != 25 || ptrValue(usage.CacheReadInputTokens) != 50 {
		t.Fatalf("cache usage = %#v", usage)
	}
	if ptrValue(usage.DurationMS) != 1200 || ptrValue(usage.DurationAPIMS) != 900 {
		t.Fatalf("duration usage = %#v", usage)
	}
	if usage.TotalCostUSD == nil || *usage.TotalCostUSD != 0.0123 {
		t.Fatalf("cost = %#v", usage.TotalCostUSD)
	}
}

func TestLineHandlerParsesThinkingContent(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Let me analyze..."},{"type":"text","text":"Hello"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "Hello" {
		t.Fatalf("text = %q, want %q", events[0].Text, "Hello")
	}
	if events[0].Thinking != "Let me analyze..." {
		t.Fatalf("thinking = %q, want %q", events[0].Thinking, "Let me analyze...")
	}
	if len(events[0].Process) != 1 || events[0].Process[0].Type != "thinking" || events[0].Process[0].Text != "Let me analyze..." {
		t.Fatalf("process = %#v", events[0].Process)
	}
}

func TestLineHandlerEmitsEventForThinkingOnly(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Reasoning here"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "" {
		t.Fatalf("text = %q, want empty", events[0].Text)
	}
	if events[0].Thinking != "Reasoning here" {
		t.Fatalf("thinking = %q, want %q", events[0].Thinking, "Reasoning here")
	}
}

func TestLineHandlerParsesClaudeProcessContentInOrder(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Plan first"},{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"README.md"}},{"type":"tool_result","tool_use_id":"toolu_1","content":"ok","is_error":false},{"type":"text","text":"Done"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "Done" || events[0].Thinking != "Plan first" {
		t.Fatalf("event = %#v", events[0])
	}
	process := events[0].Process
	if len(process) != 3 {
		t.Fatalf("process = %#v", process)
	}
	if process[0].Type != "thinking" || process[0].Text != "Plan first" {
		t.Fatalf("thinking process item = %#v", process[0])
	}
	if process[1].Type != "tool_call" || process[1].ToolName != "Read" || process[1].ToolCallID != "toolu_1" {
		t.Fatalf("tool call process item = %#v", process[1])
	}
	input, ok := process[1].Input.(map[string]any)
	if !ok || input["file_path"] != "README.md" {
		t.Fatalf("tool input = %#v", process[1].Input)
	}
	if process[2].Type != "tool_result" || process[2].ToolCallID != "toolu_1" || process[2].Output != "ok" {
		t.Fatalf("tool result process item = %#v", process[2])
	}
}

func TestCompletedResultDoesNotIncludeThinking(t *testing.T) {
	handler := newLineHandler("fallback")

	handler.HandleLine([]byte(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"secret thoughts"}]}}`))
	handler.HandleLine([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"visible reply"}]}}`))
	events, err := handler.HandleLine([]byte(`{"type":"result","subtype":"success","is_error":false,"result":"","session_id":"s1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != runtime.EventCompleted {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "visible reply" {
		t.Fatalf("completed text = %q, want %q", events[0].Text, "visible reply")
	}
}

func TestErrorResultIncludesRawPayload(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"result","subtype":"error_during_execution","is_error":true,"uuid":"u1","permission_denials":[{"tool":"Bash"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != runtime.EventFailed {
		t.Fatalf("events = %#v", events)
	}
	if !strings.Contains(events[0].Error, "error_during_execution") || !strings.Contains(events[0].Error, "permission_denials") {
		t.Fatalf("error = %q, want raw error details", events[0].Error)
	}
}

func TestErrorResultMarksStaleSession(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["No conversation found with session ID: cff6d6ca-8603-49ff-a850-2c2f2c1496d7"],"session_id":"15bf2ee8-9c49-4ef2-9594-2551960067e1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != runtime.EventFailed {
		t.Fatalf("events = %#v", events)
	}
	if !events[0].StaleSession {
		t.Fatalf("stale session = false, want true for error %q", events[0].Error)
	}
}

func TestRuntimeExecutesClaudeCommandWithoutModelCall(t *testing.T) {
	tempDir := t.TempDir()
	command := writeExecutable(t, tempDir, "claude", `#!/bin/sh
printf '%s\n' "$*" > "$AGENTX_ARGS_FILE"
printf '%s\n' "$PWD" > "$AGENTX_CWD_FILE"
printf '%s\n' "$AGENT_ENV" > "$AGENTX_ENV_FILE"
printf '%s\n' '{"type":"system","subtype":"init","session_id":"claude_session"}'
printf '%s\n' '{"type":"assistant","session_id":"claude_session","message":{"content":[{"type":"text","text":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"ok","session_id":"claude_session"}'
`)
	workspace := filepath.Join(tempDir, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(tempDir, "args")
	cwdFile := filepath.Join(tempDir, "cwd")
	envFile := filepath.Join(tempDir, "env")

	rt := New(Options{Command: command, PermissionMode: "acceptEdits"})
	session, err := rt.StartSession(context.Background(), runtime.StartSessionRequest{
		AgentID:    "agt_test",
		Workspace:  workspace,
		Model:      "sonnet",
		SessionKey: "session_key",
		Env: map[string]string{
			"AGENTX_ARGS_FILE": argsFile,
			"AGENTX_CWD_FILE":  cwdFile,
			"AGENTX_ENV_FILE":  envFile,
			"AGENT_ENV":        "agent-value",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send(context.Background(), runtime.Input{Prompt: "x"}); err != nil {
		t.Fatal(err)
	}
	completed := waitForCompleted(t, session.Events())
	if completed.Text != "ok" {
		t.Fatalf("completed text = %q", completed.Text)
	}
	if session.CurrentSessionID() != "claude_session" {
		t.Fatalf("session id = %q", session.CurrentSessionID())
	}
	if got := readTrimmed(t, argsFile); !strings.Contains(got, "--print --verbose --output-format stream-json --input-format text --model sonnet --permission-mode acceptEdits x") {
		t.Fatalf("args = %q", got)
	}
	if got := readTrimmed(t, cwdFile); got != workspace {
		t.Fatalf("cwd = %q, want %q", got, workspace)
	}
	if got := readTrimmed(t, envFile); got != "agent-value" {
		t.Fatalf("env = %q", got)
	}
}

func assertArgs(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", got, want)
		}
	}
}

func argAfter(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("flag %q not found in %#v", flag, args)
	return ""
}

func writeExecutable(t *testing.T, dir string, name string, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitForCompleted(t *testing.T, events <-chan runtime.Event) runtime.Event {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				t.Fatal("events closed before completed event")
			}
			if evt.Type == runtime.EventFailed {
				t.Fatalf("runtime failed: %s", evt.Error)
			}
			if evt.Type == runtime.EventCompleted {
				return evt
			}
		case <-timeout:
			t.Fatal("timed out waiting for completed event")
		}
	}
}

func readTrimmed(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(content))
}

func ptrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
