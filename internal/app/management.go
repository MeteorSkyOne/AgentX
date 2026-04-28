package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/store"
)

const (
	DefaultChannelTeamMaxBatches = 6
	DefaultChannelTeamMaxRuns    = 12
	MinChannelTeamMaxBatches     = 1
	MaxChannelTeamMaxBatches     = 20
	MinChannelTeamMaxRuns        = 1
	MaxChannelTeamMaxRuns        = 50
)

func (a *App) Project(ctx context.Context, id string) (domain.Project, error) {
	return a.store.Projects().ByID(ctx, id)
}

func (a *App) ListProjects(ctx context.Context, orgID string) ([]domain.Project, error) {
	return a.store.Projects().ListByOrganization(ctx, orgID)
}

type ProjectCreateRequest struct {
	Name          string
	WorkspacePath string
}

func (a *App) CreateProject(ctx context.Context, userID string, orgID string, req ProjectCreateRequest) (domain.Project, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return domain.Project{}, ErrInvalidInput
	}

	now := time.Now().UTC()
	projectID := id.New("prj")
	workspacePath := strings.TrimSpace(req.WorkspacePath)
	if workspacePath == "" {
		workspacePath = filepath.Join(a.opts.DataDir, "projects", projectID)
	}
	workspace := domain.Workspace{
		ID:             id.New("wks"),
		OrganizationID: orgID,
		Type:           "project",
		Name:           name + " Workspace",
		Path:           workspacePath,
		CreatedBy:      userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	project := domain.Project{
		ID:             projectID,
		OrganizationID: orgID,
		Name:           name,
		WorkspaceID:    workspace.ID,
		CreatedBy:      userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	err := a.store.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Workspaces().Create(ctx, workspace); err != nil {
			return err
		}
		return tx.Projects().Create(ctx, project)
	})
	if err != nil {
		return domain.Project{}, err
	}
	_ = os.MkdirAll(workspace.Path, 0o755)
	return project, nil
}

type ProjectUpdateRequest struct {
	Name          *string
	WorkspacePath *string
}

func (a *App) UpdateProject(ctx context.Context, projectID string, req ProjectUpdateRequest) (domain.Project, error) {
	project, err := a.store.Projects().ByID(ctx, projectID)
	if err != nil {
		return domain.Project{}, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return domain.Project{}, ErrInvalidInput
		}
		project.Name = name
	}
	now := time.Now().UTC()
	project.UpdatedAt = now
	if req.WorkspacePath != nil {
		wsPath := strings.TrimSpace(*req.WorkspacePath)
		if wsPath == "" {
			return domain.Project{}, ErrInvalidInput
		}
		workspace, err := a.store.Workspaces().ByID(ctx, project.WorkspaceID)
		if err != nil {
			return domain.Project{}, err
		}
		workspace.Path = wsPath
		workspace.UpdatedAt = now
		if err := a.store.Workspaces().Update(ctx, workspace); err != nil {
			return domain.Project{}, err
		}
		_ = os.MkdirAll(wsPath, 0o755)
	}
	if err := a.store.Projects().Update(ctx, project); err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func (a *App) DeleteProject(ctx context.Context, projectID string) error {
	return a.store.Projects().Delete(ctx, projectID)
}

func (a *App) Channel(ctx context.Context, id string) (domain.Channel, error) {
	return a.store.Channels().ByID(ctx, id)
}

type ChannelTeamBudgetUpdate struct {
	MaxBatches *int
	MaxRuns    *int
}

func (a *App) CreateChannel(ctx context.Context, projectID string, name string, channelType domain.ChannelType, budgetUpdates ...ChannelTeamBudgetUpdate) (domain.Channel, error) {
	project, err := a.store.Projects().ByID(ctx, projectID)
	if err != nil {
		return domain.Channel{}, err
	}
	name = strings.TrimSpace(strings.TrimPrefix(name, "#"))
	if name == "" {
		return domain.Channel{}, ErrInvalidInput
	}
	if channelType == "" {
		channelType = domain.ChannelTypeText
	}
	if channelType != domain.ChannelTypeText && channelType != domain.ChannelTypeThread {
		return domain.Channel{}, ErrInvalidInput
	}
	now := time.Now().UTC()
	channel := domain.Channel{
		ID:             id.New("chn"),
		OrganizationID: project.OrganizationID,
		ProjectID:      project.ID,
		Type:           channelType,
		Name:           name,
		TeamMaxBatches: DefaultChannelTeamMaxBatches,
		TeamMaxRuns:    DefaultChannelTeamMaxRuns,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := applyChannelTeamBudget(&channel, optionalChannelTeamBudgetUpdate(budgetUpdates)); err != nil {
		return domain.Channel{}, err
	}
	if err := a.store.Channels().Create(ctx, channel); err != nil {
		return domain.Channel{}, err
	}
	return channel, nil
}

func (a *App) UpdateChannel(ctx context.Context, channelID string, name string, channelType domain.ChannelType, budgetUpdates ...ChannelTeamBudgetUpdate) (domain.Channel, error) {
	channel, err := a.store.Channels().ByID(ctx, channelID)
	if err != nil {
		return domain.Channel{}, err
	}
	name = strings.TrimSpace(strings.TrimPrefix(name, "#"))
	if name == "" {
		return domain.Channel{}, ErrInvalidInput
	}
	if channelType == "" {
		channelType = channel.Type
	}
	if channelType != domain.ChannelTypeText && channelType != domain.ChannelTypeThread {
		return domain.Channel{}, ErrInvalidInput
	}
	channel.Name = name
	channel.Type = channelType
	channel.UpdatedAt = time.Now().UTC()
	if err := applyChannelTeamBudget(&channel, optionalChannelTeamBudgetUpdate(budgetUpdates)); err != nil {
		return domain.Channel{}, err
	}
	if err := a.store.Channels().Update(ctx, channel); err != nil {
		return domain.Channel{}, err
	}
	return channel, nil
}

func optionalChannelTeamBudgetUpdate(updates []ChannelTeamBudgetUpdate) ChannelTeamBudgetUpdate {
	if len(updates) == 0 {
		return ChannelTeamBudgetUpdate{}
	}
	return updates[0]
}

func applyChannelTeamBudget(channel *domain.Channel, update ChannelTeamBudgetUpdate) error {
	if channel.TeamMaxBatches <= 0 {
		channel.TeamMaxBatches = DefaultChannelTeamMaxBatches
	}
	if channel.TeamMaxRuns <= 0 {
		channel.TeamMaxRuns = DefaultChannelTeamMaxRuns
	}
	if update.MaxBatches != nil {
		channel.TeamMaxBatches = *update.MaxBatches
	}
	if update.MaxRuns != nil {
		channel.TeamMaxRuns = *update.MaxRuns
	}
	return validateChannelTeamBudget(channel.TeamMaxBatches, channel.TeamMaxRuns)
}

func validateChannelTeamBudget(maxBatches int, maxRuns int) error {
	if maxBatches < MinChannelTeamMaxBatches || maxBatches > MaxChannelTeamMaxBatches {
		return invalidInput("team max batches must be between 1 and 20")
	}
	if maxRuns < MinChannelTeamMaxRuns || maxRuns > MaxChannelTeamMaxRuns {
		return invalidInput("team max runs must be between 1 and 50")
	}
	if maxRuns < maxBatches {
		return invalidInput("team max runs must be greater than or equal to team max batches")
	}
	return nil
}

func (a *App) ArchiveChannel(ctx context.Context, channelID string) error {
	return a.store.Channels().Archive(ctx, channelID, time.Now().UTC())
}

func (a *App) Thread(ctx context.Context, id string) (domain.Thread, error) {
	return a.store.Threads().ByID(ctx, id)
}

func (a *App) ListThreads(ctx context.Context, channelID string) ([]domain.Thread, error) {
	return a.store.Threads().ListByChannel(ctx, channelID)
}

func (a *App) CreateThread(ctx context.Context, userID string, channelID string, title string, body string) (domain.Thread, domain.Message, error) {
	channel, err := a.store.Channels().ByID(ctx, channelID)
	if err != nil {
		return domain.Thread{}, domain.Message{}, err
	}
	if channel.Type != domain.ChannelTypeThread {
		return domain.Thread{}, domain.Message{}, ErrInvalidInput
	}
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" || body == "" {
		return domain.Thread{}, domain.Message{}, ErrInvalidInput
	}
	now := time.Now().UTC()
	thread := domain.Thread{
		ID:             id.New("thr"),
		OrganizationID: channel.OrganizationID,
		ProjectID:      channel.ProjectID,
		ChannelID:      channel.ID,
		Title:          title,
		CreatedBy:      userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	message := domain.Message{
		ID:               id.New("msg"),
		OrganizationID:   channel.OrganizationID,
		ConversationType: domain.ConversationThread,
		ConversationID:   thread.ID,
		SenderType:       domain.SenderUser,
		SenderID:         userID,
		Kind:             domain.MessageText,
		Body:             body,
		CreatedAt:        now,
	}

	if err := a.store.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Threads().Create(ctx, thread); err != nil {
			return err
		}
		return tx.Messages().Create(ctx, message)
	}); err != nil {
		return domain.Thread{}, domain.Message{}, err
	}

	a.publishConversationEvent(domain.Event{
		Type:             domain.EventMessageCreated,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.MessageCreatedPayload{Message: message},
	})

	scope, err := a.conversationScope(ctx, domain.ConversationThread, thread.ID)
	if err == nil {
		if agents, resolveErr := a.conversationAgents(ctx, scope); resolveErr == nil {
			a.dispatchAgentRunsForMessage(context.WithoutCancel(ctx), message, scope, agents)
		}
	}

	return thread, message, nil
}

func (a *App) UpdateThread(ctx context.Context, threadID string, title string) (domain.Thread, error) {
	thread, err := a.store.Threads().ByID(ctx, threadID)
	if err != nil {
		return domain.Thread{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return domain.Thread{}, ErrInvalidInput
	}
	thread.Title = title
	if err := a.store.Threads().Update(ctx, thread); err != nil {
		return domain.Thread{}, err
	}
	return thread, nil
}

func (a *App) ArchiveThread(ctx context.Context, threadID string) error {
	return a.store.Threads().Archive(ctx, threadID, time.Now().UTC())
}

func (a *App) ListAgents(ctx context.Context, orgID string) ([]domain.Agent, error) {
	return a.store.Agents().ListByOrganization(ctx, orgID)
}

func (a *App) Agent(ctx context.Context, id string) (domain.Agent, error) {
	return a.store.Agents().ByID(ctx, id)
}

func (a *App) Workspace(ctx context.Context, id string) (domain.Workspace, error) {
	return a.store.Workspaces().ByID(ctx, id)
}

type AgentCreateRequest struct {
	UserID         string
	OrganizationID string
	Name           string
	Description    string
	Handle         string
	Kind           string
	Model          string
	Effort         string
	FastMode       bool
	YoloMode       bool
	Env            map[string]string
}

func (a *App) CreateAgent(ctx context.Context, req AgentCreateRequest) (domain.Agent, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return domain.Agent{}, ErrInvalidInput
	}
	handle := normalizeHandle(req.Handle)
	if handle == "" {
		handle = normalizeHandle(name)
	}
	if handle == "" {
		return domain.Agent{}, ErrInvalidInput
	}
	description := strings.TrimSpace(req.Description)
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = domain.AgentKindFake
	}
	now := time.Now().UTC()
	bot := domain.BotUser{ID: id.New("bot"), OrganizationID: req.OrganizationID, DisplayName: name, CreatedAt: now}
	workspace := domain.Workspace{
		ID:             id.New("wks"),
		OrganizationID: req.OrganizationID,
		Type:           "agent_default",
		Name:           name + " Workspace",
		Path:           filepath.Join(a.opts.DataDir, "agents", handle),
		CreatedBy:      req.UserID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	agent := domain.Agent{
		ID:                 id.New("agt"),
		OrganizationID:     req.OrganizationID,
		BotUserID:          bot.ID,
		Kind:               kind,
		Name:               name,
		Handle:             handle,
		Description:        description,
		Model:              strings.TrimSpace(req.Model),
		Effort:             strings.TrimSpace(req.Effort),
		ConfigWorkspaceID:  workspace.ID,
		DefaultWorkspaceID: workspace.ID,
		Enabled:            true,
		FastMode:           req.FastMode,
		YoloMode:           req.YoloMode,
		Env:                copyStringMap(req.Env),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	err := a.store.Tx(ctx, func(tx store.Tx) error {
		if _, err := tx.Agents().ByHandle(ctx, req.OrganizationID, handle); err == nil {
			return ErrInvalidInput
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := tx.BotUsers().Create(ctx, bot); err != nil {
			return err
		}
		if err := tx.Workspaces().Create(ctx, workspace); err != nil {
			return err
		}
		if err := tx.Agents().Create(ctx, agent); err != nil {
			return err
		}
		return ensureAgentInstructionFiles(workspace.Path, agent)
	})
	if err != nil {
		return domain.Agent{}, err
	}
	return agent, nil
}

type AgentUpdateRequest struct {
	Name        *string
	Description *string
	Handle      *string
	Kind        *string
	Model       *string
	Effort      *string
	Enabled     *bool
	FastMode    *bool
	YoloMode    *bool
	Env         map[string]string
	EnvSet      bool
}

func (a *App) UpdateAgent(ctx context.Context, agentID string, req AgentUpdateRequest) (domain.Agent, error) {
	agent, err := a.store.Agents().ByID(ctx, agentID)
	if err != nil {
		return domain.Agent{}, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return domain.Agent{}, ErrInvalidInput
		}
		agent.Name = name
	}
	if req.Description != nil {
		agent.Description = strings.TrimSpace(*req.Description)
	}
	if req.Handle != nil {
		handle := normalizeHandle(*req.Handle)
		if handle == "" {
			return domain.Agent{}, ErrInvalidInput
		}
		if handle != agent.Handle {
			if _, err := a.store.Agents().ByHandle(ctx, agent.OrganizationID, handle); err == nil {
				return domain.Agent{}, ErrInvalidInput
			} else if !errors.Is(err, sql.ErrNoRows) {
				return domain.Agent{}, err
			}
		}
		agent.Handle = handle
	}
	if req.Kind != nil {
		kind := strings.TrimSpace(*req.Kind)
		if kind == "" {
			return domain.Agent{}, ErrInvalidInput
		}
		agent.Kind = kind
	}
	if req.Model != nil {
		agent.Model = strings.TrimSpace(*req.Model)
	}
	if req.Effort != nil {
		agent.Effort = strings.TrimSpace(*req.Effort)
	}
	if req.Enabled != nil {
		agent.Enabled = *req.Enabled
	}
	if req.FastMode != nil {
		agent.FastMode = *req.FastMode
	}
	if req.YoloMode != nil {
		agent.YoloMode = *req.YoloMode
	}
	if req.EnvSet {
		agent.Env = copyStringMap(req.Env)
	}
	agent.UpdatedAt = time.Now().UTC()
	if err := a.store.Agents().Update(ctx, agent); err != nil {
		return domain.Agent{}, err
	}
	return agent, nil
}

func (a *App) DeleteAgent(ctx context.Context, agentID string) error {
	agent, err := a.store.Agents().ByID(ctx, agentID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	agent.Enabled = false
	agent.Handle = deletedAgentHandle(agent)
	agent.UpdatedAt = now
	return a.store.Tx(ctx, func(tx store.Tx) error {
		if err := tx.ChannelAgents().DeleteForAgent(ctx, agent.ID); err != nil {
			return err
		}
		return tx.Agents().Update(ctx, agent)
	})
}

func deletedAgentHandle(agent domain.Agent) string {
	handle := normalizeHandle(agent.Handle)
	if handle == "" {
		handle = normalizeHandle(agent.Name)
	}
	if handle == "" {
		handle = "agent"
	}
	return normalizeHandle(handle + "_deleted_" + agent.ID)
}

func ensureAgentInstructionFiles(workspacePath string, agent domain.Agent) error {
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		return err
	}
	defaultContent := agentMemoryContent(agent)
	memoryPath := filepath.Join(workspacePath, "memory.md")
	if err := writeFileIfMissing(memoryPath, defaultContent); err != nil {
		return err
	}
	instructionContent := defaultContent
	if memoryContent, err := os.ReadFile(memoryPath); err == nil && strings.TrimSpace(string(memoryContent)) != "" {
		instructionContent = string(memoryContent)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(workspacePath, "AGENTS.md"), instructionContent); err != nil {
		return err
	}
	return writeFileIfMissing(filepath.Join(workspacePath, "CLAUDE.md"), "@AGENTS.md\n")
}

func writeFileIfMissing(path string, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func agentMemoryContent(agent domain.Agent) string {
	var b strings.Builder
	b.WriteString("# Agent Memory\n\n")
	b.WriteString("Name: ")
	b.WriteString(strings.TrimSpace(agent.Name))
	b.WriteString("\n")
	b.WriteString("Description: ")
	b.WriteString(strings.TrimSpace(agent.Description))
	b.WriteString("\n")
	return b.String()
}

func (a *App) ChannelAgents(ctx context.Context, channelID string) ([]ConversationAgentContext, error) {
	channel, err := a.store.Channels().ByID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	project, err := a.store.Projects().ByID(ctx, channel.ProjectID)
	if err != nil {
		return nil, err
	}
	return a.conversationAgents(ctx, conversationScope{
		organizationID: channel.OrganizationID,
		project:        project,
		channel:        channel,
	})
}

func (a *App) AgentChannels(ctx context.Context, agentID string) ([]AgentChannelContext, error) {
	agent, err := a.store.Agents().ByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	bindings, err := a.store.ChannelAgents().ListByAgent(ctx, agent.ID)
	if err != nil {
		return nil, err
	}

	result := make([]AgentChannelContext, 0, len(bindings))
	for _, binding := range bindings {
		channel, err := a.store.Channels().ByID(ctx, binding.ChannelID)
		if err != nil {
			return nil, err
		}
		if channel.ArchivedAt != nil {
			continue
		}
		if channel.OrganizationID != agent.OrganizationID {
			return nil, ErrInvalidInput
		}
		project, err := a.store.Projects().ByID(ctx, channel.ProjectID)
		if err != nil {
			return nil, err
		}
		runWorkspaceID := binding.RunWorkspaceID
		if runWorkspaceID == "" {
			runWorkspaceID = project.WorkspaceID
		}
		runWorkspace, err := a.store.Workspaces().ByID(ctx, runWorkspaceID)
		if err != nil {
			return nil, err
		}
		result = append(result, AgentChannelContext{
			Binding:      binding,
			Channel:      channel,
			Project:      project,
			RunWorkspace: runWorkspace,
		})
	}
	return result, nil
}

func (a *App) SetChannelAgents(ctx context.Context, channelID string, bindings []domain.ChannelAgent) ([]ConversationAgentContext, error) {
	channel, err := a.store.Channels().ByID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	normalized := make([]domain.ChannelAgent, 0, len(bindings))
	for _, binding := range bindings {
		agent, err := a.store.Agents().ByID(ctx, binding.AgentID)
		if err != nil {
			return nil, err
		}
		if agent.OrganizationID != channel.OrganizationID {
			return nil, ErrInvalidInput
		}
		if binding.RunWorkspaceID != "" {
			workspace, err := a.store.Workspaces().ByID(ctx, binding.RunWorkspaceID)
			if err != nil {
				return nil, err
			}
			if workspace.OrganizationID != channel.OrganizationID {
				return nil, ErrInvalidInput
			}
		}
		normalized = append(normalized, domain.ChannelAgent{
			ChannelID:      channel.ID,
			AgentID:        agent.ID,
			RunWorkspaceID: binding.RunWorkspaceID,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}
	if err := a.store.ChannelAgents().ReplaceForChannel(ctx, channel.ID, normalized); err != nil {
		return nil, err
	}
	return a.ChannelAgents(ctx, channel.ID)
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		copied[key] = value
	}
	return copied
}

func normalizeHandle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			if b.Len() > 0 {
				b.WriteByte('_')
			}
		case r == ' ' || r == '.':
			if b.Len() > 0 {
				b.WriteByte('_')
			}
		}
	}
	return strings.Trim(b.String(), "_")
}
