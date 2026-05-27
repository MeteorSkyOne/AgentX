package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/config"
	"github.com/meteorsky/agentx/internal/domain"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/store"
	"github.com/robfig/cron/v3"
)

const (
	toolUpdateProviderClaude = "claude"
	toolUpdateProviderCodex  = "codex"
	toolUpdateAll            = "all"
	toolUpdateOutputLimit    = 16 * 1024
	defaultToolUpdateTimeout = 5 * time.Minute
)

type ToolUpdateOptions struct {
	Settings      config.ToolUpdateSettings
	ClaudeCommand string
	CodexCommand  string
	Exec          toolUpdateExecFunc
	Now           func() time.Time
}

type toolUpdateExecFunc func(ctx context.Context, name string, args ...string) (string, error)

type ToolUpdateSettings = config.ToolUpdateSettings

type ToolUpdateOverview struct {
	Settings config.ToolUpdateSettings `json:"settings"`
	Tools    []ToolUpdateStatus        `json:"tools"`
}

type ToolUpdateStatus struct {
	Tool                string     `json:"tool"`
	DisplayName         string     `json:"display_name"`
	Command             string     `json:"command"`
	CurrentVersion      string     `json:"current_version,omitempty"`
	LatestVersion       string     `json:"latest_version,omitempty"`
	UpdateAvailable     *bool      `json:"update_available,omitempty"`
	State               string     `json:"state"`
	Message             string     `json:"message,omitempty"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
	LastUpdatedAt       *time.Time `json:"last_updated_at,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	ActiveRunCount      int        `json:"active_run_count"`
	RuntimeResetPending bool       `json:"runtime_reset_pending"`
}

type ToolUpdateRequest struct {
	Tool string `json:"tool"`
}

type toolUpdateService struct {
	dataDir  string
	runtimes map[string]agentruntime.Runtime
	settings config.ToolUpdateSettings

	claudeCommand string
	codexCommand  string
	exec          toolUpdateExecFunc
	now           func() time.Time

	mu     sync.Mutex
	states map[string]*toolUpdateState
	cron   *cron.Cron
}

type toolUpdateState struct {
	CurrentVersion      string
	LatestVersion       string
	UpdateAvailable     *bool
	State               string
	Message             string
	LastCheckedAt       *time.Time
	LastUpdatedAt       *time.Time
	LastError           string
	RuntimeResetPending bool
}

func newToolUpdateService(_ store.Store, dataDir string, runtimes map[string]agentruntime.Runtime, opts ToolUpdateOptions) *toolUpdateService {
	claudeCommand := strings.TrimSpace(opts.ClaudeCommand)
	if claudeCommand == "" {
		claudeCommand = "claude"
	}
	codexCommand := strings.TrimSpace(opts.CodexCommand)
	if codexCommand == "" {
		codexCommand = "codex"
	}
	execFn := opts.Exec
	if execFn == nil {
		execFn = defaultToolUpdateExec
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	settings := opts.Settings
	if normalized, err := config.NormalizeToolUpdateSettings(settings, "tool update settings"); err == nil {
		settings = normalized
	} else {
		settings = config.DefaultToolUpdateSettings()
	}
	return &toolUpdateService{
		dataDir:       dataDir,
		runtimes:      runtimes,
		settings:      settings,
		claudeCommand: claudeCommand,
		codexCommand:  codexCommand,
		exec:          execFn,
		now:           now,
		states: map[string]*toolUpdateState{
			toolUpdateProviderClaude: {State: "idle"},
			toolUpdateProviderCodex:  {State: "idle"},
		},
	}
}

func defaultToolUpdateExec(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultToolUpdateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = providerProbeEnv(nil)
	configureProviderProbeCommand(cmd)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	if len(out) > toolUpdateOutputLimit {
		out = out[:toolUpdateOutputLimit]
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("%s %s timed out", name, strings.Join(args, " "))
	}
	if err != nil {
		if out != "" {
			return out, fmt.Errorf("%s %s failed: %s: %w", name, strings.Join(args, " "), out, err)
		}
		return out, fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

func (a *App) StartToolUpdates(ctx context.Context) error {
	if a.toolUpdates == nil {
		return nil
	}
	return a.toolUpdates.start(ctx, func() {
		a.startBackground("tool-auto-update", func(ctx context.Context) {
			if _, err := a.RunToolUpdate(ctx, toolUpdateAll); err != nil {
				slog.Warn("tool auto update failed", "error", err)
			}
		})
	})
}

func (a *App) StopToolUpdates() {
	if a.toolUpdates != nil {
		a.toolUpdates.stop()
	}
}

func (s *toolUpdateService) start(_ context.Context, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		return nil
	}
	if !s.settings.AutoEnabled {
		return nil
	}
	scheduler := cron.New(cron.WithParser(cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow)), cron.WithLocation(time.UTC), cron.WithChain(cron.Recover(cronSlogLogger{})))
	spec := toolUpdateCronSpec(s.settings)
	if _, err := scheduler.AddFunc(spec, fn); err != nil {
		return err
	}
	scheduler.Start()
	s.cron = scheduler
	return nil
}

func (s *toolUpdateService) stop() {
	s.mu.Lock()
	scheduler := s.cron
	s.cron = nil
	s.mu.Unlock()
	if scheduler != nil {
		ctx := scheduler.Stop()
		<-ctx.Done()
	}
}

func (s *toolUpdateService) restart(ctx context.Context, fn func()) error {
	s.stop()
	return s.start(ctx, fn)
}

func toolUpdateCronSpec(settings config.ToolUpdateSettings) string {
	parts := strings.Split(settings.TimeOfDay, ":")
	timezone := strings.TrimSpace(settings.Timezone)
	if timezone == "" || timezone == "Local" {
		timezone = "UTC"
	}
	return fmt.Sprintf("CRON_TZ=%s %s %s * * *", timezone, parts[1], parts[0])
}

func (a *App) ToolUpdateOverview(_ context.Context, _ string) (ToolUpdateOverview, error) {
	if a.toolUpdates == nil {
		return ToolUpdateOverview{}, nil
	}
	counts := a.activeProviderRunCounts()
	return a.toolUpdates.overview(func(provider string) int {
		return counts[provider]
	}), nil
}

func (a *App) UpdateToolUpdateSettings(ctx context.Context, _ string, settings config.ToolUpdateSettings) (ToolUpdateOverview, error) {
	saved, err := config.SaveToolUpdateSettings(a.opts.DataDir, settings)
	if err != nil {
		return ToolUpdateOverview{}, invalidInput(err.Error())
	}
	a.toolUpdates.mu.Lock()
	a.toolUpdates.settings = saved
	a.toolUpdates.mu.Unlock()
	if err := a.toolUpdates.restart(ctx, func() {
		a.startBackground("tool-auto-update", func(ctx context.Context) {
			if _, err := a.RunToolUpdate(ctx, toolUpdateAll); err != nil {
				slog.Warn("tool auto update failed", "error", err)
			}
		})
	}); err != nil {
		return ToolUpdateOverview{}, invalidInput(err.Error())
	}
	return a.ToolUpdateOverview(ctx, "")
}

func (s *toolUpdateService) overview(active func(string) int) ToolUpdateOverview {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ToolUpdateOverview{
		Settings: s.settings,
		Tools: []ToolUpdateStatus{
			s.statusLocked(toolUpdateProviderClaude, active(toolUpdateProviderClaude)),
			s.statusLocked(toolUpdateProviderCodex, active(toolUpdateProviderCodex)),
		},
	}
}

func (s *toolUpdateService) statusLocked(provider string, active int) ToolUpdateStatus {
	state := s.stateLocked(provider)
	return ToolUpdateStatus{
		Tool:                provider,
		DisplayName:         toolUpdateDisplayName(provider),
		Command:             s.commandForProvider(provider),
		CurrentVersion:      state.CurrentVersion,
		LatestVersion:       state.LatestVersion,
		UpdateAvailable:     cloneBoolPtr(state.UpdateAvailable),
		State:               state.State,
		Message:             state.Message,
		LastCheckedAt:       cloneTimePtr(state.LastCheckedAt),
		LastUpdatedAt:       cloneTimePtr(state.LastUpdatedAt),
		LastError:           state.LastError,
		ActiveRunCount:      active,
		RuntimeResetPending: state.RuntimeResetPending,
	}
}

func (a *App) CheckToolUpdates(ctx context.Context, tool string) (ToolUpdateOverview, error) {
	providers := requestedToolProviders(tool)
	if len(providers) == 0 {
		return ToolUpdateOverview{}, invalidInput("tool must be claude, codex, or all")
	}
	if err := runToolUpdateProviders(providers, func(provider string) error {
		if err := a.toolUpdates.check(ctx, provider); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return ToolUpdateOverview{}, err
	}
	return a.ToolUpdateOverview(ctx, "")
}

func (a *App) RunToolUpdate(ctx context.Context, tool string) (ToolUpdateOverview, error) {
	providers := requestedToolProviders(tool)
	if len(providers) == 0 {
		return ToolUpdateOverview{}, invalidInput("tool must be claude, codex, or all")
	}
	enabled := make([]string, 0, len(providers))
	for _, provider := range providers {
		if !a.toolUpdates.providerEnabled(provider) {
			continue
		}
		enabled = append(enabled, provider)
	}
	if err := runToolUpdateProviders(enabled, func(provider string) error {
		if err := a.toolUpdates.check(ctx, provider); err != nil {
			return err
		}
		if err := a.toolUpdates.update(ctx, provider); err != nil {
			return err
		}
		a.markToolUpdateRuntimeReset(ctx, provider)
		return nil
	}); err != nil {
		return ToolUpdateOverview{}, err
	}
	return a.ToolUpdateOverview(ctx, "")
}

func (a *App) StartRunToolUpdate(ctx context.Context, tool string) (ToolUpdateOverview, error) {
	providers := requestedToolProviders(tool)
	if len(providers) == 0 {
		return ToolUpdateOverview{}, invalidInput("tool must be claude, codex, or all")
	}
	started := make([]string, 0, len(providers))
	for _, provider := range providers {
		if !a.toolUpdates.providerEnabled(provider) {
			continue
		}
		if a.toolUpdates.beginUpdate(provider) {
			started = append(started, provider)
		}
	}
	if len(started) > 0 {
		a.startBackground("tool-manual-update", func(ctx context.Context) {
			var wg sync.WaitGroup
			wg.Add(len(started))
			for _, provider := range started {
				provider := provider
				go func() {
					defer wg.Done()
					if err := a.runStartedToolUpdate(ctx, provider); err != nil {
						slog.Warn("tool manual update failed", "provider", provider, "error", err)
					}
				}()
			}
			wg.Wait()
		})
	}
	return a.ToolUpdateOverview(ctx, "")
}

func runToolUpdateProviders(providers []string, fn func(string) error) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(providers))
	wg.Add(len(providers))
	for _, provider := range providers {
		provider := provider
		go func() {
			defer wg.Done()
			if err := fn(provider); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	var joined []error
	for err := range errs {
		joined = append(joined, err)
	}
	return errors.Join(joined...)
}

func (a *App) runStartedToolUpdate(ctx context.Context, provider string) error {
	if err := a.toolUpdates.checkStarted(ctx, provider, "updating"); err != nil {
		return err
	}
	if err := a.toolUpdates.updateStarted(ctx, provider); err != nil {
		return err
	}
	a.markToolUpdateRuntimeReset(ctx, provider)
	return nil
}

func (s *toolUpdateService) check(ctx context.Context, provider string) error {
	s.setState(provider, "checking", "", "")
	return s.checkStarted(ctx, provider, "idle")
}

func (s *toolUpdateService) checkStarted(ctx context.Context, provider string, completedState string) error {
	currentOut, err := s.exec(ctx, s.commandForProvider(provider), "--version")
	if err != nil {
		s.setState(provider, "error", "", err.Error())
		return err
	}
	latestOut, latestErr := s.exec(ctx, "npm", "view", toolUpdatePackage(provider), "version")
	current := parseToolVersion(provider, currentOut)
	latest := parseToolVersion(provider, latestOut)
	var available *bool
	if latestErr == nil && current != "" && latest != "" {
		v := current != latest
		available = &v
	}
	now := s.now().UTC()
	s.mu.Lock()
	state := s.stateLocked(provider)
	state.CurrentVersion = current
	state.LatestVersion = latest
	state.UpdateAvailable = available
	state.State = completedState
	state.Message = ""
	state.LastCheckedAt = &now
	state.LastError = ""
	if latestErr != nil {
		state.Message = "Latest version unavailable: " + latestErr.Error()
	}
	s.mu.Unlock()
	return nil
}

func (s *toolUpdateService) update(ctx context.Context, provider string) error {
	s.setState(provider, "updating", "", "")
	return s.updateStarted(ctx, provider)
}

func (s *toolUpdateService) updateStarted(ctx context.Context, provider string) error {
	out, err := s.exec(ctx, s.commandForProvider(provider), "update")
	now := s.now().UTC()
	s.mu.Lock()
	state := s.stateLocked(provider)
	if err != nil {
		state.State = "error"
		state.LastError = err.Error()
		state.Message = err.Error()
		s.mu.Unlock()
		return err
	}
	state.State = "idle"
	state.LastUpdatedAt = &now
	state.LastError = ""
	state.Message = out
	if state.LatestVersion != "" {
		state.CurrentVersion = state.LatestVersion
		v := false
		state.UpdateAvailable = &v
	}
	s.mu.Unlock()
	return nil
}

func (s *toolUpdateService) beginUpdate(provider string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.stateLocked(provider)
	if state.State == "checking" || state.State == "updating" {
		return false
	}
	state.State = "updating"
	state.Message = ""
	state.LastError = ""
	return true
}

func (s *toolUpdateService) setState(provider string, state string, message string, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stateLocked(provider)
	st.State = state
	st.Message = message
	st.LastError = lastErr
}

func (s *toolUpdateService) stateLocked(provider string) *toolUpdateState {
	state := s.states[provider]
	if state == nil {
		state = &toolUpdateState{State: "idle"}
		s.states[provider] = state
	}
	return state
}

func (s *toolUpdateService) providerEnabled(provider string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch provider {
	case toolUpdateProviderClaude:
		return s.settings.ClaudeEnabled
	case toolUpdateProviderCodex:
		return s.settings.CodexEnabled
	default:
		return false
	}
}

func (s *toolUpdateService) commandForProvider(provider string) string {
	switch provider {
	case toolUpdateProviderClaude:
		return s.claudeCommand
	case toolUpdateProviderCodex:
		return s.codexCommand
	default:
		return ""
	}
}

func (a *App) markToolUpdateRuntimeReset(ctx context.Context, provider string) {
	if a.toolUpdates == nil || provider == "" {
		return
	}
	a.toolUpdates.mu.Lock()
	a.toolUpdates.stateLocked(provider).RuntimeResetPending = true
	a.toolUpdates.mu.Unlock()
	a.resetUpdatedRuntimeIfIdle(ctx, provider)
}

func (a *App) resetUpdatedRuntimeIfIdle(ctx context.Context, provider string) {
	a.runtimeResetMu.Lock()
	defer a.runtimeResetMu.Unlock()
	if a.activeProviderRunCount(provider) > 0 {
		return
	}
	a.toolUpdates.mu.Lock()
	state := a.toolUpdates.stateLocked(provider)
	if !state.RuntimeResetPending {
		a.toolUpdates.mu.Unlock()
		return
	}
	state.RuntimeResetPending = false
	a.toolUpdates.mu.Unlock()
	if err := a.resetProviderRuntime(ctx, provider); err != nil {
		a.toolUpdates.mu.Lock()
		state := a.toolUpdates.stateLocked(provider)
		state.RuntimeResetPending = true
		state.LastError = err.Error()
		state.Message = err.Error()
		a.toolUpdates.mu.Unlock()
	}
}

func (a *App) handleToolUpdateAgentRunTerminated(ctx context.Context, provider string) {
	if provider == "" || a.toolUpdates == nil {
		return
	}
	a.resetUpdatedRuntimeIfIdle(ctx, provider)
}

func (a *App) resetProviderRuntime(ctx context.Context, provider string) error {
	kinds := []string{}
	switch provider {
	case toolUpdateProviderClaude:
		kinds = []string{domain.AgentKindClaudePersistent}
	case toolUpdateProviderCodex:
		kinds = []string{domain.AgentKindCodexPersistent}
	default:
		return nil
	}
	for _, kind := range kinds {
		if rt, ok := a.opts.Runtimes[kind]; ok {
			if resetter, ok := rt.(agentruntime.ProcessResetter); ok {
				resetCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
				err := resetter.ResetProcesses(resetCtx)
				cancel()
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (a *App) activeProviderRunCount(provider string) int {
	a.activeRunsMu.Lock()
	defer a.activeRunsMu.Unlock()
	return a.activeProviderRunCountLocked(provider)
}

func (a *App) activeProviderRunCounts() map[string]int {
	a.activeRunsMu.Lock()
	defer a.activeRunsMu.Unlock()
	counts := map[string]int{}
	for _, runs := range a.activeRuns {
		for _, run := range runs {
			if run.provider != "" {
				counts[run.provider]++
			}
		}
	}
	return counts
}

func (a *App) activeProviderRunCountLocked(provider string) int {
	var count int
	for _, runs := range a.activeRuns {
		for _, run := range runs {
			if run.provider == provider {
				count++
			}
		}
	}
	return count
}

func agentProvider(kind string) string {
	switch kind {
	case domain.AgentKindClaude, domain.AgentKindClaudePersistent:
		return toolUpdateProviderClaude
	case domain.AgentKindCodex, domain.AgentKindCodexPersistent:
		return toolUpdateProviderCodex
	default:
		return ""
	}
}

func requestedToolProviders(tool string) []string {
	switch strings.TrimSpace(tool) {
	case "", toolUpdateAll:
		return []string{toolUpdateProviderClaude, toolUpdateProviderCodex}
	case toolUpdateProviderClaude:
		return []string{toolUpdateProviderClaude}
	case toolUpdateProviderCodex:
		return []string{toolUpdateProviderCodex}
	default:
		return nil
	}
}

func toolUpdatePackage(provider string) string {
	if provider == toolUpdateProviderClaude {
		return "@anthropic-ai/claude-code"
	}
	return "@openai/codex"
}

func toolUpdateDisplayName(provider string) string {
	if provider == toolUpdateProviderClaude {
		return "Claude Code"
	}
	return "Codex"
}

var toolVersionPattern = regexp.MustCompile(`\d+(?:\.\d+)+(?:[-+][A-Za-z0-9.-]+)?`)

func parseToolVersion(_ string, output string) string {
	return toolVersionPattern.FindString(output)
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
