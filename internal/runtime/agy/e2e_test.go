package agy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

// TestRealAgyRoundTrip talks to an installed `agy` binary over ACP (`agy acp`).
// It runs only when AGENTX_AGY_E2E=1 because it requires an authenticated Antigravity account.
func TestRealAgyRoundTrip(t *testing.T) {
	if os.Getenv("AGENTX_AGY_E2E") != "1" {
		t.Skip("set AGENTX_AGY_E2E=1 to run against a real agy binary")
	}
	cmd := strings.TrimSpace(os.Getenv("AGENTX_AGY_COMMAND"))
	if cmd == "" {
		cmd = "agy"
	}
	model := strings.TrimSpace(os.Getenv("AGENTX_AGY_E2E_MODEL"))
	if model == "" {
		model = "gemini-3.8-flash"
	}

	rt := New(Options{Command: cmd})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "probe.txt"), []byte("antigravity-e2e-token-777\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := runtime.StartSessionRequest{
		AgentID:    "agent-agy",
		SessionKey: "agent-agy:channel:e2e",
		Workspace:  workspace,
		Model:      model,
		Effort:     "low",
		YoloMode:   true,
	}
	session, err := rt.StartSession(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send(ctx, runtime.Input{
		Prompt: "Read probe.txt in the workspace using read_file and reply with ONLY its exact contents.",
	}); err != nil {
		t.Fatal(err)
	}

	var final runtime.Event
	var sawTool bool
	for evt := range session.Events() {
		switch evt.Type {
		case runtime.EventDelta:
			for _, item := range evt.Process {
				t.Logf("process item: type=%s tool=%s status=%s", item.Type, item.ToolName, item.Status)
				if item.Type == "tool_call" {
					sawTool = true
				}
			}
		default:
			final = evt
		}
	}
	sid := session.CurrentSessionID()
	t.Logf("turn 1 finished: sid=%s type=%s text=%q err=%q", sid, final.Type, final.Text, final.Error)
	if final.Type != runtime.EventCompleted {
		t.Fatalf("unexpected final event: %+v", final)
	}
	if !sawTool {
		t.Fatalf("expected at least one tool_call in process timeline")
	}
	if !strings.Contains(final.Text, "antigravity-e2e-token-777") {
		t.Fatalf("reply = %q, want antigravity-e2e-token-777", final.Text)
	}
	if final.Usage == nil || final.Usage.InputTokens == nil || final.Usage.Context == nil {
		t.Fatalf("expected token and context usage on completed event, got %+v", final.Usage)
	}

	// Verify ContextUsage reader (/status) and session resume across process restart.
	if err := rt.ResetProcesses(ctx); err != nil {
		t.Fatal(err)
	}
	session2, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:           "agent-agy",
		SessionKey:        "agent-agy:channel:e2e",
		Workspace:         workspace,
		Model:             model,
		Effort:            "low",
		YoloMode:          true,
		PreviousSessionID: sid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reader, ok := session2.(runtime.ContextUsageReader); ok {
		cu, err := reader.ContextUsage(ctx)
		t.Logf("resumed context usage: %+v err=%v", cu, err)
		if err != nil {
			t.Fatalf("ContextUsage error: %v", err)
		}
	}
	if err := session2.Send(ctx, runtime.Input{
		Prompt: "Without reading the file again, what was the token from probe.txt in our previous turn?",
	}); err != nil {
		t.Fatal(err)
	}
	var final2 runtime.Event
	for evt := range session2.Events() {
		if evt.Type != runtime.EventDelta {
			final2 = evt
		}
	}
	t.Logf("turn 2 finished: type=%s text=%q err=%q", final2.Type, final2.Text, final2.Error)
	if final2.Type != runtime.EventCompleted || !strings.Contains(final2.Text, "777") {
		t.Fatalf("resumed turn failed to recall token: %+v", final2)
	}
}
