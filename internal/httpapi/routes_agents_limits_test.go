package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

func TestHTTPAgentLimitsAuthorizationAndUnsupportedProvider(t *testing.T) {
	ts := newTestServer(t)
	bootstrap := setupHTTP(t, ts.URL)

	getJSON(t, ts.URL+"/api/agents/"+bootstrap.Agent.ID+"/limits", "", http.StatusUnauthorized, nil)
	getJSON(t, ts.URL+"/api/agents/not-a-real-agent/limits", bootstrap.SessionToken, http.StatusNotFound, nil)

	var limits app.AgentProviderLimits
	getJSON(t, ts.URL+"/api/agents/"+bootstrap.Agent.ID+"/limits", bootstrap.SessionToken, http.StatusOK, &limits)
	if limits.Status != app.ProviderLimitStatusUnavailable || limits.Provider != domain.AgentKindFake {
		t.Fatalf("limits = %#v", limits)
	}
	if !strings.Contains(limits.Message, "Claude Code and Codex") {
		t.Fatalf("message = %q", limits.Message)
	}
}

func TestHTTPAgentLimitsProviderJSON(t *testing.T) {
	codexScript := writeHTTPExecutable(t, "codex", `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*|*'"id": 1'*)
      echo '{"id":1,"result":{}}'
      ;;
    *'"account/read"'*)
      case "$line" in
        *'"params":{}'*|*'"params": {}'*) ;;
        *) echo "account/read missing params" >&2; exit 4 ;;
      esac
      echo '{"id":2,"result":{"account":{"type":"chatgpt","email":"person@example.com","planType":"team"},"requiresOpenaiAuth":true}}'
      ;;
    *'"account/rateLimits/read"'*)
      case "$line" in
        *'"params":{}'*|*'"params": {}'*) ;;
        *) echo "account/rateLimits/read missing params" >&2; exit 4 ;;
      esac
      echo '{"id":3,"result":{"rateLimits":{"primary":{"usedPercent":33,"windowDurationMins":300,"resetsAt":1893456000},"secondary":{"usedPercent":5,"windowDurationMins":10080,"resetsAt":1894060800}}}}'
      exit 0
      ;;
  esac
done
`)
	claudeScript := writeHTTPExecutable(t, "claude", `#!/bin/sh
cat <<'JSON'
{"loggedIn":true,"method":"oauth","provider":"claude.ai","subscriptionType":"pro"}
JSON
`)
	claudeHome := t.TempDir()
	writeHTTPClaudeCredentials(t, claudeHome, `{"claudeAiOauth":{"accessToken":"claude-token"}}`)
	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer claude-token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":22,"resets_at":"2030-01-01T00:00:00Z"},"seven_day":{"utilization":8,"resets_at":"2030-01-08T00:00:00Z"}}`))
	}))
	t.Cleanup(usageServer.Close)

	env := newTestEnvWithProviderLimits(t, app.ProviderLimitOptions{
		CodexCommand:   codexScript,
		ClaudeCommand:  claudeScript,
		ClaudeUsageURL: usageServer.URL,
		ProbeTimeout:   time.Second,
	})
	bootstrap := setupHTTP(t, env.server.URL)

	var codexAgent domain.Agent
	postJSON(t, env.server.URL+"/api/organizations/"+bootstrap.Organization.ID+"/agents", bootstrap.SessionToken, map[string]any{
		"name":   "Codex Probe",
		"handle": "codex-probe",
		"kind":   domain.AgentKindCodex,
	}, http.StatusOK, &codexAgent)

	var codexLimits app.AgentProviderLimits
	getJSON(t, env.server.URL+"/api/agents/"+codexAgent.ID+"/limits", bootstrap.SessionToken, http.StatusOK, &codexLimits)
	if codexLimits.Status != app.ProviderLimitStatusOK || len(codexLimits.Windows) != 2 {
		t.Fatalf("codex limits = %#v", codexLimits)
	}
	if codexLimits.Windows[0].Kind != "five_hour" || codexLimits.Windows[1].Kind != "seven_day" {
		t.Fatalf("codex windows = %#v", codexLimits.Windows)
	}

	var claudeAgent domain.Agent
	postJSON(t, env.server.URL+"/api/organizations/"+bootstrap.Organization.ID+"/agents", bootstrap.SessionToken, map[string]any{
		"name":   "Claude Probe",
		"handle": "claude-probe",
		"kind":   domain.AgentKindClaude,
		"env":    map[string]string{"HOME": claudeHome},
	}, http.StatusOK, &claudeAgent)

	var claudeLimits app.AgentProviderLimits
	getJSON(t, env.server.URL+"/api/agents/"+claudeAgent.ID+"/limits", bootstrap.SessionToken, http.StatusOK, &claudeLimits)
	if claudeLimits.Status != app.ProviderLimitStatusOK || !claudeLimits.Auth.LoggedIn || claudeLimits.Auth.Plan != "pro" {
		t.Fatalf("claude limits = %#v", claudeLimits)
	}
	if len(claudeLimits.Windows) != 2 || claudeLimits.Windows[0].Kind != "five_hour" || claudeLimits.Windows[1].Kind != "seven_day" {
		t.Fatalf("claude windows = %#v", claudeLimits.Windows)
	}
}

func newTestEnvWithProviderLimits(t *testing.T, providerLimits app.ProviderLimitOptions) testEnv {
	t.Helper()

	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	})

	bus := eventbus.New()
	a := app.New(st, bus, app.Options{
		AdminToken:     "secret",
		DataDir:        t.TempDir(),
		ProviderLimits: providerLimits,
	})
	ts := httptest.NewServer(NewRouter(a, bus))
	t.Cleanup(ts.Close)
	return testEnv{server: ts, store: st, app: a, bus: bus}
}

func writeHTTPExecutable(t *testing.T, name string, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeHTTPClaudeCredentials(t *testing.T, home string, body string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
