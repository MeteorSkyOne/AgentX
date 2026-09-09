package claude

import (
	"encoding/json"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

func claudeUsage(payload map[string]any) *runtime.Usage {
	usageValue, _ := firstPresent(payload, "usage", "modelUsage", "model_usage")
	usageMap := usageMapValue(usageValue)
	if usageMap == nil && !hasClaudeUsagePayload(payload) {
		return nil
	}
	usage := &runtime.Usage{
		Model:         firstTextValue(payload, "model", "model_id"),
		InputTokens:   int64Field(usageMap, "input_tokens", "inputTokens"),
		OutputTokens:  int64Field(usageMap, "output_tokens", "outputTokens"),
		TotalTokens:   int64Field(usageMap, "total_tokens", "totalTokens"),
		TotalCostUSD:  float64Field(payload, "total_cost_usd", "totalCostUsd", "cost_usd", "costUsd"),
		DurationMS:    int64Field(payload, "duration_ms", "durationMs"),
		DurationAPIMS: int64Field(payload, "duration_api_ms", "durationApiMs"),
		Raw:           usageValue,
	}
	usage.CachedInputTokens = int64Field(usageMap, "cached_input_tokens", "cachedInputTokens")
	usage.CacheCreationInputTokens = int64Field(usageMap, "cache_creation_input_tokens", "cacheCreationInputTokens")
	usage.CacheReadInputTokens = int64Field(usageMap, "cache_read_input_tokens", "cacheReadInputTokens")
	if usage.Raw == nil && hasAnyUsageFields(payload) {
		usage.Raw = payload
	}
	if usage.TotalTokens == nil && usage.InputTokens != nil && usage.OutputTokens != nil {
		total := *usage.InputTokens + *usage.OutputTokens
		usage.TotalTokens = &total
	}
	return usage
}

// claudeContextUsage reads the context-window fill from an assistant event.
// A structured context_usage block wins when present; otherwise the fill is
// derived from the API call's own usage the way Claude Code's status line does:
// everything the model was sent (fresh input plus cache writes and reads) is
// what currently occupies the window.
func claudeContextUsage(payload map[string]any) *runtime.ContextUsage {
	value, _ := firstPresent(payload, "context_usage", "contextUsage")
	values, _ := value.(map[string]any)
	message, _ := payload["message"].(map[string]any)
	if values == nil {
		value, _ = firstPresent(message, "context_usage", "contextUsage")
		values, _ = value.(map[string]any)
	}
	if values == nil {
		return claudeMessageContextUsage(message)
	}

	usage := &runtime.ContextUsage{
		TotalTokens:         int64Field(values, "total_tokens", "totalTokens"),
		ContextWindowTokens: int64Field(values, "raw_max_tokens", "rawMaxTokens", "max_tokens", "maxTokens"),
		UsedPercent:         float64Field(values, "percentage", "used_percent", "usedPercent"),
		Model:               firstTextValue(values, "model", "model_id", "modelId"),
		Source:              "claude_context_usage",
	}
	if usage.TotalTokens == nil && usage.ContextWindowTokens == nil && usage.UsedPercent == nil && usage.Model == "" {
		return nil
	}
	return usage
}

func claudeMessageContextUsage(message map[string]any) *runtime.ContextUsage {
	usageMap, _ := message["usage"].(map[string]any)
	if usageMap == nil {
		return nil
	}
	input := int64Field(usageMap, "input_tokens", "inputTokens")
	cacheCreation := int64Field(usageMap, "cache_creation_input_tokens", "cacheCreationInputTokens")
	cacheRead := int64Field(usageMap, "cache_read_input_tokens", "cacheReadInputTokens")
	if input == nil && cacheCreation == nil && cacheRead == nil {
		return nil
	}
	total := ptrInt64(input) + ptrInt64(cacheCreation) + ptrInt64(cacheRead)
	return &runtime.ContextUsage{
		TotalTokens:       &total,
		InputTokens:       input,
		CachedInputTokens: cacheRead,
		OutputTokens:      int64Field(usageMap, "output_tokens", "outputTokens"),
		Model:             firstTextValue(message, "model"),
		Source:            "claude_message_usage",
	}
}

// CompleteContextUsage finishes a turn's last context snapshot with the model's
// window size, which Claude Code only reports on the result event under
// modelUsage. The percentage is derived once both sides are known.
func CompleteContextUsage(last *runtime.ContextUsage, result map[string]any) *runtime.ContextUsage {
	if last == nil {
		return nil
	}
	usage := *last
	if usage.ContextWindowTokens == nil {
		usage.ContextWindowTokens = claudeResultContextWindow(result, usage.Model)
	}
	if usage.UsedPercent == nil && usage.TotalTokens != nil && usage.ContextWindowTokens != nil && *usage.ContextWindowTokens > 0 {
		percent := float64(*usage.TotalTokens) / float64(*usage.ContextWindowTokens) * 100
		usage.UsedPercent = &percent
	}
	return &usage
}

func claudeResultContextWindow(result map[string]any, model string) *int64 {
	value, _ := firstPresent(result, "modelUsage", "model_usage")
	models, _ := value.(map[string]any)
	if len(models) == 0 {
		return nil
	}
	if entry, ok := models[model].(map[string]any); ok {
		if window := int64Field(entry, "contextWindow", "context_window"); window != nil {
			return window
		}
	}
	// Without a per-model match, only an unambiguous single entry is trusted.
	if len(models) == 1 {
		for _, raw := range models {
			if entry, ok := raw.(map[string]any); ok {
				return int64Field(entry, "contextWindow", "context_window")
			}
		}
	}
	return nil
}

func ptrInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func hasClaudeUsagePayload(values map[string]any) bool {
	if stringValue(values, "model") != "" || stringValue(values, "model_id") != "" {
		return true
	}
	if hasAnyUsageFields(values) {
		return true
	}
	for _, key := range []string{
		"duration_ms", "durationMs", "duration_api_ms", "durationApiMs",
		"total_cost_usd", "totalCostUsd", "cost_usd", "costUsd",
	} {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func usageMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if hasAnyUsageFields(typed) {
			return typed
		}
		merged := map[string]any{}
		for _, nestedValue := range typed {
			nested, ok := nestedValue.(map[string]any)
			if !ok || !hasAnyUsageFields(nested) {
				continue
			}
			sumUsageFields(merged, nested)
		}
		if len(merged) > 0 {
			return merged
		}
		return typed
	default:
		return nil
	}
}

func sumUsageFields(dst map[string]any, src map[string]any) {
	for _, key := range []string{
		"input_tokens", "output_tokens", "total_tokens", "cached_input_tokens",
		"cache_creation_input_tokens", "cache_read_input_tokens",
	} {
		value := int64Field(src, key)
		if value == nil {
			continue
		}
		current := int64(0)
		if existing := int64Field(dst, key); existing != nil {
			current = *existing
		}
		dst[key] = current + *value
	}
}

func hasAnyUsageFields(values map[string]any) bool {
	if values == nil {
		return false
	}
	for _, key := range []string{
		"input_tokens", "inputTokens", "output_tokens", "outputTokens", "total_tokens", "totalTokens",
		"cached_input_tokens", "cachedInputTokens", "cache_creation_input_tokens", "cacheCreationInputTokens",
		"cache_read_input_tokens", "cacheReadInputTokens",
	} {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func firstTextValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := stringValue(values, key); text != "" {
			return text
		}
	}
	return ""
}

func int64Field(values map[string]any, keys ...string) *int64 {
	if values == nil {
		return nil
	}
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		if parsed, ok := numberInt64(value); ok {
			return &parsed
		}
	}
	return nil
}

func float64Field(values map[string]any, keys ...string) *float64 {
	if values == nil {
		return nil
	}
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		if parsed, ok := numberFloat64(value); ok {
			return &parsed
		}
	}
	return nil
}

func numberInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
		asFloat, err := typed.Float64()
		if err == nil {
			return int64(asFloat), true
		}
	case string:
		var parsed json.Number = json.Number(strings.TrimSpace(typed))
		value, err := parsed.Int64()
		if err == nil {
			return value, true
		}
	}
	return 0, false
}

func numberFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		var parsed json.Number = json.Number(strings.TrimSpace(typed))
		value, err := parsed.Float64()
		return value, err == nil
	}
	return 0, false
}
