package app

import (
	"testing"

	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

// Subagent attribution has to survive the runtime-to-domain conversion, or the
// UI never learns which items belong to a subagent.
func TestRuntimeProcessItemsCarrySubagentAttribution(t *testing.T) {
	items := runtimeProcessItems(agentruntime.Event{
		Process: []agentruntime.ProcessItem{
			{Type: "tool_call", ToolName: "Agent", ToolCallID: "toolu_parent"},
			{Type: "tool_call", ToolName: "Bash", ToolCallID: "toolu_child", ParentToolCallID: "toolu_parent"},
		},
	})

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ParentToolCallID != "" {
		t.Errorf("main-agent item gained an attribution: %#v", items[0])
	}
	if items[1].ParentToolCallID != "toolu_parent" {
		t.Errorf("subagent attribution lost in conversion: %#v", items[1])
	}
}
