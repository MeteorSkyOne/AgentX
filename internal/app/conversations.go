package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

var ErrEmptyMessage = errors.New("empty message")

var ErrInvalidInput = errors.New("invalid input")

type SendMessageRequest struct {
	UserID           string
	OrganizationID   string
	ConversationType domain.ConversationType
	ConversationID   string
	Body             string
	ReplyToMessageID string
	Attachments      []AttachmentUpload
}

type ConversationAgentContext struct {
	Binding         domain.ChannelAgent `json:"binding"`
	Agent           domain.Agent        `json:"agent"`
	ConfigWorkspace domain.Workspace    `json:"config_workspace"`
	RunWorkspace    domain.Workspace    `json:"run_workspace"`
}

type AgentChannelContext struct {
	Binding      domain.ChannelAgent `json:"binding"`
	Channel      domain.Channel      `json:"channel"`
	Project      domain.Project      `json:"project"`
	RunWorkspace domain.Workspace    `json:"run_workspace"`
}

type ConversationContext struct {
	Project   domain.Project             `json:"project"`
	Channel   domain.Channel             `json:"channel"`
	Thread    *domain.Thread             `json:"thread,omitempty"`
	Agents    []ConversationAgentContext `json:"agents"`
	Binding   domain.ConversationBinding `json:"binding,omitempty"`
	Agent     domain.Agent               `json:"agent,omitempty"`
	Workspace domain.Workspace           `json:"workspace,omitempty"`
}

func (a *App) ListOrganizations(ctx context.Context, userID string) ([]domain.Organization, error) {
	return a.store.Organizations().ListForUser(ctx, userID)
}

func (a *App) ListChannels(ctx context.Context, orgID string) ([]domain.Channel, error) {
	return a.store.Channels().ListByOrganization(ctx, orgID)
}

func (a *App) ListProjectChannels(ctx context.Context, projectID string) ([]domain.Channel, error) {
	return a.store.Channels().ListByProject(ctx, projectID)
}

func (a *App) ListMessages(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	messages, err := a.store.Messages().List(ctx, conversationType, conversationID, limit)
	if err != nil {
		return nil, err
	}
	return a.resolveMessageReferences(ctx, messages)
}

func (a *App) ListRecentMessages(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	messages, err := a.store.Messages().ListRecent(ctx, conversationType, conversationID, limit)
	if err != nil {
		return nil, err
	}
	return a.resolveMessageReferences(ctx, messages)
}

func (a *App) ListRecentMessagesBefore(ctx context.Context, conversationType domain.ConversationType, conversationID string, before time.Time, limit int) ([]domain.Message, error) {
	messages, err := a.store.Messages().ListRecentBefore(ctx, conversationType, conversationID, before, limit)
	if err != nil {
		return nil, err
	}
	return a.resolveMessageReferences(ctx, messages)
}

func (a *App) Message(ctx context.Context, id string) (domain.Message, error) {
	message, err := a.store.Messages().ByID(ctx, id)
	if err != nil {
		return domain.Message{}, err
	}
	return a.resolveMessageReference(ctx, message)
}

func (a *App) ConversationBinding(ctx context.Context, conversationType domain.ConversationType, conversationID string) (domain.ConversationBinding, error) {
	return a.store.Bindings().ByConversation(ctx, conversationType, conversationID)
}

func (a *App) ConversationContext(ctx context.Context, conversationType domain.ConversationType, conversationID string) (ConversationContext, error) {
	scope, err := a.conversationScope(ctx, conversationType, conversationID)
	if err != nil {
		return ConversationContext{}, err
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		return ConversationContext{}, err
	}

	result := ConversationContext{
		Project: scope.project,
		Channel: scope.channel,
		Thread:  scope.thread,
		Agents:  agents,
	}

	if len(agents) > 0 {
		result.Agent = agents[0].Agent
		result.Workspace = agents[0].ConfigWorkspace
		result.Binding = domain.ConversationBinding{
			OrganizationID:   scope.organizationID,
			ConversationType: conversationType,
			ConversationID:   conversationID,
			AgentID:          agents[0].Agent.ID,
			WorkspaceID:      agents[0].ConfigWorkspace.ID,
			CreatedAt:        agents[0].Binding.CreatedAt,
			UpdatedAt:        agents[0].Binding.UpdatedAt,
		}
	}

	return result, nil
}

func (a *App) SendMessage(ctx context.Context, req SendMessageRequest) (domain.Message, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" && len(req.Attachments) == 0 {
		return domain.Message{}, ErrEmptyMessage
	}
	scope, err := a.conversationScope(ctx, req.ConversationType, req.ConversationID)
	if err != nil {
		return domain.Message{}, err
	}
	req.ReplyToMessageID = strings.TrimSpace(req.ReplyToMessageID)
	if err := a.validateReplyTarget(ctx, req); err != nil {
		return domain.Message{}, err
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		return domain.Message{}, err
	}

	if command, ok, err := parseSlashCommand(body); ok {
		if err != nil {
			return domain.Message{}, err
		}
		if len(req.Attachments) > 0 {
			return domain.Message{}, invalidInput("slash commands cannot include attachments")
		}
		return a.dispatchSlashCommand(ctx, req, agents, command)
	}

	message, err := a.createConversationMessage(ctx, req, domain.SenderUser, req.UserID, body, nil)
	if err != nil {
		return domain.Message{}, err
	}

	for _, agent := range targetAgentsForBody(agents, message.Body) {
		go a.runAgentForMessage(context.WithoutCancel(ctx), message, agent)
	}

	return message, nil
}

func (a *App) UpdateMessage(ctx context.Context, messageID string, body string) (domain.Message, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.Message{}, ErrEmptyMessage
	}
	message, err := a.store.Messages().ByID(ctx, messageID)
	if err != nil {
		return domain.Message{}, err
	}
	message.Body = body
	if err := a.store.Messages().Update(ctx, message); err != nil {
		return domain.Message{}, err
	}
	message, err = a.resolveMessageReference(ctx, message)
	if err != nil {
		return domain.Message{}, err
	}
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventMessageUpdated,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.MessageUpdatedPayload{Message: message},
	})
	return message, nil
}

func (a *App) validateReplyTarget(ctx context.Context, req SendMessageRequest) error {
	if req.ReplyToMessageID == "" {
		return nil
	}
	referenced, err := a.store.Messages().ByID(ctx, req.ReplyToMessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidInput
		}
		return err
	}
	if referenced.OrganizationID != req.OrganizationID ||
		referenced.ConversationType != req.ConversationType ||
		referenced.ConversationID != req.ConversationID {
		return ErrInvalidInput
	}
	return nil
}

func (a *App) resolveMessageReferences(ctx context.Context, messages []domain.Message) ([]domain.Message, error) {
	for i := range messages {
		resolved, err := a.resolveMessageReference(ctx, messages[i])
		if err != nil {
			return nil, err
		}
		messages[i] = resolved
	}
	return messages, nil
}

func (a *App) resolveMessageReference(ctx context.Context, message domain.Message) (domain.Message, error) {
	attachments, err := a.store.MessageAttachments().ListByMessage(ctx, message.ID)
	if err != nil {
		return domain.Message{}, err
	}
	message.Attachments = attachments
	message.ReplyTo = nil
	message.ReplyToMessageID = strings.TrimSpace(message.ReplyToMessageID)
	if message.ReplyToMessageID == "" {
		return message, nil
	}
	referenced, err := a.store.Messages().ByID(ctx, message.ReplyToMessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			message.ReplyTo = &domain.MessageReference{MessageID: message.ReplyToMessageID, Deleted: true}
			return message, nil
		}
		return domain.Message{}, err
	}
	if referenced.OrganizationID != message.OrganizationID ||
		referenced.ConversationType != message.ConversationType ||
		referenced.ConversationID != message.ConversationID {
		message.ReplyTo = &domain.MessageReference{MessageID: message.ReplyToMessageID, Deleted: true}
		return message, nil
	}
	referencedAttachments, err := a.store.MessageAttachments().ListByMessage(ctx, referenced.ID)
	if err != nil {
		return domain.Message{}, err
	}
	referenced.Attachments = referencedAttachments
	message.ReplyTo = messageReferenceFromMessage(referenced)
	return message, nil
}

func messageReferenceFromMessage(message domain.Message) *domain.MessageReference {
	createdAt := message.CreatedAt
	return &domain.MessageReference{
		MessageID:       message.ID,
		SenderType:      message.SenderType,
		SenderID:        message.SenderID,
		Body:            message.Body,
		AttachmentCount: len(message.Attachments),
		CreatedAt:       &createdAt,
	}
}

func (a *App) DeleteMessage(ctx context.Context, messageID string) error {
	message, err := a.store.Messages().ByID(ctx, messageID)
	if err != nil {
		return err
	}
	attachments, err := a.store.MessageAttachments().ListByMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if err := a.store.Messages().Delete(ctx, messageID); err != nil {
		return err
	}
	if err := removeAttachmentFiles(attachments); err != nil {
		slog.Warn("failed to remove attachment files", "message_id", messageID, "error", err)
	}
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventMessageDeleted,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.MessageDeletedPayload{MessageID: message.ID},
	})
	return nil
}

type conversationScope struct {
	organizationID string
	project        domain.Project
	channel        domain.Channel
	thread         *domain.Thread
	legacyBinding  *domain.ConversationBinding
}

func (a *App) conversationScope(ctx context.Context, conversationType domain.ConversationType, conversationID string) (conversationScope, error) {
	switch conversationType {
	case domain.ConversationChannel:
		channel, err := a.store.Channels().ByID(ctx, conversationID)
		if err != nil {
			return conversationScope{}, err
		}
		project, err := a.store.Projects().ByID(ctx, channel.ProjectID)
		if err != nil {
			return conversationScope{}, err
		}
		return conversationScope{
			organizationID: channel.OrganizationID,
			project:        project,
			channel:        channel,
		}, nil
	case domain.ConversationThread:
		thread, err := a.store.Threads().ByID(ctx, conversationID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				binding, err := a.store.Bindings().ByConversation(ctx, conversationType, conversationID)
				if err != nil {
					return conversationScope{}, err
				}
				return conversationScope{organizationID: binding.OrganizationID, legacyBinding: &binding}, nil
			}
			return conversationScope{}, err
		}
		channel, err := a.store.Channels().ByID(ctx, thread.ChannelID)
		if err != nil {
			return conversationScope{}, err
		}
		project, err := a.store.Projects().ByID(ctx, thread.ProjectID)
		if err != nil {
			return conversationScope{}, err
		}
		return conversationScope{
			organizationID: thread.OrganizationID,
			project:        project,
			channel:        channel,
			thread:         &thread,
		}, nil
	default:
		binding, err := a.store.Bindings().ByConversation(ctx, conversationType, conversationID)
		if err != nil {
			return conversationScope{}, err
		}
		return conversationScope{organizationID: binding.OrganizationID, legacyBinding: &binding}, nil
	}
}

func (a *App) conversationAgents(ctx context.Context, scope conversationScope) ([]ConversationAgentContext, error) {
	if scope.channel.ID == "" {
		if scope.legacyBinding == nil {
			return nil, nil
		}
		agent, err := a.store.Agents().ByID(ctx, scope.legacyBinding.AgentID)
		if err != nil {
			return nil, err
		}
		if !agent.Enabled {
			return nil, nil
		}
		configWorkspaceID := agent.ConfigWorkspaceID
		if configWorkspaceID == "" {
			configWorkspaceID = agent.DefaultWorkspaceID
		}
		configWorkspace, err := a.store.Workspaces().ByID(ctx, configWorkspaceID)
		if err != nil {
			return nil, err
		}
		runWorkspace, err := a.store.Workspaces().ByID(ctx, scope.legacyBinding.WorkspaceID)
		if err != nil {
			return nil, err
		}
		return []ConversationAgentContext{{
			Binding: domain.ChannelAgent{
				AgentID:        scope.legacyBinding.AgentID,
				RunWorkspaceID: scope.legacyBinding.WorkspaceID,
				CreatedAt:      scope.legacyBinding.CreatedAt,
				UpdatedAt:      scope.legacyBinding.UpdatedAt,
			},
			Agent:           agent,
			ConfigWorkspace: configWorkspace,
			RunWorkspace:    runWorkspace,
		}}, nil
	}

	bindings, err := a.store.ChannelAgents().ListByChannel(ctx, scope.channel.ID)
	if err != nil {
		return nil, err
	}
	result := make([]ConversationAgentContext, 0, len(bindings))
	for _, binding := range bindings {
		agent, err := a.store.Agents().ByID(ctx, binding.AgentID)
		if err != nil {
			return nil, err
		}
		if !agent.Enabled {
			continue
		}
		configWorkspaceID := agent.ConfigWorkspaceID
		if configWorkspaceID == "" {
			configWorkspaceID = agent.DefaultWorkspaceID
		}
		configWorkspace, err := a.store.Workspaces().ByID(ctx, configWorkspaceID)
		if err != nil {
			return nil, err
		}
		runWorkspaceID := binding.RunWorkspaceID
		if runWorkspaceID == "" {
			runWorkspaceID = scope.project.WorkspaceID
		}
		runWorkspace, err := a.store.Workspaces().ByID(ctx, runWorkspaceID)
		if err != nil {
			return nil, err
		}
		result = append(result, ConversationAgentContext{
			Binding:         binding,
			Agent:           agent,
			ConfigWorkspace: configWorkspace,
			RunWorkspace:    runWorkspace,
		})
	}
	return result, nil
}

func targetAgentsForBody(agents []ConversationAgentContext, body string) []ConversationAgentContext {
	known := make(map[string]ConversationAgentContext, len(agents))
	for _, agent := range agents {
		if agent.Agent.Handle != "" {
			known[strings.ToLower(agent.Agent.Handle)] = agent
		}
	}

	targetsByID := make(map[string]ConversationAgentContext)
	for _, mention := range agentMentions(body) {
		if agent, ok := known[strings.ToLower(mention)]; ok {
			targetsByID[agent.Agent.ID] = agent
		}
	}
	if len(targetsByID) == 0 {
		return agents
	}

	targets := make([]ConversationAgentContext, 0, len(targetsByID))
	for _, agent := range agents {
		if target, ok := targetsByID[agent.Agent.ID]; ok {
			targets = append(targets, target)
		}
	}
	return targets
}

func agentMentions(body string) []string {
	var mentions []string
	for i := 0; i < len(body); i++ {
		if body[i] != '@' {
			continue
		}
		start := i + 1
		end := start
		for end < len(body) {
			ch := body[end]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
				end++
				continue
			}
			break
		}
		if end > start {
			mentions = append(mentions, body[start:end])
		}
		i = end
	}
	return mentions
}
