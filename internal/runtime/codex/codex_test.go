package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestBuildArgsStartsAndResumesCodexExec(t *testing.T) {
	rt := Runtime{opts: Options{FullAuto: true, SkipGitRepoCheck: true}}
	req := runtime.StartSessionRequest{Model: "gpt-test"}
	args := rt.buildArgs(req, runtime.Input{Prompt: "hello"})
	want := []string{"exec", "--json", "--model", "gpt-test", "--full-auto", "--skip-git-repo-check", "hello"}
	assertArgs(t, args, want)

	req.PreviousSessionID = "0199a213-81c0-7800-8aa1-bbab2a035a53"
	args = rt.buildArgs(req, runtime.Input{Prompt: "continue"})
	want = []string{"exec", "resume", "--json", "--model", "gpt-test", "--full-auto", "--skip-git-repo-check", "0199a213-81c0-7800-8aa1-bbab2a035a53", "continue"}
	assertArgs(t, args, want)

	req = runtime.StartSessionRequest{Model: "gpt-test", YoloMode: true}
	args = rt.buildArgs(req, runtime.Input{Prompt: "ship it"})
	want = []string{"exec", "--json", "--model", "gpt-test", "--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check", "ship it"}
	assertArgs(t, args, want)

	req = runtime.StartSessionRequest{Model: "gpt-test", Effort: "high"}
	args = rt.buildArgs(req, runtime.Input{Prompt: "think"})
	want = []string{"exec", "--json", "--model", "gpt-test", "-c", `model_reasoning_effort="high"`, "--full-auto", "--skip-git-repo-check", "think"}
	assertArgs(t, args, want)

	req = runtime.StartSessionRequest{Model: "gpt-test", Effort: "high", FastMode: true}
	args = rt.buildArgs(req, runtime.Input{Prompt: "quick"})
	want = []string{"exec", "--json", "--model", "gpt-test", "-c", `model_reasoning_effort="high"`, "-c", `service_tier="fast"`, "-c", "features.fast_mode=true", "--full-auto", "--skip-git-repo-check", "quick"}
	assertArgs(t, args, want)
}

func TestLineHandlerParsesCodexJSONEvents(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"thread.started","thread_id":"thread_1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %#v", events)
	}
	if handler.CurrentSessionID() != "thread_1" {
		t.Fatalf("session id = %q", handler.CurrentSessionID())
	}

	events, err = handler.HandleLine([]byte(`{"type":"item.completed","item":{"type":"agent_message","text":"done"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %#v", events)
	}

	events, err = handler.HandleLine([]byte(`{"type":"turn.completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != runtime.EventCompleted || events[0].Text != "done" {
		t.Fatalf("events = %#v", events)
	}
}

func TestLineHandlerParsesReasoningItem(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"item.completed","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"thinking about it"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "" {
		t.Fatalf("text = %q, want empty", events[0].Text)
	}
	if events[0].Thinking != "thinking about it" {
		t.Fatalf("thinking = %q, want %q", events[0].Thinking, "thinking about it")
	}
	if len(events[0].Process) != 1 || events[0].Process[0].Type != "thinking" || events[0].Process[0].Text != "thinking about it" {
		t.Fatalf("process = %#v", events[0].Process)
	}
}

func TestLineHandlerParsesCodexToolCallProcessItem(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"item.completed","item":{"type":"mcpToolCall","id":"call_1","name":"read_file","input":{"path":"README.md"},"status":"completed"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	process := events[0].Process
	if len(process) != 1 {
		t.Fatalf("process = %#v", process)
	}
	item := process[0]
	if item.Type != "tool_call" || item.ToolName != "read_file" || item.ToolCallID != "call_1" || item.Status != "completed" {
		t.Fatalf("process item = %#v", item)
	}
	input, ok := item.Input.(map[string]any)
	if !ok || input["path"] != "README.md" {
		t.Fatalf("input = %#v", item.Input)
	}
	raw, ok := item.Raw.(map[string]any)
	if !ok || raw["type"] != "mcpToolCall" {
		t.Fatalf("raw = %#v", item.Raw)
	}
}

func TestLineHandlerParsesCodexExecJSONToolFlow(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"turn.started"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %#v", events)
	}

	events, err = handler.HandleLine([]byte(`{"type":"item.completed","item":{"type":"agent_message","content":[{"type":"output_text","text":"I will inspect the files."}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %#v", events)
	}

	events, err = handler.HandleLine([]byte(`{"type":"item.started","item":{"type":"command_execution","id":"cmd_1","command":"ls -la"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Thinking != "I will inspect the files." || len(events[0].Process) != 1 || events[0].Process[0].Type != "thinking" {
		t.Fatalf("thinking event = %#v", events[0])
	}
	if len(events[1].Process) != 1 {
		t.Fatalf("tool call event = %#v", events[1])
	}
	call := events[1].Process[0]
	if call.Type != "tool_call" || call.ToolName != "Bash" || call.ToolCallID != "cmd_1" {
		t.Fatalf("tool call = %#v", call)
	}
	input, ok := call.Input.(map[string]any)
	if !ok || input["command"] != "ls -la" {
		t.Fatalf("tool input = %#v", call.Input)
	}

	events, err = handler.HandleLine([]byte(`{"type":"item.completed","item":{"type":"command_execution","id":"cmd_1","command":"ls -la","status":"completed","aggregated_output":"README.md\n","exit_code":0}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(events[0].Process) != 1 {
		t.Fatalf("tool result events = %#v", events)
	}
	result := events[0].Process[0]
	if result.Type != "tool_result" || result.ToolName != "Bash" || result.ToolCallID != "cmd_1" || result.Output != "README.md\n" {
		t.Fatalf("tool result = %#v", result)
	}
}

func TestLineHandlerParsesCurrentCodexResponseItems(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"event_msg","payload":{"type":"agent_reasoning","text":"checking files"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Thinking != "checking files" || len(events[0].Process) != 1 {
		t.Fatalf("reasoning events = %#v", events)
	}

	events, err = handler.HandleLine([]byte(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"ls\",\"workdir\":\"/tmp\"}","call_id":"call_1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(events[0].Process) != 1 {
		t.Fatalf("tool call events = %#v", events)
	}
	call := events[0].Process[0]
	if call.Type != "tool_call" || call.ToolName != "exec_command" || call.ToolCallID != "call_1" {
		t.Fatalf("tool call = %#v", call)
	}
	input, ok := call.Input.(map[string]any)
	if !ok || input["cmd"] != "ls" || input["workdir"] != "/tmp" {
		t.Fatalf("tool input = %#v", call.Input)
	}

	events, err = handler.HandleLine([]byte(`{"type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"Exit code: 0\nOutput:\nREADME.md\n"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(events[0].Process) != 1 {
		t.Fatalf("tool result events = %#v", events)
	}
	result := events[0].Process[0]
	if result.Type != "tool_result" || result.ToolCallID != "call_1" || result.Output != "Exit code: 0\nOutput:\nREADME.md\n" {
		t.Fatalf("tool result = %#v", result)
	}

	events, err = handler.HandleLine([]byte(`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Text != "done" {
		t.Fatalf("message events = %#v", events)
	}
}

func TestReasoningNotIncludedInCompletedText(t *testing.T) {
	handler := newLineHandler("fallback")

	handler.HandleLine([]byte(`{"type":"item.completed","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"internal reasoning"}]}}`))
	handler.HandleLine([]byte(`{"type":"item.completed","item":{"type":"agent_message","text":"visible"}}`))
	events, err := handler.HandleLine([]byte(`{"type":"turn.completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != runtime.EventCompleted {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "visible" {
		t.Fatalf("completed text = %q, want %q", events[0].Text, "visible")
	}
}

func TestRuntimeExecutesCodexCommandWithoutModelCall(t *testing.T) {
	tempDir := t.TempDir()
	command := writeExecutable(t, tempDir, "codex", `#!/bin/sh
printf '%s\n' "$*" > "$AGENTX_ARGS_FILE"
printf '%s\n' "$PWD" > "$AGENTX_CWD_FILE"
printf '%s\n' "$AGENT_ENV" > "$AGENTX_ENV_FILE"
printf '%s\n' '{"type":"thread.started","thread_id":"thread_test"}'
printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"ok"}}'
printf '%s\n' '{"type":"turn.completed"}'
`)
	workspace := filepath.Join(tempDir, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(tempDir, "args")
	cwdFile := filepath.Join(tempDir, "cwd")
	envFile := filepath.Join(tempDir, "env")

	rt := New(Options{Command: command})
	session, err := rt.StartSession(context.Background(), runtime.StartSessionRequest{
		AgentID:    "agt_test",
		Workspace:  workspace,
		Model:      "gpt-test",
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
	if session.CurrentSessionID() != "thread_test" {
		t.Fatalf("session id = %q", session.CurrentSessionID())
	}
	if got := readTrimmed(t, argsFile); !strings.Contains(got, "exec --json --model gpt-test x") {
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
