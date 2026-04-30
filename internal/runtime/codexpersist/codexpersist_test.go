package codexpersist

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

func TestInitializeParamsEnableExperimentalAPI(t *testing.T) {
	params := initializeParams()

	capabilities, ok := params["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities = %#v, want map", params["capabilities"])
	}
	if got := capabilities["experimentalApi"]; got != true {
		t.Fatalf("experimentalApi = %v, want true", got)
	}
}

func TestRPCCallReturnsProcessDeadWhenRequestChannelCloses(t *testing.T) {
	pool := procpool.New(procpool.Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("codex-test", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "read _; exit 0")
	})
	if err != nil {
		t.Fatal(err)
	}

	rpc := newRPCClient(proc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = rpc.Call(ctx, "initialize", map[string]any{})
	if !errors.Is(err, procpool.ErrProcessDead) {
		t.Fatalf("Call error = %v, want ErrProcessDead", err)
	}
}
