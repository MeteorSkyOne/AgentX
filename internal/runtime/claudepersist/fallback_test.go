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
	proc.SetFallbackHandler(newBackgroundFallback(proc, "bg-key"))

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
