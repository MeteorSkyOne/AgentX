package claudepersist

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

// backgroundSubagentScript mimics Claude Code launching a background subagent:
// the turn's result lands immediately, and only afterwards does the subagent ask
// for tool permission over the stdio permission prompt.
func backgroundSubagentScript(answers string) string {
	return `
echo '{"type":"system","session_id":"sess-bg"}'
read -r line
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"launched"}]}}'
echo '{"type":"result","result":"launched","subtype":"success"}'
sleep 0.5
echo '{"type":"control_request","request_id":"req-bash","request":{"subtype":"can_use_tool","tool_name":"Bash"}}'
read -r bashResp
echo '{"type":"control_request","request_id":"req-ask","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion"}}'
read -r askResp
printf '%s\n%s\n' "$bashResp" "$askResp" > ` + answers + `
sleep 5
`
}

func TestBackgroundControlRequestsAnsweredAfterTurnEnds(t *testing.T) {
	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	answers := filepath.Join(t.TempDir(), "answers.jsonl")
	script := backgroundSubagentScript(answers)

	proc, _, err := pool.GetOrCreate("bg-key", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", script)
	})
	if err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{opts: Options{Command: "sh", PermissionMode: "acceptEdits"}, pool: pool}
	sess := newPersistentSession(proc, "bg-key", rt)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := sess.waitForSystemEvent(ctx); err != nil {
		t.Fatal(err)
	}
	proc.SetFallbackHandler(newBackgroundFallback(proc, "bg-key", nil))

	if err := sess.Send(ctx, runtime.Input{Prompt: "spawn a background subagent"}); err != nil {
		t.Fatal(err)
	}

	// Drain the turn: it completes as soon as the result arrives, well before
	// the subagent asks for permission.
	var completed bool
	for evt := range sess.Events() {
		if evt.Type == runtime.EventCompleted {
			completed = true
		}
		if evt.Type == runtime.EventFailed {
			t.Fatalf("unexpected failure event: %s", evt.Error)
		}
	}
	if !completed {
		t.Fatal("expected the turn to complete")
	}

	responses := waitForLines(t, answers, 2)

	bashResp := decodeControlResponse(t, responses[0])
	if got := bashResp["request_id"]; got != "req-bash" {
		t.Fatalf("expected response for req-bash, got %v", got)
	}
	if got := controlBehavior(bashResp); got != "allow" {
		t.Fatalf("expected background Bash permission to be allowed, got %q", got)
	}

	askResp := decodeControlResponse(t, responses[1])
	if got := askResp["request_id"]; got != "req-ask" {
		t.Fatalf("expected response for req-ask, got %v", got)
	}
	if got := controlBehavior(askResp); got != "deny" {
		t.Fatalf("expected background AskUserQuestion to be denied, got %q", got)
	}
}

func waitForLines(t *testing.T, path string, want int) []string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= want && strings.TrimSpace(lines[want-1]) != "" {
				return lines
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d control responses in %s", want, path)
	return nil
}

func decodeControlResponse(t *testing.T, line string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("failed to decode control response %q: %v", line, err)
	}
	if payload["type"] != "control_response" {
		t.Fatalf("expected a control_response, got %v", payload["type"])
	}
	response, _ := payload["response"].(map[string]any)
	if response == nil {
		t.Fatalf("control response missing response body: %q", line)
	}
	return response
}

func controlBehavior(response map[string]any) string {
	inner, _ := response["response"].(map[string]any)
	behavior, _ := inner["behavior"].(string)
	return behavior
}

// backgroundShellScript replays what Claude Code emits for a background Bash
// command: the reply ends while the command runs, and when it finishes the CLI
// wakes the agent for a turn of its own.
const backgroundShellScript = `
echo '{"type":"system","subtype":"init","session_id":"sess-sh"}'
read -r line
echo '{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"b1","task_type":"local_bash"}]}'
echo '{"type":"system","subtype":"task_started","task_id":"b1","task_type":"local_bash","tool_use_id":"toolu_sh","is_backgrounded":true}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"started"}]}}'
echo '{"type":"result","result":"started","subtype":"success"}'
sleep 0.5
echo '{"type":"system","subtype":"task_notification","task_id":"b1","status":"completed","tool_use_id":"toolu_sh"}'
echo '{"type":"system","subtype":"background_tasks_changed","tasks":[]}'
echo '{"type":"system","subtype":"init","session_id":"sess-sh"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"bg-done"}]}}'
echo '{"type":"result","result":"bg-done","subtype":"success"}'
sleep 5
`

func TestBackgroundShellReportArrivesAsItsOwnTurn(t *testing.T) {
	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("sh-key", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", backgroundShellScript)
	})
	if err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{opts: Options{Command: "sh", PermissionMode: "acceptEdits"}, pool: pool}
	offered := make(chan string, 1)
	rt.SetBackgroundTurnHandler(func(sessionKey string) bool {
		offered <- sessionKey
		return true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sess := newPersistentSession(proc, "sh-key", rt)
	if err := sess.waitForSystemEvent(ctx); err != nil {
		t.Fatal(err)
	}
	proc.SetFallbackHandler(newBackgroundFallback(proc, "sh-key", rt.startBackgroundTurn))

	if err := sess.Send(ctx, runtime.Input{Prompt: "run it in the background"}); err != nil {
		t.Fatal(err)
	}
	if text := drainCompleted(t, sess); text != "started" {
		t.Fatalf("reply = %q, want it to end without waiting for the command", text)
	}

	select {
	case key := <-offered:
		if key != "sh-key" {
			t.Fatalf("offered key = %q", key)
		}
	case <-ctx.Done():
		t.Fatal("the agent's own turn was never offered")
	}
	wake := newPersistentSession(proc, "sh-key", rt)
	if err := wake.AdoptBackgroundTurn(ctx); err != nil {
		t.Fatal(err)
	}
	if text := drainCompleted(t, wake); text != "bg-done" {
		t.Fatalf("background turn = %q, want the agent's report", text)
	}
	if proc.InUse() {
		t.Fatal("the adopted turn must release the process when it completes")
	}
	if err := newPersistentSession(proc, "sh-key", rt).AdoptBackgroundTurn(ctx); err == nil {
		t.Fatal("adopting with no claimed turn must fail")
	}
}

func drainCompleted(t *testing.T, sess *persistentSession) string {
	t.Helper()
	var text string
	var completed bool
	for evt := range sess.Events() {
		switch evt.Type {
		case runtime.EventCompleted:
			text, completed = evt.Text, true
		case runtime.EventFailed:
			t.Fatalf("unexpected failure event: %s", evt.Error)
		}
	}
	if !completed {
		t.Fatal("expected the turn to complete")
	}
	return text
}
