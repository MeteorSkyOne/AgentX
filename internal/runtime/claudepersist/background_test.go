package claudepersist

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

// runTurn drives one Send against a mock claude process and returns every event.
func runTurn(t *testing.T, key string, script string) []runtime.Event {
	t.Helper()

	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	t.Cleanup(func() { pool.Shutdown(context.Background()) })

	proc, _, err := pool.GetOrCreate(key, func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", script)
	})
	if err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{opts: Options{Command: "sh", PermissionMode: "acceptEdits"}, pool: pool}
	sess := newPersistentSession(proc, key, rt)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := sess.waitForSystemEvent(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sess.Send(ctx, runtime.Input{Prompt: "go"}); err != nil {
		t.Fatal(err)
	}

	var events []runtime.Event
	for evt := range sess.Events() {
		events = append(events, evt)
	}
	return events
}

func completedEvent(t *testing.T, events []runtime.Event) runtime.Event {
	t.Helper()
	var found []runtime.Event
	for _, evt := range events {
		switch evt.Type {
		case runtime.EventCompleted:
			found = append(found, evt)
		case runtime.EventFailed:
			t.Fatalf("unexpected failure event: %s", evt.Error)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 completed event, got %d", len(found))
	}
	return found[0]
}

// A background subagent keeps working after the main agent's result arrives,
// and Claude Code then wakes the main agent to report on it. The turn must stay
// open across both.
func TestTurnStaysOpenUntilBackgroundSubagentReports(t *testing.T) {
	script := `
echo '{"type":"system","session_id":"sess-bg-turn"}'
read -r line
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_task1","name":"Agent","input":{"subagent_type":"Explore","run_in_background":true}}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"launched"}]}}'
echo '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_task1","content":"Async agent launched"}]}}'
echo '{"type":"system","subtype":"task_started","task_id":"task-1","tool_use_id":"toolu_task1","subagent_type":"Explore"}'
echo '{"type":"result","result":"launched","subtype":"success"}'
sleep 0.4
echo '{"type":"assistant","parent_tool_use_id":"toolu_task1","message":{"content":[{"type":"text","text":"SUBAGENT INTERNAL CHATTER"}]}}'
echo '{"type":"assistant","parent_tool_use_id":"toolu_task1","message":{"content":[{"type":"tool_use","id":"toolu_sub1","name":"Bash","input":{}}]}}'
sleep 0.2
echo '{"type":"system","subtype":"task_notification","task_id":"task-1","status":"completed","summary":"did the thing"}'
sleep 0.3
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"the subagent finished and here is the report"}]}}'
echo '{"type":"result","result":"the subagent finished and here is the report","subtype":"success"}'
sleep 10
`
	events := runTurn(t, "bg-turn", script)
	completed := completedEvent(t, events)

	if !strings.Contains(completed.Text, "launched") {
		t.Errorf("completed text lost the pre-subagent reply: %q", completed.Text)
	}
	if !strings.Contains(completed.Text, "the subagent finished and here is the report") {
		t.Errorf("completed text lost the post-subagent reply: %q", completed.Text)
	}
	if strings.Contains(completed.Text, "SUBAGENT INTERNAL CHATTER") {
		t.Errorf("subagent chatter leaked into the main reply: %q", completed.Text)
	}

	var tagged int
	for _, evt := range events {
		for _, item := range evt.Process {
			if item.ParentToolCallID == "toolu_task1" {
				tagged++
			}
		}
	}
	if tagged == 0 {
		t.Error("expected subagent process items to be tagged with their parent tool call")
	}
}

// A foreground subagent finishes before the result, so the turn must not be
// held open waiting on anything.
func TestForegroundSubagentTurnCompletesPromptly(t *testing.T) {
	script := `
echo '{"type":"system","session_id":"sess-fg"}'
read -r line
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_fg","name":"Agent","input":{"subagent_type":"Explore"}}]}}'
echo '{"type":"system","subtype":"task_started","task_id":"task-fg","tool_use_id":"toolu_fg"}'
echo '{"type":"assistant","parent_tool_use_id":"toolu_fg","message":{"content":[{"type":"tool_use","id":"toolu_sub","name":"Bash","input":{}}]}}'
echo '{"type":"system","subtype":"task_updated","task_id":"task-fg","patch":{"status":"completed"}}'
echo '{"type":"system","subtype":"task_notification","task_id":"task-fg","status":"completed","summary":"counted"}'
echo '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_fg","content":"40"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"there are 40 files"}]}}'
echo '{"type":"result","result":"there are 40 files","subtype":"success"}'
sleep 10
`
	start := time.Now()
	events := runTurn(t, "fg", script)
	elapsed := time.Since(start)

	completed := completedEvent(t, events)
	if !strings.Contains(completed.Text, "there are 40 files") {
		t.Errorf("unexpected completed text: %q", completed.Text)
	}
	if elapsed > 5*time.Second {
		t.Errorf("foreground subagent turn took %s, expected it to finish promptly", elapsed)
	}
}

// A background task that never reports must not hang the turn forever.
func TestSilentBackgroundTaskFallsBackToIdleTimeout(t *testing.T) {
	original := claudeBackgroundIdleTimeout
	claudeBackgroundIdleTimeout = 400 * time.Millisecond
	t.Cleanup(func() { claudeBackgroundIdleTimeout = original })

	script := `
echo '{"type":"system","session_id":"sess-stuck"}'
read -r line
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"launched"}]}}'
echo '{"type":"system","subtype":"task_started","task_id":"task-stuck","tool_use_id":"toolu_stuck"}'
echo '{"type":"result","result":"launched","subtype":"success"}'
sleep 30
`
	start := time.Now()
	events := runTurn(t, "stuck", script)
	elapsed := time.Since(start)

	completed := completedEvent(t, events)
	if !strings.Contains(completed.Text, "launched") {
		t.Errorf("unexpected completed text: %q", completed.Text)
	}
	if elapsed > 10*time.Second {
		t.Errorf("silent background task hung the turn for %s", elapsed)
	}
}

// Whitespace-only assistant text must not open a process break, or the stored
// message ends up with an empty, doubled marker.
func TestBlankAssistantTextDoesNotEmitProcessBreak(t *testing.T) {
	script := `
echo '{"type":"system","session_id":"sess-blank"}'
read -r line
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"first"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_x","name":"Bash","input":{}}]}}'
echo '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_x","content":"ok"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"   "}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"second"}]}}'
echo '{"type":"result","result":"second","subtype":"success"}'
sleep 10
`
	events := runTurn(t, "blank", script)
	completed := completedEvent(t, events)

	if n := strings.Count(completed.Text, "process-break"); n != 1 {
		t.Errorf("expected exactly 1 process break, got %d in %q", n, completed.Text)
	}
	if !strings.Contains(completed.Text, "first") || !strings.Contains(completed.Text, "second") {
		t.Errorf("completed text lost content: %q", completed.Text)
	}
}
