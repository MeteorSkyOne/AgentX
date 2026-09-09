package httpapi

import (
	"testing"

	"github.com/meteorsky/agentx/internal/domain"
)

// Redaction strips tool payloads but must keep the subagent attribution, or the
// UI cannot group a subagent's work under the call that spawned it.
func TestRedactionKeepsSubagentAttribution(t *testing.T) {
	message := domain.Message{
		Metadata: map[string]any{
			"process": []any{
				map[string]any{
					"type":         "tool_call",
					"tool_name":    "Agent",
					"tool_call_id": "toolu_parent",
					"input":        map[string]any{"prompt": "secret prompt"},
				},
				map[string]any{
					"type":                "tool_call",
					"tool_name":           "Bash",
					"tool_call_id":        "toolu_child",
					"parent_tool_call_id": "toolu_parent",
					"input":               map[string]any{"command": "echo hi"},
				},
			},
		},
	}

	redacted := redactMessageProcessDetails(message)
	process, ok := redacted.Metadata["process"].([]map[string]any)
	if !ok {
		t.Fatalf("process = %#v", redacted.Metadata["process"])
	}
	if len(process) != 2 {
		t.Fatalf("expected 2 process items, got %d", len(process))
	}

	if _, leaked := process[1]["input"]; leaked {
		t.Errorf("redaction leaked tool input: %#v", process[1])
	}
	if got := process[1]["parent_tool_call_id"]; got != "toolu_parent" {
		t.Errorf("subagent attribution lost in redaction: %#v", process[1])
	}
	if _, present := process[0]["parent_tool_call_id"]; present {
		t.Errorf("main-agent item should not carry an attribution: %#v", process[0])
	}
}

// A skill invocation is only meaningful by its name, so redaction keeps the
// skill name and arguments while still dropping every other tool's input.
func TestRedactionKeepsSkillName(t *testing.T) {
	message := domain.Message{
		Metadata: map[string]any{
			"process": []any{
				map[string]any{
					"type":         "tool_call",
					"tool_name":    "Skill",
					"tool_call_id": "toolu_skill",
					"input":        map[string]any{"skill": "code-review", "args": "--fix", "extra": "dropped"},
				},
				map[string]any{
					"type":         "tool_call",
					"tool_name":    "Bash",
					"tool_call_id": "toolu_bash",
					"input":        map[string]any{"command": "echo hi"},
				},
			},
		},
	}

	redacted := redactMessageProcessDetails(message)
	process, ok := redacted.Metadata["process"].([]map[string]any)
	if !ok || len(process) != 2 {
		t.Fatalf("process = %#v", redacted.Metadata["process"])
	}

	input, ok := process[0]["input"].(map[string]any)
	if !ok {
		t.Fatalf("skill input missing from summary: %#v", process[0])
	}
	if input["skill"] != "code-review" || input["args"] != "--fix" {
		t.Errorf("skill summary input = %#v", input)
	}
	if _, leaked := input["extra"]; leaked {
		t.Errorf("skill summary leaked unrelated input: %#v", input)
	}
	if _, leaked := process[1]["input"]; leaked {
		t.Errorf("redaction leaked tool input: %#v", process[1])
	}
}
