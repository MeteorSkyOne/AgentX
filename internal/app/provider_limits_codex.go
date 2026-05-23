package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

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
	limits := codexRateLimitMap(result)
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

func codexRateLimitMap(result map[string]any) map[string]any {
	for _, key := range []string{"rateLimitsByLimitId", "rate_limits_by_limit_id"} {
		byID := asMap(result[key])
		if byID == nil {
			continue
		}
		if codex := asMap(byID["codex"]); codex != nil {
			return codex
		}
	}
	if limits := asMap(result["rateLimits"]); limits != nil {
		return limits
	}
	if limits := asMap(result["rate_limits"]); limits != nil {
		return limits
	}
	return result
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
