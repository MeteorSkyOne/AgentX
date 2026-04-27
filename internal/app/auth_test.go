package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

const (
	testSetupToken = "secret"
	testUsername   = "meteorsky"
	testPassword   = "correct-password-123"
)

func TestAuthStatusReportsSetupRequiredUntilAdminPasswordExists(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	status, err := app.AuthStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.SetupRequired || !status.SetupTokenRequired {
		t.Fatalf("status before setup = %#v", status)
	}

	if _, err := app.SetupAdmin(ctx, setupRequest("Meteorsky")); err != nil {
		t.Fatal(err)
	}
	status, err = app.AuthStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.SetupRequired || status.SetupTokenRequired {
		t.Fatalf("status after setup = %#v", status)
	}
}

func TestSetupAdminCreatesAdminOrgChannelAgentAndWorkspace(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	result, err := app.Bootstrap(ctx, setupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionToken == "" {
		t.Fatal("SessionToken is empty")
	}
	if result.User.Username != testUsername || result.User.DisplayName != "Meteorsky" || result.User.PasswordHash == "" {
		t.Fatalf("user = %#v", result.User)
	}
	if result.Organization.Name != "Default" {
		t.Fatalf("organization = %#v", result.Organization)
	}
	if result.Channel.Name != "general" {
		t.Fatalf("channel = %#v", result.Channel)
	}
	if result.Agent.Kind != "fake" {
		t.Fatalf("agent = %#v", result.Agent)
	}
	if result.Workspace.Type != "agent_default" {
		t.Fatalf("workspace = %#v", result.Workspace)
	}
}

func TestSetupAdminUsesConfiguredDefaultAgent(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{
		AdminToken:        testSetupToken,
		DataDir:           t.TempDir(),
		DefaultAgentKind:  "codex",
		DefaultAgentModel: "gpt-test",
	})
	result, err := app.Bootstrap(ctx, setupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.Kind != "codex" || result.Agent.Name != "Codex" || result.Agent.Model != "gpt-test" {
		t.Fatalf("agent = %#v", result.Agent)
	}
	if result.BotUser.DisplayName != "Codex" {
		t.Fatalf("bot = %#v", result.BotUser)
	}
}

func TestSetupAdminRejectsWrongSetupToken(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	req := setupRequest("Bad")
	req.SetupToken = "wrong"
	_, err := app.SetupAdmin(ctx, req)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestSetupAdminRejectsEmptyConfiguredSetupToken(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "", DataDir: t.TempDir()})
	req := setupRequest("Bad")
	req.SetupToken = ""
	_, err := app.SetupAdmin(ctx, req)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestSetupAdminRejectsRepeatedSetup(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	if _, err := app.SetupAdmin(ctx, setupRequest("First")); err != nil {
		t.Fatal(err)
	}
	_, err := app.SetupAdmin(ctx, setupRequest("Second"))
	if !errors.Is(err, ErrAlreadyBootstrapped) {
		t.Fatalf("error = %v, want %v", err, ErrAlreadyBootstrapped)
	}
}

func TestSetupAdminValidatesUsernameAndPassword(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	for _, tc := range []struct {
		name        string
		req         SetupAdminRequest
		wantMessage string
	}{
		{
			name:        "short username",
			req:         SetupAdminRequest{SetupToken: testSetupToken, Username: "no", Password: testPassword, DisplayName: "Bad"},
			wantMessage: "username must be 3-32 characters",
		},
		{
			name:        "bad username characters",
			req:         SetupAdminRequest{SetupToken: testSetupToken, Username: "bad name", Password: testPassword, DisplayName: "Bad"},
			wantMessage: "username may only contain lowercase letters, numbers, dots, underscores, or hyphens",
		},
		{
			name:        "short password",
			req:         SetupAdminRequest{SetupToken: testSetupToken, Username: testUsername, Password: "too-short", DisplayName: "Bad"},
			wantMessage: "password must be at least 12 bytes",
		},
		{
			name:        "long password",
			req:         SetupAdminRequest{SetupToken: testSetupToken, Username: testUsername, Password: string(make([]byte, 73)), DisplayName: "Bad"},
			wantMessage: "password must be no more than 72 bytes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := app.SetupAdmin(ctx, tc.req)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("SetupAdmin(%#v) error = %v, want %v", tc.req, err, ErrInvalidInput)
			}
			if got := InvalidInputMessage(err); got != tc.wantMessage {
				t.Fatalf("InvalidInputMessage() = %q, want %q", got, tc.wantMessage)
			}
		})
	}
}

func TestSetupAdminRejectsConcurrentSetupAttempts(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})

	const attempts = 8
	var wg sync.WaitGroup
	type setupAttempt struct {
		result AuthResult
		err    error
	}
	results := make([]setupAttempt, attempts)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := app.SetupAdmin(ctx, setupRequest("Meteorsky"))
			results[i] = setupAttempt{result: result, err: err}
		}(i)
	}
	wg.Wait()

	successes := 0
	var successfulUserID string
	for _, result := range results {
		switch {
		case result.err == nil:
			successes++
			successfulUserID = result.result.User.ID
		case errors.Is(result.err, ErrAlreadyBootstrapped):
		default:
			t.Fatalf("unexpected error: %v", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want 1; results = %#v", successes, results)
	}

	orgs, err := st.Organizations().ListForUser(ctx, successfulUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 {
		t.Fatalf("organizations for successful user = %#v, want exactly one", orgs)
	}
}

func TestSetupAdminCompletesLegacyTokenOnlyDatabase(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	legacyUser := domain.User{ID: "usr_legacy", DisplayName: "Legacy Admin", CreatedAt: now}
	org := domain.Organization{ID: "org_legacy", Name: "Legacy Org", CreatedAt: now}
	if err := st.Users().Create(ctx, legacyUser); err != nil {
		t.Fatal(err)
	}
	if err := st.Organizations().Create(ctx, org); err != nil {
		t.Fatal(err)
	}
	if err := st.Organizations().AddMember(ctx, org.ID, legacyUser.ID, domain.RoleOwner); err != nil {
		t.Fatal(err)
	}

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	result, err := app.SetupAdmin(ctx, setupRequest("Updated Admin"))
	if err != nil {
		t.Fatal(err)
	}
	if result.User.ID != legacyUser.ID || result.User.Username != testUsername || result.User.DisplayName != "Updated Admin" {
		t.Fatalf("result user = %#v, want legacy user with credentials", result.User)
	}
	orgs, err := st.Organizations().ListForUser(ctx, legacyUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Fatalf("orgs = %#v, want existing org", orgs)
	}
}

func TestLoginCreatesSessionAfterSetup(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	first, err := app.SetupAdmin(ctx, setupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}

	second, err := app.Login(ctx, LoginRequest{Username: "Meteorsky", Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionToken == "" || second.SessionToken == first.SessionToken {
		t.Fatalf("second session token = %q, first = %q", second.SessionToken, first.SessionToken)
	}
	if second.User.ID != first.User.ID {
		t.Fatalf("second user = %#v, want %#v", second.User, first.User)
	}
	if _, err := app.UserForToken(ctx, second.SessionToken); err != nil {
		t.Fatal(err)
	}
}

func TestLoginRejectsWrongCredentials(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	if _, err := app.SetupAdmin(ctx, setupRequest("Meteorsky")); err != nil {
		t.Fatal(err)
	}

	for _, req := range []LoginRequest{
		{Username: testUsername, Password: "wrong-password"},
		{Username: "missing", Password: testPassword},
		{Username: "bad name", Password: testPassword},
	} {
		if _, err := app.Login(ctx, req); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("Login(%#v) error = %v, want %v", req, err, ErrUnauthorized)
		}
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	result, err := app.SetupAdmin(ctx, setupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Logout(ctx, result.SessionToken); err != nil {
		t.Fatal(err)
	}
	_, err = app.UserForToken(ctx, result.SessionToken)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestUserForTokenRejectsExpiredSession(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	result, err := app.SetupAdmin(ctx, setupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	expiredToken := "expired-token"
	now := time.Now().UTC()
	if err := st.Users().CreateAPISession(ctx, hashSessionToken(expiredToken), result.User.ID, now.Add(-2*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	_, err = app.UserForToken(ctx, expiredToken)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestUserForTokenReturnsSetupUser(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	result, err := app.SetupAdmin(ctx, setupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}

	user, err := app.UserForToken(ctx, result.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != result.User.ID || user.Username != testUsername || user.DisplayName != result.User.DisplayName {
		t.Fatalf("user = %#v, want %#v", user, result.User)
	}
}

func TestUserForTokenRejectsInvalidToken(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	if _, err := app.SetupAdmin(ctx, setupRequest("Meteorsky")); err != nil {
		t.Fatal(err)
	}

	_, err := app.UserForToken(ctx, "missing")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestUserForTokenPropagatesStoreFailure(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	result, err := app.SetupAdmin(ctx, setupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = app.UserForToken(ctx, result.SessionToken)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want operational store error", err)
	}
}

func TestResetAdminUpdatesCredentialsAndDeletesSessions(t *testing.T) {
	ctx := context.Background()
	st := newAuthTestStore(t)
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: testSetupToken, DataDir: t.TempDir()})
	result, err := app.SetupAdmin(ctx, setupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}

	resetUser, err := app.ResetAdmin(ctx, ResetAdminRequest{Username: "reset_admin", Password: "new-password-1234"})
	if err != nil {
		t.Fatal(err)
	}
	if resetUser.ID != result.User.ID || resetUser.Username != "reset_admin" {
		t.Fatalf("reset user = %#v", resetUser)
	}
	if _, err := app.UserForToken(ctx, result.SessionToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old session error = %v, want %v", err, ErrUnauthorized)
	}
	login, err := app.Login(ctx, LoginRequest{Username: "reset_admin", Password: "new-password-1234"})
	if err != nil {
		t.Fatal(err)
	}
	if login.User.ID != result.User.ID {
		t.Fatalf("login user = %#v, want user id %q", login.User, result.User.ID)
	}
}

func setupRequest(displayName string) SetupAdminRequest {
	return SetupAdminRequest{
		SetupToken:  testSetupToken,
		Username:    testUsername,
		Password:    testPassword,
		DisplayName: displayName,
	}
}

func newAuthTestStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	st, err := sqlitestore.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return st
}
