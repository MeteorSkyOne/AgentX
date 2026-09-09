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
