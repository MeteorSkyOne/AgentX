package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestUpdateAgentEnvPatchEditsIndividualEntries(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)
	if _, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{
		EnvSet: true,
		Env:    map[string]string{"KEEP": "keep", "OLD_NAME": "secret", "DROP": "drop", "EDIT": "before"},
	}); err != nil {
		t.Fatal(err)
	}

	agent, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{
		EnvPatch: &AgentEnvPatch{
			Rename: map[string]string{"OLD_NAME": "NEW_NAME"},
			Unset:  []string{"DROP"},
			Set:    map[string]string{"EDIT": "after", " ADDED ": "added"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"KEEP": "keep", "NEW_NAME": "secret", "EDIT": "after", "ADDED": "added"}
	if !reflect.DeepEqual(agent.Env, want) {
		t.Fatalf("env = %#v, want %#v", agent.Env, want)
	}
	stored, err := app.Agent(ctx, bootstrap.Agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Env, want) {
		t.Fatalf("stored env = %#v, want %#v", stored.Env, want)
	}
}

func TestUpdateAgentEnvPatchRejectsInvalidPatches(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)
	if _, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{
		EnvSet: true,
		Env:    map[string]string{"A": "a", "B": "b"},
	}); err != nil {
		t.Fatal(err)
	}

	for name, req := range map[string]AgentUpdateRequest{
		"rename missing":     {EnvPatch: &AgentEnvPatch{Rename: map[string]string{"MISSING": "C"}}},
		"rename onto key":    {EnvPatch: &AgentEnvPatch{Rename: map[string]string{"A": "B"}}},
		"rename same target": {EnvPatch: &AgentEnvPatch{Rename: map[string]string{"A": "C", "B": "C"}}},
		"empty set key":      {EnvPatch: &AgentEnvPatch{Set: map[string]string{" ": "x"}}},
		"env and patch":      {EnvSet: true, Env: map[string]string{}, EnvPatch: &AgentEnvPatch{}},
	} {
		if _, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, req); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}

	agent, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{
		EnvPatch: &AgentEnvPatch{Rename: map[string]string{"A": "B", "B": "A"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"A": "b", "B": "a"}; !reflect.DeepEqual(agent.Env, want) {
		t.Fatalf("swapped env = %#v, want %#v", agent.Env, want)
	}

	agent, err = app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{
		EnvPatch: &AgentEnvPatch{Unset: []string{"B"}, Rename: map[string]string{"A": "B"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"B": "b"}; !reflect.DeepEqual(agent.Env, want) {
		t.Fatalf("replaced env = %#v, want %#v", agent.Env, want)
	}
}
