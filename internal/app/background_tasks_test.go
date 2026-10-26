package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

type backgroundTaskRuntime struct {
	backgroundTurnRuntime
	mu      sync.Mutex
	report  func(sessionKey string, tasks []agentruntime.BackgroundTask)
	stopped []string
}

func (r *backgroundTaskRuntime) SetBackgroundTaskHandler(handler func(sessionKey string, tasks []agentruntime.BackgroundTask)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.report = handler
}

func (r *backgroundTaskRuntime) StopBackgroundTask(ctx context.Context, sessionKey string, taskID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = append(r.stopped, sessionKey+"/"+taskID)
	return nil
}

func TestBackgroundTasksArePublishedListedReadAndStopped(t *testing.T) {
	rt := &backgroundTaskRuntime{}
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bus := eventbus.New()
	app := New(st, bus, Options{AdminToken: "secret", DataDir: t.TempDir(), Runtimes: map[string]agentruntime.Runtime{domain.AgentKindFake: rt}})
	defer shutdownAppForTest(t, app)
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{OrganizationID: bootstrap.Organization.ID, ConversationID: bootstrap.Channel.ID})
	defer unsubscribe()

	output := filepath.Join(t.TempDir(), "b1.output")
	long := strings.Repeat("old line\n", backgroundTaskOutputLimit/9+10) + "newest line\n"
	if err := os.WriteFile(output, []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	key := agentSessionKey(bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID)
	rt.report(key, []agentruntime.BackgroundTask{
		{ID: "b1", Kind: agentruntime.BackgroundTaskShell, Description: "Build", Status: agentruntime.BackgroundTaskRunning, OutputFile: output, StartedAt: started},
		{ID: "a1", Kind: agentruntime.BackgroundTaskAgent, Status: "completed", StartedAt: started.Add(time.Second), EndedAt: started.Add(2 * time.Second)},
	})

	select {
	case evt := <-events:
		payload, ok := evt.Payload.(domain.AgentBackgroundTasksPayload)
		if evt.Type != domain.EventAgentBackgroundTasksUpdated || !ok || payload.AgentID != bootstrap.Agent.ID || len(payload.Tasks) != 2 {
			t.Fatalf("event = %s %#v", evt.Type, evt.Payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("background task update was not published")
	}

	tasks := app.ListBackgroundTasks(domain.ConversationChannel, bootstrap.Channel.ID)
	if len(tasks) != 2 || tasks[0].ID != "b1" || !tasks[0].HasOutput || tasks[0].EndedAt != nil || tasks[1].EndedAt == nil || tasks[1].HasOutput {
		t.Fatalf("tasks = %#v", tasks)
	}
	if other := app.ListBackgroundTasks(domain.ConversationThread, bootstrap.Channel.ID); len(other) != 0 {
		t.Fatalf("tasks leaked into another conversation: %#v", other)
	}

	read, err := app.ReadBackgroundTaskOutput(domain.ConversationChannel, bootstrap.Channel.ID, bootstrap.Agent.ID, "b1")
	if err != nil {
		t.Fatal(err)
	}
	if !read.Truncated || !strings.HasSuffix(read.Output, "newest line\n") || !strings.HasPrefix(read.Output, "old line\n") || len(read.Output) > backgroundTaskOutputLimit {
		t.Fatalf("output tail: truncated=%v len=%d start=%q", read.Truncated, len(read.Output), read.Output[:20])
	}
	if _, err := app.ReadBackgroundTaskOutput(domain.ConversationChannel, bootstrap.Channel.ID, bootstrap.Agent.ID, "a1"); !errors.Is(err, ErrBackgroundTaskNotFound) {
		t.Fatalf("task without output: err = %v", err)
	}

	if err := app.StopBackgroundTask(ctx, domain.ConversationChannel, bootstrap.Channel.ID, bootstrap.Agent.ID, "b1"); err != nil {
		t.Fatal(err)
	}
	if err := app.StopBackgroundTask(ctx, domain.ConversationChannel, bootstrap.Channel.ID, bootstrap.Agent.ID, "a1"); !errors.Is(err, ErrBackgroundTaskNotFound) {
		t.Fatalf("stopping an ended task: err = %v", err)
	}
	if len(rt.stopped) != 1 || rt.stopped[0] != key+"/b1" {
		t.Fatalf("stopped = %v", rt.stopped)
	}
}
