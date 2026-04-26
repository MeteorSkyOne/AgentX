package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr                   string
	DataDir                string
	SQLitePath             string
	AdminToken             string
	DefaultAgentKind       string
	DefaultAgentModel      string
	CodexCommand           string
	CodexFullAuto          bool
	CodexBypassSandbox     bool
	CodexSkipGitRepoCheck  bool
	ClaudeCommand          string
	ClaudePermissionMode   string
	ClaudeAllowedTools     []string
	ClaudeDisallowedTools  []string
	ClaudeAppendSystemText string
}

func FromEnv() Config {
	dataDir := getenv("AGENTX_DATA_DIR", defaultDataDir())
	return Config{
		Addr:                   getenv("AGENTX_ADDR", "127.0.0.1:8080"),
		DataDir:                dataDir,
		SQLitePath:             getenv("AGENTX_SQLITE_PATH", filepath.Join(dataDir, "agentx.db")),
		AdminToken:             getenv("AGENTX_ADMIN_TOKEN", randomToken()),
		DefaultAgentKind:       getenv("AGENTX_DEFAULT_AGENT_KIND", "fake"),
		DefaultAgentModel:      getenv("AGENTX_DEFAULT_AGENT_MODEL", ""),
		CodexCommand:           getenv("AGENTX_CODEX_COMMAND", "codex"),
		CodexFullAuto:          getenvBool("AGENTX_CODEX_FULL_AUTO", true),
		CodexBypassSandbox:     getenvBool("AGENTX_CODEX_BYPASS_SANDBOX", false),
		CodexSkipGitRepoCheck:  getenvBool("AGENTX_CODEX_SKIP_GIT_REPO_CHECK", true),
		ClaudeCommand:          getenv("AGENTX_CLAUDE_COMMAND", "claude"),
		ClaudePermissionMode:   getenv("AGENTX_CLAUDE_PERMISSION_MODE", "acceptEdits"),
		ClaudeAllowedTools:     getenvList("AGENTX_CLAUDE_ALLOWED_TOOLS"),
		ClaudeDisallowedTools:  getenvList("AGENTX_CLAUDE_DISALLOWED_TOOLS"),
		ClaudeAppendSystemText: getenv("AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT", ""),
	}
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".agentx"
	}
	return filepath.Join(home, ".agentx")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	switch os.Getenv(key) {
	case "":
		return fallback
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	default:
		return false
	}
}

func getenvList(key string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' '
	})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func randomToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("generate admin token: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
