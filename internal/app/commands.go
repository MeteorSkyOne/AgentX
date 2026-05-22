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
	"github.com/meteorsky/agentx/internal/id"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/store"
)

var ErrUnknownCommand = errors.New("unknown command")

const statusContextProbeTimeout = 8 * time.Second

var (
	contextUsageRatioPattern        = regexp.MustCompile(`(?i)(\d[\d,]*(?:\.\d+)?)\s*([km])?\s*/\s*(\d[\d,]*(?:\.\d+)?)\s*([km])?`)
	contextUsageParenPercentPattern = regexp.MustCompile(`\((\d+(?:\.\d+)?)\s*%\)`)
	contextUsedPercentPattern       = regexp.MustCompile(`(?i)(?:used|usage|context)[^\n]{0,40}?(\d+(?:\.\d+)?)\s*%|(\d+(?:\.\d+)?)\s*%\s*(?:used|usage|context)`)
	contextRemainingPercentPattern  = regexp.MustCompile(`(?i)(?:remaining|left)[^\n]{0,40}?(\d+(?:\.\d+)?)\s*%|(\d+(?:\.\d+)?)\s*%\s*(?:remaining|left)`)
	ansiEscapePattern               = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

type CommandInputError struct {
	Message string
}

func (e CommandInputError) Error() string {
	return e.Message
}

func commandInputError(message string) error {
	return CommandInputError{Message: message}
}

func IsCommandInputError(err error) bool {
	var target CommandInputError
	return errors.As(err, &target)
}

type slashCommand struct {
	Name    string
	Args    string
	Targets []string
}

var builtinSlashCommands = map[string]struct{}{
	"new":     {},
	"skills":  {},
	"compact": {},
	"plan":    {},
	"init":    {},
	"model":   {},
	"effort":  {},
	"commit":  {},
	"push":    {},
	"review":  {},
	"stop":    {},
	"cancel":  {},
	"discuss": {},
	"goal":    {},
	"status":  {},
}

func builtinSlashCommandNames() []string {
	names := make([]string, 0, len(builtinSlashCommands))
	for name := range builtinSlashCommands {
		names = append(names, name)
	}
	return names
}

func parseSlashCommand(body string) (slashCommand, bool, error) {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "/") {
		return slashCommand{}, false, nil
	}
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return slashCommand{}, true, ErrUnknownCommand
	}
	name := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	if name == "" {
		return slashCommand{}, true, ErrUnknownCommand
	}

	var targets []string
	var args []string
	for _, field := range fields[1:] {
		if handle, ok := slashTargetToken(field); ok {
			targets = append(targets, handle)
			continue
		}
		args = append(args, field)
	}
	return slashCommand{Name: name, Args: strings.Join(args, " "), Targets: targets}, true, nil
}

func slashTargetToken(field string) (string, bool) {
	if !strings.HasPrefix(field, "@") || len(field) == 1 {
		return "", false
	}
	handle := field[1:]
	for i := 0; i < len(handle); i++ {
		ch := handle[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			continue
		}
		return "", false
	}
	return strings.ToLower(handle), true
}

func (a *App) dispatchSlashCommand(ctx context.Context, req SendMessageRequest, scope conversationScope, agents []ConversationAgentContext, command slashCommand) (domain.Message, error) {
	if command.Name == "new" {
		targets, err := resolveNewCommandTargets(agents, command.Targets)
		if err != nil {
			return domain.Message{}, err
		}
		return a.handleNewCommand(ctx, req, targets, len(command.Targets) == 0)
	}
	if command.Name == "stop" {
		targets, err := resolveNewCommandTargets(agents, command.Targets)
		if err != nil {
			return domain.Message{}, err
		}
		return a.handleStopCommand(ctx, req, targets, len(command.Targets) == 0)
	}
	if command.Name == "cancel" {
		targets, err := resolveNewCommandTargets(agents, command.Targets)
		if err != nil {
			return domain.Message{}, err
		}
		return a.handleCancelCommand(ctx, req, targets, len(command.Targets) == 0)
	}
	if command.Name == "discuss" {
		targets, err := resolveDiscussCommandTargets(agents, command.Targets)
		if err != nil {
			return domain.Message{}, err
		}
		return a.handleDiscussCommand(ctx, req, scope, targets, command.Args)
	}

	target, err := resolveSlashCommandTarget(agents, command.Targets)
	if err != nil {
		return domain.Message{}, err
	}

	switch command.Name {
	case "skills":
		return a.handleListSkillsCommand(ctx, req, target)
	case "compact":
		return a.handleCompactCommand(ctx, req, target, command.Args)
	case "goal":
		return a.handleGoalCommand(ctx, req, target, command.Args)
	case "model":
		return a.handleModelCommand(ctx, req, target, command.Args)
	case "effort":
		return a.handleEffortCommand(ctx, req, target, command.Args)
	case "status":
		return a.handleStatusCommand(ctx, req, target, command.Args)
	case "plan", "init", "commit", "push", "review":
		prompt, permissionMode, err := commandRunPrompt(command.Name, command.Args, target.Agent)
		if err != nil {
			return domain.Message{}, err
		}
		return a.createCommandRun(ctx, req, target, prompt, permissionMode, nil)
	default:
		return a.handleSkillCommand(ctx, req, target, command)
	}
}

func resolveSlashCommandTarget(agents []ConversationAgentContext, handles []string) (ConversationAgentContext, error) {
	uniqueHandles := make([]string, 0, len(handles))
	seen := make(map[string]struct{}, len(handles))
	for _, handle := range handles {
		handle = strings.ToLower(strings.TrimSpace(handle))
		if handle == "" {
			continue
		}
		if _, ok := seen[handle]; ok {
			continue
		}
		seen[handle] = struct{}{}
		uniqueHandles = append(uniqueHandles, handle)
	}

	if len(uniqueHandles) == 0 {
		switch len(agents) {
		case 0:
			return ConversationAgentContext{}, commandInputError("no agents are available in this conversation")
		case 1:
			return agents[0], nil
		default:
			return ConversationAgentContext{}, commandInputError("command requires an @agent target when multiple agents are in this conversation")
		}
	}
	if len(uniqueHandles) > 1 {
		return ConversationAgentContext{}, commandInputError("command supports exactly one @agent target")
	}

	for _, agent := range agents {
		if strings.EqualFold(agent.Agent.Handle, uniqueHandles[0]) {
			return agent, nil
		}
	}
	return ConversationAgentContext{}, commandInputError(fmt.Sprintf("unknown agent @%s", uniqueHandles[0]))
}

func resolveNewCommandTargets(agents []ConversationAgentContext, handles []string) ([]ConversationAgentContext, error) {
	if len(agents) == 0 {
		return nil, commandInputError("no agents are available in this conversation")
	}
	uniqueHandles := make([]string, 0, len(handles))
	seen := make(map[string]struct{}, len(handles))
	for _, handle := range handles {
		handle = strings.ToLower(strings.TrimSpace(handle))
		if handle == "" {
			continue
		}
		if _, ok := seen[handle]; ok {
			continue
		}
		seen[handle] = struct{}{}
		uniqueHandles = append(uniqueHandles, handle)
	}
	if len(uniqueHandles) == 0 {
		return agents, nil
	}

	targets := make([]ConversationAgentContext, 0, len(uniqueHandles))
	for _, handle := range uniqueHandles {
		var found bool
		for _, agent := range agents {
			if strings.EqualFold(agent.Agent.Handle, handle) {
				targets = append(targets, agent)
				found = true
				break
			}
		}
		if !found {
			return nil, commandInputError(fmt.Sprintf("unknown agent @%s", handle))
		}
	}
	return targets, nil
}

func resolveDiscussCommandTargets(agents []ConversationAgentContext, handles []string) ([]ConversationAgentContext, error) {
	if len(agents) == 0 {
		return nil, commandInputError("no agents are available in this conversation")
	}
	if len(handles) == 0 {
		return nil, commandInputError("/discuss requires at least two @agent targets")
	}
	targets, err := resolveNewCommandTargets(agents, handles)
	if err != nil {
		return nil, err
	}
	if len(targets) < 2 {
		return nil, commandInputError("/discuss requires at least two unique @agent targets")
	}
	return targets, nil
}

func (a *App) handleNewCommand(ctx context.Context, req SendMessageRequest, targets []ConversationAgentContext, allAgents bool) (domain.Message, error) {
	agentIDs := make([]string, 0, len(targets))
	agentHandles := make([]string, 0, len(targets))
	for _, target := range targets {
		agentIDs = append(agentIDs, target.Agent.ID)
		agentHandles = append(agentHandles, target.Agent.Handle)
	}

	message, err := a.createCommandSystemMessage(ctx, req, newContextMessageBody(agentHandles, allAgents), map[string]any{
		"command_name":  "new",
		"separator":     true,
		"agent_ids":     agentIDs,
		"agent_handles": agentHandles,
		"scope":         newContextScope(allAgents),
	})
	if err != nil {
		return domain.Message{}, err
	}
	boundary := time.Now().UTC()
	for _, target := range targets {
		if err := a.store.Sessions().ResetAgentSessionContext(ctx, target.Agent.ID, req.ConversationType, req.ConversationID, boundary); err != nil {
			return domain.Message{}, err
		}
	}
	return message, nil
}

func newContextMessageBody(handles []string, allAgents bool) string {
	if allAgents {
		return "New context for all agents"
	}
	if len(handles) == 1 {
		return fmt.Sprintf("New context for @%s", handles[0])
	}
	mentions := make([]string, 0, len(handles))
	for _, handle := range handles {
		mentions = append(mentions, "@"+handle)
	}
	return "New context for " + strings.Join(mentions, ", ")
}

func newContextScope(allAgents bool) string {
	if allAgents {
		return "all"
	}
	return "selected"
}

func (a *App) handleDiscussCommand(ctx context.Context, req SendMessageRequest, scope conversationScope, targets []ConversationAgentContext, args string) (domain.Message, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return domain.Message{}, commandInputError("/discuss requires a prompt")
	}
	message, err := a.createConversationMessage(ctx, req, domain.SenderUser, req.UserID, discussMessageBody(targets, args), nil)
	if err != nil {
		return domain.Message{}, err
	}
	go a.runAgentTeamForMessage(context.WithoutCancel(ctx), message, scope, targets, targets)
	return message, nil
}

func discussMessageBody(targets []ConversationAgentContext, args string) string {
	parts := make([]string, 0, len(targets)+1)
	for _, target := range targets {
		parts = append(parts, "@"+target.Agent.Handle)
	}
	parts = append(parts, strings.TrimSpace(args))
	return strings.Join(parts, " ")
}

func (a *App) handleStopCommand(ctx context.Context, req SendMessageRequest, targets []ConversationAgentContext, allAgents bool) (domain.Message, error) {
	stopped := 0
	clearedQueued := 0
	stoppedHandles := make([]string, 0, len(targets))
	agentIDs := make([]string, 0, len(targets))
	agentHandles := make([]string, 0, len(targets))
	for _, target := range targets {
		agentIDs = append(agentIDs, target.Agent.ID)
		agentHandles = append(agentHandles, target.Agent.Handle)
		count := a.stopActiveAgentRuns(ctx, activeRunKey{
			conversationType: req.ConversationType,
			conversationID:   req.ConversationID,
			agentID:          target.Agent.ID,
		})
		if count > 0 {
			stopped += count
			stoppedHandles = append(stoppedHandles, target.Agent.Handle)
		}
		clearedQueued += a.clearQueuedAgentPrompts(messageQueueKey{
			conversationType: req.ConversationType,
			conversationID:   req.ConversationID,
			agentID:          target.Agent.ID,
		}, "canceled")
	}
	body := stopCommandMessageBody(agentHandles, stoppedHandles, stopped, allAgents)
	return a.createCommandSystemMessage(ctx, req, body, map[string]any{
		"command_name":           "stop",
		"agent_ids":              agentIDs,
		"agent_handles":          agentHandles,
		"stopped_runs":           stopped,
		"cleared_queued_prompts": clearedQueued,
		"scope":                  newContextScope(allAgents),
	})
}

func (a *App) handleCancelCommand(ctx context.Context, req SendMessageRequest, targets []ConversationAgentContext, allAgents bool) (domain.Message, error) {
	var result cancelAgentRunResult
	canceledHandles := make([]string, 0, len(targets))
	unsupportedHandles := make([]string, 0, len(targets))
	agentIDs := make([]string, 0, len(targets))
	agentHandles := make([]string, 0, len(targets))
	for _, target := range targets {
		agentIDs = append(agentIDs, target.Agent.ID)
		agentHandles = append(agentHandles, target.Agent.Handle)
		cancelResult := a.cancelActiveAgentRuns(ctx, activeRunKey{
			conversationType: req.ConversationType,
			conversationID:   req.ConversationID,
			agentID:          target.Agent.ID,
		})
		result.requested += cancelResult.requested
		result.unsupported += cancelResult.unsupported
		if cancelResult.requested > 0 {
			canceledHandles = append(canceledHandles, target.Agent.Handle)
		}
		if cancelResult.unsupported > 0 {
			unsupportedHandles = append(unsupportedHandles, target.Agent.Handle)
		}
	}
	body := cancelCommandMessageBody(agentHandles, canceledHandles, unsupportedHandles, result, allAgents)
	return a.createCommandSystemMessage(ctx, req, body, map[string]any{
		"command_name":        "cancel",
		"agent_ids":           agentIDs,
		"agent_handles":       agentHandles,
		"canceled_runs":       result.requested,
		"unsupported_runs":    result.unsupported,
		"canceled_handles":    canceledHandles,
		"unsupported_handles": unsupportedHandles,
		"scope":               newContextScope(allAgents),
	})
}

func stopCommandMessageBody(handles []string, stoppedHandles []string, stopped int, allAgents bool) string {
	if stopped == 0 {
		if allAgents {
			return "No active agent runs to stop"
		}
		if len(handles) == 1 {
			return fmt.Sprintf("No active run for @%s", handles[0])
		}
		return "No active runs for selected agents"
	}
	if allAgents {
		if stopped == 1 {
			return "Stopped 1 active agent run"
		}
		return fmt.Sprintf("Stopped %d active agent runs", stopped)
	}
	mentions := make([]string, 0, len(stoppedHandles))
	for _, handle := range stoppedHandles {
		mentions = append(mentions, "@"+handle)
	}
	if stopped == 1 && len(mentions) == 1 {
		return fmt.Sprintf("Stopped active run for %s", mentions[0])
	}
	return fmt.Sprintf("Stopped %d active agent runs for %s", stopped, strings.Join(mentions, ", "))
}

func cancelCommandMessageBody(handles []string, canceledHandles []string, unsupportedHandles []string, result cancelAgentRunResult, allAgents bool) string {
	if result.requested == 0 {
		target := targetedCommandSuffix(handles, allAgents)
		if result.unsupported > 0 {
			return "No cancellable active agent runs" + target + " (/cancel is only supported by runtimes with soft interrupt)"
		}
		return "No active agent runs to cancel" + target
	}
	body := fmt.Sprintf("Cancel requested for %d active agent run", result.requested)
	if result.requested != 1 {
		body += "s"
	}
	if len(canceledHandles) > 0 && !allAgents {
		body += " for " + strings.Join(prefixHandles(canceledHandles), ", ")
	}
	if result.unsupported > 0 {
		body += fmt.Sprintf("; %d active run", result.unsupported)
		if result.unsupported != 1 {
			body += "s"
		}
		body += " do not support /cancel"
		if len(unsupportedHandles) > 0 && !allAgents {
			body += " for " + strings.Join(prefixHandles(unsupportedHandles), ", ")
		}
	}
	return body
}

func targetedCommandSuffix(handles []string, allAgents bool) string {
	if allAgents {
		return ""
	}
	if len(handles) == 1 {
		return " for @" + handles[0]
	}
	return " for selected agents"
}

func prefixHandles(handles []string) []string {
	mentions := make([]string, 0, len(handles))
	for _, handle := range handles {
		mentions = append(mentions, "@"+handle)
	}
	return mentions
}

func (a *App) handleCompactCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, args string) (domain.Message, error) {
	if target.Agent.Kind != domain.AgentKindClaude {
		return a.createSystemMessage(ctx, req, fmt.Sprintf("/compact is not supported for @%s. Use /new to start a fresh context.", target.Agent.Handle))
	}
	prompt := "/compact"
	if args = strings.TrimSpace(args); args != "" {
		prompt += " " + args
	}
	onCompleted := func(runCtx context.Context) error {
		return a.store.Sessions().SetAgentSessionContextStartedAt(runCtx, target.Agent.ID, req.ConversationType, req.ConversationID, time.Now().UTC())
	}
	return a.createCommandRun(ctx, req, target, prompt, "", onCompleted)
}

func (a *App) handleGoalCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, args string) (domain.Message, error) {
	if target.Agent.Kind == domain.AgentKindFake {
		return a.createSystemMessage(ctx, req, fmt.Sprintf("/goal is not supported for @%s.", target.Agent.Handle))
	}
	args = strings.TrimSpace(args)
	if args == "" {
		return domain.Message{}, commandInputError("/goal requires a goal description")
	}
	prompt := "/goal " + args
	return a.createCommandRun(ctx, req, target, prompt, "", nil)
}

func (a *App) handleModelCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, model string) (domain.Message, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return domain.Message{}, commandInputError("/model requires a model name")
	}
	updated, err := a.UpdateAgent(ctx, target.Agent.ID, AgentUpdateRequest{Model: &model})
	if err != nil {
		return domain.Message{}, err
	}
	return a.createSystemMessage(ctx, req, fmt.Sprintf("Updated @%s model to %s.", updated.Handle, updated.Model))
}

func (a *App) handleEffortCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, effort string) (domain.Message, error) {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return domain.Message{}, commandInputError("/effort requires a level")
	}
	updated, err := a.UpdateAgent(ctx, target.Agent.ID, AgentUpdateRequest{Effort: &effort})
	if err != nil {
		return domain.Message{}, err
	}
	return a.createSystemMessage(ctx, req, fmt.Sprintf("Updated @%s effort to %s.", updated.Handle, updated.Effort))
}

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

func (a *App) createCommandRun(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, prompt string, permissionMode string, onCompleted func(context.Context) error) (domain.Message, error) {
	message, err := a.createConversationMessage(ctx, req, domain.SenderUser, req.UserID, strings.TrimSpace(req.Body), nil)
	if err != nil {
		return domain.Message{}, err
	}

	runID := id.New("run")
	go a.runAgentForMessageWithTarget(context.WithoutCancel(ctx), message, target, runID, agentRunOptions{
		Prompt:         prompt,
		PermissionMode: permissionMode,
		OnCompleted:    onCompleted,
	})
	return message, nil
}

func commandRunPrompt(commandName string, args string, agent domain.Agent) (string, string, error) {
	args = strings.TrimSpace(args)
	switch commandName {
	case "plan":
		if args == "" {
			return "", "", commandInputError("/plan requires a task")
		}
		return "Create a concrete implementation plan for the following task. Do not modify files. Do not run destructive commands. Return only the plan, risks, and any blocking questions.\n\nTask:\n" + args, "plan", nil
	case "init":
		if agent.Kind == domain.AgentKindClaude {
			return "/init", "", nil
		}
		return "Initialize this workspace for an AI coding agent by creating or updating AGENTS.md with concise repository instructions, commands, architecture notes, and coding conventions. Preserve existing useful guidance.", "", nil
	case "commit":
		prompt := "Inspect the current git status and diff in this workspace, run relevant tests when needed, then create a git commit. Do not push. Use a concise commit message that reflects the change."
		if args != "" {
			prompt += "\n\nAdditional instructions:\n" + args
		}
		return prompt, "", nil
	case "push":
		prompt := "Push the current branch. If no upstream is configured, set the upstream only when an origin remote exists; otherwise report the failure clearly. Do not force push unless the user explicitly asked for it."
		if args != "" {
			prompt += "\n\nAdditional instructions:\n" + args
		}
		return prompt, "", nil
	case "review":
		prompt := "Review the current workspace changes. Prioritize bugs, behavioral regressions, missing tests, and concrete risks. Start with findings ordered by severity and include file and line references when possible."
		if args != "" {
			prompt += "\n\nAdditional instructions:\n" + args
		}
		return prompt, "", nil
	default:
		return "", "", ErrUnknownCommand
	}
}

func (a *App) createSystemMessage(ctx context.Context, req SendMessageRequest, body string) (domain.Message, error) {
	return a.createCommandSystemMessage(ctx, req, body, nil)
}

func (a *App) createCommandSystemMessage(ctx context.Context, req SendMessageRequest, body string, metadata map[string]any) (domain.Message, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["command"] = true
	return a.createConversationMessage(ctx, req, domain.SenderSystem, "system", body, metadata)
}

func (a *App) createConversationMessage(ctx context.Context, req SendMessageRequest, senderType domain.SenderType, senderID string, body string, metadata map[string]any) (domain.Message, error) {
	replyToMessageID := ""
	var uploads []AttachmentUpload
	if senderType == domain.SenderUser {
		replyToMessageID = req.ReplyToMessageID
		uploads = req.Attachments
	}
	message := domain.Message{
		ID:               id.New("msg"),
		OrganizationID:   req.OrganizationID,
		ConversationType: req.ConversationType,
		ConversationID:   req.ConversationID,
		SenderType:       senderType,
		SenderID:         senderID,
		Kind:             domain.MessageText,
		Body:             body,
		Metadata:         metadata,
		ReplyToMessageID: replyToMessageID,
		CreatedAt:        time.Now().UTC(),
	}
	attachments, err := a.prepareMessageAttachments(message, uploads)
	if err != nil {
		return domain.Message{}, err
	}
	cleanupFiles := true
	defer func() {
		if cleanupFiles {
			_ = removeAttachmentFiles(attachments)
		}
	}()

	if err := a.store.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Messages().Create(ctx, message); err != nil {
			return err
		}
		for _, attachment := range attachments {
			if err := tx.MessageAttachments().Create(ctx, attachment); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return domain.Message{}, err
	}
	cleanupFiles = false
	message, err = a.resolveMessageReference(ctx, message)
	if err != nil {
		return domain.Message{}, err
	}
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventMessageCreated,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.MessageCreatedPayload{Message: message},
	})
	return message, nil
}
