package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"golang.org/x/sync/singleflight"
)

const (
	defaultProviderLimitCacheTTL     = 45 * time.Second
	defaultProviderLimitProbeTimeout = 10 * time.Second
	providerProbeWaitDelay           = 500 * time.Millisecond
	providerProbeScannerBufferMax    = 1024 * 1024
	codexFiveHourWindowMinutes       = 300
	codexWeeklyWindowMinutes         = 10080
	claudeFiveHourWindowMinutes      = 300
	claudeWeeklyWindowMinutes        = 10080
	claudeCredentialsService         = "Claude Code-credentials"
	defaultClaudeUsageAPIURL         = "https://api.anthropic.com/api/oauth/usage"
	claudeUsageAPIBeta               = "oauth-2025-04-20"
)

type ProviderLimitStatus string

const (
	ProviderLimitStatusOK          ProviderLimitStatus = "ok"
	ProviderLimitStatusUnavailable ProviderLimitStatus = "unavailable"
	ProviderLimitStatusError       ProviderLimitStatus = "error"
)

type ProviderLimitOptions struct {
	CodexCommand   string
	ClaudeCommand  string
	ClaudeUsageURL string
	HTTPClient     *http.Client
	CacheTTL       time.Duration
	ProbeTimeout   time.Duration
	Now            func() time.Time
}

type AgentProviderLimits struct {
	AgentID         string                `json:"agent_id"`
	Provider        string                `json:"provider"`
	Status          ProviderLimitStatus   `json:"status"`
	Auth            ProviderLimitAuth     `json:"auth"`
	Windows         []ProviderLimitWindow `json:"windows"`
	FetchedAt       time.Time             `json:"fetched_at"`
	CacheTTLSeconds int                   `json:"cache_ttl_seconds"`
	Message         string                `json:"message,omitempty"`
}

type ProviderLimitAuth struct {
	LoggedIn bool   `json:"logged_in"`
	Method   string `json:"method,omitempty"`
	Provider string `json:"provider,omitempty"`
	Plan     string `json:"plan,omitempty"`
}

type ProviderLimitWindow struct {
	Kind          string     `json:"kind"`
	Label         string     `json:"label"`
	UsedPercent   *float64   `json:"used_percent"`
	WindowMinutes int        `json:"window_minutes"`
	ResetsAt      *time.Time `json:"resets_at"`
}

type providerLimitService struct {
	codexCommand   string
	claudeCommand  string
	claudeUsageURL string
	httpClient     *http.Client
	cacheTTL       time.Duration
	probeTimeout   time.Duration
	now            func() time.Time

	mu    sync.Mutex
	cache map[string]providerLimitCacheEntry
	group singleflight.Group
}

type providerLimitCacheEntry struct {
	result    AgentProviderLimits
	expiresAt time.Time
	version   string
}

var (
	errProviderLimitTimeout     = errors.New("provider limit probe timed out")
	errClaudeUsageNoCredentials = errors.New("claude usage credentials not found")
	errClaudeUsageRateLimited   = errors.New("claude usage api rate limited")
	errClaudeUsageUnauthorized  = errors.New("claude usage api rejected oauth token")
	emailRedactionPattern       = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	authFieldRedactPattern      = regexp.MustCompile(`(?i)(["']?(?:email|organization|organization_name|organizationName|org_name|orgName)["']?\s*[:=]\s*)(["'][^"']*["']|[^\s,}]+)`)
)

func newProviderLimitService(opts ProviderLimitOptions) *providerLimitService {
	codexCommand := strings.TrimSpace(opts.CodexCommand)
	if codexCommand == "" {
		codexCommand = "codex"
	}
	claudeCommand := strings.TrimSpace(opts.ClaudeCommand)
	if claudeCommand == "" {
		claudeCommand = "claude"
	}
	claudeUsageURL := strings.TrimSpace(opts.ClaudeUsageURL)
	if claudeUsageURL == "" {
		claudeUsageURL = defaultClaudeUsageAPIURL
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultProviderLimitCacheTTL
	}
	probeTimeout := opts.ProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = defaultProviderLimitProbeTimeout
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &providerLimitService{
		codexCommand:   codexCommand,
		claudeCommand:  claudeCommand,
		claudeUsageURL: claudeUsageURL,
		httpClient:     opts.HTTPClient,
		cacheTTL:       cacheTTL,
		probeTimeout:   probeTimeout,
		now:            now,
		cache:          make(map[string]providerLimitCacheEntry),
	}
}

func (a *App) AgentProviderLimits(ctx context.Context, agent domain.Agent, force bool) AgentProviderLimits {
	return a.providerLimits.Read(ctx, agent, force)
}

func (s *providerLimitService) Read(ctx context.Context, agent domain.Agent, force bool) AgentProviderLimits {
	now := s.now().UTC()
	key := providerLimitCacheKey(agent)
	version := providerLimitCacheVersion(agent)

	s.mu.Lock()
	if !force {
		if entry, ok := s.cache[key]; ok && entry.version == version && now.Before(entry.expiresAt) {
			s.mu.Unlock()
			return entry.result
		}
	}
	s.mu.Unlock()

	value, _, _ := s.group.Do(key, func() (any, error) {
		fetchNow := s.now().UTC()
		s.mu.Lock()
		if !force {
			if entry, ok := s.cache[key]; ok && entry.version == version && fetchNow.Before(entry.expiresAt) {
				s.mu.Unlock()
				return entry.result, nil
			}
		}
		s.mu.Unlock()

		result := s.fetch(ctx, agent, fetchNow)
		result.CacheTTLSeconds = int(s.cacheTTL.Seconds())
		cacheNow := s.now().UTC()

		s.mu.Lock()
		s.pruneExpiredLocked(cacheNow)
		s.cache[key] = providerLimitCacheEntry{
			result:    result,
			expiresAt: cacheNow.Add(s.cacheTTL),
			version:   version,
		}
		s.mu.Unlock()

		return result, nil
	})
	return value.(AgentProviderLimits)
}

func providerLimitCacheKey(agent domain.Agent) string {
	return agent.ID
}

func providerLimitCacheVersion(agent domain.Agent) string {
	return agent.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

func (s *providerLimitService) pruneExpiredLocked(now time.Time) {
	for key, entry := range s.cache {
		if !now.Before(entry.expiresAt) {
			delete(s.cache, key)
		}
	}
}

func (s *providerLimitService) fetch(ctx context.Context, agent domain.Agent, fetchedAt time.Time) AgentProviderLimits {
	provider := strings.TrimSpace(agent.Kind)
	if provider == "" {
		provider = domain.AgentKindFake
	}
	switch provider {
	case domain.AgentKindCodex:
		return s.fetchCodex(ctx, agent, fetchedAt)
	case domain.AgentKindClaude:
		return s.fetchClaude(ctx, agent, fetchedAt)
	default:
		result := s.baseResult(agent, provider, fetchedAt)
		result.Status = ProviderLimitStatusUnavailable
		result.Message = "Usage limits are available only for Claude Code and Codex agents."
		return result
	}
}

func (s *providerLimitService) baseResult(agent domain.Agent, provider string, fetchedAt time.Time) AgentProviderLimits {
	return AgentProviderLimits{
		AgentID:   agent.ID,
		Provider:  provider,
		Status:    ProviderLimitStatusUnavailable,
		Windows:   []ProviderLimitWindow{},
		FetchedAt: fetchedAt.UTC(),
	}
}

func (s *providerLimitService) fetchCodex(ctx context.Context, agent domain.Agent, fetchedAt time.Time) AgentProviderLimits {
	result := s.baseResult(agent, domain.AgentKindCodex, fetchedAt)

	account, rateLimits, err := s.readCodexAppServer(ctx, agent.Env)
	if err != nil {
		result.Status = ProviderLimitStatusError
		result.Message = providerLimitErrorMessage("Codex rate limit probe", err)
		return result
	}

	result.Auth = codexAuthFromAccountResult(account)
	windows, reachedType := codexWindowsFromRateLimitResult(rateLimits)
	result.Windows = windows
	if len(windows) == 0 {
		result.Status = ProviderLimitStatusUnavailable
		result.Message = "Codex did not return numeric rate limit windows."
		return result
	}

	result.Status = ProviderLimitStatusOK
	if reachedType != "" {
		result.Message = "Codex reports a rate limit is currently reached: " + reachedType
	}
	return result
}

func (s *providerLimitService) readCodexAppServer(ctx context.Context, env map[string]string) (map[string]any, map[string]any, error) {
	probeCtx, cancel := context.WithTimeout(ctx, s.probeTimeout)
	defer cancel()

	cmd := providerProbeCommand(probeCtx, s.codexCommand, "app-server", "--listen", "stdio://")
	cmd.Env = providerProbeEnv(env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	cleanup := func(readErr error) error {
		_ = stdin.Close()
		cancel()
		waitErr := <-waitCh
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return errProviderLimitTimeout
		}
		if readErr != nil {
			return providerProbeError("codex app-server", readErr, stderr.String())
		}
		if waitErr != nil && !errors.Is(probeCtx.Err(), context.Canceled) {
			return providerProbeError("codex app-server", waitErr, stderr.String())
		}
		return nil
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), providerProbeScannerBufferMax)
	rpc := newJSONRPCReader(scanner)

	if err := writeJSONRPC(stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      1,
		"params": map[string]any{
			"clientInfo": map[string]any{
				"name":    "agentx",
				"title":   "AgentX",
				"version": "0.1.0",
			},
		},
	}); err != nil {
		return nil, nil, cleanup(err)
	}
	if _, err := rpc.readResult(probeCtx, 1, "initialize"); err != nil {
		return nil, nil, cleanup(err)
	}

	if err := writeJSONRPC(stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]any{},
	}); err != nil {
		return nil, nil, cleanup(err)
	}
	if err := writeJSONRPC(stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "account/read",
		"id":      2,
		"params":  map[string]any{},
	}); err != nil {
		return nil, nil, cleanup(err)
	}
	if err := writeJSONRPC(stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "account/rateLimits/read",
		"id":      3,
		"params":  map[string]any{},
	}); err != nil {
		return nil, nil, cleanup(err)
	}

	account, err := rpc.readResult(probeCtx, 2, "account/read")
	if err != nil {
		return nil, nil, cleanup(err)
	}
	rateLimits, err := rpc.readResult(probeCtx, 3, "account/rateLimits/read")
	if err != nil {
		return nil, nil, cleanup(err)
	}
	if err := cleanup(nil); err != nil {
		return nil, nil, err
	}
	return account, rateLimits, nil
}

func writeJSONRPC(w io.Writer, payload map[string]any) error {
	return json.NewEncoder(w).Encode(payload)
}

type jsonRPCReader struct {
	scanner *bufio.Scanner
	pending map[int]map[string]any
}

func newJSONRPCReader(scanner *bufio.Scanner) *jsonRPCReader {
	return &jsonRPCReader{scanner: scanner, pending: make(map[int]map[string]any)}
}

func (r *jsonRPCReader) readResult(ctx context.Context, id int, method string) (map[string]any, error) {
	if payload, ok := r.pending[id]; ok {
		delete(r.pending, id)
		return jsonRPCResultPayload(payload, method)
	}

	for r.scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(r.scanner.Bytes(), &payload); err != nil {
			return nil, fmt.Errorf("%s returned malformed JSON: %w", method, err)
		}
		responseID, ok := jsonRPCID(payload["id"])
		if !ok {
			continue
		}
		if responseID != id {
			r.pending[responseID] = payload
			continue
		}
		return jsonRPCResultPayload(payload, method)
	}
	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func jsonRPCResultPayload(payload map[string]any, method string) (map[string]any, error) {
	if msg := jsonRPCErrorMessage(payload["error"]); msg != "" {
		return nil, fmt.Errorf("%s: %s", method, msg)
	}
	if result, ok := payload["result"].(map[string]any); ok {
		return result, nil
	}
	return map[string]any{}, nil
}

func jsonRPCID(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(v)
		return i, err == nil
	default:
		return 0, false
	}
}

func jsonRPCErrorMessage(value any) string {
	errMap := asMap(value)
	if errMap == nil {
		return ""
	}
	if message := sanitizedAuthField(stringField(errMap, "message")); message != "" {
		return message
	}
	body, err := json.Marshal(errMap)
	if err != nil {
		return "JSON-RPC error"
	}
	return redactProviderLimitMessage(string(body))
}

func codexAuthFromAccountResult(result map[string]any) ProviderLimitAuth {
	maps := candidateAuthMaps(result)
	account := asMap(result["account"])
	auth := ProviderLimitAuth{
		Method:   sanitizedAuthField(stringFieldFromMaps(maps, "authMode", "auth_mode", "method", "authMethod", "loginMethod", "type")),
		Provider: sanitizedAuthField(stringFieldFromMaps(maps, "provider", "accountProvider", "authProvider")),
		Plan:     sanitizedAuthField(stringFieldFromMaps(maps, "planType", "plan", "subscriptionType", "subscription_type")),
	}
	if auth.Provider == "" {
		switch auth.Method {
		case "chatgpt", "apiKey":
			auth.Provider = "openai"
		case "amazonBedrock":
			auth.Provider = "amazon"
		}
	}
	if loggedIn, ok := boolFieldFromMaps(maps, "loggedIn", "logged_in", "authenticated", "isAuthenticated"); ok {
		auth.LoggedIn = loggedIn
		return auth
	}
	if account != nil {
		auth.LoggedIn = true
		return auth
	}
	if requiresOpenAIAuth, ok := boolField(result, "requiresOpenaiAuth", "requires_openai_auth"); ok {
		auth.LoggedIn = !requiresOpenAIAuth
		return auth
	}
	auth.LoggedIn = auth.Method != "" || auth.Provider != "" || auth.Plan != ""
	return auth
}

func codexWindowsFromRateLimitResult(result map[string]any) ([]ProviderLimitWindow, string) {
	limits := asMap(result["rateLimits"])
	if limits == nil {
		limits = asMap(result["rate_limits"])
	}
	if limits == nil {
		limits = result
	}

	windows := make([]ProviderLimitWindow, 0, 2)
	for _, key := range []string{"primary", "secondary"} {
		windowMap := asMap(limits[key])
		if windowMap == nil {
			continue
		}
		windows = append(windows, codexWindowFromMap(key, windowMap))
	}
	return windows, sanitizedAuthField(stringField(limits, "rateLimitReachedType", "rate_limit_reached_type"))
}

func codexWindowFromMap(fallback string, values map[string]any) ProviderLimitWindow {
	windowMinutes := intField(values, "windowDurationMins", "window_duration_mins", "windowMinutes", "window_minutes")
	kind, label := providerLimitWindowKind(fallback, windowMinutes)
	var usedPercent *float64
	if value, ok := floatField(values, "usedPercent", "used_percent"); ok {
		usedPercent = &value
	}
	var resetsAt *time.Time
	if reset, ok := unixTimeField(values, "resetsAt", "resets_at"); ok {
		resetsAt = &reset
	}
	return ProviderLimitWindow{
		Kind:          kind,
		Label:         label,
		UsedPercent:   usedPercent,
		WindowMinutes: windowMinutes,
		ResetsAt:      resetsAt,
	}
}

func providerLimitWindowKind(fallback string, windowMinutes int) (string, string) {
	switch windowMinutes {
	case codexFiveHourWindowMinutes:
		return "five_hour", "5-hour"
	case codexWeeklyWindowMinutes:
		return "seven_day", "Weekly"
	default:
		if fallback == "secondary" {
			return "secondary", "Secondary"
		}
		return "primary", "Primary"
	}
}

func (s *providerLimitService) fetchClaude(ctx context.Context, agent domain.Agent, fetchedAt time.Time) AgentProviderLimits {
	result := s.baseResult(agent, domain.AgentKindClaude, fetchedAt)

	auth, authPayload, err := s.readClaudeAuthStatus(ctx, agent.Env)
	if err != nil {
		result.Status = ProviderLimitStatusError
		result.Message = providerLimitErrorMessage("Claude auth status", err)
		return result
	}
	result.Auth = auth

	if windows := claudeWindowsFromStatusPayload(authPayload); len(windows) > 0 {
		result.Status = ProviderLimitStatusOK
		result.Windows = windows
		return result
	}

	if !auth.LoggedIn {
		result.Status = ProviderLimitStatusUnavailable
		result.Message = "Claude Code is not logged in; numeric usage limits are unavailable."
		return result
	}

	usagePayload, err := s.readClaudeUsageAPI(ctx, agent.Env)
	if err != nil {
		result.Status = ProviderLimitStatusError
		switch {
		case errors.Is(err, errClaudeUsageNoCredentials):
			result.Status = ProviderLimitStatusUnavailable
			result.Message = "Claude Code OAuth credentials were not found; numeric usage limits are unavailable."
		case errors.Is(err, errClaudeUsageRateLimited):
			result.Status = ProviderLimitStatusUnavailable
			result.Message = "Claude usage API is rate-limited; numeric usage limits are temporarily unavailable."
		case errors.Is(err, errClaudeUsageUnauthorized):
			result.Status = ProviderLimitStatusUnavailable
			result.Message = "Claude usage API rejected the local OAuth token; numeric usage limits are unavailable."
		default:
			result.Message = providerLimitErrorMessage("Claude usage API", err)
		}
		return result
	}

	result.Windows = claudeWindowsFromUsagePayload(usagePayload)
	if len(result.Windows) == 0 {
		result.Status = ProviderLimitStatusUnavailable
		result.Message = "Claude usage API did not return numeric usage limit windows."
		return result
	}

	result.Status = ProviderLimitStatusOK
	return result
}

func (s *providerLimitService) readClaudeAuthStatus(ctx context.Context, env map[string]string) (ProviderLimitAuth, map[string]any, error) {
	probeCtx, cancel := context.WithTimeout(ctx, s.probeTimeout)
	defer cancel()

	cmd := providerProbeCommand(probeCtx, s.claudeCommand, "auth", "status", "--json")
	cmd.Env = providerProbeEnv(env)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return ProviderLimitAuth{}, nil, errProviderLimitTimeout
		}
		return ProviderLimitAuth{}, nil, providerProbeError("claude auth status", err, stderr.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(bytes.NewReader(stdout.Bytes())).Decode(&payload); err != nil {
		return ProviderLimitAuth{}, nil, fmt.Errorf("claude auth status returned malformed JSON: %w", err)
	}
	return claudeAuthFromStatusPayload(payload), payload, nil
}

func claudeAuthFromStatusPayload(payload map[string]any) ProviderLimitAuth {
	maps := candidateAuthMaps(payload)
	auth := ProviderLimitAuth{
		Method:   sanitizedAuthField(stringFieldFromMaps(maps, "method", "authMethod", "auth_method", "loginMethod", "login_method")),
		Provider: sanitizedAuthField(stringFieldFromMaps(maps, "provider", "authProvider", "auth_provider")),
		Plan:     sanitizedAuthField(stringFieldFromMaps(maps, "subscriptionType", "subscription_type", "planType", "plan_type", "plan", "type")),
	}
	if loggedIn, ok := boolFieldFromMaps(maps, "loggedIn", "logged_in", "authenticated", "isAuthenticated", "is_authenticated"); ok {
		auth.LoggedIn = loggedIn
		return auth
	}
	status := strings.ToLower(strings.TrimSpace(stringFieldFromMaps(maps, "status", "state")))
	switch status {
	case "logged_in", "authenticated", "valid", "ok", "active":
		auth.LoggedIn = true
	case "logged_out", "unauthenticated", "invalid", "missing":
		auth.LoggedIn = false
	default:
		auth.LoggedIn = auth.Method != "" || auth.Provider != "" || auth.Plan != ""
	}
	return auth
}

func claudeWindowsFromStatusPayload(payload map[string]any) []ProviderLimitWindow {
	limits := asMap(payload["rate_limits"])
	if limits == nil {
		limits = asMap(payload["rateLimits"])
	}
	if limits == nil {
		return nil
	}
	return claudeWindowsFromLimitMap(limits, "used_percentage", "usedPercent", "used_percent", "utilization")
}

func claudeWindowsFromUsagePayload(payload map[string]any) []ProviderLimitWindow {
	if usage := asMap(payload["usage"]); usage != nil {
		payload = usage
	}
	return claudeWindowsFromLimitMap(payload, "utilization", "used_percentage", "usedPercent", "used_percent")
}

func claudeWindowsFromLimitMap(limits map[string]any, usedKeys ...string) []ProviderLimitWindow {
	specs := []struct {
		key           string
		fallback      string
		windowMinutes int
	}{
		{key: "five_hour", fallback: "primary", windowMinutes: claudeFiveHourWindowMinutes},
		{key: "seven_day", fallback: "secondary", windowMinutes: claudeWeeklyWindowMinutes},
	}

	windows := make([]ProviderLimitWindow, 0, len(specs))
	for _, spec := range specs {
		windowMap := asMap(limits[spec.key])
		if windowMap == nil {
			continue
		}
		window, ok := claudeWindowFromMap(spec.fallback, spec.windowMinutes, windowMap, usedKeys...)
		if ok {
			windows = append(windows, window)
		}
	}
	return windows
}

func claudeWindowFromMap(fallback string, windowMinutes int, values map[string]any, usedKeys ...string) (ProviderLimitWindow, bool) {
	kind, label := providerLimitWindowKind(fallback, windowMinutes)
	used, hasUsed := floatField(values, usedKeys...)
	reset, hasReset := unixTimeField(values, "resets_at", "resetsAt")
	if !hasUsed && !hasReset {
		return ProviderLimitWindow{}, false
	}

	var usedPercent *float64
	if hasUsed {
		usedPercent = &used
	}
	var resetsAt *time.Time
	if hasReset {
		resetsAt = &reset
	}
	return ProviderLimitWindow{
		Kind:          kind,
		Label:         label,
		UsedPercent:   usedPercent,
		WindowMinutes: windowMinutes,
		ResetsAt:      resetsAt,
	}, true
}

func (s *providerLimitService) readClaudeUsageAPI(ctx context.Context, env map[string]string) (map[string]any, error) {
	probeCtx, cancel := context.WithTimeout(ctx, s.probeTimeout)
	defer cancel()

	tokens, err := s.readClaudeUsageAccessTokens(probeCtx, env)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, errClaudeUsageNoCredentials
	}

	unauthorized := false
	for _, token := range tokens {
		payload, err := s.fetchClaudeUsageAPIWithToken(probeCtx, env, token)
		if err == nil {
			return payload, nil
		}
		if errors.Is(err, errClaudeUsageUnauthorized) {
			unauthorized = true
			continue
		}
		return nil, err
	}
	if unauthorized {
		return nil, errClaudeUsageUnauthorized
	}
	return nil, errClaudeUsageNoCredentials
}

func (s *providerLimitService) fetchClaudeUsageAPIWithToken(ctx context.Context, env map[string]string, token string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.claudeUsageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create Claude usage API request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", claudeUsageAPIBeta)

	resp, err := s.providerHTTPClient(env).Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errProviderLimitTimeout
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errClaudeUsageRateLimited
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errClaudeUsageUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Claude usage API returned HTTP %d", resp.StatusCode)
	}

	var payload map[string]any
	decoder := json.NewDecoder(io.LimitReader(resp.Body, providerProbeScannerBufferMax))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("Claude usage API returned malformed JSON: %w", err)
	}
	return payload, nil
}

func (s *providerLimitService) providerHTTPClient(env map[string]string) *http.Client {
	if s.httpClient != nil {
		return s.httpClient
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = providerHTTPProxy(env)
	return &http.Client{Transport: transport}
}

func providerHTTPProxy(env map[string]string) func(*http.Request) (*url.URL, error) {
	values := providerProbeEnvMap(env)
	return func(req *http.Request) (*url.URL, error) {
		if providerNoProxyMatches(values, req) {
			return nil, nil
		}
		proxyURL := providerProxyURLForRequest(values, req)
		if proxyURL == "" {
			return nil, nil
		}
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, nil
		}
		return parsed, nil
	}
}

func providerProxyURLForRequest(values map[string]string, req *http.Request) string {
	switch strings.ToLower(req.URL.Scheme) {
	case "https":
		if proxyURL := strings.TrimSpace(providerEnvMapValue(values, "HTTPS_PROXY", "https_proxy")); proxyURL != "" {
			return proxyURL
		}
	case "http":
		if proxyURL := strings.TrimSpace(providerEnvMapValue(values, "HTTP_PROXY", "http_proxy")); proxyURL != "" {
			return proxyURL
		}
	}
	return strings.TrimSpace(providerEnvMapValue(values, "ALL_PROXY", "all_proxy"))
}

func providerNoProxyMatches(values map[string]string, req *http.Request) bool {
	rawNoProxy := providerEnvMapValue(values, "NO_PROXY", "no_proxy")
	if strings.TrimSpace(rawNoProxy) == "" {
		return false
	}
	host := strings.ToLower(req.URL.Hostname())
	hostPort := strings.ToLower(req.URL.Host)
	for _, entry := range strings.Split(rawNoProxy, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if entry == "*" {
			return true
		}
		if parsed, err := url.Parse(entry); err == nil && parsed.Hostname() != "" {
			entry = parsed.Hostname()
		}
		if hostPort == entry || host == entry {
			return true
		}
		entryHost := strings.TrimPrefix(entry, ".")
		if entryHost != "" && (host == entryHost || strings.HasSuffix(host, "."+entryHost)) {
			return true
		}
	}
	return false
}

func (s *providerLimitService) readClaudeUsageAccessTokens(ctx context.Context, env map[string]string) ([]string, error) {
	tokens := make([]string, 0, 3)
	seen := make(map[string]struct{})
	appendToken := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}

	if runtime.GOOS == "darwin" {
		token, err := readClaudeUsageTokenFromMacKeychainService(ctx, env, claudeCredentialsService)
		if err != nil {
			return nil, err
		}
		appendToken(token)

		services, err := listClaudeMacKeychainCredentialCandidates(ctx, env)
		if err != nil {
			return nil, err
		}
		for _, service := range services {
			token, err := readClaudeUsageTokenFromMacKeychainService(ctx, env, service)
			if err != nil {
				return nil, err
			}
			appendToken(token)
		}
	}

	appendToken(readClaudeUsageTokenFromCredentialsFile(env))
	if len(tokens) == 0 {
		return nil, errClaudeUsageNoCredentials
	}
	return tokens, nil
}

func readClaudeUsageTokenFromMacKeychainService(ctx context.Context, env map[string]string, service string) (string, error) {
	cmd := providerProbeCommand(ctx, "security", "find-generic-password", "-s", service, "-w")
	cmd.Env = providerProbeEnv(env)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errProviderLimitTimeout
		}
		return "", nil
	}
	return parseClaudeUsageAccessToken(output), nil
}

func listClaudeMacKeychainCredentialCandidates(ctx context.Context, env map[string]string) ([]string, error) {
	cmd := providerProbeCommand(ctx, "security", "dump-keychain")
	cmd.Env = providerProbeEnv(env)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errProviderLimitTimeout
		}
		return nil, nil
	}
	return parseClaudeMacKeychainCredentialCandidates(string(output)), nil
}

func parseClaudeMacKeychainCredentialCandidates(raw string) []string {
	const marker = `"svce"<blob>="`
	seen := make(map[string]struct{})
	var services []string
	for _, line := range strings.Split(raw, "\n") {
		start := strings.Index(line, marker)
		if start < 0 {
			continue
		}
		service := line[start+len(marker):]
		if end := strings.Index(service, `"`); end >= 0 {
			service = service[:end]
		}
		if service == claudeCredentialsService || !strings.HasPrefix(service, claudeCredentialsService) {
			continue
		}
		if _, ok := seen[service]; ok {
			continue
		}
		seen[service] = struct{}{}
		services = append(services, service)
	}
	return services
}

func readClaudeUsageTokenFromCredentialsFile(env map[string]string) string {
	raw, err := os.ReadFile(filepath.Join(claudeConfigDir(env), ".credentials.json"))
	if err != nil {
		return ""
	}
	return parseClaudeUsageAccessToken(raw)
}

func claudeConfigDir(env map[string]string) string {
	if configDir := strings.TrimSpace(providerProbeEnvValue(env, "CLAUDE_CONFIG_DIR")); configDir != "" {
		if abs, err := filepath.Abs(configDir); err == nil {
			return abs
		}
		return configDir
	}
	if home := strings.TrimSpace(providerProbeEnvValue(env, "HOME", "USERPROFILE")); home != "" {
		return filepath.Join(home, ".claude")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".claude")
	}
	return ".claude"
}

func parseClaudeUsageAccessToken(raw []byte) string {
	var payload struct {
		ClaudeAIOAuth *struct {
			AccessToken    *string `json:"accessToken"`
			AccessTokenAlt *string `json:"access_token"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.ClaudeAIOAuth == nil {
		return ""
	}
	if payload.ClaudeAIOAuth.AccessToken != nil {
		return strings.TrimSpace(*payload.ClaudeAIOAuth.AccessToken)
	}
	if payload.ClaudeAIOAuth.AccessTokenAlt != nil {
		return strings.TrimSpace(*payload.ClaudeAIOAuth.AccessTokenAlt)
	}
	return ""
}

func candidateAuthMaps(payload map[string]any) []map[string]any {
	maps := []map[string]any{payload}
	for _, key := range []string{"account", "auth", "user", "subscription"} {
		if nested := asMap(payload[key]); nested != nil {
			maps = append(maps, nested)
		}
	}
	if auth := asMap(payload["auth"]); auth != nil {
		for _, key := range []string{"account", "user", "subscription"} {
			if nested := asMap(auth[key]); nested != nil {
				maps = append(maps, nested)
			}
		}
	}
	if account := asMap(payload["account"]); account != nil {
		for _, key := range []string{"subscription", "plan"} {
			if nested := asMap(account[key]); nested != nil {
				maps = append(maps, nested)
			}
		}
	}
	return maps
}

func providerProbeEnv(overrides map[string]string) []string {
	allowed := map[string]struct{}{
		"ALL_PROXY":           {},
		"APPDATA":             {},
		"CLAUDE_CONFIG_DIR":   {},
		"CURL_CA_BUNDLE":      {},
		"GIT_SSL_CAINFO":      {},
		"HOME":                {},
		"HTTP_PROXY":          {},
		"HTTPS_PROXY":         {},
		"LOCALAPPDATA":        {},
		"LOGNAME":             {},
		"NODE_EXTRA_CA_CERTS": {},
		"NO_PROXY":            {},
		"PATH":                {},
		"REQUESTS_CA_BUNDLE":  {},
		"SHELL":               {},
		"SSL_CERT_DIR":        {},
		"SSL_CERT_FILE":       {},
		"TEMP":                {},
		"TMP":                 {},
		"TMPDIR":              {},
		"USER":                {},
		"USERPROFILE":         {},
		"XDG_CACHE_HOME":      {},
		"XDG_CONFIG_HOME":     {},
		"XDG_DATA_HOME":       {},
		"all_proxy":           {},
		"http_proxy":          {},
		"https_proxy":         {},
		"no_proxy":            {},
	}
	env := make([]string, 0, len(allowed)+len(overrides))
	index := make(map[string]int, len(allowed)+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, ok := allowed[key]; !ok {
			continue
		}
		index[key] = len(env)
		env = append(env, entry)
	}
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" || strings.Contains(key, "=") {
			continue
		}
		entry := key + "=" + value
		if i, ok := index[key]; ok {
			env[i] = entry
		} else {
			index[key] = len(env)
			env = append(env, entry)
		}
	}
	return env
}

func providerProbeEnvValue(overrides map[string]string, keys ...string) string {
	values := providerProbeEnvMap(overrides)
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return ""
}

func providerProbeEnvMap(overrides map[string]string) map[string]string {
	env := providerProbeEnv(overrides)
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func providerEnvMapValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return ""
}

func providerProbeCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = providerProbeWaitDelay
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return killProviderProbeProcessGroup(cmd)
	}
	return cmd
}

func killProviderProbeProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

func providerProbeError(operation string, err error, stderr string) error {
	if detail := strings.TrimSpace(stderr); detail != "" {
		return fmt.Errorf("%s failed: %s: %w", operation, redactProviderLimitMessage(detail), err)
	}
	return fmt.Errorf("%s failed: %w", operation, err)
}

func providerLimitErrorMessage(operation string, err error) string {
	if errors.Is(err, errProviderLimitTimeout) {
		return operation + " timed out."
	}
	if err == nil {
		return operation + " failed."
	}
	return redactProviderLimitMessage(err.Error())
}

func redactProviderLimitMessage(value string) string {
	value = emailRedactionPattern.ReplaceAllString(value, "[redacted-email]")
	return authFieldRedactPattern.ReplaceAllString(value, "${1}[redacted]")
}

func sanitizedAuthField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || emailRedactionPattern.MatchString(value) {
		return ""
	}
	return value
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func stringFieldFromMaps(maps []map[string]any, keys ...string) string {
	for _, values := range maps {
		if value := stringField(values, keys...); value != "" {
			return value
		}
	}
	return ""
}

func stringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := values[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return value
			}
		case fmt.Stringer:
			if strings.TrimSpace(value.String()) != "" {
				return value.String()
			}
		}
	}
	return ""
}

func boolFieldFromMaps(maps []map[string]any, keys ...string) (bool, bool) {
	for _, values := range maps {
		if value, ok := boolField(values, keys...); ok {
			return value, true
		}
	}
	return false, false
}

func boolField(values map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		switch value := values[key].(type) {
		case bool:
			return value, true
		case string:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "true", "yes", "1", "logged_in", "authenticated":
				return true, true
			case "false", "no", "0", "logged_out", "unauthenticated":
				return false, true
			}
		}
	}
	return false, false
}

func intField(values map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := values[key].(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		case json.Number:
			i, err := value.Int64()
			if err == nil {
				return int(i)
			}
		case string:
			i, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil {
				return i
			}
		}
	}
	return 0
}

func floatField(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		switch value := values[key].(type) {
		case float64:
			return value, true
		case float32:
			return float64(value), true
		case int:
			return float64(value), true
		case int64:
			return float64(value), true
		case json.Number:
			f, err := value.Float64()
			if err == nil {
				return f, true
			}
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

func unixTimeField(values map[string]any, keys ...string) (time.Time, bool) {
	for _, key := range keys {
		switch value := values[key].(type) {
		case float64:
			return time.Unix(int64(value), 0).UTC(), true
		case int:
			return time.Unix(int64(value), 0).UTC(), true
		case int64:
			return time.Unix(value, 0).UTC(), true
		case json.Number:
			i, err := value.Int64()
			if err == nil {
				return time.Unix(i, 0).UTC(), true
			}
		case string:
			trimmed := strings.TrimSpace(value)
			if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				return time.Unix(i, 0).UTC(), true
			}
			if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
				return parsed.UTC(), true
			}
		}
	}
	return time.Time{}, false
}
