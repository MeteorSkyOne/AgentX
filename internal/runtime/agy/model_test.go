package agy

import "testing"

func TestSplitModelEffort(t *testing.T) {
	available := []string{
		"gemini-4-carbon",
		"gemini-3.8-flash",
		"claude-sonnet-5-5",
		"gpt-oss-120b-medium",
	}

	tests := []struct {
		name       string
		raw        string
		avail      []string
		wantModel  string
		wantEffort string
	}{
		{
			name:       "exact base model with medium in name is preserved",
			raw:        "gpt-oss-120b-medium",
			avail:      available,
			wantModel:  "gpt-oss-120b-medium",
			wantEffort: "",
		},
		{
			name:       "gpt-oss-120b-medium preserved even without available list",
			raw:        "gpt-oss-120b-medium",
			avail:      nil,
			wantModel:  "gpt-oss-120b-medium",
			wantEffort: "",
		},
		{
			name:       "tier suffixed model splits into base and effort",
			raw:        "gemini-3.8-flash-low",
			avail:      available,
			wantModel:  "gemini-3.8-flash",
			wantEffort: "low",
		},
		{
			name:       "provider prefix stripped",
			raw:        "agy/gemini-4-carbon-xhigh",
			avail:      available,
			wantModel:  "gemini-4-carbon",
			wantEffort: "xhigh",
		},
		{
			name:       "unknown model with suffix kept intact when base not in available",
			raw:        "custom-model-v1-high",
			avail:      available,
			wantModel:  "custom-model-v1-high",
			wantEffort: "",
		},
		{
			name:       "tier suffixed model splits when available list empty",
			raw:        "gemini-4-argon-high",
			avail:      nil,
			wantModel:  "gemini-4-argon",
			wantEffort: "high",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotModel, gotEffort := splitModelEffort(tc.raw, tc.avail)
			if gotModel != tc.wantModel || gotEffort != tc.wantEffort {
				t.Fatalf("splitModelEffort(%q) = (%q, %q), want (%q, %q)", tc.raw, gotModel, gotEffort, tc.wantModel, tc.wantEffort)
			}
		})
	}
}

func TestClampThoughtLevel(t *testing.T) {
	avail := []string{"low", "high"}
	if got := clampThoughtLevel("medium", avail); got != "low" {
		t.Fatalf("clampThoughtLevel(medium, [low, high]) = %q, want low (lower on tie)", got)
	}
	if got := clampThoughtLevel("xhigh", avail); got != "high" {
		t.Fatalf("clampThoughtLevel(xhigh, [low, high]) = %q, want high", got)
	}
	if got := clampThoughtLevel("off", avail); got != "low" {
		t.Fatalf("clampThoughtLevel(off, [low, high]) = %q, want low", got)
	}
	if got := resolveThoughtLevel("max", "low", []string{"low", "medium", "high", "xhigh"}); got != "xhigh" {
		t.Fatalf("resolveThoughtLevel(max) = %q, want xhigh", got)
	}
}

func TestNormalizePermissionMode(t *testing.T) {
	cases := map[string]string{
		"":                  "accept-edits",
		"acceptEdits":       "accept-edits",
		"accept-edits":      "accept-edits",
		"bypassPermissions": "auto",
		"auto":              "auto",
		"plan":              "plan",
		"default":           "default",
	}
	for in, want := range cases {
		if got := normalizePermissionMode(in); got != want {
			t.Fatalf("normalizePermissionMode(%q) = %q, want %q", in, got, want)
		}
	}
}
