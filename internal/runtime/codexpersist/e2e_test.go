package codexpersist

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

// TestRealCodexReplaceSystemPrompt checks that a replacement system prompt
// reaches the codex app-server via model_instructions_file. It runs only with
// AGENTX_CODEX_E2E=1 because it needs the codex CLI and credentials.
func TestRealCodexReplaceSystemPrompt(t *testing.T) {
	if os.Getenv("AGENTX_CODEX_E2E") != "1" {
		t.Skip("set AGENTX_CODEX_E2E=1 to run against a real codex binary")
	}
	rt := New(Options{Command: "codex"})
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
