package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/store"
	"golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")

var ErrAlreadyBootstrapped = errors.New("already bootstrapped")

const sessionTTL = 30 * 24 * time.Hour

type AuthStatus struct {
	SetupRequired      bool `json:"setup_required"`
	SetupTokenRequired bool `json:"setup_token_required"`
}

type SetupAdminRequest struct {
	SetupToken  string `json:"setup_token"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ResetAdminRequest struct {
	Username string
	Password string
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

func (a *App) AuthStatus(ctx context.Context) (AuthStatus, error) {
	hasPassword, err := a.store.Users().HasPassword(ctx)
	if err != nil {
		return AuthStatus{}, err
	}
	setupRequired := !hasPassword
	return AuthStatus{
		SetupRequired:      setupRequired,
		SetupTokenRequired: setupRequired,
	}, nil
}

func (a *App) SetupAdmin(ctx context.Context, req SetupAdminRequest) (AuthResult, error) {
	result, err := a.bootstrapAdmin(ctx, req)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{SessionToken: result.SessionToken, User: result.User}, nil
}

func (a *App) Bootstrap(ctx context.Context, req SetupAdminRequest) (BootstrapResult, error) {
	return a.bootstrapAdmin(ctx, req)
}

func (a *App) bootstrapAdmin(ctx context.Context, req SetupAdminRequest) (BootstrapResult, error) {
	configuredToken := strings.TrimSpace(a.opts.AdminToken)
	requestToken := strings.TrimSpace(req.SetupToken)
	if configuredToken == "" || subtle.ConstantTimeCompare([]byte(requestToken), []byte(configuredToken)) != 1 {
		return BootstrapResult{}, ErrUnauthorized
	}
	username, err := normalizeUsername(req.Username)
	if err != nil {
		return BootstrapResult{}, err
	}
	if err := validatePassword(req.Password); err != nil {
		return BootstrapResult{}, err
	}
	passwordHashBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return BootstrapResult{}, err
	}
	passwordHash := string(passwordHashBytes)
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = "Admin"
	}

	now := time.Now().UTC()
	passwordUpdatedAt := now
	user := domain.User{
		ID:                id.New("usr"),
		Username:          username,
		DisplayName:       name,
		PasswordHash:      passwordHash,
		PasswordUpdatedAt: &passwordUpdatedAt,
		CreatedAt:         now,
	}
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
	tokenHash := hashSessionToken(token)
	expiresAt := now.Add(sessionTTL)
	freshBootstrap := true

	err = a.store.Tx(ctx, func(tx store.Tx) error {
		hasPassword, err := tx.Users().HasPassword(ctx)
		if err != nil {
			return err
		}
		if hasPassword {
			return ErrAlreadyBootstrapped
		}
		existingUser, err := tx.Users().First(ctx)
		switch {
		case err == nil:
			freshBootstrap = false
			user.ID = existingUser.ID
			user.CreatedAt = existingUser.CreatedAt
			if err := tx.Users().SetCredentials(ctx, user.ID, username, name, passwordHash, passwordUpdatedAt); err != nil {
				return err
			}
			user.PasswordHash = passwordHash
			user.PasswordUpdatedAt = &passwordUpdatedAt
			return tx.Users().CreateAPISession(ctx, tokenHash, user.ID, now, expiresAt)
		case errors.Is(err, sql.ErrNoRows):
		default:
			return err
		}

		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}
		if err := tx.Users().CreateAPISession(ctx, tokenHash, user.ID, now, expiresAt); err != nil {
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
		if err := ensureAgentInstructionFiles(workspace.Path, agent); err != nil {
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

	if !freshBootstrap {
		return BootstrapResult{
			SessionToken: token,
			User:         user,
		}, nil
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

func (a *App) Login(ctx context.Context, req LoginRequest) (AuthResult, error) {
	username, err := normalizeUsername(req.Username)
	if err != nil {
		return AuthResult{}, ErrUnauthorized
	}
	user, err := a.store.Users().ByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AuthResult{}, ErrUnauthorized
		}
		return AuthResult{}, err
	}
	if user.PasswordHash == "" {
		return AuthResult{}, ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return AuthResult{}, ErrUnauthorized
	}
	now := time.Now().UTC()
	token := id.NewToken()
	if err := a.store.Users().CreateAPISession(ctx, hashSessionToken(token), user.ID, now, now.Add(sessionTTL)); err != nil {
		return AuthResult{}, err
	}
	return AuthResult{SessionToken: token, User: user}, nil
}

func (a *App) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrUnauthorized
	}
	return a.store.Users().DeleteAPISession(ctx, hashSessionToken(token))
}

func (a *App) ResetAdmin(ctx context.Context, req ResetAdminRequest) (domain.User, error) {
	username, err := normalizeUsername(req.Username)
	if err != nil {
		return domain.User{}, err
	}
	if err := validatePassword(req.Password); err != nil {
		return domain.User{}, err
	}
	passwordHashBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	passwordHash := string(passwordHashBytes)
	updatedAt := time.Now().UTC()
	var user domain.User

	err = a.store.Tx(ctx, func(tx store.Tx) error {
		existing, err := tx.Users().First(ctx)
		if err != nil {
			return err
		}
		displayName := strings.TrimSpace(existing.DisplayName)
		if displayName == "" {
			displayName = "Admin"
		}
		if err := tx.Users().SetCredentials(ctx, existing.ID, username, displayName, passwordHash, updatedAt); err != nil {
			return err
		}
		if err := tx.Users().DeleteAllAPISessions(ctx); err != nil {
			return err
		}
		existing.Username = username
		existing.DisplayName = displayName
		existing.PasswordHash = passwordHash
		existing.PasswordUpdatedAt = &updatedAt
		user = existing
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (a *App) UserForToken(ctx context.Context, token string) (domain.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.User{}, ErrUnauthorized
	}
	userID, err := a.store.Users().UserIDByAPISessionHash(ctx, hashSessionToken(token), time.Now().UTC())
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

func normalizeUsername(value string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(value))
	if len(username) < 3 || len(username) > 32 {
		return "", invalidInput("username must be 3-32 characters")
	}
	for _, r := range username {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return "", invalidInput("username may only contain lowercase letters, numbers, dots, underscores, or hyphens")
		}
	}
	return username, nil
}

func validatePassword(password string) error {
	passwordBytes := len([]byte(password))
	if passwordBytes < 12 {
		return invalidInput("password must be at least 12 bytes")
	}
	if passwordBytes > 72 {
		return invalidInput("password must be no more than 72 bytes")
	}
	return nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
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
