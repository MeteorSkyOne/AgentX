package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
)

var ErrEmptyMessage = errors.New("empty message")

var ErrInvalidInput = errors.New("invalid input")

type SendMessageRequest struct {
	UserID           string
	OrganizationID   string
	ConversationType domain.ConversationType
	ConversationID   string
	Body             string
}

type ConversationAgentContext struct {
	Binding         domain.ChannelAgent `json:"binding"`
	Agent           domain.Agent        `json:"agent"`
	ConfigWorkspace domain.Workspace    `json:"config_workspace"`
	RunWorkspace    domain.Workspace    `json:"run_workspace"`
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
	return a.store.Messages().List(ctx, conversationType, conversationID, limit)
}

func (a *App) ListRecentMessages(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	return a.store.Messages().ListRecent(ctx, conversationType, conversationID, limit)
}

func (a *App) ListRecentMessagesBefore(ctx context.Context, conversationType domain.ConversationType, conversationID string, before time.Time, limit int) ([]domain.Message, error) {
	return a.store.Messages().ListRecentBefore(ctx, conversationType, conversationID, before, limit)
}

func (a *App) Message(ctx context.Context, id string) (domain.Message, error) {
	return a.store.Messages().ByID(ctx, id)
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
	if body == "" {
		return domain.Message{}, ErrEmptyMessage
	}
	scope, err := a.conversationScope(ctx, req.ConversationType, req.ConversationID)
	if err != nil {
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
		return a.dispatchSlashCommand(ctx, req, agents, command)
	}

	message := domain.Message{
		ID:               id.New("msg"),
		OrganizationID:   req.OrganizationID,
		ConversationType: req.ConversationType,
		ConversationID:   req.ConversationID,
		SenderType:       domain.SenderUser,
		SenderID:         req.UserID,
		Kind:             domain.MessageText,
		Body:             body,
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
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventMessageUpdated,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.MessageUpdatedPayload{Message: message},
	})
	return message, nil
}

func (a *App) DeleteMessage(ctx context.Context, messageID string) error {
	message, err := a.store.Messages().ByID(ctx, messageID)
	if err != nil {
		return err
	}
	if err := a.store.Messages().Delete(ctx, messageID); err != nil {
		return err
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
