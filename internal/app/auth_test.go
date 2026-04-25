package app

import (
	"context"
	"errors"
	"testing"

	"github.com/meteorsky/agentx/internal/eventbus"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

func TestBootstrapCreatesAdminOrgChannelAgentAndWorkspace(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})
	result, err := app.Bootstrap(ctx, BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionToken == "" {
		t.Fatal("SessionToken is empty")
	}
	if result.User.DisplayName != "Meteorsky" {
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

func TestBootstrapRejectsWrongAdminToken(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})
	_, err = app.Bootstrap(ctx, BootstrapRequest{AdminToken: "wrong", DisplayName: "Bad"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBootstrapRejectsEmptyConfiguredAdminToken(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "", DataDir: t.TempDir()})
	_, err = app.Bootstrap(ctx, BootstrapRequest{AdminToken: "", DisplayName: "Bad"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestBootstrapRejectsRepeatedBootstrap(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})
	if _, err := app.Bootstrap(ctx, BootstrapRequest{AdminToken: "secret", DisplayName: "First"}); err != nil {
		t.Fatal(err)
	}
	_, err = app.Bootstrap(ctx, BootstrapRequest{AdminToken: "secret", DisplayName: "Second"})
	if !errors.Is(err, ErrAlreadyBootstrapped) {
		t.Fatalf("error = %v, want %v", err, ErrAlreadyBootstrapped)
	}
}

func TestUserForTokenReturnsBootstrappedUser(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})
	result, err := app.Bootstrap(ctx, BootstrapRequest{AdminToken: "secret", DisplayName: "Meteorsky"})
	if err != nil {
		t.Fatal(err)
	}

	user, err := app.UserForToken(ctx, result.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != result.User.ID || user.DisplayName != result.User.DisplayName {
		t.Fatalf("user = %#v, want %#v", user, result.User)
	}
}
