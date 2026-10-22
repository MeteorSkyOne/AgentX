// Package pi runs the pi coding agent (https://pi.dev) as a long-lived
// `pi --mode rpc` subprocess speaking JSONL over stdio. One process per
// AgentX session key is kept in a procpool.ProcessPool; the pi session itself
// is persisted under Options.SessionDir and resumed with --session-id.
package pi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

const (
	// ProjectTrustApprove trusts project-local pi configuration (`--approve`).
	ProjectTrustApprove = "approve"
	// ProjectTrustDeny ignores trust-gated project-local resources (`--no-approve`).
	ProjectTrustDeny = "no-approve"
	// ProjectTrustDefault leaves the decision to pi's own settings.
	ProjectTrustDefault = ""

	startupTimeout = 90 * time.Second
)

type Options struct {
	Command            string
	ExtraArgs          []string
	Env                map[string]string
	IdleTimeout        time.Duration
	SessionDir         string
	AppendSystemPrompt string
	Tools              []string
	ExcludeTools       []string
	ProjectTrust       string
}

type Runtime struct {
	opts Options
	pool *procpool.ProcessPool
}

func New(opts Options) *Runtime {
	if strings.TrimSpace(opts.Command) == "" {
		opts.Command = "pi"
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
	previousSessionID := usablePreviousSessionID(req.PreviousSessionID)
	startSessionID := previousSessionID
	if startSessionID == "" {
		startSessionID = newSessionID()
	}

	proc, isNew, err := r.pool.GetOrCreate(key, r.processStartFunc(req, startSessionID))
	if err != nil {
		return nil, err
	}

	rpc := getRPCClient(proc)
	sess := newPersistentSession(proc, rpc, key, r, req)
	if isNew {
		sess.sessionID = startSessionID
		if err := sess.waitReady(ctx); err != nil {
			r.pool.Detach(proc)
			proc.Kill()
			return nil, err
		}
	} else {
		// Reused process: drop events left over from the previous turn before
		// this session starts its own.
		rpc.drain()
		// A reused process with no stored provider session means the user asked
		// for a fresh context (/new); the turn starts a new pi session.
		sess.needsNewSession = previousSessionID == ""
		sess.model = rpc.lastModel()
		if previousSessionID != "" {
			sess.sessionID = previousSessionID
		} else {
			sess.sessionID = rpc.lastSessionID()
		}
	}
	return sess, nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	return r.pool.Shutdown(ctx)
}

func (r *Runtime) ResetProcesses(ctx context.Context) error {
	return r.pool.KillAll(ctx)
}

func (r *Runtime) processStartFunc(req runtime.StartSessionRequest, sessionID string) procpool.StartFunc {
	return func(ctx context.Context) *exec.Cmd {
		args := r.buildArgs(req, sessionID)
		cmd := exec.CommandContext(ctx, r.opts.Command, args...)
		workspace := strings.TrimSpace(req.Workspace)
		if workspace == "" {
			workspace = "."
		}
		cmd.Dir = workspace
		env := map[string]string{"PI_SKIP_VERSION_CHECK": "1"}
		cmd.Env = mergeEnv(os.Environ(), env, r.opts.Env, req.Env)
		return cmd
	}
}

// buildArgs assembles the `pi --mode rpc` command line for a new process.
// Model and thinking level are passed here for a fresh process; a reused
// process is brought in line with set_model / set_thinking_level per turn.
func (r *Runtime) buildArgs(req runtime.StartSessionRequest, sessionID string) []string {
	args := []string{"--mode", "rpc"}
	if dir := strings.TrimSpace(r.opts.SessionDir); dir != "" {
		args = append(args, "--session-dir", dir)
	}
	if sessionID != "" {
		args = append(args, "--session-id", sessionID)
	}
	model, modelThinking := splitModelThinking(req.Model)
	if model != "" {
		args = append(args, "--model", model)
	}
	if level := thinkingLevelForEffort(req.Effort, modelThinking); level != "" {
		args = append(args, "--thinking", level)
	}
	if prompt := strings.TrimSpace(r.opts.AppendSystemPrompt); prompt != "" {
		args = append(args, "--append-system-prompt", prompt)
	}
	if len(r.opts.Tools) > 0 {
		args = append(args, "--tools", strings.Join(r.opts.Tools, ","))
	}
	// Without the pi-subagents loader tool the `subagent` tool stays active from
	// the first turn, so delegation works without a mid-conversation tool swap.
	// Excluding a tool that is not installed is a no-op.
	exclude := append([]string{"subagents_enable"}, r.opts.ExcludeTools...)
	args = append(args, "--exclude-tools", strings.Join(exclude, ","))
	switch strings.TrimSpace(r.opts.ProjectTrust) {
	case ProjectTrustApprove:
		args = append(args, "--approve")
	case ProjectTrustDeny:
		args = append(args, "--no-approve")
	}
	args = append(args, r.opts.ExtraArgs...)
	return args
}

func sessionKey(req runtime.StartSessionRequest) string {
	if req.SessionKey != "" {
		return req.SessionKey
	}
	return req.AgentID
}

// usablePreviousSessionID returns a stored provider session id that pi can
// resume, or "" when there is none.
func usablePreviousSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.HasPrefix(id, "pi:") {
		return ""
	}
	return id
}

// newSessionID returns a fresh pi session id. pi accepts letters, digits, `.`,
// `_` and `-`, starting and ending with a letter or digit.
func newSessionID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("agentx-%d", time.Now().UnixNano())
	}
	return "agentx-" + hex.EncodeToString(buf[:])
}

// DefaultSessionDir returns the pi session directory under the AgentX data dir.
func DefaultSessionDir(dataDir string) string {
	return filepath.Join(dataDir, "pi-sessions")
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

func piExitedError(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return "pi exited"
	}
	return truncateDetail("pi exited: " + stderr)
}

const maxDetailBytes = 4000

func truncateDetail(text string) string {
	if len(text) <= maxDetailBytes {
		return text
	}
	return text[:maxDetailBytes] + "...(truncated)"
}

func logWarn(msg string, args ...any) {
	slog.Warn("pi: "+msg, args...)
}
