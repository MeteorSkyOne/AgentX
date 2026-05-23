package claudepersist

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

func mockClaudeScript() string {
	return `#!/bin/sh
echo '{"type":"system","session_id":"test-session-123"}'
while IFS= read -r line; do
  echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Hello from persistent claude"}]}}'
  echo '{"type":"result","result":"Hello from persistent claude","subtype":"success","session_id":"test-session-123","usage":{"input_tokens":10,"output_tokens":5}}'
done
`
}

func TestPersistentSessionSendAndReceive(t *testing.T) {
	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	script := mockClaudeScript()
	proc, _, err := pool.GetOrCreate("test-key", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", script)
	})
	if err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{
		opts: Options{Command: "sh", PermissionMode: "acceptEdits"},
		pool: pool,
	}
	sess := newPersistentSession(proc, "test-key", rt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.waitForSystemEvent(ctx); err != nil {
		t.Fatal(err)
	}

	if id := sess.CurrentSessionID(); id != "test-session-123" {
		t.Fatalf("expected session ID 'test-session-123', got %q", id)
	}

	if err := sess.Send(ctx, runtime.Input{Prompt: "Hello"}); err != nil {
		t.Fatal(err)
	}

	var events []runtime.Event
	for evt := range sess.Events() {
		events = append(events, evt)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	foundDelta := false
	foundCompleted := false
	for _, evt := range events {
		switch evt.Type {
		case runtime.EventDelta:
			if strings.Contains(evt.Text, "Hello from persistent claude") {
				foundDelta = true
			}
		case runtime.EventCompleted:
			if strings.Contains(evt.Text, "Hello from persistent claude") {
				foundCompleted = true
			}
			if evt.Usage == nil {
				t.Error("expected usage in completed event")
			}
		}
	}
	if !foundDelta {
		t.Error("expected a delta event with text")
	}
	if !foundCompleted {
		t.Error("expected a completed event")
	}
}

func TestPersistentSessionProcessDeath(t *testing.T) {
	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("test-key", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `echo '{"type":"system","session_id":"sess1"}'; exit 0`)
	})
	if err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{
		opts: Options{Command: "sh", PermissionMode: "acceptEdits"},
		pool: pool,
	}
	sess := newPersistentSession(proc, "test-key", rt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.waitForSystemEvent(ctx); err != nil {
		t.Fatal(err)
	}

	<-proc.Done()

	if err := sess.Send(ctx, runtime.Input{Prompt: "Hello"}); err != nil {
		t.Fatal(err)
	}

	var lastEvt runtime.Event
	for evt := range sess.Events() {
		lastEvt = evt
	}

	if lastEvt.Type != runtime.EventFailed {
		t.Fatalf("expected EventFailed on dead process, got %v", lastEvt.Type)
	}
	if lastEvt.Error == procpool.ErrProcessDead.Error() {
		t.Fatalf("expected process exit detail, got %q", lastEvt.Error)
	}
	if lastEvt.Error != "persistent process exited" {
		t.Fatalf("failed error = %q, want persistent process exited", lastEvt.Error)
	}
}

func TestPersistentSessionContextUsageControlRequest(t *testing.T) {
	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("test-key", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `echo '{"type":"system","session_id":"sess-context"}'
while IFS= read -r line; do
  case "$line" in
    *get_context_usage*)
      request_id=$(printf '%s\n' "$line" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
      echo '{"type":"control_response","response":{"subtype":"success","request_id":"'"$request_id"'","response":{"totalTokens":76420,"rawMaxTokens":200000,"percentage":38.21,"model":"claude-test"}}}'
      ;;
    *)
      echo '{"type":"assistant","message":{"content":[{"type":"text","text":"after context"}]}}'
      echo '{"type":"result","result":"after context","subtype":"success","session_id":"sess-context"}'
      ;;
  esac
done`)
	})
	if err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{
		opts: Options{Command: "sh", PermissionMode: "acceptEdits"},
		pool: pool,
	}
	sess := newPersistentSession(proc, "test-key", rt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.waitForSystemEvent(ctx); err != nil {
		t.Fatal(err)
	}
	usage, err := sess.ContextUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ptrValue(usage.TotalTokens) != 76420 || ptrValue(usage.ContextWindowTokens) != 200000 || usage.UsedPercent == nil || *usage.UsedPercent != 38.21 {
		t.Fatalf("usage = %#v", usage)
	}
	if usage.Model != "claude-test" || usage.Source != "claude_get_context_usage" {
		t.Fatalf("usage metadata = %#v", usage)
	}
}

func TestStartSessionReturnsStderrWhenProcessDiesDuringInitialization(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "claude")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
echo "unknown option: --bad-flag" >&2
exit 2
`), 0o755); err != nil {
		t.Fatal(err)
	}

	rt := New(Options{Command: script, IdleTimeout: time.Hour})
	defer rt.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:    "agent1",
		SessionKey: "agent1:thread:1",
		Workspace:  dir,
	})
	if err == nil {
		t.Fatal("expected StartSession error")
	}
	if !strings.Contains(err.Error(), "unknown option: --bad-flag") {
		t.Fatalf("StartSession error = %q, want stderr detail", err.Error())
	}
}

func TestInitiateStopDetachesProcessBeforeKill(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "claude")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
echo '{"type":"system","session_id":"test-session"}'
while IFS= read -r line; do
  while IFS= read -r next; do
    :
  done
done
`), 0o755); err != nil {
		t.Fatal(err)
	}

	rt := New(Options{Command: script, IdleTimeout: time.Hour})
	defer rt.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := runtime.StartSessionRequest{
		AgentID:    "agent1",
		SessionKey: "agent1:thread:1",
		Workspace:  dir,
	}
	firstSession, err := rt.StartSession(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	first := firstSession.(*persistentSession)
	if err := first.Send(ctx, runtime.Input{Prompt: "hold"}); err != nil {
		t.Fatal(err)
	}

	first.InitiateStop()
	if got, ok := rt.pool.Get(req.SessionKey); ok && got == first.process {
		t.Fatalf("stopped process remained in pool: %#v", got)
	}

	nextSession, err := rt.StartSession(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	next := nextSession.(*persistentSession)
	if next.process == first.process {
		t.Fatal("expected new session to start a replacement process")
	}

	if err := first.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := next.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestEmitAfterCloseEventStreamDoesNotPanic(t *testing.T) {
	sess := &persistentSession{
		events: make(chan runtime.Event, 1),
		done:   make(chan struct{}),
	}
	sess.closeEventStream()

	for i := 0; i < 1000; i++ {
		sess.emit(runtime.Event{Type: runtime.EventFailed, Error: "late event"})
	}
}

func TestHandleLineKeepsOverwrittenAssistantTextAsProcess(t *testing.T) {
	sess := &persistentSession{events: make(chan runtime.Event, 4)}
	state := newClaudeTurnState()

	terminal, inputReq := sess.handleLine([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"I will inspect the files first."}]}}`), state)
	if terminal || inputReq != nil {
		t.Fatalf("assistant terminal=%v inputReq=%#v", terminal, inputReq)
	}
	evt := <-sess.events
	if evt.Type != runtime.EventDelta || evt.Text != "I will inspect the files first." {
		t.Fatalf("delta = %#v", evt)
	}

	terminal, inputReq = sess.handleLine([]byte(`{"type":"result","result":"Final answer.","subtype":"success","session_id":"s1"}`), state)
	if terminal || inputReq != nil {
		t.Fatalf("result terminal=%v inputReq=%#v", terminal, inputReq)
	}
	evt = state.completionEvent()
	if evt.Type != runtime.EventCompleted || evt.Text != "Final answer." {
		t.Fatalf("completed = %#v", evt)
	}
	if evt.Thinking != "I will inspect the files first." {
		t.Fatalf("thinking = %q, want overwritten assistant text", evt.Thinking)
	}
	if len(evt.Process) != 1 || evt.Process[0].Type != "thinking" || evt.Process[0].Text != "I will inspect the files first." {
		t.Fatalf("process = %#v", evt.Process)
	}
}

func TestPersistentSessionWaitsForQuietPeriodAfterResult(t *testing.T) {
	previousSettleDelay := claudeResultSettleDelay
	claudeResultSettleDelay = 20 * time.Millisecond
	defer func() {
		claudeResultSettleDelay = previousSettleDelay
	}()

	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("test-key", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `echo '{"type":"system","session_id":"sess-late-assistant"}'
while IFS= read -r line; do
  echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Partial report."}]}}'
  echo '{"type":"result","result":"Partial report.","subtype":"success","session_id":"sess-late-assistant"}'
  sleep 0.01
  echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Late report."}]}}'
done`)
	})
	if err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{
		opts: Options{Command: "sh", PermissionMode: "acceptEdits"},
		pool: pool,
	}
	sess := newPersistentSession(proc, "test-key", rt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.waitForSystemEvent(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sess.Send(ctx, runtime.Input{Prompt: "run report"}); err != nil {
		t.Fatal(err)
	}

	var completed runtime.Event
	for evt := range sess.Events() {
		if evt.Type == runtime.EventCompleted {
			completed = evt
		}
	}

	if completed.Type != runtime.EventCompleted {
		t.Fatalf("completed = %#v", completed)
	}
	if !strings.Contains(completed.Text, "Partial report") || !strings.Contains(completed.Text, "Late report") {
		t.Fatalf("completed text = %q", completed.Text)
	}
}

func TestPersistentSessionWaitsForLateToolResultAfterResult(t *testing.T) {
	previousSettleDelay := claudeResultSettleDelay
	claudeResultSettleDelay = 20 * time.Millisecond
	defer func() {
		claudeResultSettleDelay = previousSettleDelay
	}()

	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("test-key", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `echo '{"type":"system","session_id":"sess-late-tool"}'
while IFS= read -r line; do
  echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Two agents finished; third is still running."},{"type":"tool_use","id":"toolu_task_3","name":"Task","input":{"description":"scan projects"}}]}}'
  echo '{"type":"result","result":"Two agents finished; third is still running.","subtype":"success","session_id":"sess-late-tool"}'
  sleep 0.02
  echo '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_task_3","content":"NoKV, AgentX, cc-connect, modern-cpp","is_error":false}]}}'
  echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Agent 3 scanned ~/code projects."}]}}'
done`)
	})
	if err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{
		opts: Options{Command: "sh", PermissionMode: "acceptEdits"},
		pool: pool,
	}
	sess := newPersistentSession(proc, "test-key", rt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.waitForSystemEvent(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sess.Send(ctx, runtime.Input{Prompt: "run subagents"}); err != nil {
		t.Fatal(err)
	}

	var completed runtime.Event
	var sawToolResult bool
	for evt := range sess.Events() {
		if evt.Type == runtime.EventDelta {
			for _, item := range evt.Process {
				if item.Type == "tool_result" && item.ToolCallID == "toolu_task_3" {
					sawToolResult = true
				}
			}
		}
		if evt.Type == runtime.EventCompleted {
			completed = evt
		}
	}

	if !sawToolResult {
		t.Fatal("late tool result was not emitted")
	}
	if completed.Type != runtime.EventCompleted {
		t.Fatalf("completed = %#v", completed)
	}
	if !strings.Contains(completed.Text, "Two agents finished") || !strings.Contains(completed.Text, "Agent 3 scanned") {
		t.Fatalf("completed text = %q", completed.Text)
	}
}

func ptrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
