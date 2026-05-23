package claude

import (
	"context"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/cli"
)

type Options struct {
	Command            string
	PermissionMode     string
	AllowedTools       []string
	DisallowedTools    []string
	AppendSystemPrompt string
	ExtraArgs          []string
	Env                map[string]string
}

type Runtime struct {
	opts Options
}

func New(opts Options) runtime.Runtime {
	if strings.TrimSpace(opts.Command) == "" {
		opts.Command = "claude"
	}
	if strings.TrimSpace(opts.PermissionMode) == "" {
		opts.PermissionMode = "acceptEdits"
	}
	return Runtime{opts: opts}
}

func (r Runtime) StartSession(ctx context.Context, req runtime.StartSessionRequest) (runtime.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		workspace = "."
	}
	fallbackID := "claude:" + req.SessionKey
	if req.SessionKey == "" {
		fallbackID = "claude:" + req.AgentID
	}
	handler := newLineHandler(fallbackID)
	build := func(input runtime.Input) (cli.Command, error) {
		stdin, err := claudeStreamJSONInput(input)
		if err != nil {
			return cli.Command{}, err
		}
		return cli.Command{
			Name:  r.opts.Command,
			Args:  r.buildArgs(req, input),
			Dir:   workspace,
			Env:   r.buildEnv(req),
			Stdin: stdin,
		}, nil
	}
	return cli.NewSession(fallbackID, build, handler), nil
}
