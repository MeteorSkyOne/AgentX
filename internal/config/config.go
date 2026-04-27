package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	defaultListenIP   = "127.0.0.1"
	defaultListenPort = 8080
	defaultAddr       = "127.0.0.1:8080"
	configFileName    = "config.toml"
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

type fileConfig struct {
	Server fileServerConfig `toml:"server"`
}

type fileServerConfig struct {
	ListenIP   string `toml:"listen_ip"`
	ListenPort int    `toml:"listen_port"`
}

func Load() (Config, error) {
	cfg := FromEnv()
	configPath := filepath.Join(cfg.DataDir, configFileName)
	fileCfg, err := loadOrCreateFileConfig(configPath)
	if err != nil {
		return Config{}, err
	}
	if os.Getenv("AGENTX_ADDR") == "" {
		addr, err := serverAddrFromFileConfig(fileCfg, configPath)
		if err != nil {
			return Config{}, err
		}
		cfg.Addr = addr
	}
	return cfg, nil
}

func FromEnv() Config {
	dataDir := getenv("AGENTX_DATA_DIR", defaultDataDir())
	return Config{
		Addr:                   getenv("AGENTX_ADDR", defaultAddr),
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

func loadOrCreateFileConfig(path string) (fileConfig, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fileConfig{}, fmt.Errorf("create config dir: %w", err)
	}
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return fileConfig{}, fmt.Errorf("stat config file %s: %w", path, err)
		}
		if err := os.WriteFile(path, []byte(defaultConfigFile()), 0o644); err != nil {
			return fileConfig{}, fmt.Errorf("create config file %s: %w", path, err)
		}
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, fmt.Errorf("read config file %s: %w", path, err)
	}
	var cfg fileConfig
	if _, err := toml.Decode(string(body), &cfg); err != nil {
		return fileConfig{}, fmt.Errorf("parse config file %s: %w", path, err)
	}
	return cfg, nil
}

func serverAddrFromFileConfig(cfg fileConfig, path string) (string, error) {
	listenIP := strings.TrimSpace(cfg.Server.ListenIP)
	if listenIP == "" {
		listenIP = defaultListenIP
	}
	listenPort := cfg.Server.ListenPort
	if listenPort == 0 {
		listenPort = defaultListenPort
	}
	if listenPort < 1 || listenPort > 65535 {
		return "", fmt.Errorf("config file %s: server.listen_port must be between 1 and 65535", path)
	}
	return net.JoinHostPort(listenIP, strconv.Itoa(listenPort)), nil
}

func defaultConfigFile() string {
	return fmt.Sprintf(`# AgentX configuration.
# AGENTX_ADDR overrides server.listen_ip and server.listen_port when set.

[server]
listen_ip = %q
listen_port = %d
`, defaultListenIP, defaultListenPort)
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
