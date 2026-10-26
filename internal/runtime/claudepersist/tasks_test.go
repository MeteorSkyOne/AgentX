package claudepersist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func observeLines(t *testing.T, registry *taskRegistry, lines ...string) {
	t.Helper()
	for _, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("bad line %s: %v", line, err)
		}
		registry.observe(payload)
	}
}

// The stream Claude Code emits for a Monitor: its task is a local_bash task,
// told apart only by the tool that started it, and its result names no
// output file.
func TestRegistryTracksMonitorFromStartToEnd(t *testing.T) {
	workspace := t.TempDir()
	tmp := t.TempDir()
	var reports [][]runtime.BackgroundTask
	registry := newTaskRegistry("a:channel:c", workspace, []string{"TMPDIR=" + tmp}, func(key string, tasks []runtime.BackgroundTask) {
		if key != "a:channel:c" {
			t.Errorf("report key = %q", key)
		}
		reports = append(reports, tasks)
	})
	outputDir := filepath.Join(registry.outputBase, "sess-1", "tasks")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "m1.output"), []byte("tick 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	observeLines(t, registry,
		`{"type":"system","subtype":"init","session_id":"sess-1"}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_m","name":"Monitor","input":{"command":"tail -f log"}}]}}`,
		`{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"m1","task_type":"local_bash","description":"Watch the log"}]}`,
		`{"type":"system","subtype":"task_started","task_id":"m1","task_type":"local_bash","tool_use_id":"toolu_m","description":"Watch the log","is_backgrounded":true}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_m","content":"Monitor started (task m1, expires in 5m)."}]}}`,
	)
	running := reports[len(reports)-1]
	if len(running) != 1 {
		t.Fatalf("tasks = %#v", running)
	}
	task := running[0]
	if task.Kind != runtime.BackgroundTaskMonitor || task.Status != runtime.BackgroundTaskRunning || task.Description != "Watch the log" || task.ToolCallID != "toolu_m" {
		t.Fatalf("running monitor = %#v", task)
	}
	if want := filepath.Join(outputDir, "m1.output"); task.OutputFile != want {
		t.Fatalf("output file = %q, want the derived %q", task.OutputFile, want)
	}
	if !strings.HasPrefix(registry.outputBase, filepath.Join(tmp, "claude-")) || !strings.HasSuffix(registry.outputBase, strings.ReplaceAll(workspace, "/", "-")) {
		t.Fatalf("output base = %q", registry.outputBase)
	}

	count := len(reports)
	observeLines(t, registry,
		`{"type":"system","subtype":"init","session_id":"sess-1"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"tick 1"}]}}`,
	)
	if len(reports) != count {
		t.Fatal("a wake-up with no task change must not report")
	}

	observeLines(t, registry,
		`{"type":"system","subtype":"task_updated","task_id":"m1","patch":{"status":"completed"}}`,
		`{"type":"system","subtype":"task_notification","task_id":"m1","status":"completed","summary":"Monitor stream ended","output_file":"/tmp/x/m1.output"}`,
		`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`,
	)
	ended := reports[len(reports)-1][0]
	if ended.Status != "completed" || ended.EndedAt.IsZero() {
		t.Fatalf("ended monitor = %#v", ended)
	}
	// The summary and output file come in the notification that follows the
	// status update.
	if ended.Summary != "Monitor stream ended" || ended.OutputFile != "/tmp/x/m1.output" {
		t.Fatalf("ended monitor details = %#v", ended)
	}
	if len(reports) != count+2 {
		t.Fatalf("expected a report for the end and one for its details, got %d", len(reports)-count)
	}
}

func TestRegistryReadsBackgroundBashOutputFileAndEndsOnExit(t *testing.T) {
	var last []runtime.BackgroundTask
	registry := newTaskRegistry("k", t.TempDir(), nil, func(_ string, tasks []runtime.BackgroundTask) { last = tasks })
	observeLines(t, registry,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_b","name":"Bash"}]}}`,
		`{"type":"system","subtype":"task_started","task_id":"b1","task_type":"local_bash","tool_use_id":"toolu_b","description":"Build"}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_b","content":[{"type":"text","text":"Command running in background with ID: b1. Output is being written to: /tmp/claude-1/p/s/tasks/b1.output"}]}]}}`,
		`{"type":"system","subtype":"task_started","task_id":"a1","task_type":"local_agent","tool_use_id":"toolu_a","description":"Research"}`,
	)
	if len(last) != 2 || last[0].Kind != runtime.BackgroundTaskShell || last[1].Kind != runtime.BackgroundTaskAgent {
		t.Fatalf("tasks = %#v", last)
	}
	if last[0].OutputFile != "/tmp/claude-1/p/s/tasks/b1.output" {
		t.Fatalf("bash output file = %q", last[0].OutputFile)
	}
	// A foreground subagent is never listed in background_tasks_changed, so
	// an empty list must not end it.
	observeLines(t, registry, `{"type":"system","subtype":"background_tasks_changed","tasks":[]}`)
	if last[0].Status != "completed" || last[1].Status != runtime.BackgroundTaskRunning {
		t.Fatalf("after reconcile = %#v", last)
	}

	registry.processExited()
	if last[1].Status != "stopped" || last[1].Summary == "" {
		t.Fatalf("subagent after process exit = %#v", last[1])
	}
}

func TestRegistryKeepsOnlyRecentEndedTasks(t *testing.T) {
	registry := newTaskRegistry("k", "", nil, nil)
	for i := 0; i < maxEndedTasks+5; i++ {
		id := "t" + string(rune('a'+i))
		observeLines(t, registry,
			`{"type":"system","subtype":"task_started","task_id":"`+id+`","task_type":"local_bash"}`,
			`{"type":"system","subtype":"task_notification","task_id":"`+id+`","status":"completed"}`,
		)
	}
	observeLines(t, registry, `{"type":"system","subtype":"task_started","task_id":"live","task_type":"local_bash"}`)
	tasks := registry.snapshot()
	if len(tasks) != maxEndedTasks+1 {
		t.Fatalf("kept %d tasks, want %d ended plus the running one", len(tasks), maxEndedTasks)
	}
	if tasks[len(tasks)-1].ID != "live" {
		t.Fatalf("running task dropped: %#v", tasks[len(tasks)-1])
	}
}
