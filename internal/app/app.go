package app

import (
	"strings"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/fake"
	"github.com/meteorsky/agentx/internal/store"
)

type Options struct {
	AdminToken        string
	DataDir           string
	DefaultAgentKind  string
	DefaultAgentName  string
	DefaultAgentModel string
	Runtimes          map[string]agentruntime.Runtime
}

type App struct {
	store store.Store
	bus   *eventbus.Bus
	opts  Options
}

func New(st store.Store, bus *eventbus.Bus, opts Options) *App {
	if opts.Runtimes == nil {
		opts.Runtimes = map[string]agentruntime.Runtime{
			domain.AgentKindFake: fake.New(),
		}
	}
	return &App{store: st, bus: bus, opts: opts}
}

func (a *App) runtimeForAgent(agent domain.Agent) (agentruntime.Runtime, bool) {
	kind := strings.TrimSpace(agent.Kind)
	if kind == "" {
		kind = domain.AgentKindFake
	}
	rt, ok := a.opts.Runtimes[kind]
	return rt, ok
}

func (a *App) defaultAgentKind() string {
	if kind := strings.TrimSpace(a.opts.DefaultAgentKind); kind != "" {
		return kind
	}
	return domain.AgentKindFake
}

func (a *App) defaultAgentName() string {
	if name := strings.TrimSpace(a.opts.DefaultAgentName); name != "" {
		return name
	}
	switch a.defaultAgentKind() {
	case domain.AgentKindCodex:
		return "Codex"
	case domain.AgentKindClaude:
		return "Claude Code"
	default:
		return "Fake Agent"
	}
}

func (a *App) defaultAgentModel() string {
	if model := strings.TrimSpace(a.opts.DefaultAgentModel); model != "" {
		return model
	}
	if a.defaultAgentKind() == domain.AgentKindFake {
		return "fake-echo"
	}
	return ""
}
