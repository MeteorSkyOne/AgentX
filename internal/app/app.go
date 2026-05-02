package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/config"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/fake"
	"github.com/meteorsky/agentx/internal/store"
)

type Options struct {
	AdminToken            string
	DataDir               string
	ServerSettings        config.ServerSettings
	ServerAddr            string
	AddrOverride          bool
	AddrOverrideValue     string
	DefaultAgentKind      string
	DefaultAgentName      string
	DefaultAgentModel     string
	Runtimes              map[string]agentruntime.Runtime
	ProviderLimits        ProviderLimitOptions
	WebhookHTTPClient     *http.Client
	WebhookTimeout        time.Duration
	D2Command             string
	D2Timeout             time.Duration
	D2CacheTTL            time.Duration
	D2CacheMaxEntries     int
	ScheduledShellEnabled bool
}

type pendingQuestionKey struct {
	conversationType domain.ConversationType
	conversationID   string
	questionID       string
}

type pendingQuestion struct {
	session agentruntime.Session
}

type activeRunKey struct {
	conversationType domain.ConversationType
	conversationID   string
	agentID          string
}

type activeAgentRun struct {
	runID   string
	cancel  context.CancelCauseFunc
	session agentruntime.Session
}

type App struct {
	store          store.Store
	bus            *eventbus.Bus
	opts           Options
	providerLimits *providerLimitService
	d2Renderer     *d2Renderer
	scheduledTasks *scheduledTaskScheduler

	pendingQuestionsMu sync.Mutex
	pendingQuestions   map[pendingQuestionKey]*pendingQuestion
	activeRunsMu       sync.Mutex
	activeRuns         map[activeRunKey]map[string]*activeAgentRun
	scheduledRunsMu    sync.Mutex
	scheduledRuns      map[string]struct{}
}

func New(st store.Store, bus *eventbus.Bus, opts Options) *App {
	if opts.Runtimes == nil {
		opts.Runtimes = map[string]agentruntime.Runtime{
			domain.AgentKindFake: fake.New(),
		}
	}
	return &App{
		store:          st,
		bus:            bus,
		opts:           opts,
		providerLimits: newProviderLimitService(opts.ProviderLimits),
		d2Renderer: newD2Renderer(D2RenderOptions{
			Command:         opts.D2Command,
			Timeout:         opts.D2Timeout,
			CacheTTL:        opts.D2CacheTTL,
			CacheMaxEntries: opts.D2CacheMaxEntries,
		}),
		pendingQuestions: make(map[pendingQuestionKey]*pendingQuestion),
		activeRuns:       make(map[activeRunKey]map[string]*activeAgentRun),
		scheduledRuns:    make(map[string]struct{}),
	}
}

func (a *App) registerPendingQuestion(key pendingQuestionKey, pq *pendingQuestion) {
	a.pendingQuestionsMu.Lock()
	defer a.pendingQuestionsMu.Unlock()
	a.pendingQuestions[key] = pq
}

func (a *App) removePendingQuestions(conversationType domain.ConversationType, conversationID string) {
	a.pendingQuestionsMu.Lock()
	defer a.pendingQuestionsMu.Unlock()
	for key := range a.pendingQuestions {
		if key.conversationType == conversationType && key.conversationID == conversationID {
			delete(a.pendingQuestions, key)
		}
	}
}

func (a *App) RespondToInputRequest(_ context.Context, conversationType domain.ConversationType, conversationID string, questionID string, answer string) error {
	a.pendingQuestionsMu.Lock()
	key := pendingQuestionKey{
		conversationType: conversationType,
		conversationID:   conversationID,
		questionID:       questionID,
	}
	pq, ok := a.pendingQuestions[key]
	if ok {
		delete(a.pendingQuestions, key)
	}
	a.pendingQuestionsMu.Unlock()

	if !ok {
		return errors.New("no pending question found")
	}
	return pq.session.RespondToInputRequest(questionID, answer)
}

func (a *App) registerActiveAgentRun(key activeRunKey, run *activeAgentRun) {
	a.activeRunsMu.Lock()
	defer a.activeRunsMu.Unlock()
	runs := a.activeRuns[key]
	if runs == nil {
		runs = make(map[string]*activeAgentRun)
		a.activeRuns[key] = runs
	}
	runs[run.runID] = run
}

func (a *App) setActiveAgentRunSession(key activeRunKey, runID string, session agentruntime.Session) {
	a.activeRunsMu.Lock()
	defer a.activeRunsMu.Unlock()
	if run := a.activeRuns[key][runID]; run != nil {
		run.session = session
	}
}

func (a *App) removeActiveAgentRun(key activeRunKey, runID string) {
	a.activeRunsMu.Lock()
	defer a.activeRunsMu.Unlock()
	runs := a.activeRuns[key]
	if runs == nil {
		return
	}
	delete(runs, runID)
	if len(runs) == 0 {
		delete(a.activeRuns, key)
	}
}

func (a *App) stopActiveAgentRuns(ctx context.Context, key activeRunKey) int {
	a.activeRunsMu.Lock()
	runs := a.activeRuns[key]
	if len(runs) == 0 {
		a.activeRunsMu.Unlock()
		return 0
	}
	stopping := make([]*activeAgentRun, 0, len(runs))
	for runID, run := range runs {
		stopping = append(stopping, run)
		delete(runs, runID)
	}
	delete(a.activeRuns, key)
	a.activeRunsMu.Unlock()

	a.removePendingQuestions(key.conversationType, key.conversationID)
	for _, run := range stopping {
		run.cancel(errAgentRunStopped)
		if run.session == nil {
			continue
		}
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if stopper, ok := run.session.(agentruntime.Stopper); ok {
			_ = stopper.Stop(closeCtx)
		} else {
			_ = run.session.Close(closeCtx)
		}
		cancel()
	}
	return len(stopping)
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
