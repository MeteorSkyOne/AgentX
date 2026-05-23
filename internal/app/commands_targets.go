package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

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
