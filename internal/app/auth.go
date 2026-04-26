package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/store"
)

var ErrUnauthorized = errors.New("unauthorized")

var ErrAlreadyBootstrapped = errors.New("already bootstrapped")

type BootstrapRequest struct {
	AdminToken  string `json:"admin_token"`
	DisplayName string `json:"display_name"`
}

type BootstrapResult struct {
	SessionToken     string              `json:"session_token"`
	User             domain.User         `json:"user"`
	Organization     domain.Organization `json:"organization"`
	Project          domain.Project      `json:"project"`
	Channel          domain.Channel      `json:"channel"`
	BotUser          domain.BotUser      `json:"bot_user"`
	Agent            domain.Agent        `json:"agent"`
	Workspace        domain.Workspace    `json:"workspace"`
	ProjectWorkspace domain.Workspace    `json:"project_workspace"`
}

type AuthResult struct {
	SessionToken string      `json:"session_token"`
	User         domain.User `json:"user"`
}

func (a *App) Bootstrap(ctx context.Context, req BootstrapRequest) (BootstrapResult, error) {
	configuredToken := strings.TrimSpace(a.opts.AdminToken)
	requestToken := strings.TrimSpace(req.AdminToken)
	if configuredToken == "" || subtle.ConstantTimeCompare([]byte(requestToken), []byte(configuredToken)) != 1 {
		return BootstrapResult{}, ErrUnauthorized
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = "Admin"
	}

	now := time.Now().UTC()
	user := domain.User{ID: id.New("usr"), DisplayName: name, CreatedAt: now}
	org := domain.Organization{ID: id.New("org"), Name: "Default", CreatedAt: now}
	projectWorkspace := domain.Workspace{
		ID:             id.New("wks"),
		OrganizationID: org.ID,
		Type:           "project",
		Name:           "Default Project Workspace",
		Path:           filepath.Join(a.opts.DataDir, "projects", "default"),
		CreatedBy:      user.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	project := domain.Project{
		ID:             id.New("prj"),
		OrganizationID: org.ID,
		Name:           "Default",
		WorkspaceID:    projectWorkspace.ID,
		CreatedBy:      user.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	channel := domain.Channel{
		ID:             id.New("chn"),
		OrganizationID: org.ID,
		ProjectID:      project.ID,
		Type:           domain.ChannelTypeText,
		Name:           "general",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	agentName := a.defaultAgentName()
	agentKind := a.defaultAgentKind()
	agentHandle := strings.ToLower(strings.TrimSpace(agentKind)) + "-default"
	agentDescription := "Default " + agentName + " coding agent"
	if strings.TrimSpace(agentHandle) == "-default" {
		agentHandle = defaultHandle(agentName, agentKind)
	}
	bot := domain.BotUser{ID: id.New("bot"), OrganizationID: org.ID, DisplayName: agentName, CreatedAt: now}
	workspace := domain.Workspace{
		ID:             id.New("wks"),
		OrganizationID: org.ID,
		Type:           "agent_default",
		Name:           agentName + " Workspace",
		Path:           filepath.Join(a.opts.DataDir, "agents", agentHandle),
		CreatedBy:      user.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	agent := domain.Agent{
		ID:                 id.New("agt"),
		OrganizationID:     org.ID,
		BotUserID:          bot.ID,
		Kind:               agentKind,
		Name:               agentName,
		Handle:             agentHandle,
		Description:        agentDescription,
		Model:              a.defaultAgentModel(),
		ConfigWorkspaceID:  workspace.ID,
		DefaultWorkspaceID: workspace.ID,
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	channelAgent := domain.ChannelAgent{
		ChannelID: channel.ID,
		AgentID:   agent.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	binding := domain.ConversationBinding{
		ID:               id.New("bnd"),
		OrganizationID:   org.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   channel.ID,
		AgentID:          agent.ID,
		WorkspaceID:      workspace.ID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	token := id.NewToken()

	err := a.store.Tx(ctx, func(tx store.Tx) error {
		bootstrapped, err := tx.Organizations().Any(ctx)
		if err != nil {
			return err
		}
		if bootstrapped {
			return ErrAlreadyBootstrapped
		}
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}
		if err := tx.Users().CreateAPISession(ctx, token, user.ID); err != nil {
			return err
		}
		if err := tx.Organizations().Create(ctx, org); err != nil {
			return err
		}
		if err := tx.Organizations().AddMember(ctx, org.ID, user.ID, domain.RoleOwner); err != nil {
			return err
		}
		if err := tx.Workspaces().Create(ctx, projectWorkspace); err != nil {
			return err
		}
		if err := tx.Projects().Create(ctx, project); err != nil {
			return err
		}
		if err := tx.Channels().Create(ctx, channel); err != nil {
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
		if err := seedAgentMemoryFile(workspace.Path, agent); err != nil {
			return err
		}
		if err := tx.ChannelAgents().ReplaceForChannel(ctx, channel.ID, []domain.ChannelAgent{channelAgent}); err != nil {
			return err
		}
		return tx.Bindings().Upsert(ctx, binding)
	})
	if err != nil {
		return BootstrapResult{}, err
	}

	return BootstrapResult{
		SessionToken:     token,
		User:             user,
		Organization:     org,
		Project:          project,
		Channel:          channel,
		BotUser:          bot,
		Agent:            agent,
		Workspace:        workspace,
		ProjectWorkspace: projectWorkspace,
	}, nil
}

func (a *App) Login(ctx context.Context, req BootstrapRequest) (AuthResult, error) {
	configuredToken := strings.TrimSpace(a.opts.AdminToken)
	requestToken := strings.TrimSpace(req.AdminToken)
	if configuredToken == "" || subtle.ConstantTimeCompare([]byte(requestToken), []byte(configuredToken)) != 1 {
		return AuthResult{}, ErrUnauthorized
	}

	bootstrapped, err := a.store.Organizations().Any(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	if !bootstrapped {
		result, err := a.Bootstrap(ctx, req)
		if err == nil {
			return AuthResult{SessionToken: result.SessionToken, User: result.User}, nil
		}
		if !errors.Is(err, ErrAlreadyBootstrapped) {
			return AuthResult{}, err
		}
	}

	user, err := a.store.Users().First(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	token := id.NewToken()
	if err := a.store.Users().CreateAPISession(ctx, token, user.ID); err != nil {
		return AuthResult{}, err
	}
	return AuthResult{SessionToken: token, User: user}, nil
}

func (a *App) UserForToken(ctx context.Context, token string) (domain.User, error) {
	userID, err := a.store.Users().UserIDByAPISession(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrUnauthorized
		}
		return domain.User{}, err
	}
	user, err := a.store.Users().ByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrUnauthorized
		}
		return domain.User{}, err
	}
	return user, nil
}

func defaultHandle(name string, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '_' || r == '-':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('_')
				lastDash = true
			}
		case r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('_')
				lastDash = true
			}
		}
	}
	handle := strings.Trim(b.String(), "_")
	if handle == "" {
		handle = strings.ToLower(strings.TrimSpace(fallback))
	}
	if handle == "" {
		return "agent"
	}
	return handle
}
