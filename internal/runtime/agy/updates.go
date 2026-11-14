package agy

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

type trackedTool struct {
	name             string
	input            any
	parentToolCallID string
	started          bool
	completed        bool
	isSubagent       bool
}

type turnState struct {
	text         strings.Builder
	thinking     strings.Builder
	tools        map[string]*trackedTool
	plan         *runtime.PlanState
	goal         *runtime.GoalState
	goalStarted  time.Time
	contextUsage *runtime.ContextUsage
}

func newTurnState(prompt string) *turnState {
	st := &turnState{
		tools: make(map[string]*trackedTool),
	}
	if goal := initialGoalFromPrompt(prompt); goal != nil {
		st.goal = goal
		st.goalStarted = time.Now()
	}
	return st
}

func initialGoalFromPrompt(prompt string) *runtime.GoalState {
	trimmed := strings.TrimSpace(prompt)
	if !strings.HasPrefix(trimmed, "/goal") {
		return nil
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "/goal"))
	if rest == "" {
		return nil
	}
	if strings.EqualFold(rest, "clear") || strings.EqualFold(rest, "off") || strings.EqualFold(rest, "stop") {
		return &runtime.GoalState{Status: runtime.GoalStatusCleared}
	}
	return &runtime.GoalState{
		Objective: rest,
		Status:    runtime.GoalStatusActive,
	}
}

func (s *turnState) initialEvents() []runtime.Event {
	if s.goal == nil {
		return nil
	}
	cloned := *s.goal
	return []runtime.Event{{
		Type: runtime.EventDelta,
		Goal: &cloned,
	}}
}

func (s *turnState) finishGoal(stopReason string, totalTokens *int64) *runtime.GoalState {
	if s.goal == nil || s.goal.Status != runtime.GoalStatusActive {
		return nil
	}
	cloned := *s.goal
	if !s.goalStarted.IsZero() {
		secs := int64(time.Since(s.goalStarted).Seconds())
		cloned.TimeUsedSeconds = &secs
	}
	if totalTokens != nil {
		cloned.TokensUsed = totalTokens
	}
	switch strings.ToLower(strings.TrimSpace(stopReason)) {
	case "", "end_turn":
		cloned.Status = runtime.GoalStatusComplete
	case "cancelled", "canceled":
		cloned.Status = runtime.GoalStatusPaused
	case "max_tokens", "max_turn_requests":
		cloned.Status = runtime.GoalStatusBudgetLimited
	default:
		cloned.Status = runtime.NormalizeGoalStatus(stopReason)
	}
	s.goal = &cloned
	return &cloned
}

func (s *turnState) flushThinking() *runtime.Event {
	if s.thinking.Len() == 0 {
		return nil
	}
	thought := strings.TrimSpace(s.thinking.String())
	s.thinking.Reset()
	if thought == "" {
		return nil
	}
	return &runtime.Event{
		Type:     runtime.EventDelta,
		Thinking: thought,
		Process: []runtime.ProcessItem{{
			Type: "thinking",
			Text: thought,
		}},
	}
}

func (s *turnState) promotePreToolText() *runtime.Event {
	if s.text.Len() == 0 {
		return nil
	}
	preText := strings.TrimSpace(s.text.String())
	s.text.Reset()
	if preText == "" {
		return &runtime.Event{
			Type:      runtime.EventDelta,
			ClearText: true,
		}
	}
	return &runtime.Event{
		Type:      runtime.EventDelta,
		Thinking:  preText,
		ClearText: true,
		Process: []runtime.ProcessItem{{
			Type: "thinking",
			Text: preText,
		}},
	}
}

func (s *turnState) handleUpdate(update record) []runtime.Event {
	if update == nil {
		return nil
	}
	kind := stringVal(update, "sessionUpdate")
	var events []runtime.Event

	switch kind {
	case "agent_thought_chunk":
		chunk := extractUpdateText(update)
		if chunk != "" {
			s.thinking.WriteString(chunk)
		}

	case "agent_message_chunk":
		if flushed := s.flushThinking(); flushed != nil {
			events = append(events, *flushed)
		}
		chunk := extractUpdateText(update)
		if chunk != "" {
			s.text.WriteString(chunk)
			events = append(events, runtime.Event{
				Type: runtime.EventDelta,
				Text: chunk,
			})
		}

	case "tool_call", "tool_call_update":
		if flushed := s.flushThinking(); flushed != nil {
			events = append(events, *flushed)
		}
		if promoted := s.promotePreToolText(); promoted != nil {
			events = append(events, *promoted)
		}
		if items := s.handleToolUpdate(kind, update); len(items) > 0 {
			events = append(events, runtime.Event{
				Type:    runtime.EventDelta,
				Process: items,
			})
		}

	case "plan":
		if flushed := s.flushThinking(); flushed != nil {
			events = append(events, *flushed)
		}
		if plan := parsePlanUpdate(update); plan != nil {
			s.plan = plan
			events = append(events, runtime.Event{
				Type: runtime.EventDelta,
				Plan: plan,
			})
		}

	case "usage_update":
		if usage := parseContextUsageUpdate(update); usage != nil {
			s.contextUsage = usage
		}
	}

	return events
}

func (s *turnState) flushRemaining() []runtime.Event {
	var events []runtime.Event
	if flushed := s.flushThinking(); flushed != nil {
		events = append(events, *flushed)
	}
	// Close any subagent lifecycle signals whose tool never emitted a terminal update.
	var dangling []runtime.ProcessItem
	for callID, tool := range s.tools {
		if tool.isSubagent && tool.started && !tool.completed {
			tool.completed = true
			dangling = append(dangling, runtime.ProcessItem{
				Type:       "subagent_completed",
				ToolCallID: callID,
				Status:     "completed",
			})
		}
	}
	if len(dangling) > 0 {
		events = append(events, runtime.Event{
			Type:    runtime.EventDelta,
			Process: dangling,
		})
	}
	return events
}

func (s *turnState) handleToolUpdate(kind string, update record) []runtime.ProcessItem {
	callID := stringVal(update, "toolCallId")
	if callID == "" {
		callID = stringVal(update, "id")
	}
	name := firstNonEmpty(
		stringVal(update, "name"),
		stringVal(update, "toolName"),
		stringVal(update, "title"),
		stringVal(update, "kind"),
	)
	parentID := firstNonEmpty(
		stringVal(update, "parentToolCallId"),
		stringVal(mapVal(update, "_meta"), "parentToolCallId"),
	)
	input := toolInputFromUpdate(update)
	status := strings.ToLower(strings.TrimSpace(stringVal(update, "status")))

	entry := s.tools[callID]
	if entry == nil {
		entry = &trackedTool{
			name:             name,
			input:            input,
			parentToolCallID: parentID,
			isSubagent:       name == "invoke_subagent" && parentID == "",
		}
		if callID != "" {
			s.tools[callID] = entry
		}
	} else {
		if entry.name == "" && name != "" {
			entry.name = name
			entry.isSubagent = name == "invoke_subagent" && entry.parentToolCallID == ""
		}
		if entry.input == nil && input != nil {
			entry.input = input
		}
		if entry.parentToolCallID == "" && parentID != "" {
			entry.parentToolCallID = parentID
		}
	}

	var items []runtime.ProcessItem
	if !entry.started && (kind == "tool_call" || status == "in_progress" || status == "pending" || isTerminalToolStatus(status)) {
		entry.started = true
		callStatus := status
		if callStatus == "" || isTerminalToolStatus(callStatus) {
			callStatus = "in_progress"
		}
		items = append(items, runtime.ProcessItem{
			Type:             "tool_call",
			ToolName:         entry.name,
			ToolCallID:       callID,
			Status:           callStatus,
			Input:            entry.input,
			Raw:              update,
			ParentToolCallID: entry.parentToolCallID,
		})
		if entry.isSubagent && callID != "" {
			items = append(items, runtime.ProcessItem{
				Type:       "subagent_started",
				ToolCallID: callID,
			})
		}
	}

	if !entry.completed && isTerminalToolStatus(status) {
		entry.completed = true
		normStatus := normalizeToolResultStatus(status)
		output := toolOutputFromUpdate(update)
		items = append(items, runtime.ProcessItem{
			Type:             "tool_result",
			ToolName:         entry.name,
			ToolCallID:       callID,
			Status:           normStatus,
			Input:            entry.input,
			Output:           output,
			Raw:              update,
			ParentToolCallID: entry.parentToolCallID,
		})
		if entry.isSubagent && callID != "" {
			subStatus := "completed"
			if normStatus == "failed" || normStatus == "error" {
				subStatus = "failed"
			}
			items = append(items, runtime.ProcessItem{
				Type:       "subagent_completed",
				ToolCallID: callID,
				Status:     subStatus,
			})
		}
	}

	return items
}

func isTerminalToolStatus(status string) bool {
	switch status {
	case "completed", "failed", "error", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func normalizeToolResultStatus(status string) string {
	switch status {
	case "failed", "error":
		return "failed"
	case "cancelled", "canceled":
		return "canceled"
	default:
		return "completed"
	}
}

func toolInputFromUpdate(update record) any {
	if rawInput := update["rawInput"]; rawInput != nil {
		if m, ok := rawInput.(record); !ok || len(m) > 0 {
			return rawInput
		}
	}
	if args := update["args"]; args != nil {
		if m, ok := args.(record); !ok || len(m) > 0 {
			return args
		}
	}
	if locs := sliceVal(update, "locations"); len(locs) > 0 {
		if first, ok := locs[0].(record); ok {
			if p := stringVal(first, "path"); p != "" {
				return record{"path": p}
			}
		}
	}
	return nil
}

func toolOutputFromUpdate(update record) any {
	if rawOut := update["rawOutput"]; rawOut != nil {
		return rawOut
	}
	if content := update["content"]; content != nil {
		if text := extractContentListText(content); text != "" {
			return text
		}
		return content
	}
	return nil
}

func extractUpdateText(update record) string {
	if content, ok := update["content"].(record); ok {
		if t := stringVal(content, "text"); t != "" {
			return t
		}
	}
	return stringVal(update, "text")
}

func extractContentListText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case record:
		if t := stringVal(c, "text"); t != "" {
			return t
		}
		if inner, ok := c["content"].(record); ok {
			return stringVal(inner, "text")
		}
	case []any:
		var parts []string
		for _, item := range c {
			rec, ok := item.(record)
			if !ok {
				continue
			}
			if t := stringVal(rec, "text"); t != "" {
				parts = append(parts, t)
				continue
			}
			if inner, ok := rec["content"].(record); ok {
				if t := stringVal(inner, "text"); t != "" {
					parts = append(parts, t)
					continue
				}
			}
			if stringVal(rec, "type") == "diff" {
				path := stringVal(rec, "path")
				if path != "" {
					parts = append(parts, "Updated "+path)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func parsePlanUpdate(update record) *runtime.PlanState {
	entries := sliceVal(update, "entries")
	steps := make([]runtime.PlanStep, 0, len(entries))
	for _, raw := range entries {
		rec, ok := raw.(record)
		if !ok {
			continue
		}
		text := strings.TrimSpace(firstNonEmpty(stringVal(rec, "content"), stringVal(rec, "step"), stringVal(rec, "title")))
		if text == "" {
			continue
		}
		steps = append(steps, runtime.PlanStep{
			Step:   text,
			Status: runtime.NormalizePlanStepStatus(stringVal(rec, "status")),
		})
	}
	return &runtime.PlanState{
		Explanation: strings.TrimSpace(stringVal(update, "explanation")),
		Steps:       steps,
	}
}

func parseContextUsageUpdate(update record) *runtime.ContextUsage {
	used := int64Ptr(update, "used")
	size := int64Ptr(update, "size")
	if used == nil && size == nil {
		return nil
	}
	out := &runtime.ContextUsage{
		TotalTokens:         used,
		ContextWindowTokens: size,
		Source:              "usage_update",
	}
	if used != nil && size != nil && *size > 0 {
		pct := float64(*used) * 100 / float64(*size)
		out.UsedPercent = &pct
	}
	return out
}

func cloneContextUsage(in *runtime.ContextUsage) *runtime.ContextUsage {
	if in == nil {
		return nil
	}
	out := *in
	out.TotalTokens = cloneInt64Ptr(in.TotalTokens)
	out.InputTokens = cloneInt64Ptr(in.InputTokens)
	out.CachedInputTokens = cloneInt64Ptr(in.CachedInputTokens)
	out.OutputTokens = cloneInt64Ptr(in.OutputTokens)
	out.ReasoningOutputTokens = cloneInt64Ptr(in.ReasoningOutputTokens)
	out.ContextWindowTokens = cloneInt64Ptr(in.ContextWindowTokens)
	if in.UsedPercent != nil {
		v := *in.UsedPercent
		out.UsedPercent = &v
	}
	return &out
}

func cloneInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func buildUsage(promptResult record, model string, durationMS int64, ctxUsage *runtime.ContextUsage) *runtime.Usage {
	rawUsage := mapVal(promptResult, "usage")
	if ctxUsage != nil && ctxUsage.Model == "" && model != "" {
		cloned := cloneContextUsage(ctxUsage)
		cloned.Model = model
		ctxUsage = cloned
	}
	if rawUsage == nil && ctxUsage == nil {
		return nil
	}
	out := &runtime.Usage{
		Model:   model,
		Context: ctxUsage,
		Raw:     rawUsage,
	}
	if durationMS > 0 {
		out.DurationMS = ptrInt64(durationMS)
	}
	if rawUsage != nil {
		input := int64Ptr(rawUsage, "inputTokens")
		output := int64Ptr(rawUsage, "outputTokens")
		cacheRead := int64Ptr(rawUsage, "cachedReadTokens")
		cacheWrite := int64Ptr(rawUsage, "cachedWriteTokens")
		thought := int64Ptr(rawUsage, "thoughtTokens")
		total := int64Ptr(rawUsage, "totalTokens")

		out.InputTokens = input
		out.OutputTokens = output
		out.CachedInputTokens = cloneInt64Ptr(cacheRead)
		out.CacheReadInputTokens = cloneInt64Ptr(cacheRead)
		out.CacheCreationInputTokens = cacheWrite
		if thought != nil && *thought > 0 {
			out.ReasoningOutputTokens = thought
		}
		if total != nil {
			out.TotalTokens = total
		} else if input != nil || output != nil {
			sum := int64Val(rawUsage, "inputTokens") + int64Val(rawUsage, "outputTokens") + int64Val(rawUsage, "cachedReadTokens") + int64Val(rawUsage, "cachedWriteTokens")
			out.TotalTokens = ptrInt64(sum)
		}
	}
	return out
}

func permissionQuestionFromParams(params record) (question string, toolCallID string, options []permissionChoice) {
	toolCall := mapVal(params, "toolCall")
	toolCallID = stringVal(toolCall, "toolCallId")
	title := strings.TrimSpace(stringVal(toolCall, "title"))
	name := strings.TrimSpace(stringVal(toolCall, "name"))
	args := mapVal(toolCall, "args")
	if args == nil {
		args = mapVal(toolCall, "rawInput")
	}
	cmdLine := firstNonEmpty(stringVal(args, "CommandLine"), stringVal(args, "command"), stringVal(args, "Path"), stringVal(args, "path"))

	switch {
	case title != "" && cmdLine != "" && !strings.Contains(title, cmdLine):
		question = "Allow " + title + " (`" + cmdLine + "`)?"
	case title != "":
		question = "Allow " + title + "?"
	case name != "" && cmdLine != "":
		question = "Allow `" + name + "` (`" + cmdLine + "`)?"
	case name != "":
		question = "Allow tool `" + name + "`?"
	default:
		question = "Allow requested tool operation?"
	}

	for _, rawOpt := range sliceVal(params, "options") {
		rec, ok := rawOpt.(record)
		if !ok {
			continue
		}
		optID := stringVal(rec, "optionId")
		if optID == "" {
			continue
		}
		label := stringVal(rec, "name")
		if label == "" {
			label = optID
		}
		options = append(options, permissionChoice{
			OptionID: optID,
			Name:     label,
			Kind:     stringVal(rec, "kind"),
		})
	}
	return question, toolCallID, options
}

type permissionChoice struct {
	OptionID string
	Name     string
	Kind     string
}

func selectPermissionOption(choices []permissionChoice, answer string) string {
	trimmed := strings.TrimSpace(answer)
	lower := strings.ToLower(trimmed)
	for _, c := range choices {
		if strings.EqualFold(c.OptionID, trimmed) || strings.EqualFold(c.Name, trimmed) {
			return c.OptionID
		}
	}
	wantsAllow := lower == "y" || lower == "yes" || strings.Contains(lower, "allow") || strings.Contains(lower, "approve") || strings.Contains(lower, "accept")
	wantsDeny := lower == "n" || lower == "no" || strings.Contains(lower, "deny") || strings.Contains(lower, "reject") || strings.Contains(lower, "block") || strings.Contains(lower, "cancel")

	if wantsAllow {
		if id := findChoiceByKind(choices, true); id != "" {
			return id
		}
	}
	if wantsDeny {
		if id := findChoiceByKind(choices, false); id != "" {
			return id
		}
	}
	// Default conservatively to reject/deny when user wrote an unrecognized custom answer.
	if id := findChoiceByKind(choices, false); id != "" {
		return id
	}
	if len(choices) > 0 {
		return choices[0].OptionID
	}
	return ""
}

func findChoiceByKind(choices []permissionChoice, allow bool) string {
	for _, c := range choices {
		k := strings.ToLower(c.Kind)
		id := strings.ToLower(c.OptionID)
		if allow && (strings.HasPrefix(k, "allow") || strings.Contains(id, "allow")) {
			return c.OptionID
		}
		if !allow && (strings.HasPrefix(k, "reject") || strings.Contains(id, "deny") || strings.Contains(id, "reject")) {
			return c.OptionID
		}
	}
	return ""
}

func stringVal(m record, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func boolVal(m record, key string) bool {
	if m == nil {
		return false
	}
	v, _ := m[key].(bool)
	return v
}

func mapVal(m record, key string) record {
	if m == nil {
		return nil
	}
	v, _ := m[key].(record)
	return v
}

func sliceVal(m record, key string) []any {
	if m == nil {
		return nil
	}
	v, _ := m[key].([]any)
	return v
}

func int64Ptr(m record, key string) *int64 {
	if m == nil {
		return nil
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	switch n := raw.(type) {
	case float64:
		v := int64(n)
		return &v
	case int64:
		v := n
		return &v
	case int:
		v := int64(n)
		return &v
	case json.Number:
		if v, err := n.Int64(); err == nil {
			return &v
		}
		if f, err := n.Float64(); err == nil {
			v := int64(f)
			return &v
		}
	}
	return nil
}

func int64Val(m record, key string) int64 {
	if p := int64Ptr(m, key); p != nil {
		return *p
	}
	return 0
}

func ptrInt64(v int64) *int64 {
	return &v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
