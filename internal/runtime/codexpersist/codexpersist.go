package codexpersist

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

type Options struct {
	Command     string
	ExtraArgs   []string
	Env         map[string]string
	IdleTimeout time.Duration
}

type Runtime struct {
	opts Options
	pool *procpool.ProcessPool
}

func New(opts Options) *Runtime {
	if strings.TrimSpace(opts.Command) == "" {
		opts.Command = "codex"
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
	proc, isNew, err := r.pool.GetOrCreate(key, r.processStartFunc(req))
	if err != nil {
		return nil, err
	}

	rpc := getRPCClient(proc)
	if isNew {
		if err := r.initializeServer(ctx, proc, rpc); err != nil {
			r.pool.Kill(key)
			return nil, err
		}
	}

	sess := newPersistentSession(proc, rpc, key, r, req)
	return sess, nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	return r.pool.Shutdown(ctx)
}

func (r *Runtime) processStartFunc(req runtime.StartSessionRequest) procpool.StartFunc {
	return func(ctx context.Context) *exec.Cmd {
		args := []string{"app-server", "--listen", "stdio://"}
		args = append(args, r.opts.ExtraArgs...)
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

func (r *Runtime) initializeServer(ctx context.Context, proc *procpool.ManagedProcess, rpc *rpcClient) error {
	_, err := rpc.Call(ctx, "initialize", initializeParams())
	if err != nil {
		return fmt.Errorf("codex app-server initialize: %w", err)
	}
	if err := rpc.Notify("initialized", map[string]any{}); err != nil {
		return fmt.Errorf("codex app-server initialized notification: %w", err)
	}
	slog.Info("codexpersist: server initialized", "pid", proc.Key)
	return nil
}

func initializeParams() map[string]any {
	return map[string]any{
		"clientInfo": map[string]any{
			"name":    "agentx",
			"title":   "AgentX",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}
}

func sessionKey(req runtime.StartSessionRequest) string {
	if req.SessionKey != "" {
		return req.SessionKey
	}
	return req.AgentID
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
