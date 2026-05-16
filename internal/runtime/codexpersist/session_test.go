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

	policy := sandboxPolicyFromParams(t, params)
	if got := policy["type"]; got != "readOnly" {
		t.Fatalf("sandbox policy type = %v, want readOnly", got)
	}
	assertNoPermissionProfile(t, params)
}

func TestNewPersistentSessionUsesPreviousThreadAsCurrentSessionID(t *testing.T) {
	s := newPersistentSession(nil, nil, "agent:channel", nil, runtime.StartSessionRequest{
		PreviousSessionID: "thread_previous",
	})

	if got := s.CurrentSessionID(); got != "thread_previous" {
		t.Fatalf("session id = %q, want previous thread id", got)
	}
}

func TestNewPersistentSessionIgnoresCodexFallbackSessionID(t *testing.T) {
	s := newPersistentSession(nil, nil, "agent:channel", nil, runtime.StartSessionRequest{
		PreviousSessionID: "codex:agent:channel",
	})

	if s.threadID != "" {
		t.Fatalf("thread id = %q, want empty for fallback session id", s.threadID)
	}
	if got := s.CurrentSessionID(); got != "codex:agent:channel" {
		t.Fatalf("session id = %q, want fallback id", got)
	}
}

func TestRPCFailureEventMarksThreadNotFoundAsStale(t *testing.T) {
	evt := rpcFailureEvent("turn/start failed", &jsonRPCError{
		Code:    -32600,
		Message: "thread not found: 019e2f84-98a0-77b0-9c52-277e5cd5bc4e",
	})

	if evt.Type != runtime.EventFailed {
		t.Fatalf("event type = %v, want failed", evt.Type)
	}
	if !evt.StaleSession {
		t.Fatalf("stale session = false, want true for %q", evt.Error)
	}
}

func TestNotificationErrorMarksThreadNotFoundAsStale(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 1)}
	state := newNotificationState()

	terminal := s.handleNotification(jsonRPCMessage{
		Method: "error",
		Params: map[string]any{
			"message": "thread not found: 019e2f84-98a0-77b0-9c52-277e5cd5bc4e",
		},
	}, state)

	if !terminal {
		t.Fatalf("notification should be terminal")
	}
	evt := <-s.events
	if !evt.StaleSession {
		t.Fatalf("stale session = false, want true for %q", evt.Error)
	}
}

func TestBuildUserInputTreatsOnlyImageKindAsLocalImage(t *testing.T) {
	items := buildUserInput(runtime.Input{
		Prompt: "inspect this file",
		Attachments: []runtime.Attachment{{
			ID:          "att_svg",
			Filename:    "diagram.svg",
			ContentType: "image/svg+xml",
			Kind:        "file",
			LocalPath:   "/tmp/diagram.svg",
		}},
	})

	if len(items) != 1 {
		t.Fatalf("items = %#v, want only text input", items)
	}
	if got := items[0]["type"]; got != "text" {
		t.Fatalf("first item type = %#v, want text", got)
	}
}

func TestEmitAfterCloseEventStreamDoesNotPanic(t *testing.T) {
	s := &persistentSession{
		events: make(chan runtime.Event, 1),
		done:   make(chan struct{}),
	}
	s.closeEventStream()

	for i := 0; i < 1000; i++ {
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: "late event"})
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

	policy := sandboxPolicyFromParams(t, params)
	if got := policy["type"]; got != "workspaceWrite" {
		t.Fatalf("sandbox policy type = %v, want workspaceWrite", got)
	}
	assertNoPermissionProfile(t, params)
}

func TestAddTurnOverridesDisablesSandboxForYoloNormalTurns(t *testing.T) {
	s := &persistentSession{
		req: runtime.StartSessionRequest{
			YoloMode: true,
		},
	}

	params := map[string]any{}
	s.addTurnOverrides(params)

	if got := params["approvalPolicy"]; got != "never" {
		t.Fatalf("approvalPolicy = %v, want never", got)
	}
	policy := sandboxPolicyFromParams(t, params)
	if got := policy["type"]; got != "dangerFullAccess" {
		t.Fatalf("sandbox policy type = %v, want dangerFullAccess", got)
	}
	assertNoPermissionProfile(t, params)
}

func TestAddThreadOverridesUsesDangerFullAccessForYolo(t *testing.T) {
	s := &persistentSession{
		req: runtime.StartSessionRequest{
			YoloMode: true,
		},
	}

	params := map[string]any{}
	s.addThreadOverrides(params)

	if got := params["approvalPolicy"]; got != "never" {
		t.Fatalf("approvalPolicy = %v, want never", got)
	}
	if got := params["sandbox"]; got != "danger-full-access" {
		t.Fatalf("sandbox = %v, want danger-full-access", got)
	}
	assertNoPermissionProfile(t, params)
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

func sandboxPolicyFromParams(t *testing.T, params map[string]any) map[string]any {
	t.Helper()
	policy, ok := params["sandboxPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("sandboxPolicy = %#v, want map", params["sandboxPolicy"])
	}
	return policy
}

func assertNoPermissionProfile(t *testing.T, params map[string]any) {
	t.Helper()
	if _, ok := params["permissionProfile"]; ok {
		t.Fatalf("permissionProfile should not be sent to turn/start: %#v", params["permissionProfile"])
	}
}
