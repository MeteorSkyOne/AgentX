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

// TestRealPiExtension exercises the bundled extension against a real pi:
// ask_user becomes an input request, update_plan becomes a plan snapshot and a
// dangerous command is gated when yolo is off.
func TestRealPiExtension(t *testing.T) {
	if os.Getenv("AGENTX_PI_E2E") != "1" {
		t.Skip("set AGENTX_PI_E2E=1 to run against a real pi binary")
	}
	rt := New(Options{Command: "pi", SessionDir: t.TempDir(), ExtensionDir: t.TempDir(), ProjectTrust: ProjectTrustApprove})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	workspace := t.TempDir()

	session, err := rt.StartSession(ctx, runtime.StartSessionRequest{AgentID: "agent", SessionKey: "agent:channel:ext", Workspace: workspace, YoloMode: false})
	if err != nil {
		t.Fatal(err)
	}
	prompt := "Do these three things in order, using the named tools: " +
		"1) call update_plan with steps 'ask' (in_progress) and 'run' (pending); " +
		"2) call ask_user with question 'Pick a color' and options red, blue; " +
		"3) run `sudo true` with the bash tool. " +
		"Finally reply with exactly: color=<answer> sudo=<ok|blocked>."
	if err := session.Send(ctx, runtime.Input{Prompt: prompt}); err != nil {
		t.Fatal(err)
	}
	var final runtime.Event
	var plan *runtime.PlanState
	answered := map[string]string{}
	for evt := range session.Events() {
		switch evt.Type {
		case runtime.EventInputRequest:
			q := evt.InputRequest
			t.Logf("input request: %q options=%v", q.Question, q.Options)
			answer := "Block"
			if strings.Contains(q.Question, "color") {
				answer = "blue"
			}
			answered[q.Question] = answer
			if err := session.RespondToInputRequest(q.QuestionID, answer); err != nil {
				t.Fatal(err)
			}
		case runtime.EventDelta:
			if evt.Plan != nil {
				plan = evt.Plan
			}
			for _, item := range evt.Process {
				t.Logf("process %s %s %s", item.Type, item.ToolName, item.Status)
			}
		default:
			final = evt
		}
	}
	t.Logf("final: type=%s text=%q err=%q answered=%v", final.Type, final.Text, final.Error, answered)
	if final.Type != runtime.EventCompleted {
		t.Fatalf("final = %+v", final)
	}
	if plan == nil || len(plan.Steps) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if len(answered) != 2 {
		t.Fatalf("expected an ask_user question and a sudo confirmation, got %v", answered)
	}
	lower := strings.ToLower(final.Text)
	if !strings.Contains(lower, "blue") || !strings.Contains(lower, "blocked") {
		t.Fatalf("final text = %q", final.Text)
	}
}

// TestRealPiSubagents delegates to a pi-subagents child through the runtime
// and checks that it runs in the foreground with lifecycle signals and nested
// child tool items. Needs `pi install npm:pi-subagents`.
func TestRealPiSubagents(t *testing.T) {
	if os.Getenv("AGENTX_PI_E2E") != "1" {
		t.Skip("set AGENTX_PI_E2E=1 to run against a real pi binary")
	}
	rt := New(Options{Command: "pi", SessionDir: t.TempDir(), ExtensionDir: t.TempDir(), ProjectTrust: ProjectTrustApprove, SubagentForeground: true})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	workspace := t.TempDir()
	if err := os.WriteFile(workspace+"/NOTES.md", []byte("# Notes\n\nThe secret word is pineapple.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	session, err := rt.StartSession(ctx, runtime.StartSessionRequest{AgentID: "agent", SessionKey: "agent:channel:sub", Workspace: workspace, YoloMode: true})
	if err != nil {
		t.Fatal(err)
	}
	prompt := "Use the subagent tool to delegate this task to the 'scout' agent: \"Read NOTES.md in the working directory and report the secret word.\" " +
		"Do not read the file yourself. When the subagent finishes, reply with exactly: secret=<word>."
	if err := session.Send(ctx, runtime.Input{Prompt: prompt}); err != nil {
		t.Fatal(err)
	}
	var final runtime.Event
	var started, completed, childItems int
	var subagentCallID string
	for evt := range session.Events() {
		switch evt.Type {
		case runtime.EventInputRequest:
			t.Fatalf("unexpected input request: %+v", evt.InputRequest)
		case runtime.EventDelta:
			for _, item := range evt.Process {
				t.Logf("process %s tool=%s status=%s parent=%s", item.Type, item.ToolName, item.Status, item.ParentToolCallID)
				switch item.Type {
				case "subagent_started":
					started++
					subagentCallID = item.ToolCallID
				case "subagent_completed":
					completed++
					if item.Status != "completed" {
						t.Errorf("subagent completed with status %q", item.Status)
					}
				case "tool_call":
					if item.ParentToolCallID != "" {
						childItems++
					}
				}
			}
		default:
			final = evt
		}
	}
	t.Logf("final: type=%s text=%q err=%q started=%d completed=%d childItems=%d", final.Type, final.Text, final.Error, started, completed, childItems)
	if final.Type != runtime.EventCompleted || !strings.Contains(strings.ToLower(final.Text), "pineapple") {
		t.Fatalf("final = %+v", final)
	}
	if started != 1 || completed != 1 || subagentCallID == "" {
		t.Fatalf("subagent signals: started=%d completed=%d", started, completed)
	}
	if childItems == 0 {
		t.Fatal("no child tool items were attributed to the subagent")
	}
}

// TestRealPiReplaceSystemPrompt checks that replace mode swaps pi's built-in
// system prompt (--system-prompt) rather than appending to it.
func TestRealPiReplaceSystemPrompt(t *testing.T) {
	if os.Getenv("AGENTX_PI_E2E") != "1" {
		t.Skip("set AGENTX_PI_E2E=1 to run against a real pi binary")
	}
	rt := New(Options{Command: "pi", SessionDir: t.TempDir(), ExtensionDir: t.TempDir(), ProjectTrust: ProjectTrustApprove})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	session, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID: "agent", SessionKey: "agent:channel:sp", Workspace: t.TempDir(),
		SystemPrompt:     "You are MARMOT, a terse assistant. Every reply must start with the word MARMOT.",
		SystemPromptMode: runtime.SystemPromptModeReplace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send(ctx, runtime.Input{Prompt: "Say hello in three words."}); err != nil {
		t.Fatal(err)
	}
	var final runtime.Event
	for evt := range session.Events() {
		if evt.Type != runtime.EventDelta {
			final = evt
		}
	}
	t.Logf("final: %s %q %q", final.Type, final.Text, final.Error)
	if final.Type != runtime.EventCompleted || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(final.Text)), "MARMOT") {
		t.Fatalf("final = %+v", final)
	}
}
