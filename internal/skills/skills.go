package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/meteorsky/agentx/internal/domain"
)

const skillFileName = "SKILL.md"

type DiscoverOptions struct {
	AgentKind       string
	ConfigWorkspace string
	RunWorkspace    string
	Env             map[string]string
	HomeDir         string
	ReservedNames   []string
}

type Skill struct {
	Name                 string
	DisplayName          string
	Description          string
	Prompt               string
	SourcePath           string
	UserInvocable        bool
	ConflictsWithBuiltin bool
}

func Discover(opts DiscoverOptions) ([]Skill, error) {
	reserved := reservedNameSet(opts.ReservedNames)
	filter, err := discoverFilter(opts)
	if err != nil {
		return nil, err
	}
	roots, err := Roots(opts)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]Skill)
	for _, root := range roots {
		rootSkills, err := scanRoot(root, reserved, filter)
		if err != nil {
			return nil, err
		}
		for _, skill := range rootSkills {
			key := CanonicalName(skill.Name)
			if key == "" {
				continue
			}
			if _, exists := byName[key]; exists {
				continue
			}
			byName[key] = skill
		}
	}

	result := make([]Skill, 0, len(byName))
	for _, skill := range byName {
		result = append(result, skill)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].SourcePath < result[j].SourcePath
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func Roots(opts DiscoverOptions) ([]string, error) {
	homeDir, err := homeDir(opts)
	if err != nil {
		return nil, err
	}

	var roots []string
	addRoot := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		for _, existing := range roots {
			if existing == clean {
				return
			}
		}
		roots = append(roots, clean)
	}
	addWorkspaceRoots := func(workspace string, names ...string) {
		workspace = strings.TrimSpace(workspace)
		if workspace == "" {
			return
		}
		for _, name := range names {
			addRoot(filepath.Join(workspace, name, "skills"))
		}
	}

	switch strings.TrimSpace(opts.AgentKind) {
	case domain.AgentKindClaude:
		addWorkspaceRoots(opts.ConfigWorkspace, ".claude")
		addWorkspaceRoots(opts.RunWorkspace, ".claude")
		claudeConfig := claudeConfigDir(opts, homeDir)
		addRoot(filepath.Join(claudeConfig, "skills"))
		pluginSkillRoots, err := claudePluginSkillRoots(claudeConfig)
		if err != nil {
			return nil, err
		}
		for _, root := range pluginSkillRoots {
			addRoot(root)
		}
	case domain.AgentKindCodex:
		addWorkspaceRoots(opts.ConfigWorkspace, ".codex", ".claude")
		addWorkspaceRoots(opts.RunWorkspace, ".codex", ".claude")
		codexHome := strings.TrimSpace(envValue(opts, "CODEX_HOME"))
		if codexHome == "" {
			codexHome = filepath.Join(homeDir, ".codex")
		}
		codexConfig, hasCodexConfig, err := loadCodexConfig(filepath.Join(codexHome, "config.toml"))
		if err != nil {
			return nil, err
		}
		addRoot(filepath.Join(codexHome, "skills"))
		addRoot(filepath.Join(codexHome, "superpowers", "skills"))
		for _, entry := range codexConfig.Skills.Config {
			if strings.TrimSpace(entry.Path) != "" && boolValue(entry.Enabled, true) {
				addRoot(filepath.Dir(codexConfigPath(entry.Path, codexHome, homeDir)))
			}
		}
		pluginSkillRoots, err := codexPluginSkillRoots(filepath.Join(codexHome, "plugins"), codexConfig, hasCodexConfig)
		if err != nil {
			return nil, err
		}
		for _, root := range pluginSkillRoots {
			addRoot(root)
		}
		addRoot(filepath.Join(homeDir, ".claude", "skills"))
	}

	return roots, nil
}

func Match(skills []Skill, name string) (Skill, bool) {
	key := CanonicalName(name)
	if key == "" {
		return Skill{}, false
	}
	for _, skill := range skills {
		if CanonicalName(skill.Name) == key {
			return skill, true
		}
	}
	return Skill{}, false
}

func BuildPrompt(skill Skill, args string) string {
	args = strings.TrimSpace(args)
	var b strings.Builder
	b.WriteString("Use the following skill instructions to handle the user's request.\n\n")
	b.WriteString("Skill: ")
	if strings.TrimSpace(skill.DisplayName) != "" {
		b.WriteString(strings.TrimSpace(skill.DisplayName))
	} else {
		b.WriteString(skill.Name)
	}
	if strings.TrimSpace(skill.Description) != "" {
		b.WriteString("\nDescription: ")
		b.WriteString(strings.TrimSpace(skill.Description))
	}
	b.WriteString("\n\nSkill instructions:\n")
	b.WriteString(strings.TrimSpace(skill.Prompt))
	b.WriteString("\n\nUser request:\n")
	if args == "" {
		b.WriteString("No additional request was provided. Follow the skill instructions.")
	} else {
		b.WriteString(args)
	}
	return b.String()
}

func CanonicalName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

type rootFilter struct {
	disabledSkillFiles map[string]struct{}
}

func (f rootFilter) disabled(path string) bool {
	if len(f.disabledSkillFiles) == 0 {
		return false
	}
	_, disabled := f.disabledSkillFiles[cleanAbsPath(path)]
	return disabled
}

func discoverFilter(opts DiscoverOptions) (rootFilter, error) {
	if strings.TrimSpace(opts.AgentKind) != domain.AgentKindCodex {
		return rootFilter{}, nil
	}
	homeDir, err := homeDir(opts)
	if err != nil {
		return rootFilter{}, err
	}
	codexHome := strings.TrimSpace(envValue(opts, "CODEX_HOME"))
	if codexHome == "" {
		codexHome = filepath.Join(homeDir, ".codex")
	}
	codexConfig, _, err := loadCodexConfig(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return rootFilter{}, err
	}
	disabled := make(map[string]struct{})
	for _, entry := range codexConfig.Skills.Config {
		if strings.TrimSpace(entry.Path) != "" && !boolValue(entry.Enabled, true) {
			disabled[codexConfigPath(entry.Path, codexHome, homeDir)] = struct{}{}
		}
	}
	return rootFilter{disabledSkillFiles: disabled}, nil
}

func scanRoot(root string, reserved map[string]struct{}, filter rootFilter) ([]Skill, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return nil, nil
		}
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if errors.Is(err, os.ErrPermission) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var result []Skill
	err = filepath.WalkDir(resolved, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrPermission) {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != skillFileName {
			return nil
		}
		if filter.disabled(path) {
			return nil
		}
		skill, err := ParseFile(path, reserved)
		if err != nil {
			return err
		}
		if skill.Name != "" && skill.UserInvocable {
			result = append(result, skill)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ParseFile(path string, reserved map[string]struct{}) (Skill, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	displayName, description, prompt, userInvocable := parseMarkdownSkill(string(body), filepath.Base(filepath.Dir(path)))
	name := commandName(displayName)
	if name == "" {
		return Skill{}, fmt.Errorf("skill %s has no usable name", path)
	}
	_, conflicts := reserved[CanonicalName(name)]
	return Skill{
		Name:                 name,
		DisplayName:          displayName,
		Description:          description,
		Prompt:               prompt,
		SourcePath:           path,
		UserInvocable:        userInvocable,
		ConflictsWithBuiltin: conflicts,
	}, nil
}

func parseMarkdownSkill(content string, fallbackName string) (string, string, string, bool) {
	content = strings.TrimPrefix(content, "\ufeff")
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	displayName := strings.TrimSpace(fallbackName)
	description := ""
	prompt := normalized
	userInvocable := true

	if strings.HasPrefix(normalized, "---\n") {
		rest := normalized[len("---\n"):]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			frontmatter := rest[:end]
			after := rest[end+len("\n---"):]
			if strings.HasPrefix(after, "\n") {
				after = after[1:]
			}
			prompt = after
			for _, line := range strings.Split(frontmatter, "\n") {
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "name":
					if v := frontmatterValue(value); v != "" {
						displayName = v
					}
				case "description":
					description = frontmatterValue(value)
				case "user-invocable":
					userInvocable = frontmatterBool(value, true)
				}
			}
		}
	}

	return displayName, description, strings.TrimSpace(prompt), userInvocable
}

func frontmatterValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return strings.TrimSpace(value)
}

func frontmatterBool(value string, fallback bool) bool {
	switch strings.ToLower(frontmatterValue(value)) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	default:
		return fallback
	}
}

func commandName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	var previousSeparator bool
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			previousSeparator = false
		case r == '-' || r == '_':
			if b.Len() > 0 && !previousSeparator {
				b.WriteRune(r)
				previousSeparator = true
			}
		case r == ' ' || r == '.':
			if b.Len() > 0 && !previousSeparator {
				b.WriteByte('-')
				previousSeparator = true
			}
		}
	}
	return strings.Trim(b.String(), "-_")
}

func pluginSkillRoots(pluginsDir string) ([]string, error) {
	info, err := os.Stat(pluginsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	var roots []string
	err = filepath.WalkDir(pluginsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrPermission) {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Name() == "skills" {
			roots = append(roots, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(roots)
	return roots, nil
}

type codexConfigFile struct {
	Plugins map[string]struct {
		Enabled *bool `toml:"enabled"`
	} `toml:"plugins"`
	Skills struct {
		Config []struct {
			Path    string `toml:"path"`
			Enabled *bool  `toml:"enabled"`
		} `toml:"config"`
	} `toml:"skills"`
}

func loadCodexConfig(path string) (codexConfigFile, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return codexConfigFile{}, false, nil
		}
		return codexConfigFile{}, false, err
	}
	var cfg codexConfigFile
	if _, err := toml.Decode(string(raw), &cfg); err != nil {
		return codexConfigFile{}, false, err
	}
	return cfg, true, nil
}

func codexPluginSkillRoots(pluginsDir string, cfg codexConfigFile, hasConfig bool) ([]string, error) {
	if !hasConfig || len(cfg.Plugins) == 0 {
		return pluginSkillRoots(pluginsDir)
	}
	var roots []string
	seen := make(map[string]struct{})
	pluginKeys := make([]string, 0, len(cfg.Plugins))
	for key, plugin := range cfg.Plugins {
		if boolValue(plugin.Enabled, false) {
			pluginKeys = append(pluginKeys, strings.ToLower(strings.TrimSpace(key)))
		}
	}
	sort.Strings(pluginKeys)
	for _, key := range pluginKeys {
		for _, pluginPath := range codexPluginPaths(pluginsDir, key) {
			pluginRoots, err := pluginSkillRoots(pluginPath)
			if err != nil {
				return nil, err
			}
			for _, root := range pluginRoots {
				clean := filepath.Clean(root)
				if _, ok := seen[clean]; ok {
					continue
				}
				seen[clean] = struct{}{}
				roots = append(roots, clean)
			}
		}
	}
	return roots, nil
}

func codexPluginPaths(pluginsDir string, pluginKey string) []string {
	plugin, marketplace, ok := strings.Cut(strings.ToLower(strings.TrimSpace(pluginKey)), "@")
	if !ok || plugin == "" || marketplace == "" {
		return []string{
			filepath.Join(pluginsDir, "cache", pluginKey),
			filepath.Join(pluginsDir, pluginKey),
		}
	}
	return []string{
		filepath.Join(pluginsDir, "cache", marketplace, plugin),
		filepath.Join(pluginsDir, marketplace, plugin),
		filepath.Join(pluginsDir, plugin),
	}
}

func codexConfigPath(path string, codexHome string, homeDir string) string {
	path = expandHomePath(strings.TrimSpace(path), homeDir)
	if !filepath.IsAbs(path) {
		path = filepath.Join(codexHome, path)
	}
	return cleanAbsPath(path)
}

func claudePluginSkillRoots(claudeConfig string) ([]string, error) {
	var roots []string
	seen := make(map[string]struct{})
	addRoots := func(next []string) {
		for _, root := range next {
			clean := filepath.Clean(root)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			roots = append(roots, clean)
		}
	}

	installedPaths, err := installedClaudePluginPaths(filepath.Join(claudeConfig, "plugins", "installed_plugins.json"))
	if err != nil {
		return nil, err
	}
	enabledPlugins, hasEnabledPlugins, err := enabledClaudePlugins(filepath.Join(claudeConfig, "settings.json"))
	if err != nil {
		return nil, err
	}
	if !hasEnabledPlugins {
		for key := range installedPaths {
			enabledPlugins[key] = true
		}
	}
	marketplaces, err := knownClaudeMarketplaces(filepath.Join(claudeConfig, "plugins", "known_marketplaces.json"))
	if err != nil {
		return nil, err
	}

	pluginKeys := make([]string, 0, len(enabledPlugins))
	for key, enabled := range enabledPlugins {
		if enabled {
			pluginKeys = append(pluginKeys, key)
		}
	}
	sort.Strings(pluginKeys)
	for _, key := range pluginKeys {
		for _, installPath := range installedPaths[key] {
			installRoots, err := pluginSkillRoots(installPath)
			if err != nil {
				return nil, err
			}
			addRoots(installRoots)
		}
		for _, marketPath := range marketplacePluginPaths(key, marketplaces) {
			marketRoots, err := pluginSkillRoots(marketPath)
			if err != nil {
				return nil, err
			}
			addRoots(marketRoots)
		}
	}

	return roots, nil
}

func installedClaudePluginPaths(path string) (map[string][]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return nil, nil
		}
		return nil, err
	}
	var payload struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	paths := make(map[string][]string, len(payload.Plugins))
	for key, installs := range payload.Plugins {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		for _, install := range installs {
			path := strings.TrimSpace(install.InstallPath)
			if path != "" {
				paths[key] = append(paths[key], path)
			}
		}
	}
	return paths, nil
}

func enabledClaudePlugins(path string) (map[string]bool, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return map[string]bool{}, false, nil
		}
		return nil, false, err
	}
	var payload struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false, err
	}
	result := make(map[string]bool, len(payload.EnabledPlugins))
	for key, enabled := range payload.EnabledPlugins {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" {
			result[key] = enabled
		}
	}
	return result, payload.EnabledPlugins != nil, nil
}

func knownClaudeMarketplaces(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return nil, nil
		}
		return nil, err
	}
	var payload map[string]struct {
		InstallLocation string `json:"installLocation"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(payload))
	for name, marketplace := range payload {
		name = strings.ToLower(strings.TrimSpace(name))
		location := strings.TrimSpace(marketplace.InstallLocation)
		if name != "" && location != "" {
			result[name] = location
		}
	}
	return result, nil
}

func marketplacePluginPaths(pluginKey string, marketplaces map[string]string) []string {
	plugin, marketplace, ok := strings.Cut(strings.ToLower(strings.TrimSpace(pluginKey)), "@")
	if !ok || plugin == "" || marketplace == "" {
		return nil
	}
	marketplacePath := strings.TrimSpace(marketplaces[marketplace])
	if marketplacePath == "" {
		return nil
	}
	return []string{
		filepath.Join(marketplacePath, "plugins", plugin),
		filepath.Join(marketplacePath, "external_plugins", plugin),
		filepath.Join(marketplacePath, plugin),
	}
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func homeDir(opts DiscoverOptions) (string, error) {
	if home := strings.TrimSpace(opts.HomeDir); home != "" {
		return home, nil
	}
	if home := strings.TrimSpace(envValue(opts, "HOME")); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home, nil
}

func claudeConfigDir(opts DiscoverOptions, homeDir string) string {
	if configDir := strings.TrimSpace(envValue(opts, "CLAUDE_CONFIG_DIR")); configDir != "" {
		if abs, err := filepath.Abs(configDir); err == nil {
			return abs
		}
		return filepath.Clean(configDir)
	}
	return filepath.Join(homeDir, ".claude")
}

func expandHomePath(path string, homeDir string) string {
	if path == "~" {
		return homeDir
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

func cleanAbsPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func envValue(opts DiscoverOptions, key string) string {
	if opts.Env != nil {
		if value, ok := opts.Env[key]; ok {
			return value
		}
	}
	return os.Getenv(key)
}

func reservedNameSet(names []string) map[string]struct{} {
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		key := CanonicalName(name)
		if key != "" {
			result[key] = struct{}{}
		}
	}
	return result
}
