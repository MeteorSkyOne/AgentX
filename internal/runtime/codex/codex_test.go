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

	instructionWorkspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(instructionWorkspace, "AGENTS.md"), []byte("agent rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req = runtime.StartSessionRequest{Workspace: "/tmp/project", InstructionWorkspace: instructionWorkspace}
	args = rt.buildArgs(req, runtime.Input{Prompt: "use memory"})
	want = []string{"exec", "--json", "-c", `developer_instructions="agent rule"`, "--full-auto", "--skip-git-repo-check", "use memory"}
	assertArgs(t, args, want)
}

func TestBuildArgsAddsCodexAttachmentDirsAndImages(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "image.png")
	textPath := filepath.Join(dir, "notes.txt")
	rt := Runtime{opts: Options{FullAuto: true}}
	args, stdin := rt.buildArgsAndStdin(runtime.StartSessionRequest{}, runtime.Input{
		Prompt: "use these",
		Attachments: []runtime.Attachment{
			{ID: "att_img", Filename: "image.png", ContentType: "image/png", Kind: "image", LocalPath: imagePath},
			{ID: "att_txt", Filename: "notes.txt", ContentType: "text/plain", Kind: "text", LocalPath: textPath},
		},
	})
	if got := countArg(args, "--add-dir"); got != 1 {
		t.Fatalf("args = %#v, want one --add-dir, got %d", args, got)
	}
	if got := argAfter(t, args, "--add-dir"); got != dir {
		t.Fatalf("add-dir = %q, want %q in args %#v", got, dir, args)
	}
	if got := argAfter(t, args, "--image"); got != imagePath {
		t.Fatalf("image path = %q, want %q in args %#v", got, imagePath, args)
	}
	assertArgsSuffix(t, args, []string{"--", "-"})
	prompt := string(stdin)
	if !strings.Contains(prompt, "image.png") || !strings.Contains(prompt, "notes.txt") || !strings.Contains(prompt, textPath) {
		t.Fatalf("stdin prompt = %q, want attachment references", prompt)
	}
}

func TestBuildArgsUsesStdinPromptAfterCodexImages(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "image.png")
	rt := Runtime{opts: Options{SkipGitRepoCheck: true}}
	args, stdin := rt.buildArgsAndStdin(runtime.StartSessionRequest{
		Model:             "gpt-test",
		PreviousSessionID: "0199a213-81c0-7800-8aa1-bbab2a035a53",
	}, runtime.Input{
		Prompt: "describe this image",
		Attachments: []runtime.Attachment{{
			ID:          "att_img",
			Filename:    "image.png",
			ContentType: "image/png",
			Kind:        "image",
			LocalPath:   imagePath,
		}},
	})

	wantSuffix := []string{"--", "0199a213-81c0-7800-8aa1-bbab2a035a53", "-"}
	assertArgsSuffix(t, args, wantSuffix)
	assertArgBefore(t, args, "--add-dir", "resume")
	if got := argAfter(t, args, "--add-dir"); got != dir {
		t.Fatalf("add-dir = %q, want %q in args %#v", got, dir, args)
	}
	if got := argAfter(t, args, "--image"); got != imagePath {
		t.Fatalf("image path = %q, want %q in args %#v", got, imagePath, args)
	}
	if strings.Contains(strings.Join(args, "\n"), "describe this image") {
		t.Fatalf("args = %#v, prompt should be passed via stdin", args)
	}
	if prompt := string(stdin); !strings.Contains(prompt, "describe this image") || !strings.Contains(prompt, imagePath) {
		t.Fatalf("stdin prompt = %q, want rendered prompt with attachment path", prompt)
	}
}

func TestBuildArgsSupportsMultipleCodexImages(t *testing.T) {
	dir := t.TempDir()
	firstImagePath := filepath.Join(dir, "first.png")
	secondImagePath := filepath.Join(dir, "second.webp")
	rt := Runtime{opts: Options{SkipGitRepoCheck: true}}
	args, stdin := rt.buildArgsAndStdin(runtime.StartSessionRequest{}, runtime.Input{
		Prompt: "compare these images",
		Attachments: []runtime.Attachment{
			{ID: "att_first", Filename: "first.png", ContentType: "image/png", Kind: "image", LocalPath: firstImagePath},
			{ID: "att_second", Filename: "second.webp", ContentType: "image/webp", Kind: "image", LocalPath: secondImagePath},
		},
	})

	if got := countArg(args, "--image"); got != 2 {
		t.Fatalf("args = %#v, want two --image args, got %d", args, got)
	}
	imagePaths := argsAfter(t, args, "--image")
	wantImages := []string{firstImagePath, secondImagePath}
	assertArgs(t, imagePaths, wantImages)
	assertArgsSuffix(t, args, []string{"--", "-"})
	prompt := string(stdin)
	if !strings.Contains(prompt, "compare these images") || !strings.Contains(prompt, firstImagePath) || !strings.Contains(prompt, secondImagePath) {
		t.Fatalf("stdin prompt = %q, want both image paths", prompt)
	}
}

func TestBuildArgsSupportsMultipleCodexImagesWhenResuming(t *testing.T) {
	dir := t.TempDir()
	firstImagePath := filepath.Join(dir, "first.png")
	secondImagePath := filepath.Join(dir, "second.png")
	rt := Runtime{opts: Options{SkipGitRepoCheck: true}}
	args, stdin := rt.buildArgsAndStdin(runtime.StartSessionRequest{
		PreviousSessionID: "0199a213-81c0-7800-8aa1-bbab2a035a53",
	}, runtime.Input{
		Prompt: "compare these images",
		Attachments: []runtime.Attachment{
			{ID: "att_first", Filename: "first.png", ContentType: "image/png", Kind: "image", LocalPath: firstImagePath},
			{ID: "att_second", Filename: "second.png", ContentType: "image/png", Kind: "image", LocalPath: secondImagePath},
		},
	})

	assertArgBefore(t, args, "--add-dir", "resume")
	if got := argAfter(t, args, "--add-dir"); got != dir {
		t.Fatalf("add-dir = %q, want %q in args %#v", got, dir, args)
	}
	if got := countArg(args, "--image"); got != 2 {
		t.Fatalf("args = %#v, want two --image args, got %d", args, got)
	}
	imagePaths := argsAfter(t, args, "--image")
	wantImages := []string{firstImagePath, secondImagePath}
	assertArgs(t, imagePaths, wantImages)
	assertArgsSuffix(t, args, []string{"--", "0199a213-81c0-7800-8aa1-bbab2a035a53", "-"})
	prompt := string(stdin)
	if !strings.Contains(prompt, "compare these images") || !strings.Contains(prompt, firstImagePath) || !strings.Contains(prompt, secondImagePath) {
		t.Fatalf("stdin prompt = %q, want both image paths", prompt)
	}
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

func TestLineHandlerParsesCodexUsageEvents(t *testing.T) {
	handler := newLineHandler("fallback")

	events, err := handler.HandleLine([]byte(`{"type":"token_count.info","info":{"last_token_usage":{"input_tokens":80,"cached_input_tokens":30,"output_tokens":10,"reasoning_output_tokens":4,"total_tokens":94}}}`))
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
	if len(events) != 1 || events[0].Type != runtime.EventCompleted {
		t.Fatalf("events = %#v", events)
	}
	usage := events[0].Usage
	if usage == nil {
		t.Fatal("usage is nil")
	}
	if ptrValue(usage.InputTokens) != 80 || ptrValue(usage.CachedInputTokens) != 30 || ptrValue(usage.OutputTokens) != 10 || ptrValue(usage.ReasoningOutputTokens) != 4 || ptrValue(usage.TotalTokens) != 94 {
		t.Fatalf("usage = %#v", usage)
	}

	handler = newLineHandler("fallback")
	events, err = handler.HandleLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12,"total_cost_usd":0.001}}`))
	if err != nil {
		t.Fatal(err)
	}
	usage = events[0].Usage
	if ptrValue(usage.InputTokens) != 5 || ptrValue(usage.OutputTokens) != 7 || ptrValue(usage.TotalTokens) != 12 {
		t.Fatalf("turn usage = %#v", usage)
	}
	if usage.TotalCostUSD == nil || *usage.TotalCostUSD != 0.001 {
		t.Fatalf("cost = %#v", usage.TotalCostUSD)
	}

	handler = newLineHandler("fallback")
	events, err = handler.HandleLine([]byte(`{"type":"turn.completed","turn":{"usage":{"input_tokens":11,"total_tokens":17,"cache_read_input_tokens":3}}}`))
	if err != nil {
		t.Fatal(err)
	}
	usage = events[0].Usage
	if ptrValue(usage.InputTokens) != 11 || ptrValue(usage.OutputTokens) != 6 || ptrValue(usage.TotalTokens) != 17 || ptrValue(usage.CachedInputTokens) != 3 {
		t.Fatalf("nested turn usage = %#v", usage)
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
	if got := readTrimmed(t, argsFile); got != "exec --json --model gpt-test x" {
		t.Fatalf("args = %q", got)
	}
	if got := readTrimmed(t, cwdFile); got != workspace {
		t.Fatalf("cwd = %q, want %q", got, workspace)
	}
	if got := readTrimmed(t, envFile); got != "agent-value" {
		t.Fatalf("env = %q", got)
	}
}

func TestRuntimePassesCodexImagePromptViaStdin(t *testing.T) {
	tempDir := t.TempDir()
	command := writeExecutable(t, tempDir, "codex", `#!/bin/sh
printf '%s\n' "$*" > "$AGENTX_ARGS_FILE"
cat > "$AGENTX_STDIN_FILE"
printf '%s\n' '{"type":"thread.started","thread_id":"thread_image"}'
printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"ok"}}'
printf '%s\n' '{"type":"turn.completed"}'
`)
	workspace := filepath.Join(tempDir, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(tempDir, "image.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(tempDir, "args")
	stdinFile := filepath.Join(tempDir, "stdin")

	rt := New(Options{Command: command})
	session, err := rt.StartSession(context.Background(), runtime.StartSessionRequest{
		AgentID:   "agt_test",
		Workspace: workspace,
		Env: map[string]string{
			"AGENTX_ARGS_FILE":  argsFile,
			"AGENTX_STDIN_FILE": stdinFile,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send(context.Background(), runtime.Input{
		Prompt: "describe this image",
		Attachments: []runtime.Attachment{{
			ID:          "att_img",
			Filename:    "image.png",
			ContentType: "image/png",
			Kind:        "image",
			LocalPath:   imagePath,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	completed := waitForCompleted(t, session.Events())
	if completed.Text != "ok" {
		t.Fatalf("completed text = %q", completed.Text)
	}
	if got := readTrimmed(t, argsFile); got != "exec --json --add-dir "+tempDir+" --image "+imagePath+" -- -" {
		t.Fatalf("args = %q", got)
	}
	stdin := readFile(t, stdinFile)
	if !strings.Contains(stdin, "describe this image") || !strings.Contains(stdin, imagePath) {
		t.Fatalf("stdin = %q, want rendered prompt with attachment path", stdin)
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

func assertArgsSuffix(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) < len(want) {
		t.Fatalf("args = %#v, want suffix %#v", got, want)
	}
	suffix := got[len(got)-len(want):]
	for i := range want {
		if suffix[i] != want[i] {
			t.Fatalf("args suffix = %#v, want %#v in args %#v", suffix, want, got)
		}
	}
}

func assertArgBefore(t *testing.T, args []string, before string, after string) {
	t.Helper()
	beforeIndex := -1
	afterIndex := -1
	for i, arg := range args {
		if arg == before && beforeIndex == -1 {
			beforeIndex = i
		}
		if arg == after && afterIndex == -1 {
			afterIndex = i
		}
	}
	if beforeIndex == -1 || afterIndex == -1 || beforeIndex >= afterIndex {
		t.Fatalf("args = %#v, want %q before %q", args, before, after)
	}
}

func argAfter(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	t.Fatalf("flag %s missing in args %#v", flag, args)
	return ""
}

func argsAfter(t *testing.T, args []string, flag string) []string {
	t.Helper()
	var values []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			values = append(values, args[i+1])
		}
	}
	if len(values) == 0 {
		t.Fatalf("flag %s missing in args %#v", flag, args)
	}
	return values
}

func countArg(args []string, want string) int {
	var count int
	for _, arg := range args {
		if arg == want {
			count++
		}
	}
	return count
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
	return strings.TrimSpace(readFile(t, path))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func ptrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
