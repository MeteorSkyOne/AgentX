package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meteorsky/agentx/internal/domain"
)

func TestDiscoverScansRecursivelyAndParsesFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, ".codex", "skills", "reviewer", "SKILL.md"), `---
name: Code Reviewer
description: Review code carefully
---
# Instructions

Find concrete issues.
`)
	writeSkill(t, filepath.Join(root, ".codex", "skills", "nested", "tester", "SKILL.md"), `Test the change.`)
	writeSkill(t, filepath.Join(root, ".codex", "skills", "internal-only", "SKILL.md"), `---
name: internal-only
description: Hidden background skill
user-invocable: false
---
Hidden.
`)

	result, err := Discover(DiscoverOptions{
		AgentKind:       domain.AgentKindCodex,
		ConfigWorkspace: root,
		HomeDir:         filepath.Join(root, "home"),
		Env:             map[string]string{"CODEX_HOME": filepath.Join(root, "codex-home")},
	})
	if err != nil {
		t.Fatal(err)
	}

	reviewer, ok := Match(result, "code_reviewer")
	if !ok {
		t.Fatalf("code reviewer skill missing from %#v", result)
	}
	if reviewer.Name != "code-reviewer" || reviewer.DisplayName != "Code Reviewer" || reviewer.Description != "Review code carefully" {
		t.Fatalf("reviewer = %#v", reviewer)
	}
	if strings.Contains(reviewer.Prompt, "description:") || !strings.Contains(reviewer.Prompt, "Find concrete issues.") {
		t.Fatalf("prompt = %q, want frontmatter stripped and body retained", reviewer.Prompt)
	}

	tester, ok := Match(result, "tester")
	if !ok {
		t.Fatalf("tester skill missing from %#v", result)
	}
	if tester.DisplayName != "tester" || tester.Prompt != "Test the change." {
		t.Fatalf("tester = %#v", tester)
	}
	if _, ok := Match(result, "internal-only"); ok {
		t.Fatalf("internal-only user-invocable=false skill unexpectedly present in %#v", result)
	}
}

func TestDiscoverMarksReservedNamesAndDedupesByNormalizedName(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, ".codex", "skills", "first", "SKILL.md"), `---
name: review_plan
description: Local copy
---
local instructions
`)
	writeSkill(t, filepath.Join(root, ".claude", "skills", "second", "SKILL.md"), `---
name: review-plan
description: Later copy
---
later instructions
`)

	result, err := Discover(DiscoverOptions{
		AgentKind:       domain.AgentKindCodex,
		ConfigWorkspace: root,
		HomeDir:         filepath.Join(root, "home"),
		Env:             map[string]string{"CODEX_HOME": filepath.Join(root, "codex-home")},
		ReservedNames:   []string{"review-plan"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("skills = %#v, want one deduped skill", result)
	}
	if !result[0].ConflictsWithBuiltin {
		t.Fatalf("skill conflict flag = false, want true: %#v", result[0])
	}
	if result[0].Description != "Local copy" {
		t.Fatalf("skill = %#v, want first root to win", result[0])
	}
	if _, ok := Match(result, "review-plan"); !ok {
		t.Fatal("review-plan did not match review_plan")
	}
}

func TestCodexRootsIncludeHomeSuperpowersPluginsAndClaudeSkills(t *testing.T) {
	base := t.TempDir()
	codexHome := filepath.Join(base, "codex")
	home := filepath.Join(base, "home")
	writeSkill(t, filepath.Join(codexHome, "skills", "home-skill", "SKILL.md"), "home skill")
	writeSkill(t, filepath.Join(codexHome, "superpowers", "skills", "power", "SKILL.md"), "power skill")
	writeSkill(t, filepath.Join(codexHome, "plugins", "vendor", "plugin", "skills", "plugin-skill", "SKILL.md"), "plugin skill")
	writeSkill(t, filepath.Join(home, ".claude", "skills", "claude-skill", "SKILL.md"), "claude skill")

	result, err := Discover(DiscoverOptions{
		AgentKind: domain.AgentKindCodex,
		HomeDir:   home,
		Env:       map[string]string{"CODEX_HOME": codexHome},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"home-skill", "power", "plugin-skill", "claude-skill"} {
		if _, ok := Match(result, name); !ok {
			t.Fatalf("skill %q missing from %#v", name, result)
		}
	}
}

func TestCodexRootsRespectConfiguredPluginAndSkillEnablement(t *testing.T) {
	base := t.TempDir()
	codexHome := filepath.Join(base, "codex")
	home := filepath.Join(base, "home")
	enabledPlugin := filepath.Join(codexHome, "plugins", "cache", "openai-curated", "github", "abcd")
	disabledPlugin := filepath.Join(codexHome, "plugins", "cache", "openai-curated", "slack", "abcd")
	disabledSkill := filepath.Join(codexHome, "superpowers", "skills", "disabled-power", "SKILL.md")
	explicitSkill := filepath.Join(base, "extra", "explicit", "SKILL.md")
	writeSkill(t, filepath.Join(enabledPlugin, "skills", "github-skill", "SKILL.md"), "github")
	writeSkill(t, filepath.Join(disabledPlugin, "skills", "slack-skill", "SKILL.md"), "slack")
	writeSkill(t, disabledSkill, "disabled")
	writeSkill(t, explicitSkill, "explicit")
	writeFile(t, filepath.Join(codexHome, "config.toml"), `
[plugins."github@openai-curated"]
enabled = true

[plugins."slack@openai-curated"]
enabled = false

[[skills.config]]
path = "`+disabledSkill+`"
enabled = false

[[skills.config]]
path = "`+explicitSkill+`"
enabled = true
`)

	result, err := Discover(DiscoverOptions{
		AgentKind: domain.AgentKindCodex,
		HomeDir:   home,
		Env:       map[string]string{"CODEX_HOME": codexHome},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"github-skill", "explicit"} {
		if _, ok := Match(result, name); !ok {
			t.Fatalf("skill %q missing from %#v", name, result)
		}
	}
	for _, name := range []string{"slack-skill", "disabled-power"} {
		if _, ok := Match(result, name); ok {
			t.Fatalf("disabled skill %q unexpectedly present in %#v", name, result)
		}
	}
}

func TestClaudeRootsUseWorkspaceHomeAndPluginSkills(t *testing.T) {
	base := t.TempDir()
	configWorkspace := filepath.Join(base, "config")
	runWorkspace := filepath.Join(base, "run")
	home := filepath.Join(base, "home")
	claudeConfigDir := filepath.Join(base, "custom-claude")
	cachePlugin := filepath.Join(claudeConfigDir, "plugins", "cache", "vendor", "plugin", "1.0.0")
	externalPlugin := filepath.Join(base, "external-plugin")
	disabledPlugin := filepath.Join(base, "disabled-plugin")
	marketplace := filepath.Join(claudeConfigDir, "plugins", "marketplaces", "official")
	writeSkill(t, filepath.Join(configWorkspace, ".claude", "skills", "config-skill", "SKILL.md"), "config")
	writeSkill(t, filepath.Join(runWorkspace, ".claude", "skills", "run-skill", "SKILL.md"), "run")
	writeSkill(t, filepath.Join(claudeConfigDir, "skills", "home-skill", "SKILL.md"), "home")
	writeSkill(t, filepath.Join(cachePlugin, "skills", "plugin-skill", "SKILL.md"), "plugin")
	writeSkill(t, filepath.Join(externalPlugin, "skills", "installed-skill", "SKILL.md"), "installed")
	writeSkill(t, filepath.Join(disabledPlugin, "skills", "disabled-skill", "SKILL.md"), "disabled")
	writeSkill(t, filepath.Join(marketplace, "plugins", "market-plugin", "skills", "market-skill", "SKILL.md"), "market")
	writeSkill(t, filepath.Join(marketplace, "plugins", "disabled-market", "skills", "disabled-market", "SKILL.md"), "disabled market")
	writeFile(t, filepath.Join(claudeConfigDir, "plugins", "installed_plugins.json"), `{
  "version": 2,
  "plugins": {
    "cache@local": [
      {
        "scope": "user",
        "installPath": "`+cachePlugin+`",
        "version": "1.0.0"
      }
    ],
    "external@local": [
      {
        "scope": "user",
        "installPath": "`+externalPlugin+`",
        "version": "1.0.0"
      }
    ],
    "disabled@local": [
      {
        "scope": "user",
        "installPath": "`+disabledPlugin+`",
        "version": "1.0.0"
      }
    ]
  }
}`)
	writeFile(t, filepath.Join(claudeConfigDir, "plugins", "known_marketplaces.json"), `{
  "official": {
    "installLocation": "`+marketplace+`"
  }
}`)
	writeFile(t, filepath.Join(claudeConfigDir, "settings.json"), `{
  "enabledPlugins": {
    "cache@local": true,
    "external@local": true,
    "disabled@local": false,
    "market-plugin@official": true,
    "disabled-market@official": false
  }
}`)

	result, err := Discover(DiscoverOptions{
		AgentKind:       domain.AgentKindClaude,
		ConfigWorkspace: configWorkspace,
		RunWorkspace:    runWorkspace,
		HomeDir:         home,
		Env:             map[string]string{"CLAUDE_CONFIG_DIR": claudeConfigDir},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"config-skill", "run-skill", "home-skill", "plugin-skill", "installed-skill", "market-skill"} {
		if _, ok := Match(result, name); !ok {
			t.Fatalf("skill %q missing from %#v", name, result)
		}
	}
	for _, name := range []string{"disabled-skill", "disabled-market"} {
		if _, ok := Match(result, name); ok {
			t.Fatalf("disabled skill %q unexpectedly present in %#v", name, result)
		}
	}
}

func writeSkill(t *testing.T, path string, body string) {
	t.Helper()
	writeFile(t, path, body)
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
