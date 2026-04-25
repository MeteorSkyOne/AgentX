package app

import (
	"context"
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
