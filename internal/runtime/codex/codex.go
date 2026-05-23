package codex

import (
	"context"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/cli"
)

type Options struct {
	Command          string
	FullAuto         bool
	BypassSandbox    bool
	SkipGitRepoCheck bool
	ExtraArgs        []string
	Env              map[string]string
}

type Runtime struct {
	opts Options
}

func New(opts Options) runtime.Runtime {
	if strings.TrimSpace(opts.Command) == "" {
		opts.Command = "codex"
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
	fallbackID := "codex:" + req.SessionKey
	if req.SessionKey == "" {
		fallbackID = "codex:" + req.AgentID
	}
	handler := newLineHandler(fallbackID)
	build := func(input runtime.Input) (cli.Command, error) {
		args, stdin := r.buildArgsAndStdin(req, input)
		return cli.Command{
			Name:  r.opts.Command,
			Args:  args,
			Dir:   workspace,
			Env:   mergeMaps(r.opts.Env, req.Env),
			Stdin: stdin,
		}, nil
	}
	return cli.NewSession(fallbackID, build, handler), nil
}
