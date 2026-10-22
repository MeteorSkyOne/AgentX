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
