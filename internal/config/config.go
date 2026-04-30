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
	defaultListenIP      = "127.0.0.1"
	defaultListenPort    = 8080
	defaultTLSListenPort = 8443
	defaultAddr          = "127.0.0.1:8080"
	configFileName       = "config.toml"
)

type Config struct {
	Addr                        string
	AddrOverrideActive          bool
	AddrOverrideValue           string
	DataDir                     string
	SQLitePath                  string
	AdminToken                  string
	Server                      ServerSettings
	DefaultAgentKind            string
	DefaultAgentModel           string
	CodexCommand                string
	CodexFullAuto               bool
	CodexBypassSandbox          bool
	CodexSkipGitRepoCheck       bool
	ClaudeCommand               string
	ClaudePermissionMode        string
	ClaudeAllowedTools          []string
	ClaudeDisallowedTools       []string
	ClaudeAppendSystemText      string
	ClaudePersistentIdleMinutes int
	CodexPersistentIdleMinutes  int
	D2Command                   string
	D2TimeoutSeconds            int
	D2CacheTTLMinutes           int
	D2CacheMaxEntries           int
}

type ServerSettings struct {
	ListenIP   string            `json:"listen_ip" toml:"listen_ip"`
	ListenPort int               `json:"listen_port" toml:"listen_port"`
	TLS        ServerTLSSettings `json:"tls" toml:"tls"`
}

type ServerTLSSettings struct {
	Enabled    bool   `json:"enabled" toml:"enabled"`
	ListenPort int    `json:"listen_port" toml:"listen_port"`
	CertFile   string `json:"cert_file" toml:"cert_file"`
	KeyFile    string `json:"key_file" toml:"key_file"`
}

type fileConfig struct {
	Server ServerSettings `toml:"server"`
}

func Load() (Config, error) {
	cfg := FromEnv()
	settings, err := LoadServerSettings(cfg.DataDir)
	if err != nil {
		return Config{}, err
	}
	cfg.Server = settings
	addrOverride := strings.TrimSpace(os.Getenv("AGENTX_ADDR"))
	cfg.AddrOverrideActive = addrOverride != ""
	cfg.AddrOverrideValue = addrOverride
	if !cfg.AddrOverrideActive {
		cfg.Addr = ServerAddr(settings)
	}
	return cfg, nil
}

func FromEnv() Config {
	dataDir := getenv("AGENTX_DATA_DIR", defaultDataDir())
	addrOverride := strings.TrimSpace(os.Getenv("AGENTX_ADDR"))
	return Config{
		Addr:                        getenv("AGENTX_ADDR", defaultAddr),
		AddrOverrideActive:          addrOverride != "",
		AddrOverrideValue:           addrOverride,
		DataDir:                     dataDir,
		SQLitePath:                  getenv("AGENTX_SQLITE_PATH", filepath.Join(dataDir, "agentx.db")),
		AdminToken:                  getenv("AGENTX_ADMIN_TOKEN", randomToken()),
		Server:                      DefaultServerSettings(),
		DefaultAgentKind:            getenv("AGENTX_DEFAULT_AGENT_KIND", "fake"),
		DefaultAgentModel:           getenv("AGENTX_DEFAULT_AGENT_MODEL", ""),
		CodexCommand:                getenv("AGENTX_CODEX_COMMAND", "codex"),
		CodexFullAuto:               getenvBool("AGENTX_CODEX_FULL_AUTO", true),
		CodexBypassSandbox:          getenvBool("AGENTX_CODEX_BYPASS_SANDBOX", false),
		CodexSkipGitRepoCheck:       getenvBool("AGENTX_CODEX_SKIP_GIT_REPO_CHECK", true),
		ClaudeCommand:               getenv("AGENTX_CLAUDE_COMMAND", "claude"),
		ClaudePermissionMode:        getenv("AGENTX_CLAUDE_PERMISSION_MODE", "acceptEdits"),
		ClaudeAllowedTools:          getenvList("AGENTX_CLAUDE_ALLOWED_TOOLS"),
		ClaudeDisallowedTools:       getenvList("AGENTX_CLAUDE_DISALLOWED_TOOLS"),
		ClaudeAppendSystemText:      getenv("AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT", ""),
		ClaudePersistentIdleMinutes: getenvInt("AGENTX_CLAUDE_PERSISTENT_IDLE_MINUTES", 30),
		CodexPersistentIdleMinutes:  getenvInt("AGENTX_CODEX_PERSISTENT_IDLE_MINUTES", 30),
		D2Command:                   getenv("AGENTX_D2_COMMAND", "d2"),
		D2TimeoutSeconds:            getenvInt("AGENTX_D2_TIMEOUT_SECONDS", 10),
		D2CacheTTLMinutes:           getenvInt("AGENTX_D2_CACHE_TTL_MINUTES", 1440),
		D2CacheMaxEntries:           getenvInt("AGENTX_D2_CACHE_MAX_ENTRIES", 256),
	}
}

func LoadServerSettings(dataDir string) (ServerSettings, error) {
	configPath := ConfigPath(dataDir)
	fileCfg, err := loadOrCreateFileConfig(configPath)
	if err != nil {
		return ServerSettings{}, err
	}
	return NormalizeServerSettings(fileCfg.Server, configPath)
}

func SaveServerSettings(dataDir string, settings ServerSettings) (ServerSettings, error) {
	configPath := ConfigPath(dataDir)
	normalized, err := NormalizeServerSettings(settings, configPath)
	if err != nil {
		return ServerSettings{}, err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return ServerSettings{}, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(configFile(normalized)), 0o644); err != nil {
		return ServerSettings{}, fmt.Errorf("write config file %s: %w", configPath, err)
	}
	return normalized, nil
}

func ConfigPath(dataDir string) string {
	return filepath.Join(dataDir, configFileName)
}

func DefaultServerSettings() ServerSettings {
	return ServerSettings{
		ListenIP:   defaultListenIP,
		ListenPort: defaultListenPort,
		TLS: ServerTLSSettings{
			Enabled:    false,
			ListenPort: defaultTLSListenPort,
			CertFile:   "",
			KeyFile:    "",
		},
	}
}

func NormalizeServerSettings(settings ServerSettings, path string) (ServerSettings, error) {
	settings.ListenIP = strings.TrimSpace(settings.ListenIP)
	if settings.ListenIP == "" {
		settings.ListenIP = defaultListenIP
	}
	if settings.ListenPort == 0 {
		settings.ListenPort = defaultListenPort
	}
	if settings.ListenPort < 1 || settings.ListenPort > 65535 {
		return ServerSettings{}, fmt.Errorf("config file %s: server.listen_port must be between 1 and 65535", path)
	}

	settings.TLS.CertFile = strings.TrimSpace(settings.TLS.CertFile)
	settings.TLS.KeyFile = strings.TrimSpace(settings.TLS.KeyFile)
	if settings.TLS.ListenPort == 0 {
		settings.TLS.ListenPort = defaultTLSListenPort
	}
	if settings.TLS.ListenPort < 1 || settings.TLS.ListenPort > 65535 {
		return ServerSettings{}, fmt.Errorf("config file %s: server.tls.listen_port must be between 1 and 65535", path)
	}
	if settings.TLS.Enabled {
		if settings.TLS.ListenPort == settings.ListenPort {
			return ServerSettings{}, fmt.Errorf("config file %s: server.tls.listen_port must differ from server.listen_port when TLS is enabled", path)
		}
		if settings.TLS.CertFile == "" {
			return ServerSettings{}, fmt.Errorf("config file %s: server.tls.cert_file is required when TLS is enabled", path)
		}
		if settings.TLS.KeyFile == "" {
			return ServerSettings{}, fmt.Errorf("config file %s: server.tls.key_file is required when TLS is enabled", path)
		}
	}
	return settings, nil
}

func ServerAddr(settings ServerSettings) string {
	return net.JoinHostPort(settings.ListenIP, strconv.Itoa(settings.ListenPort))
}

func ServerTLSAddr(settings ServerSettings) string {
	return net.JoinHostPort(settings.ListenIP, strconv.Itoa(settings.TLS.ListenPort))
}

func ServerSettingsEqual(a ServerSettings, b ServerSettings) bool {
	return a.ListenIP == b.ListenIP &&
		a.ListenPort == b.ListenPort &&
		a.TLS.Enabled == b.TLS.Enabled &&
		a.TLS.ListenPort == b.TLS.ListenPort &&
		a.TLS.CertFile == b.TLS.CertFile &&
		a.TLS.KeyFile == b.TLS.KeyFile
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

func defaultConfigFile() string {
	return configFile(DefaultServerSettings())
}

func configFile(settings ServerSettings) string {
	return fmt.Sprintf(`# AgentX configuration.
# AGENTX_ADDR overrides server.listen_ip and server.listen_port when set.
# HTTPS changes require restarting AgentX.

[server]
listen_ip = %q
listen_port = %d

[server.tls]
enabled = %t
listen_port = %d
cert_file = %q
key_file = %q
`, settings.ListenIP, settings.ListenPort, settings.TLS.Enabled, settings.TLS.ListenPort, settings.TLS.CertFile, settings.TLS.KeyFile)
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

func getenvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
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
