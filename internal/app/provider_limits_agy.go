package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

func (s *providerLimitService) fetchAgy(ctx context.Context, agent domain.Agent, fetchedAt time.Time) AgentProviderLimits {
	result := s.baseResult(agent, domain.AgentKindAgy, fetchedAt)

	payload, err := s.readAgyUsage(ctx, agent.Env)
	if err != nil {
		if errors.Is(err, errAgyNotLoggedIn) {
			result.Auth = ProviderLimitAuth{
				LoggedIn: false,
				Provider: "google",
			}
			result.Status = ProviderLimitStatusUnavailable
			result.Message = "Antigravity is not logged in; numeric usage limits are unavailable."
			return result
		}
		result.Status = ProviderLimitStatusError
		result.Message = providerLimitErrorMessage("Antigravity usage probe", err)
		return result
	}

	result.Auth = agyAuthFromUsagePayload(payload, agent.Env)
	if !result.Auth.LoggedIn {
		result.Status = ProviderLimitStatusUnavailable
		result.Message = "Antigravity is not logged in; numeric usage limits are unavailable."
		return result
	}

	result.Windows = agyWindowsFromUsagePayload(payload, fetchedAt)
	if len(result.Windows) == 0 {
		result.Status = ProviderLimitStatusUnavailable
		result.Message = "Antigravity did not return numeric usage limit windows."
		return result
	}

	result.Status = ProviderLimitStatusOK
	return result
}

func (s *providerLimitService) readAgyUsage(ctx context.Context, env map[string]string) (map[string]any, error) {
	probeCtx, cancel := context.WithTimeout(ctx, s.probeTimeout)
	defer cancel()

	cmd := providerProbeCommand(
		probeCtx,
		s.agyCommand,
		"--print", "/usage",
		"--output-format", "json",
		"--settings", `{"disableSlashCommands":false}`,
	)
	cmd.Env = providerProbeEnv(env)
	if tmpDir, err := os.MkdirTemp("", "agentx-agy-probe-*"); err == nil {
		defer os.RemoveAll(tmpDir)
		cmd.Dir = tmpDir
	}

	var stdout bytes.Buffer
	stderr := &agyAuthDetectingBuffer{cancel: cancel}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	if stderr.isNotLoggedIn() {
		return nil, errAgyNotLoggedIn
	}

	var payload map[string]any
	decodeErr := decodeAgyJSON(stdout.Bytes(), &payload)
	if decodeErr == nil && payload != nil {
		if err := agyPayloadError(payload, stderr.String()); err != nil {
			return nil, err
		}
		if runErr == nil {
			return payload, nil
		}
	}

	if runErr != nil {
		if isAgyAuthRequiredText(stderr.String()) || isAgyAuthRequiredText(stdout.String()) {
			return nil, errAgyNotLoggedIn
		}
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return nil, errProviderLimitTimeout
		}
		return nil, providerProbeError("agy usage", runErr, stderr.String())
	}

	if isAgyAuthRequiredText(stdout.String()) {
		return nil, errAgyNotLoggedIn
	}
	return nil, fmt.Errorf("agy usage returned malformed JSON: %w", decodeErr)
}

func decodeAgyJSON(raw []byte, out *map[string]any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return fmt.Errorf("empty output")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func agyPayloadError(payload map[string]any, stderr string) error {
	status := strings.ToUpper(strings.TrimSpace(stringField(payload, "status")))
	errText := strings.TrimSpace(stringField(payload, "error"))
	if errText == "" {
		if errObj := asMap(payload["error"]); errObj != nil {
			errText = strings.TrimSpace(stringField(errObj, "message", "error"))
		}
	}
	if status != "ERROR" && errText == "" {
		return nil
	}
	if isAgyAuthRequiredText(errText) || isAgyAuthRequiredText(stderr) {
		return errAgyNotLoggedIn
	}
	if errText == "" {
		errText = "command returned ERROR status"
	}
	return fmt.Errorf("agy usage failed: %s", redactProviderLimitMessage(errText))
}

// agyAuthDetectingBuffer watches stderr from `agy --print /usage` so an
// unauthenticated CLI that prompts for interactive OAuth login is cancelled
// immediately instead of blocking until the probe timeout.
type agyAuthDetectingBuffer struct {
	mu          sync.Mutex
	buf         bytes.Buffer
	notLoggedIn bool
	cancel      context.CancelFunc
}

func (b *agyAuthDetectingBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	if b.buf.Len() < providerProbeScannerBufferMax {
		remaining := providerProbeScannerBufferMax - b.buf.Len()
		if len(p) <= remaining {
			_, _ = b.buf.Write(p)
		} else {
			_, _ = b.buf.Write(p[:remaining])
		}
	}
	var cancel context.CancelFunc
	if !b.notLoggedIn && isAgyAuthRequiredText(b.buf.String()) {
		b.notLoggedIn = true
		cancel = b.cancel
	}
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return len(p), nil
}

func (b *agyAuthDetectingBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *agyAuthDetectingBuffer) isNotLoggedIn() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.notLoggedIn
}

func isAgyAuthRequiredText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"authentication required",
		"please visit the url to log in",
		"you are not logged into antigravity",
		"not logged in",
		"authentication failed",
		"authentication interrupted",
		"unauthenticated",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func agyAuthFromUsagePayload(payload map[string]any, env map[string]string) ProviderLimitAuth {
	maps := candidateAuthMaps(payload)
	if cmd := asMap(payload["command"]); cmd != nil {
		if data := asMap(cmd["data"]); data != nil {
			maps = append(maps, candidateAuthMaps(data)...)
		}
	}

	method := sanitizedAuthField(stringFieldFromMaps(maps, "method", "authMethod", "auth_method", "authMode", "auth_mode", "loginMethod", "login_method"))
	if method == "" {
		if adcRaw := strings.TrimSpace(providerProbeEnvValue(env, "AGY_ADC_AUTH")); adcRaw != "" {
			if adc, ok := boolField(map[string]any{"adc": adcRaw}, "adc"); ok && adc {
				method = "adc"
			}
		}
	}
	if method == "" {
		method = "oauth"
	}

	provider := sanitizedAuthField(stringFieldFromMaps(maps, "provider", "authProvider", "auth_provider"))
	if provider == "" {
		provider = "google"
	}

	plan := sanitizedAuthField(stringFieldFromMaps(
		maps,
		"tierDisplayName",
		"tier_display_name",
		"userTier",
		"user_tier",
		"planType",
		"plan_type",
		"plan",
		"subscriptionType",
		"subscription_type",
		"tier",
	))

	loggedIn := true
	if v, ok := boolFieldFromMaps(maps, "loggedIn", "logged_in", "authenticated", "isAuthenticated", "is_authenticated"); ok {
		loggedIn = v
	}

	return ProviderLimitAuth{
		LoggedIn: loggedIn,
		Method:   method,
		Provider: provider,
		Plan:     plan,
	}
}

func agyWindowsFromUsagePayload(payload map[string]any, now time.Time) []ProviderLimitWindow {
	if payload == nil {
		return nil
	}
	tsvRemaining := parseAgyResponseTSVRemaining(stringField(payload, "response"))
	data := agyUsageDataContainer(payload)

	var windows []ProviderLimitWindow
	seenKinds := make(map[string]struct{})
	appendUnique := func(w ProviderLimitWindow, disambiguator string) {
		kind := w.Kind
		if _, dup := seenKinds[kind]; dup && disambiguator != "" {
			kind = kind + "_" + disambiguator
		}
		if _, dup := seenKinds[kind]; dup {
			return
		}
		w.Kind = kind
		seenKinds[kind] = struct{}{}
		windows = append(windows, w)
	}

	if rawGroups, ok := data["groups"].([]any); ok && len(rawGroups) > 0 {
		for _, rawGroup := range rawGroups {
			group := asMap(rawGroup)
			if group == nil {
				continue
			}
			groupName := sanitizedAuthField(stringField(group, "name", "display_name", "displayName"))
			rawBuckets, _ := group["buckets"].([]any)
			parsed := parseAgyBucketSlice(rawBuckets, groupName, tsvRemaining, now)
			groupSlug := claudeScopedModelSlug(groupName)
			for _, item := range parsed {
				w := item.window
				if groupName != "" && groupSlug != "" {
					w.Kind += "_" + groupSlug
					w.Label += " · " + groupName
				}
				appendUnique(w, item.idSlug)
			}
		}
		return windows
	}

	if rawBuckets, ok := data["buckets"].([]any); ok && len(rawBuckets) > 0 {
		for _, item := range parseAgyBucketSlice(rawBuckets, "", tsvRemaining, now) {
			appendUnique(item.window, item.idSlug)
		}
	}
	return windows
}

type parsedAgyBucket struct {
	window ProviderLimitWindow
	idSlug string
}

func parseAgyBucketSlice(rawBuckets []any, groupName string, tsvRemaining map[string]float64, now time.Time) []parsedAgyBucket {
	if len(rawBuckets) == 0 {
		return nil
	}
	out := make([]parsedAgyBucket, 0, len(rawBuckets))
	for _, rawBucket := range rawBuckets {
		bucket := asMap(rawBucket)
		if bucket == nil {
			continue
		}
		w, idSlug, ok := agyWindowFromBucket(bucket, groupName, tsvRemaining, now)
		if ok {
			out = append(out, parsedAgyBucket{window: w, idSlug: idSlug})
		}
	}
	// Order shorter windows (5-hour) ahead of longer windows (Weekly) within a
	// group to match the Codex and Claude window ordering.
	sort.SliceStable(out, func(i, j int) bool {
		mi, mj := out[i].window.WindowMinutes, out[j].window.WindowMinutes
		if mi > 0 && mj > 0 && mi != mj {
			return mi < mj
		}
		if mi > 0 && mj <= 0 {
			return true
		}
		return false
	})
	return out
}

func agyWindowFromBucket(bucket map[string]any, groupName string, tsvRemaining map[string]float64, now time.Time) (ProviderLimitWindow, string, bool) {
	if disabled, ok := boolField(bucket, "disabled"); ok && disabled {
		return ProviderLimitWindow{}, "", false
	}

	bucketID := sanitizedAuthField(stringField(bucket, "id", "bucket_id", "bucketId"))
	bucketName := sanitizedAuthField(stringField(bucket, "name", "display_name", "displayName"))
	idSlug := claudeScopedModelSlug(bucketID)
	nameSlug := claudeScopedModelSlug(bucketName)

	windowMinutes := agyWindowMinutes(bucket)
	fallback := idSlug
	if fallback == "" {
		fallback = nameSlug
	}
	if fallback == "" {
		fallback = "window"
	}
	kind, label := providerLimitWindowKind(fallback, windowMinutes)
	if windowMinutes <= 0 && bucketName != "" {
		label = bucketName
	}

	used, hasUsed := agyUsedPercentFromBucket(bucket, groupName, bucketName, tsvRemaining)
	reset, hasReset := agyResetTimeFromBucket(bucket, now)
	if !hasUsed && !hasReset {
		return ProviderLimitWindow{}, "", false
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
	}, idSlug, true
}

func agyUsedPercentFromBucket(bucket map[string]any, groupName string, bucketName string, tsvRemaining map[string]float64) (float64, bool) {
	if rem, ok := floatField(bucket, "remaining_fraction", "remainingFraction"); ok {
		return roundAndClampPercent((1.0 - rem) * 100.0), true
	}
	if used, ok := floatField(bucket, "used_percentage", "usedPercent", "used_percent", "utilization", "percent"); ok {
		return roundAndClampPercent(used), true
	}
	if remPct, ok := floatField(bucket, "remaining_percentage", "remainingPercent", "remaining_percent"); ok {
		return roundAndClampPercent(100.0 - remPct), true
	}
	// Go's `json:"remaining_fraction,omitempty"` on a non-pointer float omits 0
	// when a bucket is 100% exhausted; recover the formatted remaining percent
	// from `response` TSV (`<group>\t<bucket>\t0%\t<reset>`) when available.
	if len(tsvRemaining) > 0 {
		if remPct, ok := tsvRemaining[agyTSVKey(groupName, bucketName)]; ok {
			return roundAndClampPercent(100.0 - remPct), true
		}
	}
	return 0, false
}

func agyResetTimeFromBucket(bucket map[string]any, now time.Time) (time.Time, bool) {
	if reset, ok := unixTimeField(bucket, "reset_time", "resetTime", "resets_at", "resetsAt"); ok {
		return reset, true
	}
	if sec, ok := floatField(bucket, "reset_in_seconds", "resetInSeconds"); ok && sec >= 0 {
		return now.UTC().Add(time.Duration(sec * float64(time.Second))), true
	}
	return time.Time{}, false
}

func agyWindowMinutes(bucket map[string]any) int {
	if mins := intField(bucket, "windowDurationMins", "window_duration_mins", "windowMinutes", "window_minutes", "minutes_per_bucket"); mins > 0 {
		return mins
	}
	for _, raw := range []string{
		stringField(bucket, "window", "refresh_window_label", "refreshWindowLabel"),
		stringField(bucket, "id", "bucket_id", "bucketId"),
		stringField(bucket, "name", "display_name", "displayName"),
	} {
		if mins := parseAgyWindowMinutesString(raw); mins > 0 {
			return mins
		}
	}
	return 0
}

func parseAgyWindowMinutesString(raw string) int {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0
	}
	switch s {
	case "5h", "five_hour", "five-hour", "session":
		return agyFiveHourWindowMinutes
	case "weekly", "week", "7d", "seven_day", "seven-day":
		return agyWeeklyWindowMinutes
	case "daily", "day", "1d", "24h":
		return 24 * 60
	}

	if mins := parseSimpleDurationMinutes(s); mins > 0 {
		return mins
	}

	switch {
	case strings.Contains(s, "5h"), strings.Contains(s, "five hour"), strings.Contains(s, "5-hour"), strings.Contains(s, "5 hour"):
		return agyFiveHourWindowMinutes
	case strings.Contains(s, "weekly"), strings.Contains(s, "7d"), strings.Contains(s, "seven day"), strings.Contains(s, "7-day"):
		return agyWeeklyWindowMinutes
	case strings.Contains(s, "daily"), strings.Contains(s, "24h"), strings.Contains(s, "24-hour"):
		return 24 * 60
	}
	return 0
}

func parseSimpleDurationMinutes(s string) int {
	switch {
	case strings.HasSuffix(s, "m"):
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "m")); err == nil && n > 0 {
			return n
		}
	case strings.HasSuffix(s, "h"):
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "h")); err == nil && n > 0 {
			return n * 60
		}
	case strings.HasSuffix(s, "d"):
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && n > 0 {
			return n * 24 * 60
		}
	}
	return 0
}

func agyUsageDataContainer(payload map[string]any) map[string]any {
	if cmd := asMap(payload["command"]); cmd != nil {
		if data := asMap(cmd["data"]); data != nil {
			return data
		}
	}
	for _, key := range []string{"data", "quota_summary", "quotaSummary"} {
		if sub := asMap(payload[key]); sub != nil {
			return sub
		}
	}
	return payload
}

func parseAgyResponseTSVRemaining(response string) map[string]float64 {
	if strings.TrimSpace(response) == "" {
		return nil
	}
	out := make(map[string]float64)
	for _, line := range strings.Split(response, "\n") {
		cols := strings.Split(strings.TrimSpace(line), "\t")
		if len(cols) < 3 {
			continue
		}
		group := strings.TrimSpace(cols[0])
		name := strings.TrimSpace(cols[1])
		pctText := strings.TrimSuffix(strings.TrimSpace(cols[2]), "%")
		pct, err := strconv.ParseFloat(strings.TrimSpace(pctText), 64)
		if err != nil {
			continue
		}
		out[agyTSVKey(group, name)] = pct
	}
	return out
}

func agyTSVKey(group string, name string) string {
	return strings.ToLower(strings.TrimSpace(group)) + "\x00" + strings.ToLower(strings.TrimSpace(name))
}

func roundAndClampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	rounded := math.Round(value*10) / 10
	if rounded < 0 {
		return 0
	}
	if rounded > 100 {
		return 100
	}
	return rounded
}
