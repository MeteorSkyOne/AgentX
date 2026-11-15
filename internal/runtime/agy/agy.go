// Package agy runs the Google Antigravity CLI (`agy acp`) as a long-lived
// Agent Client Protocol (ACP) subprocess speaking JSON-RPC 2.0 over stdio.
// One process per AgentX session key is kept in a procpool.ProcessPool;
// conversations persist in Antigravity's own session store and resume via
// `session/resume` (with `session/load` fallback).
package agy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

const (
	startupTimeout = 90 * time.Second
	maxDetailBytes = 4000
)

type Options struct {
	Command            string
	PermissionMode     string
	AppendSystemPrompt string
	ExtraArgs          []string
	Env                map[string]string
	IdleTimeout        time.Duration

	pollInterval time.Duration
	claimTimeout time.Duration
}

type Runtime struct {
	opts Options
	pool *procpool.ProcessPool
	// backgroundTurn receives turns Antigravity starts on its own when a
	// background task, subagent, or schedule wakes the session; see
	// runtime.BackgroundTurnNotifier.
	backgroundTurn atomic.Pointer[func(sessionKey string) bool]
	// backgroundTasks receives each session's background task list as it
	// changes; see runtime.BackgroundTaskReporter.
	backgroundTasks atomic.Pointer[func(sessionKey string, tasks []runtime.BackgroundTask)]
}

func New(opts Options) *Runtime {
	if strings.TrimSpace(opts.Command) == "" {
		opts.Command = "agy"
	}
	if strings.TrimSpace(opts.PermissionMode) == "" {
		opts.PermissionMode = "accept-edits"
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = 30 * time.Minute
	}
	if opts.pollInterval <= 0 {
		opts.pollInterval = defaultBrainPollInterval
	}
	if opts.claimTimeout <= 0 {
		opts.claimTimeout = defaultBackgroundTurnClaimTimeout
	}
	return &Runtime{
		opts: opts,
		pool: procpool.New(procpool.Options{IdleTimeout: opts.IdleTimeout}),
	}
}

func (r *Runtime) StartSession(ctx context.Context, req runtime.StartSessionRequest) (runtime.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := sessionKey(req)
	proc, isNew, err := r.pool.GetOrCreateSigned(key, r.launchSignature(req), r.processStartFunc(req))
	if err != nil {
		return nil, err
	}

	env := mergeEnv(os.Environ(), r.opts.Env, req.Env)
	rpc := getRPCClient(proc)
	mon := getBrainMonitor(proc, key, r, env)
	sess := newPersistentSession(proc, rpc, mon, key, r, req)
	if isNew {
		if err := sess.initializeProcess(ctx); err != nil {
			r.pool.Detach(proc)
			proc.Kill()
			return nil, err
		}
	} else {
		if !proc.InUse() {
			// Reused idle process: process any buffered config/usage updates and
			// discard leftover turn notifications before starting the next turn.
			rpc.drainAndCacheUpdates()
		}
		prevID := usablePreviousSessionID(req.PreviousSessionID)
		if prevID == "" {
			sess.needsNewSession = true
		} else {
			sess.sessionID = prevID
		}
	}
	return sess, nil
}

func (r *Runtime) SetBackgroundTurnHandler(handler func(sessionKey string) bool) {
	if handler == nil {
		r.backgroundTurn.Store(nil)
		return
	}
	r.backgroundTurn.Store(&handler)
}

func (r *Runtime) startBackgroundTurn(sessionKey string) bool {
	handler := r.backgroundTurn.Load()
	return handler != nil && (*handler)(sessionKey)
}

func (r *Runtime) SetBackgroundTaskHandler(handler func(sessionKey string, tasks []runtime.BackgroundTask)) {
	if handler == nil {
		r.backgroundTasks.Store(nil)
		return
	}
	r.backgroundTasks.Store(&handler)
}

func (r *Runtime) reportBackgroundTasks(sessionKey string, tasks []runtime.BackgroundTask) {
	if handler := r.backgroundTasks.Load(); handler != nil {
		(*handler)(sessionKey, tasks)
	}
}

func (r *Runtime) StopBackgroundTask(_ context.Context, sessionKey string, taskID string) error {
	proc, ok := r.pool.Get(sessionKey)
	if !ok {
		return errNoSuchBackgroundTask
	}
	mon := monitorOf(proc)
	if mon == nil || mon.registry == nil {
		return errNoSuchBackgroundTask
	}
	kind, cmdLine, running := mon.registry.taskCommandLine(taskID)
	if !running {
		return errNoSuchBackgroundTask
	}
	if kind == runtime.BackgroundTaskShell {
		if cmdLine != "" {
			if err := stopProcessBackgroundTask(proc.PID(), cmdLine); err != nil && !errors.Is(err, errNoSuchBackgroundTask) {
				return err
			}
		}
		mon.registry.markStopped(taskID, "Stopped by user.")
		return nil
	}

	// Schedules (`monitor`) and subagents (`agent`) run inside `agy acp` with no
	// per-task ACP cancel RPC. Terminate any background child shells first, mark
	// tasks stopped, and recycle the pooled `agy acp` process; the next turn
	// resumes the conversation via `session/resume`.
	for _, shellCmd := range mon.registry.runningShellCommands() {
		_ = stopProcessBackgroundTask(proc.PID(), shellCmd)
	}
	mon.registry.markAllStopped("Stopped by user.")
	r.pool.Kill(sessionKey)
	return nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	return r.pool.Shutdown(ctx)
}

func (r *Runtime) ResetProcesses(ctx context.Context) error {
	return r.pool.KillAll(ctx)
}

func (r *Runtime) processStartFunc(req runtime.StartSessionRequest) procpool.StartFunc {
	return func(ctx context.Context) *exec.Cmd {
		args := r.buildArgs(req)
		cmd := exec.CommandContext(ctx, r.opts.Command, args...)
		workspace := strings.TrimSpace(req.Workspace)
		if workspace == "" {
			workspace = "."
		}
		cmd.Dir = workspace
		cmd.Env = mergeEnv(os.Environ(), r.opts.Env, req.Env)
		return cmd
	}
}

func (r *Runtime) buildArgs(req runtime.StartSessionRequest) []string {
	args := []string{"acp"}
	if r.effectiveSkipPermissions(req) {
		args = append(args, "--dangerously-skip-permissions")
	}
	if dir := extraInstructionWorkspace(req); dir != "" {
		args = append(args, "--add-dir", dir)
	}
	args = append(args, r.opts.ExtraArgs...)
	return args
}

func (r *Runtime) effectiveSkipPermissions(req runtime.StartSessionRequest) bool {
	if strings.EqualFold(strings.TrimSpace(req.PermissionMode), "plan") {
		return false
	}
	if req.YoloMode {
		return true
	}
	return normalizePermissionMode(r.opts.PermissionMode) == "auto"
}

// launchSignature identifies process-launch parameters (workspace, additional
// directories, skip-permissions flag, composed system prompt, and environment)
// so procpool restarts an idle process when any of them change.
func (r *Runtime) launchSignature(req runtime.StartSessionRequest) string {
	workspace := filepath.Clean(strings.TrimSpace(req.Workspace))
	extraDir := extraInstructionWorkspace(req)
	skipPerms := strconv.FormatBool(r.effectiveSkipPermissions(req))
	prompt := r.composeSystemPrompt(req)
	return strings.Join([]string{
		workspace,
		extraDir,
		skipPerms,
		runtime.NormalizeSystemPromptMode(req.SystemPromptMode),
		prompt,
		req.EnvSignature(),
	}, "\x00")
}

// composeSystemPrompt builds the prompt text passed in
// `_meta.antigravity.systemPrompt.append` on `session/new` and `session/resume`.
// Antigravity ACP exposes an append hook for custom instructions; when an agent
// selects SystemPromptModeReplace we still supply its custom prompt with an
// explicit precedence header, followed by operator-level appended text and any
// separate agent workspace instruction file.
func (r *Runtime) composeSystemPrompt(req runtime.StartSessionRequest) string {
	var parts []string
	if req.ReplacesSystemPrompt() {
		if custom := strings.TrimSpace(req.SystemPrompt); custom != "" {
			parts = append(parts, "Primary agent system instructions (take precedence over default persona instructions):\n\n"+custom)
		}
	} else if appended := req.AppendedSystemPrompt(); appended != "" {
		parts = append(parts, appended)
	}
	if op := strings.TrimSpace(r.opts.AppendSystemPrompt); op != "" {
		parts = append(parts, op)
	}
	if dir := extraInstructionWorkspace(req); dir != "" {
		if instructions := workspaceInstructions(dir); instructions != "" {
			parts = append(parts, "AgentX agent workspace instructions. Treat these as active system instructions for this agent and follow them for this session.\n\n"+instructions)
		}
	}
	return strings.Join(parts, "\n\n")
}

func extraInstructionWorkspace(req runtime.StartSessionRequest) string {
	instructionWorkspace := strings.TrimSpace(req.InstructionWorkspace)
	if instructionWorkspace == "" {
		return ""
	}
	cleanInstr := filepath.Clean(instructionWorkspace)
	cleanWork := filepath.Clean(strings.TrimSpace(req.Workspace))
	if req.Workspace != "" && cleanInstr == cleanWork {
		return ""
	}
	return cleanInstr
}

func workspaceInstructions(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	for _, name := range []string{"GEMINI.md", "AGENTS.override.md", "AGENTS.md", "CLAUDE.md", "memory.md"} {
		content, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil {
			continue
		}
		if text := strings.TrimSpace(string(content)); text != "" {
			return text
		}
	}
	return ""
}

func sessionKey(req runtime.StartSessionRequest) string {
	if req.SessionKey != "" {
		return req.SessionKey
	}
	return req.AgentID
}

func usablePreviousSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.HasPrefix(id, "agy:") {
		return ""
	}
	return id
}

func mergeEnv(base []string, layers ...map[string]string) []string {
	overrides := map[string]string{}
	for _, layer := range layers {
		for k, v := range layer {
			overrides[k] = v
		}
	}
	if len(overrides) == 0 {
		return base
	}
	env := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			env = append(env, item)
			continue
		}
		if _, override := overrides[key]; override {
			continue
		}
		env = append(env, item)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}

type rpcClientKey struct{}

func getRPCClient(proc *procpool.ManagedProcess) *rpcClient {
	proc.Mu.Lock()
	defer proc.Mu.Unlock()
	if v, ok := proc.UserData[rpcClientKey{}]; ok {
		return v.(*rpcClient)
	}
	client := newRPCClient(proc)
	if proc.UserData == nil {
		proc.UserData = make(map[any]any)
	}
	proc.UserData[rpcClientKey{}] = client
	return client
}

func agyExitedError(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return "agy acp exited"
	}
	return truncateDetail("agy acp exited: " + stderr)
}

func truncateDetail(text string) string {
	if len(text) <= maxDetailBytes {
		return text
	}
	return text[:maxDetailBytes] + "...(truncated)"
}

func logWarn(msg string, args ...any) {
	slog.Warn(fmt.Sprintf("agy: %s", msg), args...)
}
