package pi

import "testing"

func TestSplitModelThinking(t *testing.T) {
	cases := []struct {
		in, model, level string
	}{
		{"sonnet:high", "sonnet", "high"},
		{"anthropic/claude-sonnet-4:max", "anthropic/claude-sonnet-4", "max"},
		{"gpt-5", "gpt-5", ""},
		{"openai/o3:unknown", "openai/o3:unknown", ""},
		{"  deepseek-v4 ", "deepseek-v4", ""},
	}
	for _, tc := range cases {
		model, level := splitModelThinking(tc.in)
		if model != tc.model || level != tc.level {
			t.Errorf("splitModelThinking(%q) = %q, %q; want %q, %q", tc.in, model, level, tc.model, tc.level)
		}
	}
}

func TestThinkingLevelForEffort(t *testing.T) {
	cases := []struct {
		effort, suffix, want string
	}{
		{"high", "", "high"},
		{"XHIGH", "", "xhigh"},
		{"off", "", "off"},
		{"", "medium", "medium"},
		{"", "", ""},
		{"bogus", "low", "low"},
		{"none", "", "off"},
	}
	for _, tc := range cases {
		if got := thinkingLevelForEffort(tc.effort, tc.suffix); got != tc.want {
			t.Errorf("thinkingLevelForEffort(%q, %q) = %q, want %q", tc.effort, tc.suffix, got, tc.want)
		}
	}
}

func TestClampThinkingLevel(t *testing.T) {
	available := []string{"off", "low", "medium", "high"}
	cases := []struct {
		level, want string
	}{
		{"high", "high"},
		{"xhigh", "high"},
		{"max", "high"},
		{"minimal", "off"},
		{"", ""},
		{"weird", "weird"},
	}
	for _, tc := range cases {
		if got := clampThinkingLevel(tc.level, available); got != tc.want {
			t.Errorf("clampThinkingLevel(%q) = %q, want %q", tc.level, got, tc.want)
		}
	}
	if got := clampThinkingLevel("max", []string{"off"}); got != "off" {
		t.Errorf("clampThinkingLevel(max, [off]) = %q, want off", got)
	}
}

func TestFindModel(t *testing.T) {
	models := []modelRef{
		{Provider: "anthropic", ID: "claude-sonnet-4", Name: "Claude Sonnet 4"},
		{Provider: "openai", ID: "gpt-5", Name: "GPT-5"},
		{Provider: "openrouter", ID: "openai/gpt-5", Name: "GPT-5 (OpenRouter)"},
	}
	cases := []struct {
		want     string
		provider string
		id       string
		ok       bool
	}{
		{"anthropic/claude-sonnet-4", "anthropic", "claude-sonnet-4", true},
		{"gpt-5", "openai", "gpt-5", true},
		{"openrouter/openai/gpt-5", "openrouter", "openai/gpt-5", true},
		{"Claude Sonnet 4", "anthropic", "claude-sonnet-4", true},
		{"sonnet", "anthropic", "claude-sonnet-4", true},
		{"nope", "", "", false},
	}
	for _, tc := range cases {
		ref, ok := findModel(models, tc.want)
		if ok != tc.ok || ref.Provider != tc.provider || ref.ID != tc.id {
			t.Errorf("findModel(%q) = %+v, %v; want %s/%s, %v", tc.want, ref, ok, tc.provider, tc.id, tc.ok)
		}
	}
}

func TestMatchesModel(t *testing.T) {
	current := modelRef{Provider: "anthropic", ID: "claude-sonnet-4", Name: "Claude Sonnet 4"}
	if !matchesModel(current, "") || !matchesModel(current, "claude-sonnet-4") || !matchesModel(current, "anthropic/claude-sonnet-4") {
		t.Fatal("expected current model to match itself")
	}
	if matchesModel(current, "openai/claude-sonnet-4") || matchesModel(current, "gpt-5") {
		t.Fatal("expected other models not to match")
	}
}
