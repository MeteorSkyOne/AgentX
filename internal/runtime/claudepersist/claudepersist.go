package claudepersist

import (
	"context"
	"strings"
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
	ExtraArgs          []string
	Env                map[string]string
	IdleTimeout        time.Duration
}

type Runtime struct {
	opts Options
	pool *procpool.ProcessPool
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

	modeOverride := strings.TrimSpace(req.PermissionMode) != ""
	key := sessionKey(req)

	if modeOverride {
		if existing, ok := r.pool.Get(key); ok {
			r.pool.Detach(existing)
			existing.Kill()
		}
	}

	proc, isNew, err := r.pool.GetOrCreate(key, r.processStartFunc(req))
	if err != nil {
		return nil, err
	}

	sess := newPersistentSession(proc, key, r)
	sess.detachOnClose = modeOverride
	if isNew {
		if err := sess.waitForSystemEvent(ctx); err != nil {
			r.pool.Detach(proc)
			proc.Kill()
			return nil, err
		}
	}
	return sess, nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	return r.pool.Shutdown(ctx)
}

func sessionKey(req runtime.StartSessionRequest) string {
	if req.SessionKey != "" {
		return req.SessionKey
	}
	return req.AgentID
}
