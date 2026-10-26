package claudepersist

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/claude"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

// maxEndedTasks caps how many ended tasks a process keeps listing.
const maxEndedTasks = 20

// taskRegistry follows the background tasks of one Claude Code process —
// shell commands, monitors and subagents — across the turns that start and
// finish them, including output that arrives while no turn is reading.
type taskRegistry struct {
	key    string
	report func(sessionKey string, tasks []runtime.BackgroundTask)
	// outputBase is the directory Claude Code writes task output under for
	// this process: <tmp>/claude-<uid>/<sanitized cwd>.
	outputBase string

	mu        sync.Mutex
	sessionID string
	tasks     map[string]*runtime.BackgroundTask
	// toolNames and outputs are keyed by tool call id: the tool that started a
	// task tells a monitor from a plain shell command, and a background Bash
	// result names its output file.
	toolNames map[string]string
	outputs   map[string]string
}

type taskRegistryKey struct{}

var outputFilePattern = regexp.MustCompile(`Output is being written to: (\S+)`)

func newTaskRegistry(key string, workspace string, env []string, report func(string, []runtime.BackgroundTask)) *taskRegistry {
	return &taskRegistry{
		key:        key,
		report:     report,
		outputBase: claudeTaskOutputBase(workspace, env),
		tasks:      map[string]*runtime.BackgroundTask{},
		toolNames:  map[string]string{},
		outputs:    map[string]string{},
	}
}

// registryOf returns the task registry attached to a process, if any.
func registryOf(proc *procpool.ManagedProcess) *taskRegistry {
	if proc == nil {
		return nil
	}
	proc.Mu.Lock()
	defer proc.Mu.Unlock()
	registry, _ := proc.UserData[taskRegistryKey{}].(*taskRegistry)
	return registry
}

func attachRegistry(proc *procpool.ManagedProcess, registry *taskRegistry) {
	proc.Mu.Lock()
	defer proc.Mu.Unlock()
	if proc.UserData == nil {
		proc.UserData = map[any]any{}
	}
	proc.UserData[taskRegistryKey{}] = registry
}

// observe folds one stream-json message into the registry, reporting the task
// list when it changed.
func (r *taskRegistry) observe(payload map[string]any) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := r.apply(payload)
	var tasks []runtime.BackgroundTask
	if changed {
		tasks = r.snapshotLocked()
	}
	r.mu.Unlock()
	if changed && r.report != nil {
		r.report(r.key, tasks)
	}
}

func (r *taskRegistry) apply(payload map[string]any) bool {
	if id := claude.StringValue(payload, "session_id"); id != "" {
		r.sessionID = id
	}
	switch claude.StringValue(payload, "type") {
	case "system":
		return r.applySystem(payload)
	case "assistant", "user":
		message, _ := payload["message"].(map[string]any)
		content, _ := message["content"].([]any)
		for _, item := range content {
			block, _ := item.(map[string]any)
			switch claude.StringValue(block, "type") {
			case "tool_use":
				if id := claude.StringValue(block, "id"); id != "" {
					r.toolNames[id] = claude.StringValue(block, "name")
				}
			case "tool_result":
				if match := outputFilePattern.FindStringSubmatch(toolResultText(block["content"])); match != nil {
					r.outputs[claude.StringValue(block, "tool_use_id")] = match[1]
				}
			}
		}
	}
	return false
}

func (r *taskRegistry) applySystem(payload map[string]any) bool {
	switch claude.StringValue(payload, "subtype") {
	case "task_started":
		return r.start(claude.StringValue(payload, "task_id"), claude.StringValue(payload, "task_type"),
			claude.StringValue(payload, "description"), claude.StringValue(payload, "tool_use_id"))
	case "background_tasks_changed":
		tasks, _ := payload["tasks"].([]any)
		live := map[string]struct{}{}
		changed := false
		for _, item := range tasks {
			task, _ := item.(map[string]any)
			taskID := claude.StringValue(task, "task_id")
			status := claude.StringValue(task, "status")
			if taskID == "" {
				continue
			}
			if claude.IsTerminalTaskStatus(status) {
				changed = r.end(taskID, status, "", "") || changed
				continue
			}
			live[taskID] = struct{}{}
			changed = r.start(taskID, claude.StringValue(task, "task_type"), claude.StringValue(task, "description"), claude.StringValue(task, "tool_use_id")) || changed
		}
		// The list carries every live background task, so one that dropped
		// out of it has ended. Subagents running in the foreground are never
		// listed and end with their own notification.
		for taskID, task := range r.tasks {
			if _, ok := live[taskID]; !ok && task.Status == runtime.BackgroundTaskRunning && task.Kind != runtime.BackgroundTaskAgent {
				changed = r.end(taskID, "completed", "", "") || changed
			}
		}
		return changed
	case "task_updated":
		patch, _ := payload["patch"].(map[string]any)
		if status := claude.StringValue(patch, "status"); claude.IsTerminalTaskStatus(status) {
			return r.end(claude.StringValue(payload, "task_id"), status, "", "")
		}
	case "task_notification":
		if status := claude.StringValue(payload, "status"); claude.IsTerminalTaskStatus(status) {
			return r.end(claude.StringValue(payload, "task_id"), status,
				claude.StringValue(payload, "summary"), claude.StringValue(payload, "output_file"))
		}
	}
	return false
}

// start records a running task, filling in details later messages add. It
// reports whether anything changed.
func (r *taskRegistry) start(taskID string, taskType string, description string, toolUseID string) bool {
	if taskID == "" {
		return false
	}
	task, ok := r.tasks[taskID]
	if !ok {
		task = &runtime.BackgroundTask{ID: taskID, Status: runtime.BackgroundTaskRunning, StartedAt: time.Now().UTC()}
		r.tasks[taskID] = task
	}
	before := *task
	if task.ToolCallID == "" {
		task.ToolCallID = toolUseID
	}
	if task.Description == "" {
		task.Description = description
	}
	if kind := r.kind(taskType, task.ToolCallID); kind != "" {
		task.Kind = kind
	}
	return !ok || *task != before
}

// end records that a task ended. The details can arrive after the end itself
// — Claude Code reports the status in task_updated before the summary and
// output file in task_notification — so they still fill in an ended task.
func (r *taskRegistry) end(taskID string, status string, summary string, outputFile string) bool {
	task, ok := r.tasks[taskID]
	if !ok {
		return false
	}
	changed := false
	if task.Status == runtime.BackgroundTaskRunning {
		task.Status = status
		task.EndedAt = time.Now().UTC()
		changed = true
	}
	if summary != "" && task.Summary != summary {
		task.Summary = summary
		changed = true
	}
	if outputFile != "" && task.OutputFile != outputFile {
		task.OutputFile = outputFile
		changed = true
	}
	return changed
}

func (r *taskRegistry) kind(taskType string, toolUseID string) string {
	switch taskType {
	case "local_bash":
		if r.toolNames[toolUseID] == "Monitor" {
			return runtime.BackgroundTaskMonitor
		}
		return runtime.BackgroundTaskShell
	case "local_agent", "remote_agent":
		return runtime.BackgroundTaskAgent
	}
	return taskType
}

// processExited ends every task still running: they go away with the process.
func (r *taskRegistry) processExited() {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := false
	for taskID, task := range r.tasks {
		if task.Status == runtime.BackgroundTaskRunning {
			r.end(taskID, "stopped", "The agent process exited.", "")
			changed = true
		}
	}
	tasks := r.snapshotLocked()
	r.mu.Unlock()
	if changed && r.report != nil {
		r.report(r.key, tasks)
	}
}

func (r *taskRegistry) snapshot() []runtime.BackgroundTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

// snapshotLocked lists running tasks and the most recent ended ones, oldest
// first, dropping older ended tasks for good.
func (r *taskRegistry) snapshotLocked() []runtime.BackgroundTask {
	var ended []*runtime.BackgroundTask
	for _, task := range r.tasks {
		if task.Status != runtime.BackgroundTaskRunning {
			ended = append(ended, task)
		}
	}
	if len(ended) > maxEndedTasks {
		sort.Slice(ended, func(i, j int) bool { return ended[i].EndedAt.After(ended[j].EndedAt) })
		for _, task := range ended[maxEndedTasks:] {
			delete(r.tasks, task.ID)
		}
	}
	tasks := make([]runtime.BackgroundTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.OutputFile == "" {
			task.OutputFile = r.outputFile(task)
		}
		tasks = append(tasks, *task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if !tasks[i].StartedAt.Equal(tasks[j].StartedAt) {
			return tasks[i].StartedAt.Before(tasks[j].StartedAt)
		}
		return tasks[i].ID < tasks[j].ID
	})
	return tasks
}

// outputFile finds where a task writes its output: named by its tool result
// for a background Bash command, or at the path Claude Code uses for every
// task otherwise — a monitor's result does not name it.
func (r *taskRegistry) outputFile(task *runtime.BackgroundTask) string {
	if path := r.outputs[task.ToolCallID]; path != "" {
		return path
	}
	if r.outputBase == "" || r.sessionID == "" || task.Kind == runtime.BackgroundTaskAgent {
		return ""
	}
	path := filepath.Join(r.outputBase, r.sessionID, "tasks", task.ID+".output")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// claudeTaskOutputBase mirrors where Claude Code keeps per-project task
// output: <tmp>/claude-<uid>/<cwd with every non-alphanumeric byte as '-'>.
func claudeTaskOutputBase(workspace string, env []string) string {
	if workspace == "" {
		return ""
	}
	tmp := envValue(env, "CLAUDE_CODE_TMPDIR")
	if tmp == "" {
		tmp = envValue(env, "TMPDIR")
	}
	if tmp == "" {
		tmp = "/tmp"
	}
	var sanitized strings.Builder
	for _, b := range []byte(workspace) {
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' {
			sanitized.WriteByte(b)
		} else {
			sanitized.WriteByte('-')
		}
	}
	return filepath.Join(tmp, "claude-"+strconv.Itoa(os.Getuid()), sanitized.String())
}

func envValue(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if value, ok := strings.CutPrefix(env[i], key+"="); ok {
			return value
		}
	}
	return ""
}

func toolResultText(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		var b strings.Builder
		for _, item := range typed {
			block, _ := item.(map[string]any)
			b.WriteString(claude.StringValue(block, "text"))
		}
		return b.String()
	}
	return ""
}
