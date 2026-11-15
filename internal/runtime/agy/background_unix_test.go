//go:build unix

package agy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestStopBackgroundTaskKillsChildProcessGroupAndMarksStopped(t *testing.T) {
	script := writeFakeBackgroundAgyScript(t)
	tempHome := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "child.pid")

	rt := New(Options{
		Command: script,
		Env: map[string]string{
			"HOME":                    tempHome,
			"FAKE_AGY_CHILD_PID_FILE": pidFile,
		},
		pollInterval: testBrainPollInterval,
	})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	taskReady := make(chan struct{}, 1)
	var (
		taskMu sync.Mutex
		latest []runtime.BackgroundTask
	)
	rt.SetBackgroundTaskHandler(func(_ string, tasks []runtime.BackgroundTask) {
		taskMu.Lock()
		latest = append([]runtime.BackgroundTask(nil), tasks...)
		if len(tasks) > 0 && tasks[0].Status == runtime.BackgroundTaskRunning {
			select {
			case taskReady <- struct{}{}:
			default:
			}
		}
		taskMu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := "agent-agy:channel:stop-test"
	sess, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:    "agent-agy",
		SessionKey: key,
		Workspace:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Send(ctx, runtime.Input{Prompt: "SPAWN_REAL_SLEEP"}); err != nil {
		t.Fatal(err)
	}
	for range sess.Events() {
	}

	<-taskReady
	pidRaw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(pidRaw)))
	if err != nil || childPID <= 0 {
		t.Fatalf("invalid child pid %q: %v", string(pidRaw), err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-childPID, syscall.SIGKILL) })

	if err := syscall.Kill(childPID, 0); err != nil {
		t.Fatalf("expected child sleep process %d to be running: %v", childPID, err)
	}

	if err := rt.StopBackgroundTask(ctx, key, "task-9"); err != nil {
		t.Fatalf("StopBackgroundTask error: %v", err)
	}

	// Child process must be gone shortly after StopBackgroundTask returns.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(childPID, 0); errors.Is(err, syscall.ESRCH) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(childPID, 0); err == nil {
		t.Fatalf("child process %d is still alive after StopBackgroundTask", childPID)
	}

	taskMu.Lock()
	stoppedStatus := ""
	if len(latest) == 1 {
		stoppedStatus = latest[0].Status
	}
	taskMu.Unlock()
	if stoppedStatus != "stopped" {
		t.Fatalf("expected stopped task status, got %+v", latest)
	}

	if err := rt.StopBackgroundTask(ctx, key, "task-9"); !errors.Is(err, errNoSuchBackgroundTask) {
		t.Fatalf("stopping already-stopped task: got %v, want %v", err, errNoSuchBackgroundTask)
	}
}
