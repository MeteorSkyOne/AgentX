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
	"sync"
	"time"

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
	ToolUpdates                 ToolUpdateSettings
	SelfUpdates                 SelfUpdateSettings
	GitHubRepo                  string
	SelfUpdateChannel           string
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
	ScheduledShellEnabled       bool
	TerminalShell               string
	TerminalIdleMinutes         int
	TerminalMaxSessions         int
	TerminalReplayBytes         int
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

type ToolUpdateSettings struct {
	AutoEnabled   bool   `json:"auto_enabled" toml:"auto_enabled"`
	TimeOfDay     string `json:"time_of_day" toml:"time_of_day"`
	Timezone      string `json:"timezone" toml:"timezone"`
	ClaudeEnabled bool   `json:"claude_enabled" toml:"claude_enabled"`
	CodexEnabled  bool   `json:"codex_enabled" toml:"codex_enabled"`
}

type SelfUpdateSettings struct {
	AutoEnabled bool   `json:"auto_enabled" toml:"auto_enabled"`
	TimeOfDay   string `json:"time_of_day" toml:"time_of_day"`
	Timezone    string `json:"timezone" toml:"timezone"`
	Channel     string `json:"channel" toml:"channel"`
}

type fileConfig struct {
	Server      ServerSettings     `toml:"server"`
	ToolUpdates ToolUpdateSettings `toml:"tool_updates"`
	SelfUpdates SelfUpdateSettings `toml:"self_update"`
}

var configFileMu sync.Mutex

func Load() (Config, error) {
	cfg := FromEnv()
	settings, err := LoadServerSettings(cfg.DataDir)
	if err != nil {
		return Config{}, err
	}
	cfg.Server = settings
	toolUpdates, err := LoadToolUpdateSettings(cfg.DataDir)
	if err != nil {
		return Config{}, err
	}
	cfg.ToolUpdates = toolUpdates
	selfUpdates, err := LoadSelfUpdateSettings(cfg.DataDir)
	if err != nil {
		return Config{}, err
	}
	if cfg.SelfUpdateChannel != "" {
		selfUpdates.Channel = cfg.SelfUpdateChannel
		selfUpdates, err = NormalizeSelfUpdateSettings(selfUpdates, "AGENTX_SELF_UPDATE_CHANNEL")
		if err != nil {
			return Config{}, err
		}
	}
	cfg.SelfUpdates = selfUpdates
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
		ToolUpdates:                 DefaultToolUpdateSettings(),
		SelfUpdates:                 DefaultSelfUpdateSettings(),
		GitHubRepo:                  getenv("AGENTX_GITHUB_REPO", "MeteorSkyOne/AgentX"),
		SelfUpdateChannel:           strings.TrimSpace(os.Getenv("AGENTX_SELF_UPDATE_CHANNEL")),
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
		ScheduledShellEnabled:       getenvBool("AGENTX_SCHEDULED_SHELL_ENABLED", false),
		TerminalShell:               getenv("AGENTX_TERMINAL_SHELL", ""),
		TerminalIdleMinutes:         getenvInt("AGENTX_TERMINAL_IDLE_MINUTES", 30),
		TerminalMaxSessions:         getenvInt("AGENTX_TERMINAL_MAX_SESSIONS_PER_WORKSPACE", 8),
		TerminalReplayBytes:         getenvInt("AGENTX_TERMINAL_REPLAY_BYTES", 8*1024*1024),
	}
}

func LoadServerSettings(dataDir string) (ServerSettings, error) {
	configPath := ConfigPath(dataDir)
	configFileMu.Lock()
	defer configFileMu.Unlock()
	fileCfg, err := loadOrCreateRawFileConfig(configPath)
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
	configFileMu.Lock()
	defer configFileMu.Unlock()
	fileCfg, err := loadOrCreateRawFileConfig(configPath)
	if err != nil {
		return ServerSettings{}, err
	}
	if isZeroToolUpdateSettings(fileCfg.ToolUpdates) {
		fileCfg.ToolUpdates = DefaultToolUpdateSettings()
	}
	if isZeroSelfUpdateSettings(fileCfg.SelfUpdates) {
		fileCfg.SelfUpdates = DefaultSelfUpdateSettings()
	}
	fileCfg.Server = normalized
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return ServerSettings{}, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(configFile(fileCfg)), 0o644); err != nil {
		return ServerSettings{}, fmt.Errorf("write config file %s: %w", configPath, err)
	}
	return normalized, nil
}

func LoadToolUpdateSettings(dataDir string) (ToolUpdateSettings, error) {
	configPath := ConfigPath(dataDir)
	configFileMu.Lock()
	defer configFileMu.Unlock()
	fileCfg, err := loadOrCreateRawFileConfig(configPath)
	if err != nil {
		return ToolUpdateSettings{}, err
	}
	return NormalizeToolUpdateSettings(fileCfg.ToolUpdates, configPath)
}

func SaveToolUpdateSettings(dataDir string, settings ToolUpdateSettings) (ToolUpdateSettings, error) {
	configPath := ConfigPath(dataDir)
	normalized, err := NormalizeToolUpdateSettings(settings, configPath)
	if err != nil {
		return ToolUpdateSettings{}, err
	}
	configFileMu.Lock()
	defer configFileMu.Unlock()
	fileCfg, err := loadOrCreateRawFileConfig(configPath)
	if err != nil {
		return ToolUpdateSettings{}, err
	}
	if isZeroServerSettings(fileCfg.Server) {
		fileCfg.Server = DefaultServerSettings()
	}
	if isZeroSelfUpdateSettings(fileCfg.SelfUpdates) {
		fileCfg.SelfUpdates = DefaultSelfUpdateSettings()
	}
	fileCfg.ToolUpdates = normalized
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return ToolUpdateSettings{}, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(configFile(fileCfg)), 0o644); err != nil {
		return ToolUpdateSettings{}, fmt.Errorf("write config file %s: %w", configPath, err)
	}
	return normalized, nil
}

func LoadSelfUpdateSettings(dataDir string) (SelfUpdateSettings, error) {
	configPath := ConfigPath(dataDir)
	configFileMu.Lock()
	defer configFileMu.Unlock()
	fileCfg, err := loadOrCreateRawFileConfig(configPath)
	if err != nil {
		return SelfUpdateSettings{}, err
	}
	return NormalizeSelfUpdateSettings(fileCfg.SelfUpdates, configPath)
}

func SaveSelfUpdateSettings(dataDir string, settings SelfUpdateSettings) (SelfUpdateSettings, error) {
	configPath := ConfigPath(dataDir)
	normalized, err := NormalizeSelfUpdateSettings(settings, configPath)
	if err != nil {
		return SelfUpdateSettings{}, err
	}
	configFileMu.Lock()
	defer configFileMu.Unlock()
	fileCfg, err := loadOrCreateRawFileConfig(configPath)
	if err != nil {
		return SelfUpdateSettings{}, err
	}
	if isZeroServerSettings(fileCfg.Server) {
		fileCfg.Server = DefaultServerSettings()
	}
	if isZeroToolUpdateSettings(fileCfg.ToolUpdates) {
		fileCfg.ToolUpdates = DefaultToolUpdateSettings()
	}
	fileCfg.SelfUpdates = normalized
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return SelfUpdateSettings{}, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(configFile(fileCfg)), 0o644); err != nil {
		return SelfUpdateSettings{}, fmt.Errorf("write config file %s: %w", configPath, err)
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

func DefaultToolUpdateSettings() ToolUpdateSettings {
	return ToolUpdateSettings{
		AutoEnabled:   false,
		TimeOfDay:     "04:00",
		Timezone:      defaultToolUpdateTimezone(),
		ClaudeEnabled: true,
		CodexEnabled:  true,
	}
}

func DefaultSelfUpdateSettings() SelfUpdateSettings {
	return SelfUpdateSettings{
		AutoEnabled: false,
		TimeOfDay:   "04:00",
		Timezone:    defaultToolUpdateTimezone(),
		Channel:     "release",
	}
}

func isZeroServerSettings(settings ServerSettings) bool {
	return settings.ListenIP == "" &&
		settings.ListenPort == 0 &&
		!settings.TLS.Enabled &&
		settings.TLS.ListenPort == 0 &&
		settings.TLS.CertFile == "" &&
		settings.TLS.KeyFile == ""
}

func isZeroToolUpdateSettings(settings ToolUpdateSettings) bool {
	return strings.TrimSpace(settings.TimeOfDay) == "" &&
		strings.TrimSpace(settings.Timezone) == "" &&
		!settings.AutoEnabled &&
		!settings.ClaudeEnabled &&
		!settings.CodexEnabled
}

func isZeroSelfUpdateSettings(settings SelfUpdateSettings) bool {
	return strings.TrimSpace(settings.TimeOfDay) == "" &&
		strings.TrimSpace(settings.Timezone) == "" &&
		strings.TrimSpace(settings.Channel) == "" &&
		!settings.AutoEnabled
}

func defaultToolUpdateTimezone() string {
	timezone := strings.TrimSpace(os.Getenv("TZ"))
	if timezone != "" && timezone != "Local" {
		if _, err := time.LoadLocation(timezone); err == nil {
			return timezone
		}
	}
	return "UTC"
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

func NormalizeToolUpdateSettings(settings ToolUpdateSettings, path string) (ToolUpdateSettings, error) {
	if strings.TrimSpace(settings.TimeOfDay) == "" && strings.TrimSpace(settings.Timezone) == "" && !settings.AutoEnabled && !settings.ClaudeEnabled && !settings.CodexEnabled {
		return DefaultToolUpdateSettings(), nil
	}
	if strings.TrimSpace(settings.TimeOfDay) == "" {
		settings.TimeOfDay = "04:00"
	}
	settings.TimeOfDay = strings.TrimSpace(settings.TimeOfDay)
	parts := strings.Split(settings.TimeOfDay, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return ToolUpdateSettings{}, fmt.Errorf("config file %s: tool_updates.time_of_day must use HH:MM", path)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return ToolUpdateSettings{}, fmt.Errorf("config file %s: tool_updates.time_of_day must use HH:MM", path)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return ToolUpdateSettings{}, fmt.Errorf("config file %s: tool_updates.time_of_day must use HH:MM", path)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return ToolUpdateSettings{}, fmt.Errorf("config file %s: tool_updates.time_of_day must use HH:MM", path)
	}
	settings.Timezone = strings.TrimSpace(settings.Timezone)
	if settings.Timezone == "" || settings.Timezone == "Local" {
		settings.Timezone = defaultToolUpdateTimezone()
	}
	if _, err := time.LoadLocation(settings.Timezone); err != nil {
		return ToolUpdateSettings{}, fmt.Errorf("config file %s: tool_updates.timezone is invalid: %w", path, err)
	}
	return settings, nil
}

func NormalizeSelfUpdateSettings(settings SelfUpdateSettings, path string) (SelfUpdateSettings, error) {
	if isZeroSelfUpdateSettings(settings) {
		return DefaultSelfUpdateSettings(), nil
	}
	if strings.TrimSpace(settings.TimeOfDay) == "" {
		settings.TimeOfDay = "04:00"
	}
	settings.TimeOfDay = strings.TrimSpace(settings.TimeOfDay)
	parts := strings.Split(settings.TimeOfDay, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return SelfUpdateSettings{}, fmt.Errorf("config file %s: self_update.time_of_day must use HH:MM", path)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return SelfUpdateSettings{}, fmt.Errorf("config file %s: self_update.time_of_day must use HH:MM", path)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return SelfUpdateSettings{}, fmt.Errorf("config file %s: self_update.time_of_day must use HH:MM", path)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return SelfUpdateSettings{}, fmt.Errorf("config file %s: self_update.time_of_day must use HH:MM", path)
	}
	settings.Timezone = strings.TrimSpace(settings.Timezone)
	if settings.Timezone == "" || settings.Timezone == "Local" {
		settings.Timezone = defaultToolUpdateTimezone()
	}
	if _, err := time.LoadLocation(settings.Timezone); err != nil {
		return SelfUpdateSettings{}, fmt.Errorf("config file %s: self_update.timezone is invalid: %w", path, err)
	}
	settings.Channel = strings.ToLower(strings.TrimSpace(settings.Channel))
	if settings.Channel == "" {
		settings.Channel = "release"
	}
	if settings.Channel != "release" && settings.Channel != "dev" {
		return SelfUpdateSettings{}, fmt.Errorf("config file %s: self_update.channel must be release or dev", path)
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

func loadOrCreateRawFileConfig(path string) (fileConfig, error) {
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

func loadOrCreateFileConfig(path string) (fileConfig, error) {
	cfg, err := loadOrCreateRawFileConfig(path)
	if err != nil {
		return fileConfig{}, err
	}
	var normErr error
	cfg.Server, normErr = NormalizeServerSettings(cfg.Server, path)
	if normErr != nil {
		return fileConfig{}, normErr
	}
	cfg.ToolUpdates, normErr = NormalizeToolUpdateSettings(cfg.ToolUpdates, path)
	if normErr != nil {
		return fileConfig{}, normErr
	}
	cfg.SelfUpdates, normErr = NormalizeSelfUpdateSettings(cfg.SelfUpdates, path)
	if normErr != nil {
		return fileConfig{}, normErr
	}
	return cfg, nil
}

func defaultConfigFile() string {
	return configFile(fileConfig{Server: DefaultServerSettings(), ToolUpdates: DefaultToolUpdateSettings(), SelfUpdates: DefaultSelfUpdateSettings()})
}

func configFile(cfg fileConfig) string {
	settings := cfg.Server
	toolUpdates := cfg.ToolUpdates
	selfUpdates := cfg.SelfUpdates
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

[tool_updates]
auto_enabled = %t
time_of_day = %q
timezone = %q
claude_enabled = %t
codex_enabled = %t

[self_update]
auto_enabled = %t
time_of_day = %q
timezone = %q
channel = %q
`, settings.ListenIP, settings.ListenPort, settings.TLS.Enabled, settings.TLS.ListenPort, settings.TLS.CertFile, settings.TLS.KeyFile, toolUpdates.AutoEnabled, toolUpdates.TimeOfDay, toolUpdates.Timezone, toolUpdates.ClaudeEnabled, toolUpdates.CodexEnabled, selfUpdates.AutoEnabled, selfUpdates.TimeOfDay, selfUpdates.Timezone, selfUpdates.Channel)
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
