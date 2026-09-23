package claudepersist

import (
	"slices"
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestBuildArgsEnablesPrintModeForStreamJSON(t *testing.T) {
	rt := New(Options{})
	defer rt.pool.Shutdown(t.Context())

	args := rt.buildArgs(runtime.StartSessionRequest{})
	if !slices.Contains(args, "--print") {
		t.Fatalf("args = %q, want --print for stream-json input", args)
	}
}

func TestBuildArgsRequestsThinkingDisplay(t *testing.T) {
	rt := New(Options{ThinkingDisplay: "summarized"})
	defer rt.pool.Shutdown(t.Context())

	args := rt.buildArgs(runtime.StartSessionRequest{})
	i := slices.Index(args, "--thinking-display")
	if i < 0 || i+1 >= len(args) || args[i+1] != "summarized" {
		t.Fatalf("args = %q, want --thinking-display summarized", args)
	}

	rt = New(Options{})
	defer rt.pool.Shutdown(t.Context())
	if args := rt.buildArgs(runtime.StartSessionRequest{}); slices.Contains(args, "--thinking-display") {
		t.Fatalf("args = %q, want no --thinking-display when unset", args)
	}
}
