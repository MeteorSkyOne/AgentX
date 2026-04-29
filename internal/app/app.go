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
	AdminToken        string
	DataDir           string
	ServerSettings    config.ServerSettings
	ServerAddr        string
	AddrOverride      bool
	AddrOverrideValue string
	DefaultAgentKind  string
	DefaultAgentName  string
	DefaultAgentModel string
	Runtimes          map[string]agentruntime.Runtime
	ProviderLimits    ProviderLimitOptions
	WebhookHTTPClient *http.Client
	WebhookTimeout    time.Duration
}

type pendingQuestionKey struct {
	conversationType domain.ConversationType
	conversationID   string
	questionID       string
}

type pendingQuestion struct {
	session agentruntime.Session
}

type App struct {
	store          store.Store
	bus            *eventbus.Bus
	opts           Options
	providerLimits *providerLimitService

	pendingQuestionsMu sync.Mutex
	pendingQuestions    map[pendingQuestionKey]*pendingQuestion
}

func New(st store.Store, bus *eventbus.Bus, opts Options) *App {
	if opts.Runtimes == nil {
		opts.Runtimes = map[string]agentruntime.Runtime{
			domain.AgentKindFake: fake.New(),
		}
	}
	return &App{
		store:           st,
		bus:             bus,
		opts:            opts,
		providerLimits:  newProviderLimitService(opts.ProviderLimits),
		pendingQuestions: make(map[pendingQuestionKey]*pendingQuestion),
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
