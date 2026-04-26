package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("AGENTX_ADDR", "")
	t.Setenv("AGENTX_DATA_DIR", "")
	t.Setenv("AGENTX_SQLITE_PATH", "")
	t.Setenv("AGENTX_ADMIN_TOKEN", "")
	t.Setenv("AGENTX_DEFAULT_AGENT_KIND", "")
	t.Setenv("AGENTX_DEFAULT_AGENT_MODEL", "")
	t.Setenv("AGENTX_CODEX_COMMAND", "")
	t.Setenv("AGENTX_CODEX_FULL_AUTO", "")
	t.Setenv("AGENTX_CODEX_BYPASS_SANDBOX", "")
	t.Setenv("AGENTX_CODEX_SKIP_GIT_REPO_CHECK", "")
	t.Setenv("AGENTX_CLAUDE_COMMAND", "")
	t.Setenv("AGENTX_CLAUDE_PERMISSION_MODE", "")
	t.Setenv("AGENTX_CLAUDE_ALLOWED_TOOLS", "")
	t.Setenv("AGENTX_CLAUDE_DISALLOWED_TOOLS", "")
	t.Setenv("AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT", "")

	cfg := FromEnv()

	if cfg.Addr != "127.0.0.1:8080" {
		t.Fatalf("Addr = %q, want 127.0.0.1:8080", cfg.Addr)
	}
	home, _ := os.UserHomeDir()
	wantDataDir := filepath.Join(home, ".agentx")
	if cfg.DataDir != wantDataDir {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, wantDataDir)
	}
	wantDBPath := filepath.Join(wantDataDir, "agentx.db")
	if cfg.SQLitePath != wantDBPath {
		t.Fatalf("SQLitePath = %q, want %q", cfg.SQLitePath, wantDBPath)
	}
	if cfg.AdminToken == "" {
		t.Fatal("AdminToken should have a generated token when unset")
	}
	if cfg.DefaultAgentKind != "fake" {
		t.Fatalf("DefaultAgentKind = %q", cfg.DefaultAgentKind)
	}
	if cfg.CodexCommand != "codex" || !cfg.CodexFullAuto || !cfg.CodexSkipGitRepoCheck || cfg.CodexBypassSandbox {
		t.Fatalf("codex config = %#v", cfg)
	}
	if cfg.ClaudeCommand != "claude" || cfg.ClaudePermissionMode != "acceptEdits" {
		t.Fatalf("claude config = %#v", cfg)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("AGENTX_ADDR", "0.0.0.0:9000")
	t.Setenv("AGENTX_DATA_DIR", "/tmp/agentx")
	t.Setenv("AGENTX_SQLITE_PATH", "/tmp/agentx/custom.db")
	t.Setenv("AGENTX_ADMIN_TOKEN", "dev-token")
	t.Setenv("AGENTX_DEFAULT_AGENT_KIND", "codex")
	t.Setenv("AGENTX_DEFAULT_AGENT_MODEL", "gpt-test")
	t.Setenv("AGENTX_CODEX_COMMAND", "/usr/local/bin/codex")
	t.Setenv("AGENTX_CODEX_FULL_AUTO", "false")
	t.Setenv("AGENTX_CODEX_BYPASS_SANDBOX", "true")
	t.Setenv("AGENTX_CODEX_SKIP_GIT_REPO_CHECK", "false")
	t.Setenv("AGENTX_CLAUDE_COMMAND", "/usr/local/bin/claude")
	t.Setenv("AGENTX_CLAUDE_PERMISSION_MODE", "bypassPermissions")
	t.Setenv("AGENTX_CLAUDE_ALLOWED_TOOLS", "Read,Bash")
	t.Setenv("AGENTX_CLAUDE_DISALLOWED_TOOLS", "WebSearch Edit")
	t.Setenv("AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT", "be brief")

	cfg := FromEnv()

	if cfg.Addr != "0.0.0.0:9000" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
	if cfg.DataDir != "/tmp/agentx" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.SQLitePath != "/tmp/agentx/custom.db" {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
	if cfg.AdminToken != "dev-token" {
		t.Fatalf("AdminToken = %q", cfg.AdminToken)
	}
	if cfg.DefaultAgentKind != "codex" || cfg.DefaultAgentModel != "gpt-test" {
		t.Fatalf("agent defaults = %#v", cfg)
	}
	if cfg.CodexCommand != "/usr/local/bin/codex" || cfg.CodexFullAuto || !cfg.CodexBypassSandbox || cfg.CodexSkipGitRepoCheck {
		t.Fatalf("codex overrides = %#v", cfg)
	}
	if cfg.ClaudeCommand != "/usr/local/bin/claude" || cfg.ClaudePermissionMode != "bypassPermissions" {
		t.Fatalf("claude overrides = %#v", cfg)
	}
	if len(cfg.ClaudeAllowedTools) != 2 || cfg.ClaudeAllowedTools[0] != "Read" || cfg.ClaudeAllowedTools[1] != "Bash" {
		t.Fatalf("ClaudeAllowedTools = %#v", cfg.ClaudeAllowedTools)
	}
	if len(cfg.ClaudeDisallowedTools) != 2 || cfg.ClaudeDisallowedTools[0] != "WebSearch" || cfg.ClaudeDisallowedTools[1] != "Edit" {
		t.Fatalf("ClaudeDisallowedTools = %#v", cfg.ClaudeDisallowedTools)
	}
	if cfg.ClaudeAppendSystemText != "be brief" {
		t.Fatalf("ClaudeAppendSystemText = %q", cfg.ClaudeAppendSystemText)
	}
}
