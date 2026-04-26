package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
)

var ErrUnknownCommand = errors.New("unknown command")

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
	"compact": {},
	"plan":    {},
	"init":    {},
	"model":   {},
	"effort":  {},
	"commit":  {},
	"push":    {},
	"review":  {},
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
	if _, ok := builtinSlashCommands[name]; !ok {
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

func (a *App) dispatchSlashCommand(ctx context.Context, req SendMessageRequest, agents []ConversationAgentContext, command slashCommand) (domain.Message, error) {
	if command.Name == "new" {
		targets, err := resolveNewCommandTargets(agents, command.Targets)
		if err != nil {
			return domain.Message{}, err
		}
		return a.handleNewCommand(ctx, req, targets, len(command.Targets) == 0)
	}

	target, err := resolveSlashCommandTarget(agents, command.Targets)
	if err != nil {
		return domain.Message{}, err
	}

	switch command.Name {
	case "compact":
		return a.handleCompactCommand(ctx, req, target, command.Args)
	case "model":
		return a.handleModelCommand(ctx, req, target, command.Args)
	case "effort":
		return a.handleEffortCommand(ctx, req, target, command.Args)
	case "plan", "init", "commit", "push", "review":
		prompt, permissionMode, err := commandRunPrompt(command.Name, command.Args, target.Agent)
		if err != nil {
			return domain.Message{}, err
		}
		return a.createCommandRun(ctx, req, target, prompt, permissionMode, nil)
	default:
		return domain.Message{}, ErrUnknownCommand
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
		CreatedAt:        time.Now().UTC(),
	}
	if err := a.store.Messages().Create(ctx, message); err != nil {
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
