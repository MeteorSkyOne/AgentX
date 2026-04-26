package claude

import (
	"context"
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
