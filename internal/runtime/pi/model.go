package pi

import (
	"strings"
)

// thinkingLevels are pi's reasoning levels, lowest to highest.
var thinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

func isThinkingLevel(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, level := range thinkingLevels {
		if level == value {
			return true
		}
	}
	return false
}

// splitModelThinking separates a pi model pattern from its optional
// `:<thinking>` suffix (`sonnet:high` -> `sonnet`, `high`).
func splitModelThinking(model string) (string, string) {
	model = strings.TrimSpace(model)
	if idx := strings.LastIndex(model, ":"); idx > 0 {
		suffix := model[idx+1:]
		if isThinkingLevel(suffix) {
			return strings.TrimSpace(model[:idx]), strings.ToLower(suffix)
		}
	}
	return model, ""
}

// thinkingLevelForEffort maps an AgentX effort value onto a pi thinking level.
// AgentX efforts (low/medium/high/xhigh) are pi levels already; pi's extra
// levels (off/minimal/max) are accepted too. The model suffix is the fallback.
func thinkingLevelForEffort(effort string, modelThinking string) string {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if isThinkingLevel(effort) {
		return effort
	}
	switch effort {
	case "none":
		return "off"
	case "min":
		return "minimal"
	case "mid":
		return "medium"
	case "extra_high", "extra-high", "very_high", "very-high":
		return "xhigh"
	}
	return modelThinking
}

// clampThinkingLevel picks the closest level pi reports as available for the
// current model. Unknown or empty levels are returned unchanged.
func clampThinkingLevel(level string, available []string) string {
	if level == "" || len(available) == 0 {
		return level
	}
	rank := func(value string) int {
		for i, candidate := range thinkingLevels {
			if candidate == value {
				return i
			}
		}
		return -1
	}
	want := rank(level)
	if want < 0 {
		return level
	}
	best := ""
	bestDistance := len(thinkingLevels) + 1
	for _, candidate := range available {
		if candidate == level {
			return level
		}
		r := rank(candidate)
		if r < 0 {
			continue
		}
		distance := want - r
		if distance < 0 {
			distance = -distance
		}
		// Prefer the nearest lower level on ties so a cap never raises effort.
		if distance < bestDistance || (distance == bestDistance && r < rank(best)) {
			best = candidate
			bestDistance = distance
		}
	}
	if best == "" {
		return level
	}
	return best
}

// modelRef identifies a configured pi model.
type modelRef struct {
	Provider string
	ID       string
	Name     string
}

func modelRefFromRecord(m record) modelRef {
	return modelRef{
		Provider: stringVal(m, "provider"),
		ID:       stringVal(m, "id"),
		Name:     stringVal(m, "name"),
	}
}

// matchesModel reports whether the configured model `current` is what the
// agent asked for. `want` may be `provider/id`, an exact id, or a fuzzy
// id/name fragment, with an optional thinking suffix already stripped.
func matchesModel(current modelRef, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" || current.ID == "" {
		return true
	}
	if provider, id, ok := strings.Cut(want, "/"); ok && provider != "" {
		return strings.EqualFold(provider, current.Provider) && strings.EqualFold(id, current.ID)
	}
	return strings.EqualFold(want, current.ID) || strings.EqualFold(want, current.Name)
}

// findModel resolves `want` against the models pi reports as available, using
// the same preference order as pi's --model: exact provider/id, exact id, exact
// name, then case-insensitive substring of id or name.
func findModel(models []modelRef, want string) (modelRef, bool) {
	want = strings.TrimSpace(want)
	if want == "" {
		return modelRef{}, false
	}
	if provider, id, ok := strings.Cut(want, "/"); ok && provider != "" {
		for _, m := range models {
			if strings.EqualFold(m.Provider, provider) && strings.EqualFold(m.ID, id) {
				return m, true
			}
		}
	}
	for _, m := range models {
		if strings.EqualFold(m.ID, want) {
			return m, true
		}
	}
	for _, m := range models {
		if strings.EqualFold(m.Name, want) {
			return m, true
		}
	}
	lower := strings.ToLower(want)
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ID), lower) || strings.Contains(strings.ToLower(m.Name), lower) {
			return m, true
		}
	}
	return modelRef{}, false
}

func modelRefsFromData(data record) []modelRef {
	raw := sliceVal(data, "models")
	refs := make([]modelRef, 0, len(raw))
	for _, item := range raw {
		m, _ := item.(record)
		if m == nil {
			continue
		}
		refs = append(refs, modelRefFromRecord(m))
	}
	return refs
}

func stringsFromData(data record, key string) []string {
	raw := sliceVal(data, key)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
