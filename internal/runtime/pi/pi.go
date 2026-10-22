// Package pi runs the pi coding agent (https://pi.dev) as a long-lived
// `pi --mode rpc` subprocess speaking JSONL over stdio. One process per
// AgentX session key is kept in a procpool.ProcessPool; the pi session itself
// is persisted under Options.SessionDir and resumed with --session-id.
package pi

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	// ExtensionDir is where the bundled AgentX extension is written before it
	// is loaded with `-e`. Empty disables the extension (no ask_user /
	// update_plan tools, no plan or yolo enforcement).
	ExtensionDir string
	// SubagentForeground makes pi-subagents run children in the foreground by
	// default (asyncByDefault=false in its config.json) so their progress and
	// results stream into the AgentX reply instead of arriving as a background
	// notification on a later turn. An explicit value in the file is kept.
	SubagentForeground bool
}

type Runtime struct {
	opts Options
	pool *procpool.ProcessPool

	extOnce sync.Once
	extPath string
	extErr  error

	subagentOnce sync.Once
}

// extensionSource is the AgentX bridge extension loaded into every pi process.
//
//go:embed extension/agentx.ts
var extensionSource string

const extensionFileName = "agentx.ts"

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

	if err := r.ensureExtension(); err != nil {
		return nil, err
	}
	r.ensureSubagentConfig(req.Env)

	key := sessionKey(req)
	previousSessionID := usablePreviousSessionID(req.PreviousSessionID)
	startSessionID := previousSessionID
	if startSessionID == "" {
		startSessionID = newSessionID()
	}

	// The system prompt only applies at process start; a changed prompt
	// restarts the idle process, and --session-id resumes the conversation.
	proc, isNew, err := r.pool.GetOrCreateSigned(key, strings.TrimSpace(req.SystemPrompt), r.processStartFunc(req, startSessionID))
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
	if prompt := joinPrompts(r.opts.AppendSystemPrompt, req.SystemPrompt); prompt != "" {
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
	if r.extPath != "" {
		args = append(args, "-e", r.extPath)
	}
	args = append(args, r.opts.ExtraArgs...)
	return args
}

// ensureExtension writes the bundled extension to Options.ExtensionDir (once
// per process, and only when the content changed) so pi can load it.
func (r *Runtime) ensureExtension() error {
	r.extOnce.Do(func() {
		dir := strings.TrimSpace(r.opts.ExtensionDir)
		if dir == "" {
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.extErr = fmt.Errorf("pi extension dir: %w", err)
			return
		}
		path := filepath.Join(dir, extensionFileName)
		if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, []byte(extensionSource)) {
			r.extPath = path
			return
		}
		if err := os.WriteFile(path, []byte(extensionSource), 0o644); err != nil {
			r.extErr = fmt.Errorf("write pi extension: %w", err)
			return
		}
		r.extPath = path
	})
	return r.extErr
}

// subagentConfigPath is the pi-subagents configuration file, relative to pi's
// agent dir (`~/.pi/agent` unless PI_CODING_AGENT_DIR overrides it).
const subagentConfigPath = "extensions/subagent/config.json"

// ensureSubagentConfig writes asyncByDefault=false into the pi-subagents
// config when the key is not set yet. It runs once per AgentX process and
// never overrides a value the user chose. Failures are logged, not fatal.
func (r *Runtime) ensureSubagentConfig(reqEnv map[string]string) {
	if !r.opts.SubagentForeground {
		return
	}
	r.subagentOnce.Do(func() {
		agentDir := piAgentDir(r.opts.Env, reqEnv)
		if agentDir == "" {
			return
		}
		path := filepath.Join(agentDir, filepath.FromSlash(subagentConfigPath))
		if err := mergeSubagentConfig(path); err != nil {
			logWarn("could not set pi-subagents asyncByDefault=false", "path", path, "error", err)
		}
	})
}

// mergeSubagentConfig adds asyncByDefault=false to the JSON object at path,
// creating the file when missing and leaving an existing key untouched.
func mergeSubagentConfig(path string) error {
	config := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("parse: %w", err)
			}
		}
	case os.IsNotExist(err):
	default:
		return err
	}
	if _, set := config["asyncByDefault"]; set {
		return nil
	}
	config["asyncByDefault"] = false
	out, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return err
	}
	slog.Info("pi: set pi-subagents asyncByDefault=false so subagents run in the foreground", "path", path)
	return nil
}

// piAgentDir resolves pi's agent directory from the runtime or agent env
// (PI_CODING_AGENT_DIR), the process env, or the default ~/.pi/agent.
func piAgentDir(layers ...map[string]string) string {
	for i := len(layers) - 1; i >= 0; i-- {
		if dir := strings.TrimSpace(layers[i]["PI_CODING_AGENT_DIR"]); dir != "" {
			return dir
		}
	}
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".pi", "agent")
}

// DefaultExtensionDir returns where the bundled pi extension lives under the
// AgentX data dir.
func DefaultExtensionDir(dataDir string) string {
	return filepath.Join(dataDir, "pi-extension")
}

// joinPrompts concatenates non-empty system prompt fragments.
func joinPrompts(prompts ...string) string {
	var parts []string
	for _, prompt := range prompts {
		if prompt = strings.TrimSpace(prompt); prompt != "" {
			parts = append(parts, prompt)
		}
	}
	return strings.Join(parts, "\n\n")
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

// modeState is the plan/yolo mode last pushed to a pi process through the
// bundled extension's /agentx-mode command.
type modeState struct {
	Plan bool
	Yolo bool
}

type modeStateKey struct{}

func processMode(proc *procpool.ManagedProcess) (modeState, bool) {
	proc.Mu.Lock()
	defer proc.Mu.Unlock()
	v, ok := proc.UserData[modeStateKey{}]
	if !ok {
		return modeState{}, false
	}
	return v.(modeState), true
}

func setProcessMode(proc *procpool.ManagedProcess, mode modeState) {
	proc.Mu.Lock()
	defer proc.Mu.Unlock()
	if proc.UserData == nil {
		proc.UserData = make(map[any]any)
	}
	proc.UserData[modeStateKey{}] = mode
}

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
