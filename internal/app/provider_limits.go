package app

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
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
	case domain.AgentKindCodex, domain.AgentKindCodexPersistent:
		return s.fetchCodex(ctx, agent, fetchedAt)
	case domain.AgentKindClaude, domain.AgentKindClaudePersistent:
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
