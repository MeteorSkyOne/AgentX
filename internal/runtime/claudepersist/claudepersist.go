package claudepersist

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

type Options struct {
	Command            string
	PermissionMode     string
	AllowedTools       []string
	DisallowedTools    []string
	AppendSystemPrompt string
	// ThinkingDisplay is passed as --thinking-display (summarized, omitted,
	// highlights). Newer models omit thinking text in print mode unless
	// summaries are requested explicitly.
	ThinkingDisplay string
	ExtraArgs       []string
	Env             map[string]string
	IdleTimeout     time.Duration
}

type Runtime struct {
	opts Options
	pool *procpool.ProcessPool
	// backgroundTurn is the handler for turns Claude Code starts on its own;
	// see runtime.BackgroundTurnNotifier.
	backgroundTurn atomic.Pointer[func(sessionKey string) bool]
	// backgroundTasks receives each session's task list when it changes; see
	// runtime.BackgroundTaskReporter.
	backgroundTasks atomic.Pointer[func(sessionKey string, tasks []runtime.BackgroundTask)]
}

func New(opts Options) *Runtime {
	if strings.TrimSpace(opts.Command) == "" {
		opts.Command = "claude"
	}
	if strings.TrimSpace(opts.PermissionMode) == "" {
		opts.PermissionMode = "acceptEdits"
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = 30 * time.Minute
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
	// The system prompt and env only apply at process start; a change restarts
	// the idle process, and --resume continues the conversation.
	proc, isNew, err := r.pool.GetOrCreateSigned(key, launchSignature(req), r.processStartFunc(req))
	if err != nil {
		return nil, err
	}

	sess := newPersistentSession(proc, key, r)
	if override := strings.TrimSpace(req.PermissionMode); override != "" {
		sess.modeOverride = override
		sess.baseMode = r.basePermissionMode(req)
	}
	if isNew {
		registry := newTaskRegistry(key, req.Workspace, mergeEnv(os.Environ(), r.opts.Env, req.Env), r.reportBackgroundTasks)
		attachRegistry(proc, registry)
		go func() {
			<-proc.Done()
			registry.processExited()
		}()
		if err := sess.waitForSystemEvent(ctx); err != nil {
			r.pool.Detach(proc)
			proc.Kill()
			return nil, err
		}
		proc.SetFallbackHandler(newBackgroundFallback(proc, key, r.startBackgroundTurn))
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

// startBackgroundTurn offers a turn the process started on its own to the
// handler, reporting whether it will be adopted.
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

var errNoSuchBackgroundTask = errors.New("no such running background task")

// StopBackgroundTask stops a running background task of the session's process,
// whether or not a turn is in progress.
func (r *Runtime) StopBackgroundTask(_ context.Context, sessionKey string, taskID string) error {
	proc, ok := r.pool.Get(sessionKey)
	if !ok {
		return errNoSuchBackgroundTask
	}
	registry := registryOf(proc)
	if registry == nil {
		return errNoSuchBackgroundTask
	}
	running := false
	for _, task := range registry.snapshot() {
		if task.ID == taskID && task.Status == runtime.BackgroundTaskRunning {
			running = true
		}
	}
	if !running {
		return errNoSuchBackgroundTask
	}
	return writeStopTask(proc, taskID)
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	return r.pool.Shutdown(ctx)
}

func (r *Runtime) ResetProcesses(ctx context.Context) error {
	return r.pool.KillAll(ctx)
}

func (r *Runtime) basePermissionMode(req runtime.StartSessionRequest) string {
	if req.YoloMode {
		return "bypassPermissions"
	}
	return r.opts.PermissionMode
}

func sessionKey(req runtime.StartSessionRequest) string {
	if req.SessionKey != "" {
		return req.SessionKey
	}
	return req.AgentID
}
