package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

func TestProviderLimitServiceCodexReadsRateLimits(t *testing.T) {
	script := writeExecutable(t, "codex", `#!/bin/sh
if [ "$AGENTX_TEST_ENV" != "from-agent" ]; then
  echo "missing agent env" >&2
  exit 3
fi
if [ "$1" != "app-server" ] || [ "$2" != "--listen" ] || [ "$3" != "stdio://" ]; then
  echo "unexpected args: $*" >&2
  exit 2
fi
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*|*'"id": 1'*)
      echo '{"id":1,"result":{"userAgent":"fake-codex"}}'
      ;;
    *'"account/read"'*)
      case "$line" in
        *'"params":{}'*|*'"params": {}'*) ;;
        *) echo "account/read missing params" >&2; exit 4 ;;
      esac
      saw_account=1
      ;;
    *'"account/rateLimits/read"'*)
      case "$line" in
        *'"params":{}'*|*'"params": {}'*) ;;
        *) echo "account/rateLimits/read missing params" >&2; exit 4 ;;
      esac
      echo '{"id":3,"result":{"rateLimits":{"primary":{"usedPercent":42.5,"windowDurationMins":300,"resetsAt":1893456000},"secondary":{"usedPercent":10,"windowDurationMins":10080,"resetsAt":1894060800},"rateLimitReachedType":null}}}'
      if [ "$saw_account" = "1" ]; then
        echo '{"id":2,"result":{"account":{"type":"chatgpt","email":"person@example.com","planType":"plus"},"requiresOpenaiAuth":true}}'
      fi
      exit 0
      ;;
  esac
done
`)

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	service := newProviderLimitService(ProviderLimitOptions{
		CodexCommand: script,
		ProbeTimeout: time.Second,
		CacheTTL:     time.Minute,
		Now:          func() time.Time { return now },
	})

	got := service.Read(context.Background(), domain.Agent{
		ID:        "agt_codex",
		Kind:      domain.AgentKindCodex,
		Env:       map[string]string{"AGENTX_TEST_ENV": "from-agent"},
		UpdatedAt: now,
	}, false)

	if got.Status != ProviderLimitStatusOK {
		t.Fatalf("status = %q, want %q; message=%q", got.Status, ProviderLimitStatusOK, got.Message)
	}
	if !got.Auth.LoggedIn || got.Auth.Method != "chatgpt" || got.Auth.Provider != "openai" || got.Auth.Plan != "plus" {
		t.Fatalf("auth = %#v", got.Auth)
	}
	if len(got.Windows) != 2 {
		t.Fatalf("windows = %#v, want two windows", got.Windows)
	}
	if got.Windows[0].Kind != "five_hour" || got.Windows[0].WindowMinutes != 300 || got.Windows[0].UsedPercent == nil || *got.Windows[0].UsedPercent != 42.5 {
		t.Fatalf("primary window = %#v", got.Windows[0])
	}
	if got.Windows[1].Kind != "seven_day" || got.Windows[1].WindowMinutes != 10080 {
		t.Fatalf("secondary window = %#v", got.Windows[1])
	}
	assertProviderLimitJSONRedacted(t, got, "person@example.com")
}

func TestProviderLimitServiceClaudeReadsUsageAPI(t *testing.T) {
	script := writeExecutable(t, "claude", `#!/bin/sh
if [ "$1" != "auth" ] || [ "$2" != "status" ] || [ "$3" != "--json" ]; then
  echo "unexpected args: $*" >&2
  exit 2
fi
cat <<'JSON'
{"loggedIn":true,"method":"oauth","provider":"claude.ai","subscription":{"type":"max"},"email":"person@example.com","organization":"Secret Org"}
JSON
`)
	home := t.TempDir()
	writeClaudeCredentials(t, home, `{"claudeAiOauth":{"accessToken":"secret-token"},"email":"person@example.com"}`)
	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/usage" {
			t.Fatalf("usage path = %q, want /api/oauth/usage", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("authorization header = %q", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != claudeUsageAPIBeta {
			t.Fatalf("anthropic-beta header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":41.5,"resets_at":"2030-01-01T00:00:00Z"},"seven_day":{"utilization":17,"resets_at":"2030-01-08T00:00:00Z"}}`))
	}))
	t.Cleanup(usageServer.Close)

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	service := newProviderLimitService(ProviderLimitOptions{
		ClaudeCommand:  script,
		ClaudeUsageURL: usageServer.URL + "/api/oauth/usage",
		ProbeTimeout:   time.Second,
		Now:            func() time.Time { return now },
	})

	got := service.Read(context.Background(), domain.Agent{
		ID:        "agt_claude",
		Kind:      domain.AgentKindClaude,
		Env:       map[string]string{"HOME": home},
		UpdatedAt: now,
	}, false)

	if got.Status != ProviderLimitStatusOK {
		t.Fatalf("status = %q, want %q; message=%q", got.Status, ProviderLimitStatusOK, got.Message)
	}
	if !got.Auth.LoggedIn || got.Auth.Method != "oauth" || got.Auth.Provider != "claude.ai" || got.Auth.Plan != "max" {
		t.Fatalf("auth = %#v", got.Auth)
	}
	if len(got.Windows) != 2 {
		t.Fatalf("windows = %#v, want two windows", got.Windows)
	}
	if got.Windows[0].Kind != "five_hour" || got.Windows[0].WindowMinutes != 300 || got.Windows[0].UsedPercent == nil || *got.Windows[0].UsedPercent != 41.5 {
		t.Fatalf("five hour window = %#v", got.Windows[0])
	}
	if got.Windows[1].Kind != "seven_day" || got.Windows[1].WindowMinutes != 10080 || got.Windows[1].UsedPercent == nil || *got.Windows[1].UsedPercent != 17 {
		t.Fatalf("weekly window = %#v", got.Windows[1])
	}
	assertProviderLimitJSONRedacted(t, got, "person@example.com", "Secret Org", "secret-token")
}

func TestProviderLimitServiceClaudeReadsStatusRateLimits(t *testing.T) {
	script := writeExecutable(t, "claude", `#!/bin/sh
cat <<'JSON'
{"loggedIn":true,"method":"oauth","provider":"claude.ai","subscriptionType":"pro","rate_limits":{"five_hour":{"used_percentage":42,"resets_at":1893456000},"seven_day":{"used_percentage":15,"resets_at":1894060800}}}
JSON
`)

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	service := newProviderLimitService(ProviderLimitOptions{
		ClaudeCommand: script,
		ProbeTimeout:  time.Second,
		Now:           func() time.Time { return now },
	})

	got := service.Read(context.Background(), domain.Agent{
		ID:        "agt_claude",
		Kind:      domain.AgentKindClaude,
		Env:       map[string]string{"HOME": t.TempDir()},
		UpdatedAt: now,
	}, false)

	if got.Status != ProviderLimitStatusOK {
		t.Fatalf("status = %q, want %q; message=%q", got.Status, ProviderLimitStatusOK, got.Message)
	}
	if len(got.Windows) != 2 || got.Windows[0].Kind != "five_hour" || got.Windows[1].Kind != "seven_day" {
		t.Fatalf("windows = %#v", got.Windows)
	}
}

func TestProviderLimitServiceClaudeNoCredentialsIsUnavailableAndRedacted(t *testing.T) {
	script := writeExecutable(t, "claude", `#!/bin/sh
if [ "$1" != "auth" ] || [ "$2" != "status" ] || [ "$3" != "--json" ]; then
  echo "unexpected args: $*" >&2
  exit 2
fi
cat <<'JSON'
{"loggedIn":true,"method":"oauth","provider":"claude.ai","subscription":{"type":"max"},"email":"person@example.com","organization":"Secret Org"}
JSON
`)

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	service := newProviderLimitService(ProviderLimitOptions{
		ClaudeCommand: script,
		ProbeTimeout:  time.Second,
		Now:           func() time.Time { return now },
	})

	got := service.Read(context.Background(), domain.Agent{
		ID:        "agt_claude",
		Kind:      domain.AgentKindClaude,
		Env:       map[string]string{"HOME": t.TempDir()},
		UpdatedAt: now,
	}, false)

	if got.Status != ProviderLimitStatusUnavailable {
		t.Fatalf("status = %q, want %q", got.Status, ProviderLimitStatusUnavailable)
	}
	if !got.Auth.LoggedIn || got.Auth.Method != "oauth" || got.Auth.Provider != "claude.ai" || got.Auth.Plan != "max" {
		t.Fatalf("auth = %#v", got.Auth)
	}
	if len(got.Windows) != 0 {
		t.Fatalf("windows = %#v, want none", got.Windows)
	}
	if !strings.Contains(got.Message, "OAuth credentials were not found") {
		t.Fatalf("message = %q", got.Message)
	}
	assertProviderLimitJSONRedacted(t, got, "person@example.com", "Secret Org")
}

func TestProviderLimitServiceClaudeUsageForbiddenIsUnavailable(t *testing.T) {
	script := writeExecutable(t, "claude", `#!/bin/sh
cat <<'JSON'
{"loggedIn":true,"method":"oauth","provider":"claude.ai","subscriptionType":"pro"}
JSON
`)
	home := t.TempDir()
	writeClaudeCredentials(t, home, `{"claudeAiOauth":{"accessToken":"forbidden-token"}}`)
	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"email":"person@example.com","organization":"Secret Org"}`, http.StatusForbidden)
	}))
	t.Cleanup(usageServer.Close)

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	service := newProviderLimitService(ProviderLimitOptions{
		ClaudeCommand:  script,
		ClaudeUsageURL: usageServer.URL,
		ProbeTimeout:   time.Second,
		Now:            func() time.Time { return now },
	})

	got := service.Read(context.Background(), domain.Agent{
		ID:        "agt_claude",
		Kind:      domain.AgentKindClaude,
		Env:       map[string]string{"HOME": home},
		UpdatedAt: now,
	}, false)

	if got.Status != ProviderLimitStatusUnavailable {
		t.Fatalf("status = %q, want %q; message=%q", got.Status, ProviderLimitStatusUnavailable, got.Message)
	}
	if !strings.Contains(got.Message, "rejected the local OAuth token") {
		t.Fatalf("message = %q", got.Message)
	}
	assertProviderLimitJSONRedacted(t, got, "forbidden-token", "person@example.com", "Secret Org")
}

func TestProviderLimitServiceUnavailableAndTimeoutPaths(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	service := newProviderLimitService(ProviderLimitOptions{Now: func() time.Time { return now }})
	got := service.Read(context.Background(), domain.Agent{
		ID:        "agt_fake",
		Kind:      domain.AgentKindFake,
		UpdatedAt: now,
	}, false)
	if got.Status != ProviderLimitStatusUnavailable || !strings.Contains(got.Message, "Claude Code and Codex") {
		t.Fatalf("unsupported result = %#v", got)
	}

	slowClaude := writeExecutable(t, "claude-slow", `#!/bin/sh
sleep 5
`)
	service = newProviderLimitService(ProviderLimitOptions{
		ClaudeCommand: slowClaude,
		ProbeTimeout:  20 * time.Millisecond,
		Now:           func() time.Time { return now },
	})
	startedAt := time.Now()
	got = service.Read(context.Background(), domain.Agent{
		ID:        "agt_claude",
		Kind:      domain.AgentKindClaude,
		UpdatedAt: now,
	}, false)
	if elapsed := time.Since(startedAt); elapsed >= time.Second {
		t.Fatalf("timeout took %s, want under 1s", elapsed)
	}
	if got.Status != ProviderLimitStatusError || !strings.Contains(got.Message, "timed out") {
		t.Fatalf("timeout result = %#v", got)
	}
}

func TestProviderLimitServiceCacheForceAndAgentUpdate(t *testing.T) {
	counterPath := filepath.Join(t.TempDir(), "count")
	script := writeExecutable(t, "claude-count", `#!/bin/sh
count=0
if [ -f "`+counterPath+`" ]; then
  count=$(cat "`+counterPath+`")
fi
count=$((count + 1))
echo "$count" > "`+counterPath+`"
printf '{"loggedIn":true,"method":"oauth","provider":"claude.ai","subscriptionType":"pro-%s"}\n' "$count"
`)

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	service := newProviderLimitService(ProviderLimitOptions{
		ClaudeCommand: script,
		CacheTTL:      time.Minute,
		ProbeTimeout:  time.Second,
		Now:           func() time.Time { return now },
	})
	agent := domain.Agent{ID: "agt_claude", Kind: domain.AgentKindClaude, Env: map[string]string{"HOME": t.TempDir()}, UpdatedAt: now}

	first := service.Read(context.Background(), agent, false)
	second := service.Read(context.Background(), agent, false)
	if first.Auth.Plan != "pro-1" || second.Auth.Plan != "pro-1" {
		t.Fatalf("cache results = %q then %q, want both pro-1", first.Auth.Plan, second.Auth.Plan)
	}

	forced := service.Read(context.Background(), agent, true)
	if forced.Auth.Plan != "pro-2" {
		t.Fatalf("forced plan = %q, want pro-2", forced.Auth.Plan)
	}

	agent.UpdatedAt = now.Add(time.Second)
	updated := service.Read(context.Background(), agent, false)
	if updated.Auth.Plan != "pro-3" {
		t.Fatalf("updated agent plan = %q, want pro-3", updated.Auth.Plan)
	}
}

func TestProviderLimitServiceCoalescesConcurrentFetches(t *testing.T) {
	counterPath := filepath.Join(t.TempDir(), "count")
	script := writeExecutable(t, "claude-count", `#!/bin/sh
sleep 0.1
count=0
if [ -f "`+counterPath+`" ]; then
  count=$(cat "`+counterPath+`")
fi
count=$((count + 1))
echo "$count" > "`+counterPath+`"
printf '{"loggedIn":true,"method":"oauth","provider":"claude.ai","subscriptionType":"pro-%s"}\n' "$count"
`)

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	service := newProviderLimitService(ProviderLimitOptions{
		ClaudeCommand: script,
		CacheTTL:      time.Minute,
		ProbeTimeout:  time.Second,
		Now:           func() time.Time { return now },
	})
	agent := domain.Agent{ID: "agt_claude", Kind: domain.AgentKindClaude, Env: map[string]string{"HOME": t.TempDir()}, UpdatedAt: now}

	var wg sync.WaitGroup
	results := make([]AgentProviderLimits, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = service.Read(context.Background(), agent, false)
		}(i)
	}
	wg.Wait()

	for i, result := range results {
		if result.Auth.Plan != "pro-1" {
			t.Fatalf("result %d plan = %q, want pro-1", i, result.Auth.Plan)
		}
	}
	count, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(count)) != "1" {
		t.Fatalf("fetch count = %q, want 1", strings.TrimSpace(string(count)))
	}
}

func TestProviderProbeEnvFiltersHostAndAppliesAgentEnv(t *testing.T) {
	t.Setenv("AGENTX_ADMIN_TOKEN", "host-secret")
	t.Setenv("HTTPS_PROXY", "http://proxy.local:8080")
	t.Setenv("PATH", "/usr/bin")

	env := providerProbeEnv(map[string]string{
		"AGENTX_TEST_ENV": "from-agent",
		"OPENAI_API_KEY":  "agent-key",
	})
	values := envMap(env)

	if _, ok := values["AGENTX_ADMIN_TOKEN"]; ok {
		t.Fatalf("host AGENTX_ADMIN_TOKEN leaked into probe env: %#v", values)
	}
	if values["PATH"] != "/usr/bin" {
		t.Fatalf("PATH = %q, want /usr/bin", values["PATH"])
	}
	if values["HTTPS_PROXY"] != "http://proxy.local:8080" {
		t.Fatalf("HTTPS_PROXY = %q, want host proxy", values["HTTPS_PROXY"])
	}
	if values["AGENTX_TEST_ENV"] != "from-agent" || values["OPENAI_API_KEY"] != "agent-key" {
		t.Fatalf("agent env not applied: %#v", values)
	}
}

func TestProviderHTTPProxyUsesProviderEnv(t *testing.T) {
	clearProxyEnv(t)

	assertProxyURL(t,
		providerHTTPProxy(map[string]string{"HTTPS_PROXY": "http://https-proxy.local:8080"}),
		"https://api.anthropic.com/api/oauth/usage",
		"http://https-proxy.local:8080",
	)
	assertProxyURL(t,
		providerHTTPProxy(map[string]string{"HTTP_PROXY": "http://http-proxy.local:8080"}),
		"http://example.test/path",
		"http://http-proxy.local:8080",
	)
	assertProxyURL(t,
		providerHTTPProxy(map[string]string{"ALL_PROXY": "socks5://all-proxy.local:1080"}),
		"https://api.anthropic.com/api/oauth/usage",
		"socks5://all-proxy.local:1080",
	)
	assertProxyURL(t,
		providerHTTPProxy(map[string]string{
			"HTTPS_PROXY": "http://https-proxy.local:8080",
			"NO_PROXY":    "api.anthropic.com",
		}),
		"https://api.anthropic.com/api/oauth/usage",
		"",
	)
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ALL_PROXY", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
		"all_proxy", "http_proxy", "https_proxy", "no_proxy",
	} {
		t.Setenv(key, "")
	}
}

func assertProxyURL(t *testing.T, proxy func(*http.Request) (*url.URL, error), requestURL string, want string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if want == "" {
		if got != nil {
			t.Fatalf("proxy for %s = %s, want nil", requestURL, got)
		}
		return
	}
	if got == nil || got.String() != want {
		t.Fatalf("proxy for %s = %v, want %s", requestURL, got, want)
	}
}

func envMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func writeExecutable(t *testing.T, name string, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeClaudeCredentials(t *testing.T, home string, body string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertProviderLimitJSONRedacted(t *testing.T, value AgentProviderLimits, forbidden ...string) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, needle := range forbidden {
		if strings.Contains(text, needle) {
			t.Fatalf("provider limit response leaked %q: %s", needle, text)
		}
	}
}
