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

	sess.waitForSystemEvent(ctx)

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

	sess.waitForSystemEvent(ctx)

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
}
