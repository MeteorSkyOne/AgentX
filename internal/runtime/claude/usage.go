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
