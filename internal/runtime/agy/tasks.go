package agy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

const maxEndedTasks = 20

var (
	errNoSuchBackgroundTask = errors.New("no such running background task")

	bgTaskStartedPattern  = regexp.MustCompile(`(?m)^Tool is running as a background task with task id:\s*(\S+)\s*$`)
	bgTaskDescPattern     = regexp.MustCompile(`(?m)^Task Description:\s*(.+?)\s*$`)
	bgTaskLogPattern      = regexp.MustCompile(`(?m)^Task logs are available at:\s*(\S+)\s*$`)
	bgTaskKilledPattern   = regexp.MustCompile(`(?m)^Task\s+"([^"]+)"\s+(?:killed|canceled|cancelled|stopped)`)
	bgTaskStatusPattern   = regexp.MustCompile(`(?ms)^Task:\s*(\S+)\s*\nStatus:\s*(\S+)`)
	sysMsgHeaderPattern   = regexp.MustCompile(`\[Message\]\s+timestamp=(\S+)\s+sender=(\S+)\s+priority=(\S+)\s+content=(?s:(.*))$`)
	exitCodeLinePattern   = regexp.MustCompile(`(?m)^Command exited with code (-?\d+)\.`)
	subagentBlockLeadText = "Created the following subagent"
)

type transcriptStep struct {
	StepIndex    int                  `json:"step_index"`
	Source       string               `json:"source"`
	Type         string               `json:"type"`
	Status       string               `json:"status"`
	CreatedAt    string               `json:"created_at"`
	Model        string               `json:"model"`
	InputTokens  int64                `json:"input_tokens"`
	CacheTokens  int64                `json:"cache_read_tokens"`
	OutputTokens int64                `json:"output_tokens"`
	Content      string               `json:"content"`
	Thinking     string               `json:"thinking"`
	ToolCalls    []transcriptToolCall `json:"tool_calls"`
}

type transcriptToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type agyMessageFile struct {
	ID            string `json:"id"`
	Sender        string `json:"sender"`
	Priority      string `json:"priority"`
	Timestamp     string `json:"timestamp"`
	HideFromUser  bool   `json:"hideFromUser"`
	Content       string `json:"content"`
	RenderDetails struct {
		MessageTitle string `json:"messageTitle"`
	} `json:"renderDetails"`
	SourceMetadata struct {
		Tool struct {
			ToolCall struct {
				ID            string `json:"id"`
				Name          string `json:"name"`
				ArgumentsJSON string `json:"argumentsJson"`
			} `json:"toolCall"`
		} `json:"tool"`
	} `json:"sourceMetadata"`
}

type pendingToolMeta struct {
	toolCallID  string
	name        string
	description string
	commandLine string
	kind        string
}

type taskDetail struct {
	commandLine string
}

type taskRegistry struct {
	key    string
	report func(sessionKey string, tasks []runtime.BackgroundTask)

	mu            sync.Mutex
	appDataDir    string
	sessionID     string
	tasks         map[string]*runtime.BackgroundTask
	details       map[string]*taskDetail
	pendingTools  []pendingToolMeta
	acpCallsByCmd map[string]string
}

func newTaskRegistry(key string, appDataDir string, report func(string, []runtime.BackgroundTask)) *taskRegistry {
	return &taskRegistry{
		key:           key,
		report:        report,
		appDataDir:    appDataDir,
		tasks:         make(map[string]*runtime.BackgroundTask),
		details:       make(map[string]*taskDetail),
		acpCallsByCmd: make(map[string]string),
	}
}

func (r *taskRegistry) setSession(sessionID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.sessionID = sessionID
	r.pendingTools = nil
	r.mu.Unlock()
}

func (r *taskRegistry) closeSession() {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := false
	for id, t := range r.tasks {
		if t.Status == runtime.BackgroundTaskRunning {
			changed = r.endLocked(id, "stopped", "Session closed.", "", time.Now().UTC()) || changed
		}
	}
	r.pendingTools = nil
	r.acpCallsByCmd = make(map[string]string)
	var snap []runtime.BackgroundTask
	if changed {
		snap = r.snapshotLocked()
	}
	r.mu.Unlock()
	if changed && r.report != nil {
		r.report(r.key, snap)
	}
}

func (r *taskRegistry) processExited() {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := false
	now := time.Now().UTC()
	for id, t := range r.tasks {
		if t.Status == runtime.BackgroundTaskRunning {
			changed = r.endLocked(id, "stopped", "The agent process exited.", "", now) || changed
		}
	}
	var snap []runtime.BackgroundTask
	if changed {
		snap = r.snapshotLocked()
	}
	r.mu.Unlock()
	if changed && r.report != nil {
		r.report(r.key, snap)
	}
}

func (r *taskRegistry) hasRunning() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.Status == runtime.BackgroundTaskRunning {
			return true
		}
	}
	return false
}

func (r *taskRegistry) snapshot() []runtime.BackgroundTask {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *taskRegistry) taskCommandLine(taskID string) (kind string, cmdLine string, running bool) {
	if r == nil {
		return "", "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[taskID]
	if !ok || t.Status != runtime.BackgroundTaskRunning {
		return "", "", false
	}
	var cmd string
	if d := r.details[taskID]; d != nil {
		cmd = d.commandLine
	}
	return t.Kind, cmd, true
}

func (r *taskRegistry) runningShellCommands() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var cmds []string
	for id, t := range r.tasks {
		if t.Status != runtime.BackgroundTaskRunning || t.Kind != runtime.BackgroundTaskShell {
			continue
		}
		if d := r.details[id]; d != nil && strings.TrimSpace(d.commandLine) != "" {
			cmds = append(cmds, d.commandLine)
		}
	}
	return cmds
}

func (r *taskRegistry) markStopped(taskID string, summary string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := r.endLocked(taskID, "stopped", summary, "", time.Now().UTC())
	var snap []runtime.BackgroundTask
	if changed {
		snap = r.snapshotLocked()
	}
	r.mu.Unlock()
	if changed && r.report != nil {
		r.report(r.key, snap)
	}
}

func (r *taskRegistry) markAllStopped(summary string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := false
	now := time.Now().UTC()
	for id, t := range r.tasks {
		if t.Status == runtime.BackgroundTaskRunning {
			changed = r.endLocked(id, "stopped", summary, "", now) || changed
		}
	}
	var snap []runtime.BackgroundTask
	if changed {
		snap = r.snapshotLocked()
	}
	r.mu.Unlock()
	if changed && r.report != nil {
		r.report(r.key, snap)
	}
}

func (r *taskRegistry) observeACPToolUpdate(update record) {
	if r == nil || update == nil {
		return
	}
	callID := firstNonEmpty(stringVal(update, "toolCallId"), stringVal(update, "id"))
	name := firstNonEmpty(stringVal(update, "name"), stringVal(update, "toolName"))
	args := mapVal(update, "args")
	if args == nil {
		args = mapVal(update, "rawInput")
	}
	cmdLine := strings.TrimSpace(firstNonEmpty(stringVal(args, "CommandLine"), stringVal(args, "command")))
	if callID != "" && name == "run_command" && cmdLine != "" {
		r.mu.Lock()
		r.acpCallsByCmd[cmdLine] = callID
		r.mu.Unlock()
	}
}

func (r *taskRegistry) observeStep(step transcriptStep) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := r.applyStepLocked(step)
	var snap []runtime.BackgroundTask
	if changed {
		snap = r.snapshotLocked()
	}
	r.mu.Unlock()
	if changed && r.report != nil {
		r.report(r.key, snap)
	}
}

func (r *taskRegistry) observeMessageFile(msg agyMessageFile) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := r.applyMessageLocked(msg.Sender, msg.Content, msg.Timestamp, msg.SourceMetadata.Tool.ToolCall.ID, msg.SourceMetadata.Tool.ToolCall.ArgumentsJSON)
	var snap []runtime.BackgroundTask
	if changed {
		snap = r.snapshotLocked()
	}
	r.mu.Unlock()
	if changed && r.report != nil {
		r.report(r.key, snap)
	}
}

func (r *taskRegistry) applyStepLocked(step transcriptStep) bool {
	stepTime := parseStepTime(step.CreatedAt)

	switch step.Type {
	case "PLANNER_RESPONSE":
		r.pendingTools = r.pendingTools[:0]
		for _, tc := range step.ToolCalls {
			args := normalizeToolArgs(tc.Args)
			meta := pendingToolMeta{
				toolCallID: strings.TrimSpace(tc.ID),
				name:       tc.Name,
			}
			switch tc.Name {
			case "run_command":
				cmdLine := strings.TrimSpace(stringArg(args, "CommandLine"))
				summary := strings.TrimSpace(stringArg(args, "toolSummary"))
				meta.commandLine = cmdLine
				meta.description = firstNonEmpty(summary, cmdLine)
				meta.kind = runtime.BackgroundTaskShell
				if meta.toolCallID == "" && cmdLine != "" {
					meta.toolCallID = r.acpCallsByCmd[cmdLine]
				}
			case "schedule":
				prompt := strings.TrimSpace(stringArg(args, "Prompt"))
				cronExpr := strings.TrimSpace(stringArg(args, "CronExpression"))
				meta.description = firstNonEmpty(prompt, cronExpr, "Scheduled task")
				meta.kind = runtime.BackgroundTaskMonitor
			case "invoke_subagent":
				role := strings.TrimSpace(stringArg(args, "Role"))
				typeName := strings.TrimSpace(stringArg(args, "TypeName"))
				switch {
				case role != "" && typeName != "":
					meta.description = role + " (" + typeName + ")"
				case role != "":
					meta.description = role
				case typeName != "":
					meta.description = typeName
				default:
					meta.description = "Subagent"
				}
				meta.kind = runtime.BackgroundTaskAgent
			}
			r.pendingTools = append(r.pendingTools, meta)
		}
		return false

	case "GENERIC":
		var meta pendingToolMeta
		if len(r.pendingTools) > 0 {
			meta = r.pendingTools[0]
			r.pendingTools = r.pendingTools[1:]
		}
		return r.applyGenericStepLocked(step.Content, meta, stepTime)

	case "SYSTEM_MESSAGE":
		return r.applySystemMessageTextLocked(step.Content, stepTime)
	}
	return false
}

func (r *taskRegistry) applyGenericStepLocked(content string, meta pendingToolMeta, stepTime time.Time) bool {
	changed := false

	// 1. Background task spawned (`run_command` exceeding WaitMsBeforeAsync, or `schedule`).
	if m := bgTaskStartedPattern.FindStringSubmatch(content); m != nil {
		taskID := normalizeTaskID(m[1])
		var cmdLine string
		if dm := bgTaskDescPattern.FindStringSubmatch(content); dm != nil {
			cmdLine = strings.TrimSpace(dm[1])
		}
		if cmdLine == "" {
			cmdLine = meta.commandLine
		}
		var outputFile string
		if lm := bgTaskLogPattern.FindStringSubmatch(content); lm != nil {
			outputFile = fileURIToPath(lm[1])
		}
		if outputFile == "" && r.appDataDir != "" && r.sessionID != "" && taskID != "" {
			outputFile = filepath.Join(r.appDataDir, "brain", r.sessionID, ".system_generated", "tasks", taskID+".log")
		}
		kind := meta.kind
		if kind == "" {
			kind = runtime.BackgroundTaskShell
		}
		desc := meta.description
		if desc == "" {
			desc = cmdLine
		}
		callID := meta.toolCallID
		if callID == "" && cmdLine != "" {
			callID = r.acpCallsByCmd[cmdLine]
		}
		changed = r.startLocked(taskID, kind, desc, callID, outputFile, cmdLine, stepTime) || changed
	}

	// 2. Background subagent(s) spawned (`invoke_subagent`).
	if strings.Contains(content, subagentBlockLeadText) {
		for _, subID := range extractSpawnedSubagentIDs(content) {
			desc := meta.description
			if desc == "" {
				short := subID
				if len(short) > 8 {
					short = short[:8]
				}
				desc = "Subagent " + short
			}
			changed = r.startLocked(subID, runtime.BackgroundTaskAgent, desc, meta.toolCallID, "", "", stepTime) || changed
		}
	}

	// 3. Explicit `manage_task` kill or status check.
	if km := bgTaskKilledPattern.FindStringSubmatch(content); km != nil {
		changed = r.endLocked(normalizeTaskID(km[1]), "stopped", "Task was canceled.", "", stepTime) || changed
	}
	if sm := bgTaskStatusPattern.FindStringSubmatch(content); sm != nil {
		taskID := normalizeTaskID(sm[1])
		st := strings.ToUpper(strings.TrimSpace(sm[2]))
		switch st {
		case "DONE", "COMPLETED":
			changed = r.endLocked(taskID, "completed", "", "", stepTime) || changed
		case "FAILED", "ERROR":
			changed = r.endLocked(taskID, "failed", "", "", stepTime) || changed
		case "KILLED", "CANCELED", "CANCELLED", "STOPPED":
			changed = r.endLocked(taskID, "stopped", "", "", stepTime) || changed
		}
	}

	// 4. `manage_task` status can also drain and embed a `<SYSTEM_MESSAGE>` inline.
	if strings.Contains(content, "<SYSTEM_MESSAGE>") {
		changed = r.applySystemMessageTextLocked(content, stepTime) || changed
	}

	return changed
}

func (r *taskRegistry) applySystemMessageTextLocked(content string, fallbackTime time.Time) bool {
	bodies := extractAllTagInners(content, "SYSTEM_MESSAGE")
	if len(bodies) == 0 {
		bodies = []string{content}
	}
	changed := false
	for _, body := range bodies {
		m := sysMsgHeaderPattern.FindStringSubmatch(strings.TrimSpace(body))
		if m == nil {
			continue
		}
		ts, sender, _, msgContent := m[1], m[2], m[3], m[4]
		endedAt := parseStepTime(ts)
		if endedAt.IsZero() {
			endedAt = fallbackTime
		}
		changed = r.applyMessageLocked(sender, msgContent, endedAt.Format(time.RFC3339Nano), "", "") || changed
	}
	return changed
}

func (r *taskRegistry) applyMessageLocked(sender string, content string, timestamp string, toolCallID string, argsJSON string) bool {
	endedAt := parseStepTime(timestamp)
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}

	if sender == "system" && strings.Contains(content, "All your subagents and background tasks have been stopped") {
		changed := false
		for id, t := range r.tasks {
			if t.Status == runtime.BackgroundTaskRunning {
				changed = r.endLocked(id, "stopped", "Stopped due to server restart.", "", endedAt) || changed
			}
		}
		return changed
	}

	taskID := normalizeTaskID(sender)
	if taskID == "" {
		return false
	}

	// If a message arrives for a task whose start step hasn't been polled yet,
	// backfill its metadata from sourceMetadata when available.
	if _, exists := r.tasks[taskID]; !exists && strings.HasPrefix(taskID, "task-") {
		var cmdLine, desc string
		if argsJSON != "" {
			var parsed map[string]any
			if json.Unmarshal([]byte(argsJSON), &parsed) == nil {
				cmdLine = strings.TrimSpace(stringArg(parsed, "CommandLine"))
				desc = firstNonEmpty(strings.TrimSpace(stringArg(parsed, "toolSummary")), cmdLine)
			}
		}
		if desc != "" || toolCallID != "" {
			r.startLocked(taskID, runtime.BackgroundTaskShell, desc, toolCallID, "", cmdLine, endedAt)
		}
	}

	var outputFile string
	if lm := regexp.MustCompile(`(?m)^Log:\s*(\S+)\s*$`).FindStringSubmatch(content); lm != nil {
		outputFile = fileURIToPath(lm[1])
	}

	// Recurring cron schedule notification keeps the schedule task running.
	if strings.Contains(content, "Next scheduled fire time:") {
		return false
	}

	status, summary := terminalStatusFromMessage(content)
	return r.endLocked(taskID, status, summary, outputFile, endedAt)
}

func terminalStatusFromMessage(content string) (status string, summary string) {
	if strings.Contains(content, "was canceled") || strings.Contains(content, "was cancelled") {
		return "stopped", "Task was canceled."
	}
	if m := exitCodeLinePattern.FindStringSubmatch(content); m != nil {
		code := m[1]
		if code == "0" {
			return "completed", ""
		}
		return "failed", "Command exited with code " + code + "."
	}
	if strings.Contains(content, "terminated by a signal") {
		return "failed", "Command was terminated by a signal."
	}
	if strings.Contains(content, "failed with error") {
		return "failed", ""
	}
	return "completed", ""
}

func (r *taskRegistry) startLocked(taskID string, kind string, description string, toolCallID string, outputFile string, cmdLine string, startedAt time.Time) bool {
	if taskID == "" {
		return false
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	t, exists := r.tasks[taskID]
	if !exists {
		t = &runtime.BackgroundTask{
			ID:        taskID,
			Status:    runtime.BackgroundTaskRunning,
			StartedAt: startedAt,
		}
		r.tasks[taskID] = t
	}
	before := *t
	if t.Kind == "" && kind != "" {
		t.Kind = kind
	}
	if t.Description == "" && description != "" {
		t.Description = description
	}
	if t.ToolCallID == "" && toolCallID != "" {
		t.ToolCallID = toolCallID
	}
	if t.OutputFile == "" && outputFile != "" {
		t.OutputFile = outputFile
	}
	if cmdLine != "" {
		d := r.details[taskID]
		if d == nil {
			d = &taskDetail{}
			r.details[taskID] = d
		}
		if d.commandLine == "" {
			d.commandLine = cmdLine
		}
	}
	return !exists || *t != before
}

func (r *taskRegistry) endLocked(taskID string, status string, summary string, outputFile string, endedAt time.Time) bool {
	t, ok := r.tasks[taskID]
	if !ok {
		return false
	}
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}
	changed := false
	if t.Status == runtime.BackgroundTaskRunning {
		t.Status = status
		t.EndedAt = endedAt
		changed = true
	}
	if summary != "" && t.Summary == "" {
		t.Summary = summary
		changed = true
	}
	if outputFile != "" && t.OutputFile == "" {
		t.OutputFile = outputFile
		changed = true
	}
	return changed
}

func (r *taskRegistry) snapshotLocked() []runtime.BackgroundTask {
	var ended []*runtime.BackgroundTask
	for _, t := range r.tasks {
		if t.Status != runtime.BackgroundTaskRunning {
			ended = append(ended, t)
		}
	}
	if len(ended) > maxEndedTasks {
		sort.Slice(ended, func(i, j int) bool { return ended[i].EndedAt.After(ended[j].EndedAt) })
		for _, t := range ended[maxEndedTasks:] {
			delete(r.tasks, t.ID)
			delete(r.details, t.ID)
		}
	}
	out := make([]runtime.BackgroundTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		if t.OutputFile == "" && t.Kind == runtime.BackgroundTaskShell && r.appDataDir != "" && r.sessionID != "" {
			t.OutputFile = filepath.Join(r.appDataDir, "brain", r.sessionID, ".system_generated", "tasks", t.ID+".log")
		}
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func normalizeTaskID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "user" || raw == "system" {
		return ""
	}
	if idx := strings.LastIndexByte(raw, '/'); idx >= 0 {
		return strings.TrimSpace(raw[idx+1:])
	}
	return raw
}

func fileURIToPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "file://") {
		if u, err := url.Parse(raw); err == nil && u.Path != "" {
			return u.Path
		}
		return strings.TrimPrefix(raw, "file://")
	}
	return raw
}

func extractSpawnedSubagentIDs(content string) []string {
	idx := strings.Index(content, subagentBlockLeadText)
	if idx < 0 {
		return nil
	}
	rest := content[idx:]
	brace := strings.IndexAny(rest, "{[")
	if brace < 0 {
		return nil
	}
	rest = strings.TrimSpace(rest[brace:])
	dec := json.NewDecoder(strings.NewReader(rest))
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil
	}
	var ids []string
	switch typed := raw.(type) {
	case map[string]any:
		if id := strings.TrimSpace(stringVal(typed, "conversationId")); id != "" {
			ids = append(ids, id)
		}
	case []any:
		for _, item := range typed {
			if rec, ok := item.(map[string]any); ok {
				if id := strings.TrimSpace(stringVal(rec, "conversationId")); id != "" {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

func extractAllTagInners(text string, tag string) []string {
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	var out []string
	rest := text
	for {
		start := strings.Index(rest, openTag)
		if start < 0 {
			break
		}
		rest = rest[start+len(openTag):]
		end := strings.Index(rest, closeTag)
		if end < 0 {
			if trimmed := strings.TrimSpace(rest); trimmed != "" {
				out = append(out, trimmed)
			}
			break
		}
		if trimmed := strings.TrimSpace(rest[:end]); trimmed != "" {
			out = append(out, trimmed)
		}
		rest = rest[end+len(closeTag):]
	}
	return out
}

func normalizeToolArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if s, ok := v.(string); ok {
			trimmed := strings.TrimSpace(s)
			if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
				var unquoted string
				if json.Unmarshal([]byte(trimmed), &unquoted) == nil {
					out[k] = unquoted
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	switch v := args[key].(type) {
	case string:
		return v
	default:
		return ""
	}
}

func parseStepTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func agyAppDataDir(env []string, extraArgs []string) string {
	home := envLookup(env, "HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	geminiFlag := flagValue(extraArgs, "--gemini-dir", "--gemini_dir")
	if geminiFlag == "" {
		geminiFlag = ".gemini"
	}
	geminiDir := geminiFlag
	if !filepath.IsAbs(geminiDir) && home != "" {
		geminiDir = filepath.Join(home, geminiDir)
	}

	appDataFlag := flagValue(extraArgs, "--app-data-dir", "--app_data_dir")
	if appDataFlag == "" {
		appDataFlag = "antigravity-cli"
	}
	appDataDir := appDataFlag
	if !filepath.IsAbs(appDataDir) && geminiDir != "" {
		appDataDir = filepath.Join(geminiDir, appDataDir)
	}
	return filepath.Clean(appDataDir)
}

func envLookup(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimSpace(strings.TrimPrefix(env[i], prefix))
		}
	}
	return ""
}

func flagValue(args []string, names ...string) string {
	var found string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		for _, name := range names {
			if after, ok := strings.CutPrefix(arg, name+"="); ok {
				found = strings.TrimSpace(after)
			} else if arg == name && i+1 < len(args) {
				found = strings.TrimSpace(args[i+1])
				i++
			}
		}
	}
	return found
}

func matchesShellCommand(procArgs string, cmdLine string) bool {
	if procArgs == "" || cmdLine == "" {
		return false
	}
	if procArgs == cmdLine || strings.HasSuffix(procArgs, " "+cmdLine) || strings.HasSuffix(procArgs, "\x00"+cmdLine) {
		return true
	}
	return strings.Contains(procArgs, cmdLine)
}

func cleanTranscriptToolOutput(content string) string {
	lines := strings.Split(content, "\n")
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "Created At: ") || strings.HasPrefix(trimmed, "Completed At: ") {
			i++
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines[i:], "\n"))
}

func fmtStepCallID(stepIndex int, idx int, existing string) string {
	if strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing)
	}
	return fmt.Sprintf("agy_bg_%d_%d", stepIndex, idx)
}
