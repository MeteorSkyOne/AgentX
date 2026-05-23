package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func providerLimitWindowKind(fallback string, windowMinutes int) (string, string) {
	switch windowMinutes {
	case codexFiveHourWindowMinutes:
		return "five_hour", "5-hour"
	case codexWeeklyWindowMinutes:
		return "seven_day", "Weekly"
	default:
		if windowMinutes > 0 {
			return fallback, providerLimitDurationLabel(windowMinutes)
		}
		return fallback, providerLimitFallbackLabel(fallback)
	}
}

func providerLimitFallbackLabel(fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "Window"
	}
	return strings.ToUpper(fallback[:1]) + fallback[1:]
}

func providerLimitDurationLabel(windowMinutes int) string {
	switch {
	case windowMinutes%(24*60) == 0:
		days := windowMinutes / (24 * 60)
		if days == 1 {
			return "1-day"
		}
		return fmt.Sprintf("%d-day", days)
	case windowMinutes%60 == 0:
		hours := windowMinutes / 60
		if hours == 1 {
			return "1-hour"
		}
		return fmt.Sprintf("%d-hour", hours)
	case windowMinutes == 1:
		return "1-minute"
	default:
		return fmt.Sprintf("%d-minute", windowMinutes)
	}
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
	configureProviderProbeCommand(cmd)
	return cmd
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
