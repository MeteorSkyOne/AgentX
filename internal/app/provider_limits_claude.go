package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

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
