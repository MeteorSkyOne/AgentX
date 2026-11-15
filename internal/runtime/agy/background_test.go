package agy

import (
	"context"
	"encoding/json"
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

const testBrainPollInterval = 15 * time.Millisecond

func writeFakeBackgroundAgyScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-bg-agy")
	body := `#!/usr/bin/env python3
import json, os, subprocess, sys, threading, time

home = os.environ["HOME"]
app_data = os.path.join(home, ".gemini", "antigravity-cli")
pid_file = os.environ.get("FAKE_AGY_CHILD_PID_FILE", "")

def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()

def brain_logs_dir(sid):
    d = os.path.join(app_data, "brain", sid, ".system_generated", "logs")
    os.makedirs(d, exist_ok=True)
    return d

def brain_tasks_dir(sid):
    d = os.path.join(app_data, "brain", sid, ".system_generated", "tasks")
    os.makedirs(d, exist_ok=True)
    return d

def append_step(sid, step):
    p = os.path.join(brain_logs_dir(sid), "transcript_full.jsonl")
    with open(p, "a") as f:
        f.write(json.dumps(step) + "\n")
        f.flush()

sessions = {}
counter = 0

for raw in sys.stdin:
    raw = raw.strip()
    if not raw:
        continue
    msg = json.loads(raw)
    method = msg.get("method", "")
    req_id = msg.get("id")
    params = msg.get("params") or {}

    if method == "initialize":
        send({"jsonrpc": "2.0", "id": req_id, "result": {"protocolVersion": 1, "agentCapabilities": {"loadSession": True}}})
        continue

    if method == "session/new":
        counter += 1
        sid = f"sid-bg-{counter}"
        sessions[sid] = True
        send({"jsonrpc": "2.0", "id": req_id, "result": {"sessionId": sid, "configOptions": []}})
        continue

    if method == "session/resume":
        sid = params.get("sessionId", "")
        sessions[sid] = True
        send({"jsonrpc": "2.0", "id": req_id, "result": {"sessionId": sid, "configOptions": []}})
        continue

    if method in ("session/close", "session/set_mode", "session/set_model", "session/set_config_option"):
        send({"jsonrpc": "2.0", "id": req_id, "result": {}})
        continue

    if method == "session/cancel":
        continue

    if method == "session/prompt":
        sid = params.get("sessionId", "")
        blocks = params.get("prompt") or []
        text = blocks[0].get("text", "") if blocks else ""

        if "SPAWN_BG_WAKEUP" in text:
            log_path = os.path.join(brain_tasks_dir(sid), "task-2.log")
            with open(log_path, "w") as lf:
                lf.write("Building web...\n")
            send({
                "jsonrpc": "2.0",
                "method": "session/update",
                "params": {
                    "sessionId": sid,
                    "update": {
                        "sessionUpdate": "tool_call",
                        "toolCallId": "call_bg_1",
                        "name": "run_command",
                        "args": {"CommandLine": "make build", "WaitMsBeforeAsync": 500, "toolSummary": "Rebuild server"},
                        "status": "in_progress",
                    },
                },
            })
            append_step(sid, {"step_index": 0, "source": "USER_EXPLICIT", "type": "USER_INPUT", "status": "DONE", "created_at": "2026-11-15T15:00:00Z", "content": text})
            append_step(sid, {"step_index": 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "status": "DONE", "created_at": "2026-11-15T15:00:01Z", "tool_calls": [{"name": "run_command", "args": {"CommandLine": "make build", "WaitMsBeforeAsync": 500, "toolSummary": "Rebuild server"}}]})
            append_step(sid, {"step_index": 2, "source": "MODEL", "type": "GENERIC", "status": "RUNNING", "created_at": "2026-11-15T15:00:02Z", "content": f"Created At: 2026-11-15T23:00:02+08:00\nTool is running as a background task with task id: {sid}/task-2\nTask Description: make build\nTask logs are available at: file://{log_path}\n"})
            append_step(sid, {"step_index": 3, "source": "MODEL", "type": "PLANNER_RESPONSE", "status": "DONE", "created_at": "2026-11-15T15:00:02Z", "content": "Building in background..."})
            send({
                "jsonrpc": "2.0",
                "method": "session/update",
                "params": {
                    "sessionId": sid,
                    "update": {
                        "sessionUpdate": "agent_message_chunk",
                        "content": {"type": "text", "text": "Building in background..."},
                    },
                },
            })
            send({
                "jsonrpc": "2.0",
                "method": "session/update",
                "params": {
                    "sessionId": sid,
                    "update": {"sessionUpdate": "usage_update", "used": 12000, "size": 1000000},
                },
            })
            send({
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {"stopReason": "end_turn", "usage": {"inputTokens": 100, "outputTokens": 10, "totalTokens": 110}},
            })

            def delayed_wakeup():
                time.sleep(0.08)
                with open(log_path, "a") as lf:
                    lf.write("Build complete!\n")
                append_step(sid, {
                    "step_index": 4,
                    "source": "SYSTEM",
                    "type": "SYSTEM_MESSAGE",
                    "status": "DONE",
                    "created_at": "2026-11-15T15:00:05Z",
                    "content": f"The following is a <SYSTEM_MESSAGE>\n<SYSTEM_MESSAGE>\n[Message] timestamp=2026-11-15T15:00:05Z sender={sid}/task-2 priority=MESSAGE_PRIORITY_HIGH content=Task id \"{sid}/task-2\" finished with result:\nCommand exited with code 0.\n\nCommand output:\nBuild complete!\nLog: file://{log_path}\n</SYSTEM_MESSAGE>",
                })
                append_step(sid, {"step_index": 5, "source": "SYSTEM", "type": "CHECKPOINT", "status": "DONE", "created_at": "2026-11-15T15:00:05Z", "content": "compacted"})
                time.sleep(0.03)
                append_step(sid, {
                    "step_index": 6,
                    "source": "MODEL",
                    "type": "PLANNER_RESPONSE",
                    "status": "DONE",
                    "created_at": "2026-11-15T15:00:06Z",
                    "model": "gemini-4-carbon-high",
                    "input_tokens": 500,
                    "cache_read_tokens": 2000,
                    "output_tokens": 40,
                    "thinking": "Verifying built binary.",
                    "content": "Checking artifact...",
                    "tool_calls": [{"name": "read_file", "args": {"Path": "/tmp/agentx"}}],
                })
                append_step(sid, {
                    "step_index": 7,
                    "source": "MODEL",
                    "type": "GENERIC",
                    "status": "DONE",
                    "created_at": "2026-11-15T15:00:06Z",
                    "content": "Created At: 2026-11-15T23:00:06+08:00\nCompleted At: 2026-11-15T23:00:06+08:00\nbinary-ok",
                })
                append_step(sid, {
                    "step_index": 8,
                    "source": "MODEL",
                    "type": "PLANNER_RESPONSE",
                    "status": "DONE",
                    "created_at": "2026-11-15T15:00:07Z",
                    "model": "gemini-4-carbon-high",
                    "input_tokens": 600,
                    "cache_read_tokens": 2100,
                    "output_tokens": 35,
                    "content": "Build succeeded and binary is ready.",
                })

            threading.Thread(target=delayed_wakeup, daemon=True).start()
            continue

        if "SPAWN_FAST_INLINE_BG" in text:
            log_path = os.path.join(brain_tasks_dir(sid), "task-fast.log")
            append_step(sid, {"step_index": 0, "source": "USER_EXPLICIT", "type": "USER_INPUT", "status": "DONE", "content": text})
            append_step(sid, {"step_index": 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "status": "DONE", "tool_calls": [{"name": "run_command", "args": {"CommandLine": "sleep 1", "toolSummary": "Quick sleep"}}]})
            append_step(sid, {"step_index": 2, "source": "MODEL", "type": "GENERIC", "status": "RUNNING", "content": f"Tool is running as a background task with task id: {sid}/task-fast\nTask Description: sleep 1\nTask logs are available at: file://{log_path}\n"})
            append_step(sid, {"step_index": 3, "source": "SYSTEM", "type": "SYSTEM_MESSAGE", "status": "DONE", "content": f"<SYSTEM_MESSAGE>\n[Message] timestamp=2026-11-15T15:00:03Z sender={sid}/task-fast priority=MESSAGE_PRIORITY_HIGH content=Command exited with code 0.\n</SYSTEM_MESSAGE>"})
            append_step(sid, {"step_index": 4, "source": "MODEL", "type": "PLANNER_RESPONSE", "status": "DONE", "content": "Finished inline."})
            send({"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": sid, "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": "Finished inline."}}}})
            send({"jsonrpc": "2.0", "id": req_id, "result": {"stopReason": "end_turn", "usage": {"inputTokens": 50, "outputTokens": 5, "totalTokens": 55}}})
            continue

        if "SPAWN_REAL_SLEEP" in text:
            child = subprocess.Popen(["bash", "-c", "sleep 30"], start_new_session=True)
            threading.Thread(target=child.wait, daemon=True).start()
            if pid_file:
                with open(pid_file, "w") as pf:
                    pf.write(str(child.pid))
            log_path = os.path.join(brain_tasks_dir(sid), "task-9.log")
            append_step(sid, {"step_index": 0, "source": "USER_EXPLICIT", "type": "USER_INPUT", "status": "DONE", "content": text})
            append_step(sid, {"step_index": 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "status": "DONE", "tool_calls": [{"name": "run_command", "args": {"CommandLine": "sleep 30", "toolSummary": "Long sleep"}}]})
            append_step(sid, {"step_index": 2, "source": "MODEL", "type": "GENERIC", "status": "RUNNING", "content": f"Tool is running as a background task with task id: {sid}/task-9\nTask Description: sleep 30\nTask logs are available at: file://{log_path}\n"})
            append_step(sid, {"step_index": 3, "source": "MODEL", "type": "PLANNER_RESPONSE", "status": "DONE", "content": "Sleeping in background"})
            send({"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": sid, "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": "Sleeping in background"}}}})
            send({"jsonrpc": "2.0", "id": req_id, "result": {"stopReason": "end_turn"}})
            continue

        send({"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": sid, "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": "echo:" + text}}}})
        send({"jsonrpc": "2.0", "id": req_id, "result": {"stopReason": "end_turn"}})
        continue

    if req_id is not None:
        send({"jsonrpc": "2.0", "id": req_id, "result": {}})
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestBackgroundShellTaskAndWakeupTurn(t *testing.T) {
	script := writeFakeBackgroundAgyScript(t)
	tempHome := t.TempDir()

	rt := New(Options{
		Command:      script,
		Env:          map[string]string{"HOME": tempHome},
		pollInterval: testBrainPollInterval,
	})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	var (
		taskMu      sync.Mutex
		taskHistory [][]runtime.BackgroundTask
	)
	rt.SetBackgroundTaskHandler(func(sessionKey string, tasks []runtime.BackgroundTask) {
		taskMu.Lock()
		cloned := make([]runtime.BackgroundTask, len(tasks))
		copy(cloned, tasks)
		taskHistory = append(taskHistory, cloned)
		taskMu.Unlock()
	})

	wakeupCh := make(chan string, 2)
	rt.SetBackgroundTurnHandler(func(sessionKey string) bool {
		wakeupCh <- sessionKey
		return true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := "agent-agy:channel:conv-bg"
	req := runtime.StartSessionRequest{
		AgentID:    "agent-agy",
		SessionKey: key,
		Workspace:  t.TempDir(),
	}
	sess, err := rt.StartSession(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Send(ctx, runtime.Input{Prompt: "SPAWN_BG_WAKEUP"}); err != nil {
		t.Fatal(err)
	}
	var turn1 runtime.Event
	for evt := range sess.Events() {
		if evt.Type != runtime.EventDelta {
			turn1 = evt
		}
	}
	if turn1.Type != runtime.EventCompleted || turn1.Text != "Building in background..." {
		t.Fatalf("turn 1 = %+v", turn1)
	}
	sid := sess.CurrentSessionID()

	// Task should have been reported as running right at the end of turn 1.
	taskMu.Lock()
	if len(taskHistory) == 0 || len(taskHistory[0]) != 1 {
		taskMu.Unlock()
		t.Fatalf("expected initial running task report, got %+v", taskHistory)
	}
	initialTask := taskHistory[0][0]
	taskMu.Unlock()
	if initialTask.ID != "task-2" || initialTask.Kind != runtime.BackgroundTaskShell || initialTask.Status != runtime.BackgroundTaskRunning || initialTask.Description != "Rebuild server" || initialTask.ToolCallID != "call_bg_1" || initialTask.OutputFile == "" {
		t.Fatalf("initialTask = %+v", initialTask)
	}

	// Wait for the background wakeup notification and adopt the turn.
	select {
	case gotKey := <-wakeupCh:
		if gotKey != key {
			t.Fatalf("wakeup key = %q, want %q", gotKey, key)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for background wakeup turn")
	}

	bgSess, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:           "agent-agy",
		SessionKey:        key,
		Workspace:         req.Workspace,
		PreviousSessionID: sid,
	})
	if err != nil {
		t.Fatal(err)
	}
	adopter, ok := bgSess.(runtime.BackgroundTurnAdopter)
	if !ok {
		t.Fatal("session does not implement BackgroundTurnAdopter")
	}
	if err := adopter.AdoptBackgroundTurn(ctx); err != nil {
		t.Fatalf("AdoptBackgroundTurn error: %v", err)
	}

	var (
		sawThinking   bool
		sawToolCall   bool
		sawToolResult bool
		toolOutput    any
		bgFinal       runtime.Event
	)
	for evt := range bgSess.Events() {
		switch evt.Type {
		case runtime.EventDelta:
			if strings.Contains(evt.Thinking, "Verifying built binary.") {
				sawThinking = true
			}
			for _, item := range evt.Process {
				switch item.Type {
				case "tool_call":
					if item.ToolName == "read_file" {
						sawToolCall = true
					}
				case "tool_result":
					if item.ToolName == "read_file" {
						sawToolResult = true
						toolOutput = item.Output
					}
				}
			}
		default:
			bgFinal = evt
		}
	}

	if bgFinal.Type != runtime.EventCompleted {
		t.Fatalf("bgFinal = %+v", bgFinal)
	}
	if bgFinal.Text != "Build succeeded and binary is ready." {
		t.Fatalf("bgFinal.Text = %q", bgFinal.Text)
	}
	if !sawThinking || !sawToolCall || !sawToolResult {
		t.Fatalf("missing process events: thinking=%v toolCall=%v toolResult=%v", sawThinking, sawToolCall, sawToolResult)
	}
	if toolOutput != "binary-ok" {
		t.Fatalf("toolOutput = %#v, want %q", toolOutput, "binary-ok")
	}
	if bgFinal.Usage == nil || bgFinal.Usage.TotalTokens == nil || *bgFinal.Usage.TotalTokens != 5275 {
		t.Fatalf("unexpected usage on background turn: %+v", bgFinal.Usage)
	}
	if bgFinal.Usage.Context == nil || bgFinal.Usage.Context.TotalTokens == nil || *bgFinal.Usage.Context.TotalTokens != 12000 {
		t.Fatalf("expected cached context usage on background turn, got %+v", bgFinal.Usage.Context)
	}

	// Verify task transitioned to completed.
	taskMu.Lock()
	latestTasks := taskHistory[len(taskHistory)-1]
	taskMu.Unlock()
	if len(latestTasks) != 1 || latestTasks[0].Status != "completed" || latestTasks[0].EndedAt.IsZero() {
		t.Fatalf("latestTasks = %+v", latestTasks)
	}
}

func TestForegroundFinishedBackgroundTaskDoesNotTriggerDuplicateWakeup(t *testing.T) {
	script := writeFakeBackgroundAgyScript(t)
	tempHome := t.TempDir()

	rt := New(Options{
		Command:      script,
		Env:          map[string]string{"HOME": tempHome},
		pollInterval: testBrainPollInterval,
	})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	wakeupCalled := make(chan struct{}, 1)
	rt.SetBackgroundTurnHandler(func(sessionKey string) bool {
		select {
		case wakeupCalled <- struct{}{}:
		default:
		}
		return true
	})

	var (
		taskMu sync.Mutex
		latest []runtime.BackgroundTask
	)
	rt.SetBackgroundTaskHandler(func(_ string, tasks []runtime.BackgroundTask) {
		taskMu.Lock()
		latest = append([]runtime.BackgroundTask(nil), tasks...)
		taskMu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sess, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:    "agent-agy",
		SessionKey: "agent-agy:channel:inline",
		Workspace:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Send(ctx, runtime.Input{Prompt: "SPAWN_FAST_INLINE_BG"}); err != nil {
		t.Fatal(err)
	}
	for range sess.Events() {
	}

	// Give monitor several poll intervals to ensure no duplicate background turn is claimed.
	select {
	case <-wakeupCalled:
		t.Fatal("unexpected background turn wakeup for a task that finished inside foreground session/prompt")
	case <-time.After(120 * time.Millisecond):
	}

	taskMu.Lock()
	defer taskMu.Unlock()
	if len(latest) != 1 || latest[0].ID != "task-fast" || latest[0].Status != "completed" {
		t.Fatalf("latest tasks = %+v", latest)
	}
}

func TestSessionResumeSkipsHistoricalBackgroundWakeups(t *testing.T) {
	script := writeFakeBackgroundAgyScript(t)
	tempHome := t.TempDir()
	sid := "sid-existing-history"

	logsDir := filepath.Join(tempHome, ".gemini", "antigravity-cli", "brain", sid, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(logsDir, "transcript_full.jsonl")
	historical := []transcriptStep{
		{StepIndex: 0, Source: "USER_EXPLICIT", Type: "USER_INPUT", Status: "DONE", Content: "old prompt"},
		{StepIndex: 1, Source: "SYSTEM", Type: "SYSTEM_MESSAGE", Status: "DONE", Content: "<SYSTEM_MESSAGE>\n[Message] timestamp=2026-11-15T10:00:00Z sender=" + sid + "/task-1 priority=MESSAGE_PRIORITY_HIGH content=Command exited with code 0.\n</SYSTEM_MESSAGE>"},
		{StepIndex: 2, Source: "MODEL", Type: "PLANNER_RESPONSE", Status: "DONE", Content: "old wakeup reply"},
	}
	var initialBytes []byte
	for _, st := range historical {
		b, _ := json.Marshal(st)
		initialBytes = append(initialBytes, append(b, '\n')...)
	}
	if err := os.WriteFile(transcriptPath, initialBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	rt := New(Options{
		Command:      script,
		Env:          map[string]string{"HOME": tempHome},
		pollInterval: testBrainPollInterval,
	})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	wakeupCh := make(chan string, 2)
	rt.SetBackgroundTurnHandler(func(sessionKey string) bool {
		wakeupCh <- sessionKey
		return true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	workspace := t.TempDir()
	key := "agent-agy:channel:resume"
	sess, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:           "agent-agy",
		SessionKey:        key,
		Workspace:         workspace,
		PreviousSessionID: sid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Send(ctx, runtime.Input{Prompt: "hello after resume"}); err != nil {
		t.Fatal(err)
	}
	for range sess.Events() {
	}

	select {
	case <-wakeupCh:
		t.Fatal("historical SYSTEM_MESSAGE should not trigger a wakeup after session/resume")
	case <-time.After(100 * time.Millisecond):
	}

	// Now append a brand new wakeup after resume and ensure it IS claimed.
	f, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	newSteps := []transcriptStep{
		{StepIndex: 3, Source: "SYSTEM", Type: "SYSTEM_MESSAGE", Status: "DONE", Content: "<SYSTEM_MESSAGE>\n[Message] timestamp=2026-11-15T16:00:00Z sender=" + sid + "/task-99 priority=MESSAGE_PRIORITY_HIGH content=Command exited with code 0.\n</SYSTEM_MESSAGE>"},
		{StepIndex: 4, Source: "MODEL", Type: "PLANNER_RESPONSE", Status: "DONE", Content: "fresh wakeup after resume"},
	}
	for _, st := range newSteps {
		b, _ := json.Marshal(st)
		_, _ = f.Write(append(b, '\n'))
	}
	_ = f.Close()

	select {
	case got := <-wakeupCh:
		if got != key {
			t.Fatalf("got key %q, want %q", got, key)
		}
	case <-ctx.Done():
		t.Fatal("expected fresh post-resume wakeup")
	}

	bgSess, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:           "agent-agy",
		SessionKey:        key,
		Workspace:         workspace,
		PreviousSessionID: sid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bgSess.(runtime.BackgroundTurnAdopter).AdoptBackgroundTurn(ctx); err != nil {
		t.Fatal(err)
	}
	var final runtime.Event
	for evt := range bgSess.Events() {
		if evt.Type != runtime.EventDelta {
			final = evt
		}
	}
	if final.Type != runtime.EventCompleted || final.Text != "fresh wakeup after resume" {
		t.Fatalf("final = %+v", final)
	}
}

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

func TestUnadoptedBackgroundTurnClaimExpiresCleanly(t *testing.T) {
	script := writeFakeBackgroundAgyScript(t)
	tempHome := t.TempDir()

	rt := New(Options{
		Command:      script,
		Env:          map[string]string{"HOME": tempHome},
		pollInterval: testBrainPollInterval,
		claimTimeout: 80 * time.Millisecond,
	})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	wakeupCh := make(chan string, 2)
	rt.SetBackgroundTurnHandler(func(sessionKey string) bool {
		// Accept the claim but intentionally never call AdoptBackgroundTurn.
		wakeupCh <- sessionKey
		return true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	workspace := t.TempDir()
	key := "agent-agy:channel:expire"
	sess, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:    "agent-agy",
		SessionKey: key,
		Workspace:  workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Send(ctx, runtime.Input{Prompt: "SPAWN_BG_WAKEUP"}); err != nil {
		t.Fatal(err)
	}
	for range sess.Events() {
	}
	sid := sess.CurrentSessionID()

	<-wakeupCh
	// Wait past the 80ms claim timeout so brainMonitor abandons the claim.
	time.Sleep(160 * time.Millisecond)

	// Foreground Send must still acquire the process turn without deadlocking.
	sess2, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:           "agent-agy",
		SessionKey:        key,
		Workspace:         workspace,
		PreviousSessionID: sid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess2.Send(ctx, runtime.Input{Prompt: "ping"}); err != nil {
		t.Fatal(err)
	}
	var final runtime.Event
	for evt := range sess2.Events() {
		if evt.Type != runtime.EventDelta {
			final = evt
		}
	}
	if final.Type != runtime.EventCompleted || !strings.Contains(final.Text, "echo:") {
		t.Fatalf("foreground turn after expired claim failed: %+v", final)
	}
}

func TestTaskRegistrySubagentsSchedulesAndFailures(t *testing.T) {
	var reported []runtime.BackgroundTask
	reg := newTaskRegistry("agent:channel:1", "/tmp/appdata", func(_ string, tasks []runtime.BackgroundTask) {
		reported = append([]runtime.BackgroundTask(nil), tasks...)
	})
	reg.setSession("sid-1")

	// 1. Subagent spawned and completed.
	reg.observeStep(transcriptStep{
		StepIndex: 1,
		Type:      "PLANNER_RESPONSE",
		ToolCalls: []transcriptToolCall{{
			Name: "invoke_subagent",
			Args: map[string]any{"Role": "DB Auditor", "TypeName": "research"},
		}},
	})
	reg.observeStep(transcriptStep{
		StepIndex: 2,
		Type:      "GENERIC",
		CreatedAt: "2026-11-15T15:00:01Z",
		Content:   "Created the following subagent:\n{\n  \"conversationId\": \"sub-uuid-1234\",\n  \"logAbsoluteUri\": \"file:///tmp/log\"\n}",
	})
	if len(reported) != 1 || reported[0].ID != "sub-uuid-1234" || reported[0].Kind != runtime.BackgroundTaskAgent || reported[0].Description != "DB Auditor (research)" || reported[0].Status != runtime.BackgroundTaskRunning {
		t.Fatalf("subagent start = %+v", reported)
	}

	// 2. Schedule spawned, recurring fire keeps it running, expiration finishes it.
	reg.observeStep(transcriptStep{
		StepIndex: 3,
		Type:      "PLANNER_RESPONSE",
		ToolCalls: []transcriptToolCall{{
			Name: "schedule",
			Args: map[string]any{"Prompt": "Check CI"},
		}},
	})
	reg.observeStep(transcriptStep{
		StepIndex: 4,
		Type:      "GENERIC",
		CreatedAt: "2026-11-15T15:00:02Z",
		Content:   "Tool is running as a background task with task id: sid-1/task-4\n",
	})
	if len(reported) != 2 || reported[1].ID != "task-4" || reported[1].Kind != runtime.BackgroundTaskMonitor || reported[1].Description != "Check CI" {
		t.Fatalf("schedule start = %+v", reported)
	}
	reg.observeStep(transcriptStep{
		StepIndex: 5,
		Type:      "SYSTEM_MESSAGE",
		Content:   "<SYSTEM_MESSAGE>\n[Message] timestamp=2026-11-15T15:05:00Z sender=sid-1/task-4 priority=MESSAGE_PRIORITY_HIGH content=[task-4] Next scheduled fire time: 2026-11-15T15:10:00Z\n</SYSTEM_MESSAGE>",
	})
	if reported[1].Status != runtime.BackgroundTaskRunning {
		t.Fatalf("recurring schedule should stay running: %+v", reported[1])
	}

	// 3. Subagent finishes via SYSTEM_MESSAGE.
	reg.observeStep(transcriptStep{
		StepIndex: 6,
		Type:      "SYSTEM_MESSAGE",
		Content:   "<SYSTEM_MESSAGE>\n[Message] timestamp=2026-11-15T15:06:00Z sender=sub-uuid-1234 priority=MESSAGE_PRIORITY_HIGH content=Audit complete.\n</SYSTEM_MESSAGE>",
	})
	if reported[0].Status != "completed" {
		t.Fatalf("subagent should be completed: %+v", reported[0])
	}

	// 4. Non-zero shell exit marks failed with summary.
	reg.observeStep(transcriptStep{
		StepIndex: 7,
		Type:      "PLANNER_RESPONSE",
		ToolCalls: []transcriptToolCall{{
			Name: "run_command",
			Args: map[string]any{"CommandLine": "false", "toolSummary": "Failing command"},
		}},
	})
	reg.observeStep(transcriptStep{
		StepIndex: 8,
		Type:      "GENERIC",
		CreatedAt: "2026-11-15T15:07:00Z",
		Content:   "Tool is running as a background task with task id: sid-1/task-8\nTask Description: false\n",
	})
	reg.observeStep(transcriptStep{
		StepIndex: 9,
		Type:      "SYSTEM_MESSAGE",
		Content:   "<SYSTEM_MESSAGE>\n[Message] timestamp=2026-11-15T15:07:05Z sender=sid-1/task-8 priority=MESSAGE_PRIORITY_HIGH content=Task id \"sid-1/task-8\" finished with result:\nCommand exited with code 2.\n</SYSTEM_MESSAGE>",
	})
	if len(reported) != 3 || reported[2].Status != "failed" || reported[2].Summary != "Command exited with code 2." {
		t.Fatalf("failed shell task = %+v", reported[2])
	}

	// 5. Process exit stops any remaining running tasks (`task-4`).
	reg.processExited()
	if reported[1].Status != "stopped" || reported[1].Summary != "The agent process exited." {
		t.Fatalf("expected stopped schedule on process exit, got %+v", reported[1])
	}
}
