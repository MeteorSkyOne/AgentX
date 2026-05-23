package codexpersist

import (
	"encoding/json"

	"github.com/meteorsky/agentx/internal/runtime"
)

func turnIDFromResult(result map[string]any) string {
	turn, _ := result["turn"].(map[string]any)
	return stringVal(turn, "id")
}

func turnStatus(params map[string]any) string {
	turn, _ := params["turn"].(map[string]any)
	if status := stringVal(turn, "status"); status != "" {
		return status
	}
	return stringVal(params, "status")
}

func turnCompletedUsage(params map[string]any) *runtime.Usage {
	turn, _ := params["turn"].(map[string]any)
	if turn == nil {
		return nil
	}
	usage, _ := turn["tokenUsage"].(map[string]any)
	if usage == nil {
		return nil
	}
	last, _ := usage["last"].(map[string]any)
	if last == nil {
		return nil
	}
	u := &runtime.Usage{
		Model:                 stringVal(turn, "model"),
		InputTokens:           int64Ptr(last, "inputTokens"),
		OutputTokens:          int64Ptr(last, "outputTokens"),
		CachedInputTokens:     int64Ptr(last, "cachedInputTokens"),
		ReasoningOutputTokens: int64Ptr(last, "reasoningOutputTokens"),
		TotalTokens:           int64Ptr(last, "totalTokens"),
	}
	u.Context = contextUsageFromTokenUsage(usage, stringVal(turn, "model"), "turn.completed")
	return u
}

func threadTokenUsageUpdatedUsage(params map[string]any, model string) *runtime.Usage {
	usage, _ := params["tokenUsage"].(map[string]any)
	if usage == nil {
		usage, _ = params["token_usage"].(map[string]any)
	}
	if usage == nil {
		return nil
	}
	contextUsage := contextUsageFromTokenUsage(usage, model, "thread/tokenUsage/updated")
	if contextUsage == nil {
		return nil
	}
	total := tokenUsageTotalMap(usage)
	if total == nil {
		total, _ = usage["last"].(map[string]any)
	}
	return &runtime.Usage{
		Model:                 contextUsage.Model,
		InputTokens:           int64Ptr(total, "inputTokens"),
		OutputTokens:          int64Ptr(total, "outputTokens"),
		CachedInputTokens:     int64Ptr(total, "cachedInputTokens"),
		ReasoningOutputTokens: int64Ptr(total, "reasoningOutputTokens"),
		TotalTokens:           int64Ptr(total, "totalTokens"),
		Context:               contextUsage,
	}
}

func contextUsageFromTokenUsage(usage map[string]any, model string, source string) *runtime.ContextUsage {
	total := tokenUsageTotalMap(usage)
	if total == nil {
		total, _ = usage["last"].(map[string]any)
	}
	if total == nil {
		return nil
	}
	totalTokens := int64Ptr(total, "totalTokens")
	windowTokens := int64Ptr(usage, "modelContextWindow")
	if windowTokens == nil {
		windowTokens = int64Ptr(total, "modelContextWindow")
	}
	usedPercent := float64Ptr(usage, "usedPercent")
	if usedPercent == nil {
		usedPercent = float64Ptr(total, "usedPercent")
	}
	if usedPercent == nil && totalTokens != nil && windowTokens != nil && *windowTokens > 0 {
		percent := (float64(*totalTokens) / float64(*windowTokens)) * 100
		usedPercent = &percent
	}
	if model == "" {
		model = stringVal(usage, "model")
	}
	return &runtime.ContextUsage{
		TotalTokens:           totalTokens,
		InputTokens:           int64Ptr(total, "inputTokens"),
		CachedInputTokens:     int64Ptr(total, "cachedInputTokens"),
		OutputTokens:          int64Ptr(total, "outputTokens"),
		ReasoningOutputTokens: int64Ptr(total, "reasoningOutputTokens"),
		ContextWindowTokens:   windowTokens,
		UsedPercent:           usedPercent,
		Model:                 model,
		Source:                source,
	}
}

func tokenUsageTotalMap(usage map[string]any) map[string]any {
	total, _ := usage["total"].(map[string]any)
	if total != nil {
		return total
	}
	total, _ = usage["totalTokenUsage"].(map[string]any)
	if total != nil {
		return total
	}
	total, _ = usage["total_token_usage"].(map[string]any)
	return total
}

func int64Ptr(m map[string]any, key string) *int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		i := int64(n)
		return &i
	case int64:
		return &n
	case float64:
		i := int64(n)
		return &i
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return nil
		}
		return &i
	}
	return nil
}

func float64Ptr(m map[string]any, key string) *float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	case float64:
		return &n
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return nil
		}
		return &f
	}
	return nil
}
