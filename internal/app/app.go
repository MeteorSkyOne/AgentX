package app

import (
	"context"
	"errors"
	"log/slog"
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
	ToolUpdateSettings    config.ToolUpdateSettings
	SelfUpdateSettings    config.SelfUpdateSettings
	ServerAddr            string
	AddrOverride          bool
	AddrOverrideValue     string
	DefaultAgentKind      string
	DefaultAgentName      string
	DefaultAgentModel     string
	Runtimes              map[string]agentruntime.Runtime
	ProviderLimits        ProviderLimitOptions
	ToolUpdates           ToolUpdateOptions
	SelfUpdates           SelfUpdateOptions
	WebhookHTTPClient     *http.Client
	WebhookTimeout        time.Duration
	D2Command             string
	D2Timeout             time.Duration
	D2CacheTTL            time.Duration
	D2CacheMaxEntries     int
	ScheduledShellEnabled bool
	Terminal              TerminalOptions
}

var errAppShuttingDown = errors.New("app is shutting down")

type pendingQuestionKey struct {
	conversationType domain.ConversationType
	conversationID   string
	questionID       string
}

type pendingQuestion struct {
	session agentruntime.Session
	run     *activeAgentRun
}

type activeRunKey struct {
	conversationType domain.ConversationType
	conversationID   string
	agentID          string
}

type messageQueueKey struct {
	conversationType domain.ConversationType
	conversationID   string
	agentID          string
}

type activeAgentRun struct {
	mu               sync.Mutex
	runID            string
	agentID          string
	provider         string
	organizationID   string
	conversationType domain.ConversationType
	conversationID   string
	startedAt        time.Time
	text             string
	thinking         string
	process          []domain.ProcessItem
	team             *domain.TeamMetadata
	pendingQuestion  *domain.AgentInputRequestPayload
	contextUsage     *domain.ContextUsage
	contextUsageAt   *time.Time
	cancelRequested  bool
	cancel           context.CancelCauseFunc
	session          agentruntime.Session
	sessionClosing   bool
}

type cancelAgentRunResult struct {
	requested   int
	unsupported int
}

type ActiveRunReplay struct {
	Events   []domain.Event
	Captured time.Time
}

type App struct {
	store          store.Store
	bus            *eventbus.Bus
	opts           Options
	providerLimits *providerLimitService
	toolUpdates    *toolUpdateService
	selfUpdates    *selfUpdateService
	d2Renderer     *d2Renderer
	scheduledTasks *scheduledTaskScheduler
	terminals      *terminalManager

	pendingQuestionsMu sync.Mutex
	pendingQuestions   map[pendingQuestionKey]*pendingQuestion
	runtimeResetMu     sync.Mutex
	activeRunsMu       sync.Mutex
	activeRuns         map[activeRunKey]map[string]*activeAgentRun
	messageQueueMu     sync.Mutex
	messageQueue       map[messageQueueKey][]*queuedAgentPrompt
	messageQueueByID   map[string]*queuedAgentPrompt
	scheduledRunsMu    sync.Mutex
	scheduledRuns      map[string]struct{}
	backgroundMu       sync.Mutex
	backgroundCtx      context.Context
	backgroundCancel   context.CancelFunc
	backgroundWG       sync.WaitGroup
	shuttingDown       bool
}

func New(st store.Store, bus *eventbus.Bus, opts Options) *App {
	if opts.Runtimes == nil {
		opts.Runtimes = map[string]agentruntime.Runtime{
			domain.AgentKindFake: fake.New(),
		}
	}
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	return &App{
		store:          st,
		bus:            bus,
		opts:           opts,
		providerLimits: newProviderLimitService(opts.ProviderLimits),
		toolUpdates: newToolUpdateService(st, opts.DataDir, opts.Runtimes, ToolUpdateOptions{
			Settings:      opts.ToolUpdateSettings,
			ClaudeCommand: opts.ToolUpdates.ClaudeCommand,
			CodexCommand:  opts.ToolUpdates.CodexCommand,
			Exec:          opts.ToolUpdates.Exec,
			Now:           opts.ToolUpdates.Now,
		}),
		selfUpdates: newSelfUpdateService(opts.DataDir, SelfUpdateOptions{
			Settings:   opts.SelfUpdateSettings,
			GitHubRepo: opts.SelfUpdates.GitHubRepo,
			HTTPClient: opts.SelfUpdates.HTTPClient,
			Now:        opts.SelfUpdates.Now,
			Executable: opts.SelfUpdates.Executable,
		}),
		d2Renderer: newD2Renderer(D2RenderOptions{
			Command:         opts.D2Command,
			Timeout:         opts.D2Timeout,
			CacheTTL:        opts.D2CacheTTL,
			CacheMaxEntries: opts.D2CacheMaxEntries,
		}),
		terminals:        newTerminalManager(opts.Terminal),
		pendingQuestions: make(map[pendingQuestionKey]*pendingQuestion),
		activeRuns:       make(map[activeRunKey]map[string]*activeAgentRun),
		messageQueue:     make(map[messageQueueKey][]*queuedAgentPrompt),
		messageQueueByID: make(map[string]*queuedAgentPrompt),
		scheduledRuns:    make(map[string]struct{}),
		backgroundCtx:    backgroundCtx,
		backgroundCancel: backgroundCancel,
	}
}

func (a *App) beginBackground() (context.Context, func(), bool) {
	a.backgroundMu.Lock()
	if a.shuttingDown {
		a.backgroundMu.Unlock()
		return nil, nil, false
	}
	ctx := a.backgroundCtx
	a.backgroundWG.Add(1)
	a.backgroundMu.Unlock()

	done := func() {
		a.backgroundWG.Done()
	}
	return ctx, done, true
}

func (a *App) startBackground(name string, fn func(context.Context)) bool {
	ctx, done, ok := a.beginBackground()
	if !ok {
		return false
	}
	go func() {
		defer done()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("background task panic", "task", name, "panic", recovered)
			}
		}()
		fn(ctx)
	}()
	return true
}

func (a *App) isShuttingDown() bool {
	a.backgroundMu.Lock()
	defer a.backgroundMu.Unlock()
	return a.shuttingDown
}

func (a *App) Shutdown(ctx context.Context) error {
	a.backgroundMu.Lock()
	if !a.shuttingDown {
		a.shuttingDown = true
		if a.backgroundCancel != nil {
			a.backgroundCancel()
		}
	}
	a.backgroundMu.Unlock()

	a.StopScheduledTasks()
	a.StopToolUpdates()
	a.StopSelfUpdate()
	a.StopTerminalManager()
	a.cancelAllActiveAgentRuns(ctx, errAppShuttingDown)
	a.clearAllQueuedAgentPrompts("failed")

	done := make(chan struct{})
	go func() {
		a.backgroundWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *App) registerPendingQuestion(key pendingQuestionKey, pq *pendingQuestion) {
	a.pendingQuestionsMu.Lock()
	defer a.pendingQuestionsMu.Unlock()
	a.pendingQuestions[key] = pq
}

func (a *App) removePendingQuestions(conversationType domain.ConversationType, conversationID string) {
	a.pendingQuestionsMu.Lock()
	for key := range a.pendingQuestions {
		if key.conversationType == conversationType && key.conversationID == conversationID {
			delete(a.pendingQuestions, key)
		}
	}
	a.pendingQuestionsMu.Unlock()

	a.clearActiveRunPendingQuestions(conversationType, conversationID)
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
	if pq.run != nil {
		pq.run.clearPendingQuestion(questionID)
	}
	return pq.session.RespondToInputRequest(questionID, answer)
}

func (a *App) registerActiveAgentRun(key activeRunKey, run *activeAgentRun) {
	a.runtimeResetMu.Lock()
	defer a.runtimeResetMu.Unlock()
	a.activeRunsMu.Lock()
	defer a.activeRunsMu.Unlock()
	runs := a.activeRuns[key]
	if runs == nil {
		runs = make(map[string]*activeAgentRun)
		a.activeRuns[key] = runs
	}
	runs[run.runID] = run
}

func (a *App) setActiveAgentRunSession(ctx context.Context, key activeRunKey, runID string, session agentruntime.Session) {
	var interrupt agentruntime.Session
	a.activeRunsMu.Lock()
	if run := a.activeRuns[key][runID]; run != nil {
		run.mu.Lock()
		run.session = session
		run.sessionClosing = false
		if run.cancelRequested {
			if _, ok := session.(agentruntime.Interrupter); ok {
				interrupt = session
			}
		}
		run.mu.Unlock()
	}
	a.activeRunsMu.Unlock()
	if interrupt != nil {
		interruptCtx := context.WithoutCancel(ctx)
		a.startBackground("agent-run-interrupt", func(context.Context) {
			interruptAgentRunSession(interruptCtx, interrupt)
		})
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
	stoppingSessions := make([]agentruntime.Session, 0, len(stopping))
	for _, run := range stopping {
		run.cancel(errAgentRunStopped)
		run.mu.Lock()
		session := run.session
		if session == nil || run.sessionClosing {
			run.mu.Unlock()
			continue
		}
		run.sessionClosing = true
		if initiator, ok := session.(agentruntime.StopInitiator); ok {
			initiator.InitiateStop()
		}
		stoppingSessions = append(stoppingSessions, session)
		run.mu.Unlock()
	}
	a.activeRunsMu.Unlock()

	a.removePendingQuestions(key.conversationType, key.conversationID)
	for _, session := range stoppingSessions {
		session := session
		stopCtx := context.WithoutCancel(ctx)
		a.startBackground("agent-run-stop", func(context.Context) {
			stopAgentRunSession(stopCtx, session)
		})
	}
	return len(stopping)
}

func (a *App) cancelActiveAgentRuns(ctx context.Context, key activeRunKey) cancelAgentRunResult {
	a.activeRunsMu.Lock()
	runs := a.activeRuns[key]
	if len(runs) == 0 {
		a.activeRunsMu.Unlock()
		return cancelAgentRunResult{}
	}
	canceling := make([]agentruntime.Session, 0, len(runs))
	var result cancelAgentRunResult
	for _, run := range runs {
		run.mu.Lock()
		session := run.session
		if run.cancelRequested {
			result.requested++
			run.mu.Unlock()
			continue
		}
		if session == nil {
			run.cancelRequested = true
			run.pendingQuestion = nil
			result.requested++
			run.mu.Unlock()
			continue
		}
		_, ok := session.(agentruntime.Interrupter)
		if !ok {
			result.unsupported++
			run.mu.Unlock()
			continue
		}
		run.cancelRequested = true
		run.pendingQuestion = nil
		run.mu.Unlock()
		canceling = append(canceling, session)
		result.requested++
	}
	a.activeRunsMu.Unlock()

	if len(canceling) == 0 {
		return result
	}
	a.removePendingQuestions(key.conversationType, key.conversationID)
	interruptCtx := context.WithoutCancel(ctx)
	for _, session := range canceling {
		session := session
		a.startBackground("agent-run-interrupt", func(context.Context) {
			interruptAgentRunSession(interruptCtx, session)
		})
	}
	return result
}

func (a *App) cancelAllActiveAgentRuns(ctx context.Context, cause error) {
	stopping := make([]agentruntime.Session, 0)
	a.activeRunsMu.Lock()
	for _, keyedRuns := range a.activeRuns {
		for _, run := range keyedRuns {
			run.mu.Lock()
			if run.cancel != nil {
				run.cancel(cause)
			}
			run.cancelRequested = true
			run.pendingQuestion = nil
			session := run.session
			if session != nil && !run.sessionClosing {
				run.sessionClosing = true
				stopping = append(stopping, session)
				if initiator, ok := session.(agentruntime.StopInitiator); ok {
					initiator.InitiateStop()
				}
			}
			run.mu.Unlock()
		}
	}
	a.activeRunsMu.Unlock()

	a.pendingQuestionsMu.Lock()
	for key := range a.pendingQuestions {
		delete(a.pendingQuestions, key)
	}
	a.pendingQuestionsMu.Unlock()

	stopAgentRunSessions(ctx, stopping)
}

func (r *activeAgentRun) closeSession(ctx context.Context, session agentruntime.Session) {
	if session == nil {
		return
	}
	r.mu.Lock()
	if r.session == session {
		if r.sessionClosing {
			r.mu.Unlock()
			return
		}
		r.sessionClosing = true
	}
	r.mu.Unlock()
	_ = session.Close(ctx)
}

func stopAgentRunSessions(ctx context.Context, sessions []agentruntime.Session) {
	if len(sessions) == 0 {
		return
	}
	var wg sync.WaitGroup
	wg.Add(len(sessions))
	for _, session := range sessions {
		session := session
		go func() {
			defer wg.Done()
			stopAgentRunSession(ctx, session)
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func stopAgentRunSession(ctx context.Context, session agentruntime.Session) {
	closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if stopper, ok := session.(agentruntime.Stopper); ok {
		_ = stopper.Stop(closeCtx)
		return
	}
	_ = session.Close(closeCtx)
}

func interruptAgentRunSession(ctx context.Context, session agentruntime.Session) {
	interruptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	interrupter, ok := session.(agentruntime.Interrupter)
	if !ok {
		return
	}
	_ = interrupter.Interrupt(interruptCtx)
}

func (a *App) ActiveRunReplayEvents(organizationID string, conversationType domain.ConversationType, conversationID string) map[string]ActiveRunReplay {
	a.activeRunsMu.Lock()
	var runs []*activeAgentRun
	for key, keyedRuns := range a.activeRuns {
		if key.conversationType != conversationType || key.conversationID != conversationID {
			continue
		}
		for _, run := range keyedRuns {
			if run.organizationID == organizationID {
				runs = append(runs, run)
			}
		}
	}
	a.activeRunsMu.Unlock()

	replays := make(map[string]ActiveRunReplay, len(runs))
	for _, run := range runs {
		events, captured := run.replayEvents()
		if len(events) == 0 {
			continue
		}
		replays[run.runID] = ActiveRunReplay{Events: events, Captured: captured}
	}
	return replays
}

func (a *App) clearActiveRunPendingQuestions(conversationType domain.ConversationType, conversationID string) {
	a.activeRunsMu.Lock()
	var runs []*activeAgentRun
	for key, keyedRuns := range a.activeRuns {
		if key.conversationType != conversationType || key.conversationID != conversationID {
			continue
		}
		for _, run := range keyedRuns {
			runs = append(runs, run)
		}
	}
	a.activeRunsMu.Unlock()
	for _, run := range runs {
		run.clearPendingQuestion("")
	}
}

func (r *activeAgentRun) appendDelta(text string, thinking string, process []domain.ProcessItem, clearText bool, team *domain.TeamMetadata) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if clearText {
		r.text = ""
	}
	r.text += text
	r.thinking += thinking
	if len(process) > 0 {
		r.process = append(r.process, cloneProcessItems(process)...)
	}
	if team != nil {
		r.team = cloneTeamMetadata(team)
	}
}

func (r *activeAgentRun) setPendingQuestion(payload domain.AgentInputRequestPayload) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := payload
	copied.Options = append([]domain.AgentInputRequestOption(nil), payload.Options...)
	copied.Team = cloneTeamMetadata(payload.Team)
	r.pendingQuestion = &copied
}

func (r *activeAgentRun) clearPendingQuestion(questionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if questionID == "" || r.pendingQuestion == nil || r.pendingQuestion.QuestionID == questionID {
		r.pendingQuestion = nil
	}
}

func (r *activeAgentRun) setContextUsage(usage *agentruntime.ContextUsage) {
	if usage == nil {
		return
	}
	copied := contextUsageToDomain(usage)
	now := time.Now().UTC()
	r.mu.Lock()
	r.contextUsage = copied
	r.contextUsageAt = &now
	r.mu.Unlock()
}

func (r *activeAgentRun) latestContextUsage() (*domain.ContextUsage, *time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneDomainContextUsage(r.contextUsage), cloneTimePtr(r.contextUsageAt)
}

func (r *activeAgentRun) replayEvents() ([]domain.Event, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	captured := time.Now().UTC()
	team := cloneTeamMetadata(r.team)
	events := []domain.Event{
		{
			Type:             domain.EventAgentRunStarted,
			OrganizationID:   r.organizationID,
			ConversationType: r.conversationType,
			ConversationID:   r.conversationID,
			Payload:          domain.AgentRunPayload{RunID: r.runID, AgentID: r.agentID, Team: team},
			CreatedAt:        r.startedAt,
		},
	}
	if r.text != "" || r.thinking != "" || len(r.process) > 0 {
		events = append(events, domain.Event{
			Type:             domain.EventAgentOutputDelta,
			OrganizationID:   r.organizationID,
			ConversationType: r.conversationType,
			ConversationID:   r.conversationID,
			Payload: domain.AgentOutputDeltaPayload{
				RunID:    r.runID,
				AgentID:  r.agentID,
				Text:     r.text,
				Thinking: r.thinking,
				Process:  cloneProcessItems(r.process),
				Team:     team,
			},
			CreatedAt: captured,
		})
	}
	if r.pendingQuestion != nil {
		payload := *r.pendingQuestion
		payload.Options = append([]domain.AgentInputRequestOption(nil), r.pendingQuestion.Options...)
		payload.Team = cloneTeamMetadata(r.pendingQuestion.Team)
		events = append(events, domain.Event{
			Type:             domain.EventAgentInputRequest,
			OrganizationID:   r.organizationID,
			ConversationType: r.conversationType,
			ConversationID:   r.conversationID,
			Payload:          payload,
			CreatedAt:        captured,
		})
	}
	return events, captured
}

func cloneTeamMetadata(team *domain.TeamMetadata) *domain.TeamMetadata {
	if team == nil {
		return nil
	}
	copied := *team
	return &copied
}

func cloneProcessItems(process []domain.ProcessItem) []domain.ProcessItem {
	if len(process) == 0 {
		return nil
	}
	copied := make([]domain.ProcessItem, len(process))
	copy(copied, process)
	return copied
}

func contextUsageToDomain(usage *agentruntime.ContextUsage) *domain.ContextUsage {
	if usage == nil {
		return nil
	}
	return &domain.ContextUsage{
		TotalTokens:           cloneInt64Ptr(usage.TotalTokens),
		InputTokens:           cloneInt64Ptr(usage.InputTokens),
		CachedInputTokens:     cloneInt64Ptr(usage.CachedInputTokens),
		OutputTokens:          cloneInt64Ptr(usage.OutputTokens),
		ReasoningOutputTokens: cloneInt64Ptr(usage.ReasoningOutputTokens),
		ContextWindowTokens:   cloneInt64Ptr(usage.ContextWindowTokens),
		UsedPercent:           cloneFloat64Ptr(usage.UsedPercent),
		Model:                 usage.Model,
		Source:                usage.Source,
	}
}

func cloneDomainContextUsage(usage *domain.ContextUsage) *domain.ContextUsage {
	if usage == nil {
		return nil
	}
	return &domain.ContextUsage{
		TotalTokens:           cloneInt64Ptr(usage.TotalTokens),
		InputTokens:           cloneInt64Ptr(usage.InputTokens),
		CachedInputTokens:     cloneInt64Ptr(usage.CachedInputTokens),
		OutputTokens:          cloneInt64Ptr(usage.OutputTokens),
		ReasoningOutputTokens: cloneInt64Ptr(usage.ReasoningOutputTokens),
		ContextWindowTokens:   cloneInt64Ptr(usage.ContextWindowTokens),
		UsedPercent:           cloneFloat64Ptr(usage.UsedPercent),
		Model:                 usage.Model,
		Source:                usage.Source,
	}
}

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
