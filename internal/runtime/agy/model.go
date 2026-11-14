package agy

import "strings"

type configOption struct {
	ID           string
	Name         string
	Category     string
	CurrentValue string
	Options      []configSelectOption
}

type configSelectOption struct {
	Value string
	Name  string
}

func parseConfigOptions(raw []any) []configOption {
	if len(raw) == 0 {
		return nil
	}
	out := make([]configOption, 0, len(raw))
	for _, entry := range raw {
		rec, _ := entry.(record)
		if rec == nil {
			continue
		}
		id := stringVal(rec, "id")
		if id == "" {
			continue
		}
		opt := configOption{
			ID:           id,
			Name:         stringVal(rec, "name"),
			Category:     stringVal(rec, "category"),
			CurrentValue: stringVal(rec, "currentValue"),
		}
		for _, rawChoice := range sliceVal(rec, "options") {
			choice, _ := rawChoice.(record)
			if choice == nil {
				continue
			}
			val := stringVal(choice, "value")
			if val == "" {
				continue
			}
			opt.Options = append(opt.Options, configSelectOption{
				Value: val,
				Name:  stringVal(choice, "name"),
			})
		}
		out = append(out, opt)
	}
	return out
}

func optionByID(opts []configOption, id string) (configOption, bool) {
	for _, opt := range opts {
		if opt.ID == id {
			return opt, true
		}
	}
	return configOption{}, false
}

func (o configOption) values() []string {
	out := make([]string, 0, len(o.Options))
	for _, choice := range o.Options {
		out = append(out, choice.Value)
	}
	return out
}

func (o configOption) matchValue(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	for _, choice := range o.Options {
		if choice.Value == target {
			return choice.Value, true
		}
	}
	for _, choice := range o.Options {
		if strings.EqualFold(choice.Value, target) {
			return choice.Value, true
		}
	}
	return "", false
}

var thoughtLevelOrder = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

func normalizeThoughtLevel(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "none":
		return "off"
	case "min":
		return "minimal"
	case "med":
		return "medium"
	}
	for _, known := range thoughtLevelOrder {
		if v == known {
			return known
		}
	}
	return ""
}

func thoughtLevelRank(level string) int {
	for i, known := range thoughtLevelOrder {
		if known == level {
			return i
		}
	}
	return -1
}

// clampThoughtLevel maps target to the nearest available thought_level option,
// preferring the lower level on ties so an agent never silently escalates cost.
func clampThoughtLevel(target string, available []string) string {
	target = normalizeThoughtLevel(target)
	if target == "" {
		return ""
	}
	if len(available) == 0 {
		return target
	}
	targetRank := thoughtLevelRank(target)
	if targetRank < 0 {
		return ""
	}
	best := ""
	bestDist := len(thoughtLevelOrder) + 1
	bestRank := len(thoughtLevelOrder) + 1
	for _, cand := range available {
		norm := normalizeThoughtLevel(cand)
		rank := thoughtLevelRank(norm)
		if rank < 0 {
			continue
		}
		if norm == target {
			return cand
		}
		dist := rank - targetRank
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist || (dist == bestDist && rank < bestRank) {
			best = cand
			bestDist = dist
			bestRank = rank
		}
	}
	return best
}

// splitModelEffort separates a tier-suffixed model ID (such as
// `gemini-3.8-flash-low`) into base model and effort when appropriate, while
// preserving base models whose own canonical ID ends with a level word (such as
// `gpt-oss-120b-medium`).
func splitModelEffort(raw string, availableModels []string) (modelID string, effort string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}
	for _, prefix := range []string{"agy/", "antigravity/"} {
		if strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			trimmed = strings.TrimSpace(trimmed[len(prefix):])
			break
		}
	}
	if trimmed == "" {
		return "", ""
	}

	if matched, ok := matchAvailable(trimmed, availableModels); ok {
		return matched, ""
	}
	if len(availableModels) == 0 && strings.EqualFold(trimmed, "gpt-oss-120b-medium") {
		return trimmed, ""
	}

	suffixes := []string{"-xhigh", "-medium", "-minimal", "-high", "-low", "-max", "-off"}
	lower := strings.ToLower(trimmed)
	for _, suffix := range suffixes {
		if !strings.HasSuffix(lower, suffix) {
			continue
		}
		base := strings.TrimSpace(trimmed[:len(trimmed)-len(suffix)])
		if base == "" {
			break
		}
		level := normalizeThoughtLevel(suffix[1:])
		if len(availableModels) > 0 {
			if matchedBase, ok := matchAvailable(base, availableModels); ok {
				return matchedBase, level
			}
			// Neither full string nor base matched availableModels; keep raw so
			// `session/set_model` can try the full ID directly.
			return trimmed, ""
		}
		return base, level
	}
	return trimmed, ""
}

func resolveThoughtLevel(effort string, modelEffort string, availableLevels []string) string {
	chosen := normalizeThoughtLevel(effort)
	if chosen == "" {
		chosen = normalizeThoughtLevel(modelEffort)
	}
	if chosen == "" {
		return ""
	}
	return clampThoughtLevel(chosen, availableLevels)
}

func matchAvailable(candidate string, available []string) (string, bool) {
	for _, item := range available {
		if item == candidate {
			return item, true
		}
	}
	for _, item := range available {
		if strings.EqualFold(item, candidate) {
			return item, true
		}
	}
	return "", false
}

func normalizePermissionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "accept-edits", "acceptedits":
		return "accept-edits"
	case "auto", "bypasspermissions", "bypass-permissions", "yolo":
		return "auto"
	case "plan":
		return "plan"
	case "default", "ask":
		return "default"
	default:
		return strings.TrimSpace(mode)
	}
}
