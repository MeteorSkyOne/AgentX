package claudepersist

import (
	"slices"
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestBuildArgsEnablesPrintModeForStreamJSON(t *testing.T) {
	rt := New(Options{})
	defer rt.pool.Shutdown(t.Context())

	args := rt.buildArgs(runtime.StartSessionRequest{})
	if !slices.Contains(args, "--print") {
		t.Fatalf("args = %q, want --print for stream-json input", args)
	}
}

func TestBuildArgsRequestsThinkingDisplay(t *testing.T) {
	rt := New(Options{ThinkingDisplay: "summarized"})
	defer rt.pool.Shutdown(t.Context())

	args := rt.buildArgs(runtime.StartSessionRequest{})
	i := slices.Index(args, "--thinking-display")
	if i < 0 || i+1 >= len(args) || args[i+1] != "summarized" {
		t.Fatalf("args = %q, want --thinking-display summarized", args)
	}

	rt = New(Options{})
	defer rt.pool.Shutdown(t.Context())
	if args := rt.buildArgs(runtime.StartSessionRequest{}); slices.Contains(args, "--thinking-display") {
		t.Fatalf("args = %q, want no --thinking-display when unset", args)
	}
}

func TestBuildArgsAppendsAgentSystemPromptAfterGlobalOne(t *testing.T) {
	rt := New(Options{AppendSystemPrompt: "global rule"})
	defer rt.pool.Shutdown(t.Context())

	args := rt.buildArgs(runtime.StartSessionRequest{SystemPrompt: " agent rule "})
	i := slices.Index(args, "--append-system-prompt")
	if i < 0 || i+1 >= len(args) || args[i+1] != "global rule\n\nagent rule" {
		t.Fatalf("args = %q, want global and agent prompts joined", args)
	}

	rt = New(Options{})
	defer rt.pool.Shutdown(t.Context())
	args = rt.buildArgs(runtime.StartSessionRequest{SystemPrompt: "agent rule"})
	i = slices.Index(args, "--append-system-prompt")
	if i < 0 || args[i+1] != "agent rule" {
		t.Fatalf("args = %q, want agent prompt alone", args)
	}
}

func TestBuildArgsReplacesSystemPromptInReplaceMode(t *testing.T) {
	rt := New(Options{AppendSystemPrompt: "global rule"})
	defer rt.pool.Shutdown(t.Context())

	args := rt.buildArgs(runtime.StartSessionRequest{SystemPrompt: "you are a poet", SystemPromptMode: runtime.SystemPromptModeReplace})
	i := slices.Index(args, "--system-prompt")
	if i < 0 || args[i+1] != "you are a poet" {
		t.Fatalf("args = %q, want --system-prompt with the agent prompt", args)
	}
	j := slices.Index(args, "--append-system-prompt")
	if j < 0 || args[j+1] != "global rule" {
		t.Fatalf("args = %q, want only the global prompt appended", args)
	}
	if launchSignature(runtime.StartSessionRequest{SystemPrompt: "p"}) == launchSignature(runtime.StartSessionRequest{SystemPrompt: "p", SystemPromptMode: "replace"}) {
		t.Fatal("mode change must change the launch signature")
	}
	withEnv := runtime.StartSessionRequest{SystemPrompt: "p", Env: map[string]string{"HTTPS_PROXY": "http://a"}}
	if launchSignature(runtime.StartSessionRequest{SystemPrompt: "p"}) == launchSignature(withEnv) {
		t.Fatal("env change must change the launch signature")
	}
}
