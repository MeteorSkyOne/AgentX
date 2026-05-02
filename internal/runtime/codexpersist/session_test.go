package codexpersist

import (
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func TestAddTurnOverridesUsesPlanMode(t *testing.T) {
	s := &persistentSession{
		req: runtime.StartSessionRequest{
			Model:          "gpt-test",
			Effort:         "high",
			PermissionMode: "plan",
		},
	}

	params := map[string]any{}
	s.addTurnOverrides(params)

	mode := collaborationModeFromParams(t, params)
	if got := mode["mode"]; got != "plan" {
		t.Fatalf("mode = %v, want plan", got)
	}

	settings := settingsFromMode(t, mode)
	if got := settings["model"]; got != "gpt-test" {
		t.Fatalf("model = %v, want gpt-test", got)
	}
	if got := settings["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %v, want high", got)
	}
	if _, ok := settings["developer_instructions"]; !ok {
		t.Fatalf("developer_instructions key missing")
	}
	if got := settings["developer_instructions"]; got != nil {
		t.Fatalf("developer_instructions = %v, want nil", got)
	}

	profile := permissionProfileFromParams(t, params)
	if got := profile["type"]; got != "managed" {
		t.Fatalf("permission profile type = %v, want managed", got)
	}
	entries := fileSystemEntriesFromProfile(t, profile)
	if len(entries) != 1 {
		t.Fatalf("read-only entries = %d, want 1", len(entries))
	}
	if got := entries[0]["access"]; got != "read" {
		t.Fatalf("plan access = %v, want read", got)
	}
}

func TestAddTurnOverridesDefaultsPlanEffortToMedium(t *testing.T) {
	s := &persistentSession{
		req: runtime.StartSessionRequest{
			PermissionMode: "plan",
		},
		model: "gpt-thread",
	}

	params := map[string]any{}
	s.addTurnOverrides(params)

	mode := collaborationModeFromParams(t, params)
	settings := settingsFromMode(t, mode)
	if got := settings["model"]; got != "gpt-thread" {
		t.Fatalf("model = %v, want gpt-thread", got)
	}
	if got := settings["reasoning_effort"]; got != "medium" {
		t.Fatalf("reasoning_effort = %v, want medium", got)
	}
}

func TestAddTurnOverridesUsesDefaultModeForNormalTurns(t *testing.T) {
	s := &persistentSession{
		req: runtime.StartSessionRequest{
			Model:  "gpt-test",
			Effort: "low",
		},
	}

	params := map[string]any{}
	s.addTurnOverrides(params)

	mode := collaborationModeFromParams(t, params)
	if got := mode["mode"]; got != "default" {
		t.Fatalf("mode = %v, want default", got)
	}

	settings := settingsFromMode(t, mode)
	if got := settings["model"]; got != "gpt-test" {
		t.Fatalf("model = %v, want gpt-test", got)
	}
	if got := settings["reasoning_effort"]; got != "low" {
		t.Fatalf("reasoning_effort = %v, want low", got)
	}

	profile := permissionProfileFromParams(t, params)
	if got := profile["type"]; got != "managed" {
		t.Fatalf("permission profile type = %v, want managed", got)
	}
	entries := fileSystemEntriesFromProfile(t, profile)
	if !hasSpecialEntry(entries, "project_roots", "write") {
		t.Fatalf("workspace-write profile missing project_roots write entry: %#v", entries)
	}
	if !hasSpecialEntry(entries, "root", "read") {
		t.Fatalf("workspace-write profile missing root read entry: %#v", entries)
	}
}

func TestAddTurnOverridesDisablesSandboxForYoloNormalTurns(t *testing.T) {
	s := &persistentSession{
		req: runtime.StartSessionRequest{
			YoloMode: true,
		},
	}

	params := map[string]any{}
	s.addTurnOverrides(params)

	profile := permissionProfileFromParams(t, params)
	if got := profile["type"]; got != "disabled" {
		t.Fatalf("permission profile type = %v, want disabled", got)
	}
}

func TestPlanDeltaIsEmittedAsText(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 4)}
	state := newNotificationState()

	terminal := s.handleNotification(jsonRPCMessage{
		Method: "item/plan/delta",
		Params: map[string]any{
			"itemId": "plan-1",
			"delta":  "1. Inspect\n",
		},
	}, state)
	if terminal {
		t.Fatalf("plan delta should not be terminal")
	}
	if got := state.textString(); got != "1. Inspect\n" {
		t.Fatalf("state text = %q, want plan delta", got)
	}

	evt := <-s.events
	if evt.Type != runtime.EventDelta || evt.Text != "1. Inspect\n" {
		t.Fatalf("event = %#v, want text delta", evt)
	}
}

func TestCompletedPlanUsesAuthoritativeTextWithoutDuplicatingDelta(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 4)}
	state := newNotificationState()
	state.streamedPlanItems["plan-1"] = true

	s.handleNotification(jsonRPCMessage{
		Method: "item/completed",
		Params: map[string]any{
			"item": map[string]any{
				"id":   "plan-1",
				"type": "plan",
				"text": "authoritative plan",
			},
		},
	}, state)

	if got := state.textString(); got != "authoritative plan" {
		t.Fatalf("state text = %q, want authoritative completed plan", got)
	}
	select {
	case evt := <-s.events:
		t.Fatalf("unexpected event: %#v", evt)
	default:
	}
}

func TestCompletedPlanFallsBackWhenNoDeltaStreamed(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 4)}
	state := newNotificationState()

	s.handleNotification(jsonRPCMessage{
		Method: "item/completed",
		Params: map[string]any{
			"item": map[string]any{
				"id":   "plan-1",
				"type": "plan",
				"text": "fallback plan",
			},
		},
	}, state)

	if got := state.textString(); got != "fallback plan" {
		t.Fatalf("state text = %q, want fallback plan", got)
	}
	evt := <-s.events
	if evt.Type != runtime.EventDelta || evt.Text != "fallback plan" {
		t.Fatalf("event = %#v, want fallback plan delta", evt)
	}
}

func TestItemToProcessItemNormalizesTypes(t *testing.T) {
	cases := []struct {
		name     string
		itemType string
		wantType string
	}{
		{"snake_case", "command_execution", "tool_call"},
		{"camelCase", "commandExecution", "tool_call"},
		{"function_call", "function_call", "tool_call"},
		{"functionCall", "functionCall", "tool_call"},
		{"fileChange", "fileChange", "tool_call"},
		{"file_change", "file_change", "tool_call"},
		{"mcpToolCall", "mcpToolCall", "tool_call"},
		{"mcp_tool_call", "mcp_tool_call", "tool_call"},
		{"webSearch", "webSearch", "tool_call"},
		{"web_search", "web_search", "tool_call"},
		{"fileSearch", "fileSearch", "tool_call"},
		{"dynamicToolCall", "dynamicToolCall", "tool_call"},
		{"reasoning", "reasoning", "thinking"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{
				"type": tc.itemType,
				"id":   "call_1",
				"name": "test_tool",
			}
			if tc.wantType == "thinking" {
				item["text"] = "thinking about it"
			}
			pi := itemToProcessItem(item, "started")
			if pi == nil {
				t.Fatalf("itemToProcessItem(%q) = nil, want %q", tc.itemType, tc.wantType)
			}
			if pi.Type != tc.wantType {
				t.Fatalf("process item type = %q, want %q", pi.Type, tc.wantType)
			}
		})
	}
}

func TestItemToProcessItemRejectsUnknownTypes(t *testing.T) {
	unknowns := []string{"message", "agent_message", "plan", "unknown", ""}
	for _, itemType := range unknowns {
		t.Run(itemType, func(t *testing.T) {
			item := map[string]any{"type": itemType, "id": "x"}
			if pi := itemToProcessItem(item, "started"); pi != nil {
				t.Fatalf("itemToProcessItem(%q) = %#v, want nil", itemType, pi)
			}
		})
	}
}

func TestItemStartedEmitsToolCallProcessItem(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 4)}
	state := newNotificationState()

	s.handleNotification(jsonRPCMessage{
		Method: "item/started",
		Params: map[string]any{
			"item": map[string]any{
				"type":    "commandExecution",
				"id":      "cmd_1",
				"command": "ls -la",
			},
		},
	}, state)

	evt := <-s.events
	if evt.Type != runtime.EventDelta {
		t.Fatalf("event type = %v, want EventDelta", evt.Type)
	}
	if len(evt.Process) != 1 {
		t.Fatalf("process items = %d, want 1", len(evt.Process))
	}
	pi := evt.Process[0]
	if pi.Type != "tool_call" || pi.ToolCallID != "cmd_1" || pi.Status != "started" {
		t.Fatalf("process item = %#v", pi)
	}
}

func TestItemCompletedEmitsToolCallProcessItem(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 4)}
	state := newNotificationState()

	s.handleNotification(jsonRPCMessage{
		Method: "item/completed",
		Params: map[string]any{
			"item": map[string]any{
				"type":   "commandExecution",
				"id":     "cmd_1",
				"name":   "Bash",
				"output": "README.md\n",
			},
		},
	}, state)

	evt := <-s.events
	if evt.Type != runtime.EventDelta {
		t.Fatalf("event type = %v, want EventDelta", evt.Type)
	}
	if len(evt.Process) != 1 {
		t.Fatalf("process items = %d, want 1", len(evt.Process))
	}
	pi := evt.Process[0]
	if pi.Type != "tool_call" || pi.ToolCallID != "cmd_1" || pi.Status != "completed" {
		t.Fatalf("process item = %#v", pi)
	}
	if pi.Output != "README.md\n" {
		t.Fatalf("output = %v, want README.md", pi.Output)
	}
}

func collaborationModeFromParams(t *testing.T, params map[string]any) map[string]any {
	t.Helper()
	mode, ok := params["collaborationMode"].(map[string]any)
	if !ok {
		t.Fatalf("collaborationMode = %#v, want map", params["collaborationMode"])
	}
	return mode
}

func settingsFromMode(t *testing.T, mode map[string]any) map[string]any {
	t.Helper()
	settings, ok := mode["settings"].(map[string]any)
	if !ok {
		t.Fatalf("settings = %#v, want map", mode["settings"])
	}
	return settings
}

func permissionProfileFromParams(t *testing.T, params map[string]any) map[string]any {
	t.Helper()
	profile, ok := params["permissionProfile"].(map[string]any)
	if !ok {
		t.Fatalf("permissionProfile = %#v, want map", params["permissionProfile"])
	}
	return profile
}

func fileSystemEntriesFromProfile(t *testing.T, profile map[string]any) []map[string]any {
	t.Helper()
	fileSystem, ok := profile["fileSystem"].(map[string]any)
	if !ok {
		t.Fatalf("fileSystem = %#v, want map", profile["fileSystem"])
	}
	rawEntries, ok := fileSystem["entries"].([]map[string]any)
	if !ok {
		t.Fatalf("entries = %#v, want []map[string]any", fileSystem["entries"])
	}
	return rawEntries
}

func hasSpecialEntry(entries []map[string]any, kind string, access string) bool {
	for _, entry := range entries {
		if entry["access"] != access {
			continue
		}
		path, _ := entry["path"].(map[string]any)
		value, _ := path["value"].(map[string]any)
		if value["kind"] == kind {
			return true
		}
	}
	return false
}
