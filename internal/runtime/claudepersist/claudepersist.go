package claudepersist

import (
	"context"
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
