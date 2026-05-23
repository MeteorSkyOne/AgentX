package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/store"
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
