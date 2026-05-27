package app

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/config"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

func TestRunToolUpdateResetsPersistentRuntimeWhenIdle(t *testing.T) {
	var resets atomic.Int32
	app := newToolUpdateTestApp(t, &resetRuntime{resets: &resets})

	overview, err := app.RunToolUpdate(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if resets.Load() != 1 {
		t.Fatalf("resets = %d, want 1", resets.Load())
	}
	status := overview.Tools[0]
	if status.CurrentVersion != "2.1.2" || status.RuntimeResetPending {
		t.Fatalf("status = %#v", status)
	}
}

func TestRunToolUpdateDefersRuntimeResetUntilActiveRunEnds(t *testing.T) {
	var resets atomic.Int32
	app := newToolUpdateTestApp(t, &resetRuntime{resets: &resets})
	key := activeRunKey{conversationType: domain.ConversationChannel, conversationID: "c1", agentID: "a1"}
	app.activeRunsMu.Lock()
	app.activeRuns[key] = map[string]*activeAgentRun{
		"run1": {runID: "run1", provider: toolUpdateProviderClaude},
	}
	app.activeRunsMu.Unlock()

	overview, err := app.RunToolUpdate(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if resets.Load() != 0 {
		t.Fatalf("resets = %d, want deferred", resets.Load())
	}
	if !overview.Tools[0].RuntimeResetPending {
		t.Fatalf("runtime_reset_pending = false, want true")
	}

	app.removeActiveAgentRun(key, "run1")
	app.handleToolUpdateAgentRunTerminated(context.Background(), toolUpdateProviderClaude)
	if resets.Load() != 1 {
		t.Fatalf("resets after run end = %d, want 1", resets.Load())
	}
}

func TestRunToolUpdateAllUpdatesProvidersConcurrently(t *testing.T) {
	var activeUpdates atomic.Int32
	var maxActiveUpdates atomic.Int32
	updateEntered := make(chan string, 2)
	releaseUpdates := make(chan struct{})
	exec := func(ctx context.Context, name string, args ...string) (string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "claude --version":
			return "2.1.1 (Claude Code)", nil
		case "codex --version":
			return "codex-cli 1.0.0", nil
		case "npm view @anthropic-ai/claude-code version":
			return "2.1.2", nil
		case "npm view @openai/codex version":
			return "1.0.1", nil
		case "claude update", "codex update":
			current := activeUpdates.Add(1)
			for {
				old := maxActiveUpdates.Load()
				if current <= old || maxActiveUpdates.CompareAndSwap(old, current) {
					break
				}
			}
			updateEntered <- name
			select {
			case <-releaseUpdates:
				activeUpdates.Add(-1)
				return "updated", nil
			case <-ctx.Done():
				activeUpdates.Add(-1)
				return "", ctx.Err()
			}
		default:
			return "", nil
		}
	}
	app := New(nil, eventbus.New(), Options{
		DataDir: t.TempDir(),
		ToolUpdateSettings: config.ToolUpdateSettings{
			TimeOfDay:     "04:00",
			Timezone:      "UTC",
			ClaudeEnabled: true,
			CodexEnabled:  true,
		},
		ToolUpdates: ToolUpdateOptions{
			ClaudeCommand: "claude",
			CodexCommand:  "codex",
			Exec:          exec,
		},
	})

	done := make(chan error, 1)
	go func() {
		_, err := app.RunToolUpdate(context.Background(), "all")
		done <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-updateEntered:
		case <-time.After(time.Second):
			close(releaseUpdates)
			t.Fatal("timed out waiting for both providers to enter update")
		}
	}
	if maxActiveUpdates.Load() != 2 {
		t.Fatalf("max active updates = %d, want 2", maxActiveUpdates.Load())
	}
	close(releaseUpdates)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestStartRunToolUpdateKeepsUpdatingStateDuringCheck(t *testing.T) {
	app := newToolUpdateTestApp(t, &resetRuntime{})
	if !app.toolUpdates.beginUpdate(toolUpdateProviderClaude) {
		t.Fatal("beginUpdate = false, want true")
	}
	if err := app.toolUpdates.checkStarted(context.Background(), toolUpdateProviderClaude, "updating"); err != nil {
		t.Fatal(err)
	}
	overview, err := app.ToolUpdateOverview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	status := overview.Tools[0]
	if status.State != "updating" {
		t.Fatalf("state = %q, want updating", status.State)
	}
	if app.toolUpdates.beginUpdate(toolUpdateProviderClaude) {
		t.Fatal("beginUpdate allowed a second update while state is updating")
	}
}

func TestNewToolUpdateServiceFallsBackForInvalidSettings(t *testing.T) {
	service := newToolUpdateService(nil, t.TempDir(), nil, ToolUpdateOptions{
		Settings: config.ToolUpdateSettings{
			AutoEnabled:   true,
			TimeOfDay:     "0400",
			Timezone:      "UTC",
			ClaudeEnabled: true,
			CodexEnabled:  true,
		},
	})
	if service.settings.TimeOfDay != "04:00" {
		t.Fatalf("time_of_day = %q, want default fallback", service.settings.TimeOfDay)
	}
	if spec := toolUpdateCronSpec(service.settings); !strings.Contains(spec, " 00 04 ") {
		t.Fatalf("cron spec = %q, want default 04:00", spec)
	}
}

func TestRuntimeResetBlocksNewRunRegistration(t *testing.T) {
	rt := &blockingResetRuntime{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	app := newToolUpdateTestApp(t, rt)
	app.toolUpdates.mu.Lock()
	app.toolUpdates.stateLocked(toolUpdateProviderClaude).RuntimeResetPending = true
	app.toolUpdates.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		app.resetUpdatedRuntimeIfIdle(context.Background(), toolUpdateProviderClaude)
	}()
	<-rt.entered

	registered := make(chan struct{})
	attempting := make(chan struct{})
	go func() {
		close(attempting)
		app.registerActiveAgentRun(activeRunKey{conversationType: domain.ConversationChannel, conversationID: "c1", agentID: "a1"}, &activeAgentRun{
			runID:    "run1",
			provider: toolUpdateProviderClaude,
		})
		close(registered)
	}()
	<-attempting
	time.Sleep(10 * time.Millisecond)

	select {
	case <-registered:
		t.Fatal("registerActiveAgentRun completed while runtime reset was in progress")
	default:
	}
	close(rt.release)
	<-done
	<-registered
}

func newToolUpdateTestApp(t *testing.T, rt agentruntime.Runtime) *App {
	t.Helper()
	exec := func(_ context.Context, name string, args ...string) (string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "claude --version":
			return "2.1.1 (Claude Code)", nil
		case "npm view @anthropic-ai/claude-code version":
			return "2.1.2", nil
		case "claude update":
			return "updated", nil
		default:
			return "", nil
		}
	}
	return New(nil, eventbus.New(), Options{
		DataDir: t.TempDir(),
		ToolUpdateSettings: config.ToolUpdateSettings{
			AutoEnabled:   false,
			TimeOfDay:     "04:00",
			Timezone:      "Local",
			ClaudeEnabled: true,
			CodexEnabled:  true,
		},
		Runtimes: map[string]agentruntime.Runtime{
			domain.AgentKindClaudePersistent: rt,
		},
		ToolUpdates: ToolUpdateOptions{
			ClaudeCommand: "claude",
			CodexCommand:  "codex",
			Exec:          exec,
		},
	})
}

type resetRuntime struct {
	resets *atomic.Int32
}

func (r *resetRuntime) StartSession(context.Context, agentruntime.StartSessionRequest) (agentruntime.Session, error) {
	return nil, nil
}

func (r *resetRuntime) ResetProcesses(context.Context) error {
	if r.resets != nil {
		r.resets.Add(1)
	}
	return nil
}

type blockingResetRuntime struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (r *blockingResetRuntime) StartSession(context.Context, agentruntime.StartSessionRequest) (agentruntime.Session, error) {
	return nil, nil
}

func (r *blockingResetRuntime) ResetProcesses(context.Context) error {
	r.once.Do(func() {
		close(r.entered)
	})
	<-r.release
	return nil
}
