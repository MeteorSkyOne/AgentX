package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

const statusContextProbeTimeout = 8 * time.Second

var (
	contextUsageRatioPattern        = regexp.MustCompile(`(?i)(\d[\d,]*(?:\.\d+)?)\s*([km])?\s*/\s*(\d[\d,]*(?:\.\d+)?)\s*([km])?`)
	contextUsageParenPercentPattern = regexp.MustCompile(`\((\d+(?:\.\d+)?)\s*%\)`)
	contextUsedPercentPattern       = regexp.MustCompile(`(?i)(?:used|usage|context)[^\n]{0,40}?(\d+(?:\.\d+)?)\s*%|(\d+(?:\.\d+)?)\s*%\s*(?:used|usage|context)`)
	contextRemainingPercentPattern  = regexp.MustCompile(`(?i)(?:remaining|left)[^\n]{0,40}?(\d+(?:\.\d+)?)\s*%|(\d+(?:\.\d+)?)\s*%\s*(?:remaining|left)`)
	ansiEscapePattern               = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

func (a *App) handleStatusCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, args string) (domain.Message, error) {
	if strings.TrimSpace(args) != "" {
		return domain.Message{}, commandInputError("/status does not accept arguments")
	}
	limits := a.AgentProviderLimits(ctx, target.Agent, true)
	if usage, err := a.probeStatusContextUsage(ctx, req, target); err == nil && usage != nil {
		if err := a.store.Sessions().SetAgentSessionContextUsage(ctx, target.Agent.ID, req.ConversationType, req.ConversationID, usage); err != nil {
			return domain.Message{}, err
		}
	} else if err != nil {
		// Keep /status useful when a provider-specific context probe fails.
		// The stored/active snapshot below may still have a recent value.
		slog.Warn("status context probe failed", "agent_id", target.Agent.ID, "agent_kind", target.Agent.Kind, "error", err)
	}
	contextUsage, _, err := a.statusContextUsage(ctx, target.Agent.ID, req.ConversationType, req.ConversationID)
	if err != nil {
		return domain.Message{}, err
	}
	body := statusCommandMessageBody(target.Agent, contextUsage, limits, time.Now().UTC())
	return a.createCommandSystemMessage(ctx, req, body, map[string]any{
		"command_name": "status",
		"agent_id":     target.Agent.ID,
		"agent_handle": target.Agent.Handle,
	})
}

func (a *App) probeStatusContextUsage(ctx context.Context, req SendMessageRequest, target ConversationAgentContext) (*domain.ContextUsage, error) {
	if !isClaudeAgentKind(target.Agent.Kind) {
		return nil, nil
	}
	rt, ok := a.runtimeForAgent(target.Agent)
	if !ok {
		return nil, nil
	}
	previousSessionID, err := a.previousProviderSessionID(ctx, target.Agent.ID, domain.Message{
		ConversationType: req.ConversationType,
		ConversationID:   req.ConversationID,
	})
	if err != nil {
		return nil, err
	}

	probeCtx, cancel := context.WithTimeout(ctx, statusContextProbeTimeout)
	defer cancel()

	sessionKey := target.Agent.ID + ":" + string(req.ConversationType) + ":" + req.ConversationID
	session, err := rt.StartSession(probeCtx, agentruntime.StartSessionRequest{
		AgentID:              target.Agent.ID,
		Workspace:            target.RunWorkspace.Path,
		InstructionWorkspace: target.ConfigWorkspace.Path,
		Model:                target.Agent.Model,
		Effort:               target.Agent.Effort,
		FastMode:             target.Agent.FastMode,
		YoloMode:             target.Agent.YoloMode,
		Env:                  target.Agent.Env,
		SessionKey:           sessionKey,
		PreviousSessionID:    previousSessionID,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = session.Close(context.WithoutCancel(ctx))
	}()

	if reader, ok := session.(agentruntime.ContextUsageReader); ok {
		usage, err := reader.ContextUsage(probeCtx)
		if err != nil || usage != nil {
			return contextUsageToDomain(usage), err
		}
	}

	if err := session.Send(probeCtx, agentruntime.Input{Prompt: "/context"}); err != nil {
		return nil, err
	}

	var text strings.Builder
	for {
		select {
		case <-probeCtx.Done():
			return nil, probeCtx.Err()
		case evt, ok := <-session.Events():
			if !ok {
				return parseClaudeContextOutput(text.String()), nil
			}
			if evt.Text != "" {
				if text.Len() > 0 {
					text.WriteByte('\n')
				}
				text.WriteString(evt.Text)
			}
			if evt.Usage != nil && evt.Usage.Context != nil {
				return contextUsageToDomain(evt.Usage.Context), nil
			}
			switch evt.Type {
			case agentruntime.EventCompleted:
				if usage := parseClaudeContextOutput(evt.Text); usage != nil {
					return usage, nil
				}
				return parseClaudeContextOutput(text.String()), nil
			case agentruntime.EventFailed:
				return nil, runtimeEventError(evt)
			case agentruntime.EventCanceled:
				return nil, errAgentRunCanceled
			}
		}
	}
}

func isClaudeAgentKind(kind string) bool {
	return kind == domain.AgentKindClaude || kind == domain.AgentKindClaudePersistent
}

func (a *App) statusContextUsage(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string) (*domain.ContextUsage, *time.Time, error) {
	key := activeRunKey{conversationType: conversationType, conversationID: conversationID, agentID: agentID}
	if usage, updatedAt := a.activeRunContextUsage(key); usage != nil {
		return usage, updatedAt, nil
	}
	session, err := a.store.Sessions().ByConversation(ctx, agentID, conversationType, conversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return cloneDomainContextUsage(session.ContextUsage), cloneTimePtr(session.ContextUsageUpdatedAt), nil
}

func (a *App) activeRunContextUsage(key activeRunKey) (*domain.ContextUsage, *time.Time) {
	a.activeRunsMu.Lock()
	runs := make([]*activeAgentRun, 0, len(a.activeRuns[key]))
	for _, run := range a.activeRuns[key] {
		runs = append(runs, run)
	}
	a.activeRunsMu.Unlock()

	var latestUsage *domain.ContextUsage
	var latestAt *time.Time
	for _, run := range runs {
		usage, updatedAt := run.latestContextUsage()
		if usage == nil {
			continue
		}
		if latestAt == nil || (updatedAt != nil && updatedAt.After(*latestAt)) {
			latestUsage = usage
			latestAt = updatedAt
		}
	}
	return latestUsage, latestAt
}

func parseClaudeContextOutput(raw string) *domain.ContextUsage {
	text := strings.TrimSpace(ansiEscapePattern.ReplaceAllString(raw, ""))
	if text == "" {
		return nil
	}

	usage := &domain.ContextUsage{Source: "claude_context"}
	if match := contextUsageRatioPattern.FindStringSubmatch(text); len(match) == 5 {
		if total, ok := parseContextTokenNumber(match[1], match[2]); ok {
			usage.TotalTokens = &total
		}
		if window, ok := parseContextTokenNumber(match[3], match[4]); ok {
			usage.ContextWindowTokens = &window
		}
		if match := contextUsageParenPercentPattern.FindStringSubmatch(text); len(match) == 2 {
			if percent, err := strconv.ParseFloat(match[1], 64); err == nil {
				usage.UsedPercent = &percent
			}
		}
	}

	if usage.UsedPercent == nil {
		if percent, ok := firstContextPercent(contextRemainingPercentPattern, text); ok {
			used := 100 - percent
			if used < 0 {
				used = 0
			}
			usage.UsedPercent = &used
		} else if percent, ok := firstContextPercent(contextUsedPercentPattern, text); ok {
			usage.UsedPercent = &percent
		}
	}
	if usage.TotalTokens == nil && usage.ContextWindowTokens == nil && usage.UsedPercent == nil {
		return nil
	}
	return usage
}

func parseContextTokenNumber(raw string, suffix string) (int64, bool) {
	value, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(suffix) {
	case "k":
		value *= 1000
	case "m":
		value *= 1000000
	}
	return int64(value + 0.5), true
}

func firstContextPercent(pattern *regexp.Regexp, text string) (float64, bool) {
	match := pattern.FindStringSubmatch(text)
	if len(match) == 0 {
		return 0, false
	}
	for _, group := range match[1:] {
		if group == "" {
			continue
		}
		percent, err := strconv.ParseFloat(group, 64)
		if err == nil {
			return percent, true
		}
	}
	return 0, false
}

func statusCommandMessageBody(agent domain.Agent, usage *domain.ContextUsage, limits AgentProviderLimits, now time.Time) string {
	lines := []string{
		fmt.Sprintf("Status for @%s (%s)", agent.Handle, agent.Kind),
		"Context: " + contextUsageStatusLine(usage),
		"Auth: " + providerAuthStatusLine(limits.Auth),
		"Limits: " + providerLimitsStatusLine(limits, now),
	}
	return strings.Join(lines, "\n")
}

func contextUsageStatusLine(usage *domain.ContextUsage) string {
	if usage == nil || (usage.TotalTokens == nil && usage.ContextWindowTokens == nil && usage.UsedPercent == nil) {
		return "unavailable"
	}
	if usage.TotalTokens != nil && usage.ContextWindowTokens != nil && *usage.ContextWindowTokens > 0 {
		percent := usage.UsedPercent
		if percent == nil {
			value := (float64(*usage.TotalTokens) / float64(*usage.ContextWindowTokens)) * 100
			percent = &value
		}
		return fmt.Sprintf("%s / %s tokens (%s)", formatInt64(*usage.TotalTokens), formatInt64(*usage.ContextWindowTokens), formatPercent(*percent))
	}
	if usage.TotalTokens != nil {
		return fmt.Sprintf("%s tokens", formatInt64(*usage.TotalTokens))
	}
	if usage.UsedPercent != nil {
		return formatPercent(*usage.UsedPercent)
	}
	return "unavailable"
}

func providerAuthStatusLine(auth ProviderLimitAuth) string {
	if auth.LoggedIn {
		parts := []string{"logged in"}
		var details []string
		if auth.Method != "" {
			details = append(details, auth.Method)
		}
		if auth.Provider != "" {
			details = append(details, auth.Provider)
		}
		if auth.Plan != "" {
			details = append(details, auth.Plan)
		}
		if len(details) > 0 {
			parts = append(parts, "("+strings.Join(details, ", ")+")")
		}
		return strings.Join(parts, " ")
	}
	if auth.Method != "" || auth.Provider != "" || auth.Plan != "" {
		return "not logged in"
	}
	return "unavailable"
}

func providerLimitsStatusLine(limits AgentProviderLimits, now time.Time) string {
	if len(limits.Windows) == 0 {
		if limits.Message != "" {
			return string(limits.Status) + ": " + limits.Message
		}
		return string(limits.Status)
	}
	parts := make([]string, 0, len(limits.Windows))
	for _, window := range limits.Windows {
		label := strings.TrimSpace(window.Label)
		if label == "" {
			label = window.Kind
		}
		if label == "" {
			label = "Window"
		}
		part := label
		if window.UsedPercent != nil {
			part += " " + formatPercent(*window.UsedPercent) + " used"
		} else {
			part += " usage unavailable"
		}
		if window.ResetsAt != nil {
			part += ", resets in " + formatRelativeDuration(window.ResetsAt.Sub(now))
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func formatInt64(value int64) string {
	text := fmt.Sprintf("%d", value)
	var b strings.Builder
	for i, r := range text {
		if i > 0 && (len(text)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func formatPercent(value float64) string {
	if value < 0 {
		value = 0
	}
	if value == float64(int64(value)) {
		return fmt.Sprintf("%.0f%%", value)
	}
	return fmt.Sprintf("%.1f%%", value)
}

func formatRelativeDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	minutes := int64(d.Round(time.Minute) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	days := minutes / (24 * 60)
	minutes %= 24 * 60
	hours := minutes / 60
	minutes %= 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
