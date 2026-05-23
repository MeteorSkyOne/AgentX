package codex

import (
	"encoding/json"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

func codexTurnUsage(payload map[string]any) *runtime.Usage {
	value, ok := firstPresent(payload, "usage", "token_usage", "tokenUsage")
	var parents []map[string]any
	parents = append(parents, payload)
	if !ok {
		for _, key := range []string{"turn", "response", "payload", "info"} {
			nested, _ := payload[key].(map[string]any)
			if nested == nil {
				continue
			}
			parents = append(parents, nested)
			value, ok = firstPresent(nested, "usage", "token_usage", "tokenUsage", "last_token_usage", "lastTokenUsage")
			if ok {
				break
			}
		}
	}
	if !ok {
		return nil
	}
	usageMap, _ := value.(map[string]any)
	if usageMap == nil {
		return nil
	}
	usage := codexUsageFromMap(usageMap, value)
	usage.Context = codexContextUsageFromMap(usageMap, parents, "turn.completed")
	return usage
}

func codexTokenCountUsage(payload map[string]any) *runtime.Usage {
	value, ok := firstPresent(payload, "last_token_usage", "lastTokenUsage", "usage", "token_usage", "tokenUsage")
	parents := []map[string]any{payload}
	if !ok {
		info, _ := payload["info"].(map[string]any)
		if info != nil {
			parents = append(parents, info)
		}
		value, ok = firstPresent(info, "last_token_usage", "lastTokenUsage", "usage", "token_usage", "tokenUsage", "total_token_usage", "totalTokenUsage")
		if !ok {
			return nil
		}
	}
	usageMap, _ := value.(map[string]any)
	if usageMap == nil {
		return nil
	}
	usage := codexUsageFromMap(usageMap, value)
	usage.Context = codexContextUsageFromMap(usageMap, parents, "token_count.info")
	return usage
}

func codexUsageFromMap(values map[string]any, raw any) *runtime.Usage {
	usage := &runtime.Usage{
		Model:                    firstTextValue(values, "model", "model_id", "modelId"),
		InputTokens:              int64Field(values, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens", "input", "prompt"),
		CachedInputTokens:        int64Field(values, "cached_input_tokens", "cachedInputTokens", "cached_tokens", "cachedTokens"),
		CacheCreationInputTokens: int64Field(values, "cache_creation_input_tokens", "cacheCreationInputTokens"),
		CacheReadInputTokens:     int64Field(values, "cache_read_input_tokens", "cacheReadInputTokens"),
		OutputTokens:             int64Field(values, "output_tokens", "outputTokens", "completion_tokens", "completionTokens", "output", "completion"),
		ReasoningOutputTokens:    int64Field(values, "reasoning_output_tokens", "reasoningOutputTokens", "reasoning_tokens", "reasoningTokens"),
		TotalTokens:              int64Field(values, "total_tokens", "totalTokens", "total"),
		TotalCostUSD:             float64Field(values, "total_cost_usd", "totalCostUsd", "cost_usd", "costUsd"),
		Raw:                      raw,
	}
	if usage.CachedInputTokens == nil && usage.CacheReadInputTokens != nil {
		cached := *usage.CacheReadInputTokens
		usage.CachedInputTokens = &cached
	}
	if usage.OutputTokens == nil && usage.TotalTokens != nil && usage.InputTokens != nil {
		output := *usage.TotalTokens - *usage.InputTokens
		if output >= 0 {
			usage.OutputTokens = &output
		}
	}
	if usage.TotalTokens == nil && usage.InputTokens != nil && usage.OutputTokens != nil {
		total := *usage.InputTokens + *usage.OutputTokens
		usage.TotalTokens = &total
	}
	return usage
}

func codexContextUsageFromMap(values map[string]any, parents []map[string]any, source string) *runtime.ContextUsage {
	totalTokens := int64Field(values, "total_tokens", "totalTokens", "total")
	windowTokens := int64Field(values, "model_context_window", "modelContextWindow", "context_window_tokens", "contextWindowTokens", "context_window", "contextWindow")
	usedPercent := float64Field(values, "used_percent", "usedPercent", "context_used_percent", "contextUsedPercent")
	model := firstTextValue(values, "model", "model_id", "modelId")
	for _, parent := range parents {
		if totalTokens == nil {
			totalTokens = int64Field(parent, "total_tokens", "totalTokens", "total")
		}
		if windowTokens == nil {
			windowTokens = int64Field(parent, "model_context_window", "modelContextWindow", "context_window_tokens", "contextWindowTokens", "context_window", "contextWindow")
		}
		if usedPercent == nil {
			usedPercent = float64Field(parent, "used_percent", "usedPercent", "context_used_percent", "contextUsedPercent")
		}
		if model == "" {
			model = firstTextValue(parent, "model", "model_id", "modelId")
		}
	}
	if usedPercent == nil && totalTokens != nil && windowTokens != nil && *windowTokens > 0 {
		percent := (float64(*totalTokens) / float64(*windowTokens)) * 100
		usedPercent = &percent
	}
	if totalTokens == nil && windowTokens == nil && usedPercent == nil && model == "" {
		return nil
	}
	return &runtime.ContextUsage{
		TotalTokens:           totalTokens,
		InputTokens:           int64Field(values, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens", "input", "prompt"),
		CachedInputTokens:     int64Field(values, "cached_input_tokens", "cachedInputTokens", "cached_tokens", "cachedTokens", "cache_read_input_tokens", "cacheReadInputTokens"),
		OutputTokens:          int64Field(values, "output_tokens", "outputTokens", "completion_tokens", "completionTokens", "output", "completion"),
		ReasoningOutputTokens: int64Field(values, "reasoning_output_tokens", "reasoningOutputTokens", "reasoning_tokens", "reasoningTokens"),
		ContextWindowTokens:   windowTokens,
		UsedPercent:           usedPercent,
		Model:                 model,
		Source:                source,
	}
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
