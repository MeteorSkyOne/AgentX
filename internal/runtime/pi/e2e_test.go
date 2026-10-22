package pi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

// TestRealPiRoundTrip talks to an installed pi binary. It runs only when
// AGENTX_PI_E2E=1 because it needs pi plus configured model credentials.
func TestRealPiRoundTrip(t *testing.T) {
	if os.Getenv("AGENTX_PI_E2E") != "1" {
		t.Skip("set AGENTX_PI_E2E=1 to run against a real pi binary")
	}
	rt := New(Options{Command: "pi", SessionDir: t.TempDir(), ProjectTrust: ProjectTrustApprove})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	req := runtime.StartSessionRequest{AgentID: "agent", SessionKey: "agent:channel:e2e", Workspace: t.TempDir(), Effort: os.Getenv("AGENTX_PI_E2E_EFFORT"), Model: os.Getenv("AGENTX_PI_E2E_MODEL")}
	session, err := rt.StartSession(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("session id: %s", session.CurrentSessionID())
	if err := session.Send(ctx, runtime.Input{Prompt: "Run `echo agentx-ping` with your bash tool, then reply with exactly the word pong and nothing else."}); err != nil {
		t.Fatal(err)
	}
	var final runtime.Event
	for evt := range session.Events() {
		switch evt.Type {
		case runtime.EventDelta:
			for _, item := range evt.Process {
				t.Logf("process %s %s %s", item.Type, item.ToolName, item.Status)
			}
		default:
			final = evt
		}
	}
	t.Logf("final: type=%s text=%q err=%q", final.Type, final.Text, final.Error)
	if final.Usage != nil {
		t.Logf("usage: in=%v out=%v cost=%v ctx=%+v", deref(final.Usage.InputTokens), deref(final.Usage.OutputTokens), final.Usage.TotalCostUSD, final.Usage.Context)
	}
	if final.Type != runtime.EventCompleted || !strings.Contains(strings.ToLower(final.Text), "pong") {
		t.Fatalf("unexpected final event: %+v", final)
	}

	// Resume on the same process, then /compact through the runtime.
	session2, err := rt.StartSession(ctx, runtime.StartSessionRequest{AgentID: "agent", SessionKey: "agent:channel:e2e", PreviousSessionID: session.CurrentSessionID(), Workspace: req.Workspace})
	if err != nil {
		t.Fatal(err)
	}
	if reader, ok := session2.(runtime.ContextUsageReader); ok {
		usage, err := reader.ContextUsage(ctx)
		t.Logf("context usage: %+v err=%v", usage, err)
	}
	if err := session2.Send(ctx, runtime.Input{Prompt: "/compact"}); err != nil {
		t.Fatal(err)
	}
	for evt := range session2.Events() {
		if evt.Type != runtime.EventDelta {
			t.Logf("compact final: type=%s text=%q err=%q", evt.Type, evt.Text, evt.Error)
			if evt.Type != runtime.EventCompleted {
				t.Fatalf("compact failed: %+v", evt)
			}
			if !strings.Contains(strings.ToLower(evt.Text), "compact") {
				t.Fatalf("compact reply = %q, want a compaction report", evt.Text)
			}
		}
	}
}

func deref(v *int64) int64 {
	if v == nil {
		return -1
	}
	return *v
}
