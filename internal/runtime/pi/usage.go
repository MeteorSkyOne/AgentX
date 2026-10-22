package pi

import (
	"github.com/meteorsky/agentx/internal/runtime"
)

// usageTotals accumulates the usage of every assistant message produced during
// one AgentX turn. pi reports usage per assistant message (one per model call),
// so the turn total is the sum.
type usageTotals struct {
	model      string
	input      int64
	output     int64
	cacheRead  int64
	cacheWrite int64
	reasoning  int64
	total      int64
	cost       float64
	hasCost    bool
	seen       bool
}

func (u *usageTotals) add(message record) {
	usage := mapVal(message, "usage")
	if usage == nil {
		return
	}
	u.seen = true
	if model := stringVal(message, "model"); model != "" {
		u.model = model
	}
	u.input += int64Val(usage, "input")
	u.output += int64Val(usage, "output")
	u.cacheRead += int64Val(usage, "cacheRead")
	u.cacheWrite += int64Val(usage, "cacheWrite")
	u.reasoning += int64Val(usage, "reasoning")
	if total := int64Ptr(usage, "totalTokens"); total != nil {
		u.total += *total
	} else {
		u.total += int64Val(usage, "input") + int64Val(usage, "output") + int64Val(usage, "cacheRead") + int64Val(usage, "cacheWrite")
	}
	if cost := mapVal(usage, "cost"); cost != nil {
		if total := float64Ptr(cost, "total"); total != nil {
			u.cost += *total
			u.hasCost = true
		}
	}
}

func (u *usageTotals) toRuntime(durationMS int64, contextUsage *runtime.ContextUsage) *runtime.Usage {
	if !u.seen && contextUsage == nil {
		return nil
	}
	out := &runtime.Usage{Model: u.model, Context: contextUsage}
	if u.seen {
		out.InputTokens = ptrInt64(u.input)
		out.OutputTokens = ptrInt64(u.output)
		out.CachedInputTokens = ptrInt64(u.cacheRead)
		out.CacheReadInputTokens = ptrInt64(u.cacheRead)
		out.CacheCreationInputTokens = ptrInt64(u.cacheWrite)
		out.TotalTokens = ptrInt64(u.total)
		if u.reasoning > 0 {
			out.ReasoningOutputTokens = ptrInt64(u.reasoning)
		}
		if u.hasCost {
			cost := u.cost
			out.TotalCostUSD = &cost
		}
	}
	if durationMS > 0 {
		out.DurationMS = ptrInt64(durationMS)
	}
	if contextUsage != nil && out.Model == "" {
		out.Model = contextUsage.Model
	}
	return out
}

// contextUsageFromStats converts a get_session_stats response into the
// context-window snapshot AgentX shows in /status.
func contextUsageFromStats(stats record, model string) *runtime.ContextUsage {
	ctxUsage := mapVal(stats, "contextUsage")
	if ctxUsage == nil {
		return nil
	}
	out := &runtime.ContextUsage{
		TotalTokens:         int64Ptr(ctxUsage, "tokens"),
		ContextWindowTokens: int64Ptr(ctxUsage, "contextWindow"),
		UsedPercent:         float64Ptr(ctxUsage, "percent"),
		Model:               model,
		Source:              "get_session_stats",
	}
	if out.UsedPercent == nil && out.TotalTokens != nil && out.ContextWindowTokens != nil && *out.ContextWindowTokens > 0 {
		percent := float64(*out.TotalTokens) / float64(*out.ContextWindowTokens) * 100
		out.UsedPercent = &percent
	}
	return out
}

func int64Val(m record, key string) int64 {
	if v := int64Ptr(m, key); v != nil {
		return *v
	}
	return 0
}

func ptrInt64(v int64) *int64 {
	return &v
}
