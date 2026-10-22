package pi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestBuildArgs(t *testing.T) {
	rt := New(Options{
		SessionDir:         "/data/pi-sessions",
		AppendSystemPrompt: "Be terse.",
		Tools:              []string{"read", "bash"},
		ExcludeTools:       []string{"write"},
		ProjectTrust:       ProjectTrustApprove,
		ExtraArgs:          []string{"--verbose"},
	})
	args := rt.buildArgs(runtime.StartSessionRequest{Model: "anthropic/claude-sonnet-4:low", Effort: "high"}, "agentx-abc")
	got := strings.Join(args, " ")
	want := "--mode rpc --session-dir /data/pi-sessions --session-id agentx-abc --model anthropic/claude-sonnet-4 --thinking high --append-system-prompt Be terse. --tools read,bash --exclude-tools subagents_enable,write --approve --verbose"
	if got != want {
		t.Fatalf("args = %q\nwant   %q", got, want)
	}
}

func TestBuildArgsUsesModelThinkingSuffixWithoutEffort(t *testing.T) {
	rt := New(Options{ProjectTrust: ProjectTrustDeny})
	args := strings.Join(rt.buildArgs(runtime.StartSessionRequest{Model: "sonnet:max"}, ""), " ")
	if !strings.Contains(args, "--model sonnet --thinking max") || !strings.Contains(args, "--no-approve") || strings.Contains(args, "--session-id") {
		t.Fatalf("args = %q", args)
	}
}

func TestUsablePreviousSessionID(t *testing.T) {
	if got := usablePreviousSessionID(" pi:agent:channel:1 "); got != "" {
		t.Fatalf("placeholder id = %q, want empty", got)
	}
	if got := usablePreviousSessionID(" agentx-1234 "); got != "agentx-1234" {
		t.Fatalf("id = %q", got)
	}
}

func TestNewSessionIDShape(t *testing.T) {
	id := newSessionID()
	if !strings.HasPrefix(id, "agentx-") || len(id) != len("agentx-")+24 {
		t.Fatalf("id = %q", id)
	}
	for _, r := range id {
		if !(r == '-' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			t.Fatalf("id %q has unexpected rune %q", id, r)
		}
	}
}

func TestMergeEnvOverrides(t *testing.T) {
	env := mergeEnv([]string{"A=1", "B=2", "NOEQ"}, map[string]string{"B": "x"}, map[string]string{"C": "3"})
	joined := strings.Join(env, " ")
	if strings.Contains(joined, "B=2") || !strings.Contains(joined, "B=x") || !strings.Contains(joined, "A=1") || !strings.Contains(joined, "C=3") || !strings.Contains(joined, "NOEQ") {
		t.Fatalf("env = %v", env)
	}
}

// fakePiScript emulates `pi --mode rpc` well enough for a full turn: it answers
// get_state, streams a run for the prompt, answers get_session_stats, and
// records every stdin line to a file for assertions.
func fakePiScript(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	stdinLog := filepath.Join(dir, "stdin.log")
	script := `#!/bin/sh
while IFS= read -r line; do
  printf '%s\n' "$line" >> "` + stdinLog + `"
  id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  case "$line" in
    *'"type":"get_state"'*)
      printf '{"id":"%s","type":"response","command":"get_state","success":true,"data":{"sessionId":"sess-fake","thinkingLevel":"medium","model":{"provider":"fakeai","id":"fake-1","name":"Fake One"},"isStreaming":false}}\n' "$id" ;;
    *'"type":"prompt"'*)
      printf '{"id":"%s","type":"response","command":"prompt","success":true,"data":{"disposition":"started"}}\n' "$id"
      printf '{"type":"agent_start"}\n'
      printf '{"type":"message_start","message":{"role":"assistant","stopReason":"pending"}}\n'
      printf '{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hello "}}\n'
      printf '{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"world"}}\n'
      printf '{"type":"message_end","message":{"role":"assistant","model":"fake-1","stopReason":"stop","usage":{"input":3,"output":2,"cacheRead":0,"cacheWrite":0,"totalTokens":5,"cost":{"total":0.001}}}}\n'
      printf '{"type":"agent_end"}\n'
      printf '{"type":"agent_settled","aborted":false}\n' ;;
    *'"type":"get_session_stats"'*)
      printf '{"id":"%s","type":"response","command":"get_session_stats","success":true,"data":{"contextUsage":{"tokens":5,"contextWindow":1000,"percent":0.5}}}\n' "$id" ;;
    *'"type":"new_session"'*)
      printf '{"id":"%s","type":"response","command":"new_session","success":true,"data":{"cancelled":false}}\n' "$id" ;;
    *)
      printf '{"id":"%s","type":"response","command":"unknown","success":false,"error":"unsupported in fake"}\n' "$id" ;;
  esac
done
`
	path := filepath.Join(dir, "fake-pi.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, stdinLog
}

func TestRuntimeRunsTurnAgainstFakePi(t *testing.T) {
	script, stdinLog := fakePiScript(t)
	rt2 := New(Options{Command: script, SessionDir: t.TempDir()})
	t.Cleanup(func() { _ = rt2.Shutdown(context.Background()) })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := runtime.StartSessionRequest{AgentID: "agent", SessionKey: "agent:channel:c1", Workspace: t.TempDir(), Model: "fake-1", Effort: "medium"}
	session, err := rt2.StartSession(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := session.CurrentSessionID(); got != "sess-fake" {
		t.Fatalf("session id after start = %q, want the id pi reported", got)
	}
	if err := session.Send(ctx, runtime.Input{Prompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var final runtime.Event
loop:
	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for turn")
		case evt, ok := <-session.Events():
			if !ok {
				break loop
			}
			if evt.Type == runtime.EventDelta {
				text.WriteString(evt.Text)
			} else {
				final = evt
			}
		}
	}
	if final.Type != runtime.EventCompleted || final.Text != "Hello world" || text.String() != "Hello world" {
		t.Fatalf("final = %+v streamed=%q", final, text.String())
	}
	if final.Usage == nil || final.Usage.Context == nil || final.Usage.Context.ContextWindowTokens == nil || *final.Usage.Context.ContextWindowTokens != 1000 {
		t.Fatalf("usage = %+v", final.Usage)
	}

	// A second session on the same conversation with the stored id reuses the
	// process and does not start a new pi session.
	session2, err := rt2.StartSession(ctx, runtime.StartSessionRequest{AgentID: "agent", SessionKey: "agent:channel:c1", PreviousSessionID: "sess-fake", Model: "fake-1", Effort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session2.Send(ctx, runtime.Input{Prompt: "again"}); err != nil {
		t.Fatal(err)
	}
	for evt := range session2.Events() {
		if evt.Type == runtime.EventFailed {
			t.Fatalf("second turn failed: %s", evt.Error)
		}
	}
	// /new: no stored id on a live process starts a fresh pi session.
	session3, err := rt2.StartSession(ctx, runtime.StartSessionRequest{AgentID: "agent", SessionKey: "agent:channel:c1", Model: "fake-1", Effort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session3.Send(ctx, runtime.Input{Prompt: "fresh"}); err != nil {
		t.Fatal(err)
	}
	for evt := range session3.Events() {
		if evt.Type == runtime.EventFailed {
			t.Fatalf("third turn failed: %s", evt.Error)
		}
	}

	data, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if strings.Count(log, `"type":"prompt"`) != 3 || strings.Count(log, `"type":"new_session"`) != 1 {
		t.Fatalf("stdin log:\n%s", log)
	}
	if !strings.Contains(log, `"message":"hi"`) {
		t.Fatalf("prompt message missing from stdin log:\n%s", log)
	}
}

func TestEnsureExtensionWritesFileAndAddsFlag(t *testing.T) {
	dir := t.TempDir()
	rt := New(Options{ExtensionDir: dir})
	if err := rt.ensureExtension(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, extensionFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != extensionSource || !strings.Contains(string(data), "agentx-mode") {
		t.Fatal("extension file content mismatch")
	}
	args := strings.Join(rt.buildArgs(runtime.StartSessionRequest{}, ""), " ")
	if !strings.Contains(args, "-e "+path) {
		t.Fatalf("args = %q, want -e %s", args, path)
	}
	// A second runtime reuses the identical file without rewriting it.
	info1, _ := os.Stat(path)
	rt2 := New(Options{ExtensionDir: dir})
	if err := rt2.ensureExtension(); err != nil {
		t.Fatal(err)
	}
	info2, _ := os.Stat(path)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatal("unchanged extension was rewritten")
	}
}

func TestEnsureExtensionDisabledWhenDirEmpty(t *testing.T) {
	rt := New(Options{})
	if err := rt.ensureExtension(); err != nil || rt.extPath != "" {
		t.Fatalf("extPath = %q err = %v", rt.extPath, err)
	}
}

func TestMergeSubagentConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "extensions", "subagent", "config.json")

	// Missing file: created with the key.
	if err := mergeSubagentConfig(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"asyncByDefault": false`) {
		t.Fatalf("config = %s", data)
	}

	// Existing keys survive and an explicit value is never overridden.
	if err := os.WriteFile(path, []byte(`{"asyncByDefault": true, "toolActivation": "eager"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mergeSubagentConfig(path); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), `"asyncByDefault": true`) || !strings.Contains(string(data), `"toolActivation": "eager"`) {
		t.Fatalf("config = %s", data)
	}

	// Other keys without asyncByDefault get it added.
	if err := os.WriteFile(path, []byte(`{"toolActivation": "eager"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mergeSubagentConfig(path); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), `"asyncByDefault": false`) || !strings.Contains(string(data), `"toolActivation": "eager"`) {
		t.Fatalf("config = %s", data)
	}

	// Invalid JSON is reported, not clobbered.
	if err := os.WriteFile(path, []byte(`{nope`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mergeSubagentConfig(path); err == nil {
		t.Fatal("expected parse error")
	}
	if data, _ = os.ReadFile(path); string(data) != `{nope` {
		t.Fatalf("invalid config was rewritten: %s", data)
	}
}

func TestPiAgentDirPrefersAgentEnv(t *testing.T) {
	if got := piAgentDir(map[string]string{"PI_CODING_AGENT_DIR": "/rt"}, map[string]string{"PI_CODING_AGENT_DIR": "/agent"}); got != "/agent" {
		t.Fatalf("dir = %q, want agent env to win", got)
	}
	if got := piAgentDir(map[string]string{"PI_CODING_AGENT_DIR": "/rt"}, nil); got != "/rt" {
		t.Fatalf("dir = %q, want runtime env", got)
	}
	t.Setenv("PI_CODING_AGENT_DIR", "/proc-env")
	if got := piAgentDir(nil, nil); got != "/proc-env" {
		t.Fatalf("dir = %q, want process env", got)
	}
}

func TestBuildArgsAppendsAgentSystemPrompt(t *testing.T) {
	rt := New(Options{AppendSystemPrompt: "global"})
	args := rt.buildArgs(runtime.StartSessionRequest{SystemPrompt: "agent"}, "")
	for i, arg := range args {
		if arg == "--append-system-prompt" {
			if args[i+1] != "global\n\nagent" {
				t.Fatalf("prompt = %q, want global and agent joined", args[i+1])
			}
			return
		}
	}
	t.Fatalf("args = %q, missing --append-system-prompt", args)
}

func TestBuildArgsReplacesSystemPromptInReplaceMode(t *testing.T) {
	rt := New(Options{AppendSystemPrompt: "global"})
	args := rt.buildArgs(runtime.StartSessionRequest{SystemPrompt: "custom", SystemPromptMode: runtime.SystemPromptModeReplace}, "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--system-prompt custom") || !strings.Contains(joined, "--append-system-prompt global") || strings.Contains(joined, "global\n\ncustom") {
		t.Fatalf("args = %q", args)
	}
}
