package claudepersist

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

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
	args := []string{
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--permission-prompt-tool", "stdio",
		"--verbose",
	}

	if model := strings.TrimSpace(req.Model); model != "" && !req.FastMode {
		args = append(args, "--model", model)
	}
	if effort := strings.TrimSpace(req.Effort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if req.FastMode {
		args = append(args, "--settings", `{"fastMode":true}`)
	}

	mode := strings.TrimSpace(r.opts.PermissionMode)
	if override := strings.TrimSpace(req.PermissionMode); override != "" {
		mode = override
	} else if req.YoloMode {
		mode = "bypassPermissions"
	}
	if mode != "" {
		args = append(args, "--permission-mode", mode)
	}

	if tools := strings.Join(r.opts.AllowedTools, ","); tools != "" {
		args = append(args, "--allowedTools", tools)
	}
	if tools := strings.Join(r.opts.DisallowedTools, ","); tools != "" {
		args = append(args, "--disallowedTools", tools)
	}

	if prompt := strings.TrimSpace(r.opts.AppendSystemPrompt); prompt != "" {
		args = append(args, "--append-system-prompt", prompt)
	}

	if previousSessionID := usablePreviousSessionID(req.PreviousSessionID); previousSessionID != "" {
		args = append(args, "--resume", previousSessionID)
	}

	args = append(args, r.opts.ExtraArgs...)
	return args
}

func usablePreviousSessionID(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "claude:") {
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
	seen := make(map[string]struct{}, len(base))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			env = append(env, item)
			continue
		}
		if _, override := overrides[key]; override {
			continue
		}
		seen[key] = struct{}{}
		env = append(env, item)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}
