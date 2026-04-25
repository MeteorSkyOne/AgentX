package app

import (
	"context"
	"crypto/subtle"
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
	SessionToken string              `json:"session_token"`
	User         domain.User         `json:"user"`
	Organization domain.Organization `json:"organization"`
	Channel      domain.Channel      `json:"channel"`
	BotUser      domain.BotUser      `json:"bot_user"`
	Agent        domain.Agent        `json:"agent"`
	Workspace    domain.Workspace    `json:"workspace"`
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
	channel := domain.Channel{ID: id.New("chn"), OrganizationID: org.ID, Name: "general", CreatedAt: now}
	bot := domain.BotUser{ID: id.New("bot"), OrganizationID: org.ID, DisplayName: "Fake Agent", CreatedAt: now}
	workspace := domain.Workspace{
		ID:             id.New("wks"),
		OrganizationID: org.ID,
		Type:           "agent_default",
		Name:           "Fake Agent Workspace",
		Path:           filepath.Join(a.opts.DataDir, "agents", "fake-default"),
		CreatedBy:      user.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	agent := domain.Agent{
		ID:                 id.New("agt"),
		OrganizationID:     org.ID,
		BotUserID:          bot.ID,
		Kind:               "fake",
		Name:               "Fake Agent",
		Model:              "fake-echo",
		DefaultWorkspaceID: workspace.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
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
		return tx.Bindings().Upsert(ctx, binding)
	})
	if err != nil {
		return BootstrapResult{}, err
	}

	return BootstrapResult{
		SessionToken: token,
		User:         user,
		Organization: org,
		Channel:      channel,
		BotUser:      bot,
		Agent:        agent,
		Workspace:    workspace,
	}, nil
}

func (a *App) UserForToken(ctx context.Context, token string) (domain.User, error) {
	userID, err := a.store.Users().UserIDByAPISession(ctx, token)
	if err != nil {
		return domain.User{}, ErrUnauthorized
	}
	return a.store.Users().ByID(ctx, userID)
}
