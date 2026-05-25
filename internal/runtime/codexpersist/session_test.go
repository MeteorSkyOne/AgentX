package codexpersist

import (
	"context"
	"errors"
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

func TestNotificationErrorIncludesDetailWhenMessageMissing(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 1)}
	state := newNotificationState()

	terminal := s.handleNotification(jsonRPCMessage{
		Method: "error",
		Params: map[string]any{
			"code":   float64(-32000),
			"detail": "model gpt-test is unavailable",
			"data": map[string]any{
				"request_id": "req_123",
			},
		},
	}, state)

	if !terminal {
		t.Fatalf("notification should be terminal")
	}
	evt := <-s.events
	if evt.Error != `codex app-server error: code=-32000, detail=model gpt-test is unavailable, data={"request_id":"req_123"}` {
		t.Fatalf("error = %q", evt.Error)
	}
}

func TestNotificationErrorIncludesNestedErrorData(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 1)}
	state := newNotificationState()

	terminal := s.handleNotification(jsonRPCMessage{
		Method: "error",
		Params: map[string]any{
			"error": map[string]any{
				"message": "provider rejected request",
				"data": map[string]any{
					"type": "rate_limit",
				},
			},
		},
	}, state)

	if !terminal {
		t.Fatalf("notification should be terminal")
	}
	evt := <-s.events
	if evt.Error != `provider rejected request: error_data={"type":"rate_limit"}` {
		t.Fatalf("error = %q", evt.Error)
	}
}

func TestNotificationErrorUsesStringParams(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 1)}
	state := newNotificationState()

	terminal := s.handleNotification(jsonRPCMessage{
		Method: "error",
		Params: "provider connection reset",
	}, state)

	if !terminal {
		t.Fatalf("notification should be terminal")
	}
	evt := <-s.events
	if evt.Error != "provider connection reset" {
		t.Fatalf("error = %q", evt.Error)
	}
}

func TestJSONRPCErrorIncludesData(t *testing.T) {
	err := (&jsonRPCError{
		Code:    -32000,
		Message: "request failed",
		Data: map[string]any{
			"reason": "quota exceeded",
		},
	}).Error()

	if err != `jsonrpc error -32000: request failed: data={"reason":"quota exceeded"}` {
		t.Fatalf("error = %q", err)
	}
}

func TestCodexAppServerExitedErrorIncludesStderr(t *testing.T) {
	err := codexAppServerExitedError("fatal: missing token\n")

	if err != "codex app-server exited: fatal: missing token" {
		t.Fatalf("error = %q", err)
	}
}

func TestTurnCompletedInterruptedEmitsCanceled(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 1)}
	state := newNotificationState()
	state.writeText("partial answer")

	terminal := s.handleNotification(jsonRPCMessage{
		Method: "turn/completed",
		Params: map[string]any{
			"turn": map[string]any{
				"status": "interrupted",
			},
		},
	}, state)

	if !terminal {
		t.Fatalf("interrupted turn should be terminal")
	}
	evt := <-s.events
	if evt.Type != runtime.EventCanceled {
		t.Fatalf("event type = %v, want canceled", evt.Type)
	}
	if evt.Text != "partial answer" {
		t.Fatalf("event text = %q, want partial answer", evt.Text)
	}
}

func TestThreadTokenUsageUpdatedEmitsContextUsage(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 1), model: "gpt-thread"}
	state := newNotificationState()

	terminal := s.handleNotification(jsonRPCMessage{
		Method: "thread/tokenUsage/updated",
		Params: map[string]any{
			"tokenUsage": map[string]any{
				"total": map[string]any{
					"inputTokens":           float64(700),
					"cachedInputTokens":     float64(100),
					"outputTokens":          float64(20),
					"reasoningOutputTokens": float64(5),
					"totalTokens":           float64(725),
				},
				"modelContextWindow": float64(4000),
			},
		},
	}, state)

	if terminal {
		t.Fatalf("token usage update should not be terminal")
	}
	evt := <-s.events
	if evt.Type != runtime.EventDelta || evt.Usage == nil || evt.Usage.Context == nil {
		t.Fatalf("event = %#v", evt)
	}
	contextUsage := evt.Usage.Context
	if ptrValue(contextUsage.TotalTokens) != 725 || ptrValue(contextUsage.ContextWindowTokens) != 4000 || contextUsage.Model != "gpt-thread" || contextUsage.Source != "thread/tokenUsage/updated" {
		t.Fatalf("context usage = %#v", contextUsage)
	}
}

func TestTurnStartResultSetsActiveTurnID(t *testing.T) {
	s := &persistentSession{}

	s.setActiveTurnID(turnIDFromResult(map[string]any{
		"turn": map[string]any{
			"id": "turn_123",
		},
	}))

	if s.activeTurnID != "turn_123" {
		t.Fatalf("active turn id = %q, want turn_123", s.activeTurnID)
	}
}

func TestSteerSendsTurnSteerWithActiveThreadAndTurn(t *testing.T) {
	rpc := &recordingSessionRPC{}
	s := &persistentSession{
		rpc:          rpc,
		alive:        true,
		threadID:     "thread_123",
		activeTurnID: "turn_456",
	}

	if err := s.Steer(context.Background(), runtime.Input{Prompt: "use this too"}); err != nil {
		t.Fatal(err)
	}

	if rpc.method != "turn/steer" {
		t.Fatalf("method = %q, want turn/steer", rpc.method)
	}
	params, ok := rpc.params.(map[string]any)
	if !ok {
		t.Fatalf("params = %#v, want map", rpc.params)
	}
	if got := params["threadId"]; got != "thread_123" {
		t.Fatalf("threadId = %#v, want thread_123", got)
	}
	if got := params["expectedTurnId"]; got != "turn_456" {
		t.Fatalf("expectedTurnId = %#v, want turn_456", got)
	}
	input, ok := params["input"].([]map[string]any)
	if !ok || len(input) != 1 || input[0]["text"] != "use this too" {
		t.Fatalf("input = %#v, want text input", params["input"])
	}
}

func TestSteerReturnsErrorWithoutActiveTurn(t *testing.T) {
	s := &persistentSession{
		rpc:      &recordingSessionRPC{},
		alive:    true,
		threadID: "thread_123",
	}

	err := s.Steer(context.Background(), runtime.Input{Prompt: "late"})
	if err == nil || !errors.Is(err, errNoActiveTurn) {
		t.Fatalf("Steer error = %v, want no active turn", err)
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

type recordingSessionRPC struct {
	method string
	params any
}

func (r *recordingSessionRPC) Call(ctx context.Context, method string, params any) (map[string]any, error) {
	r.method = method
	r.params = params
	return map[string]any{}, nil
}

func (r *recordingSessionRPC) Notifications() <-chan jsonRPCMessage {
	return make(chan jsonRPCMessage)
}

func (r *recordingSessionRPC) RespondToRequest(id any, result any) error {
	return nil
}

func ptrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
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

func TestAgentMessageDeltaBeforeToolIsEmittedAsThinking(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 4)}
	state := newNotificationState()

	s.handleNotification(jsonRPCMessage{
		Method: "item/agentMessage/delta",
		Params: map[string]any{
			"delta": "I will inspect the files.",
		},
	}, state)
	select {
	case evt := <-s.events:
		t.Fatalf("unexpected event before tool: %#v", evt)
	default:
	}

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
	if evt.Type != runtime.EventDelta || evt.Thinking != "I will inspect the files." {
		t.Fatalf("thinking event = %#v", evt)
	}
	if len(evt.Process) != 1 || evt.Process[0].Type != "thinking" || evt.Process[0].Text != "I will inspect the files." {
		t.Fatalf("thinking process = %#v", evt.Process)
	}
	evt = <-s.events
	if evt.Type != runtime.EventDelta || len(evt.Process) != 1 || evt.Process[0].Type != "tool_call" {
		t.Fatalf("tool event = %#v", evt)
	}
}

func TestAgentMessageDeltaWithoutToolBecomesFinalText(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 4)}
	state := newNotificationState()

	s.handleNotification(jsonRPCMessage{
		Method: "item/agentMessage/delta",
		Params: map[string]any{
			"delta": "final answer",
		},
	}, state)
	terminal := s.handleNotification(jsonRPCMessage{
		Method: "turn/completed",
		Params: map[string]any{
			"turn": map[string]any{"status": "completed"},
		},
	}, state)

	if !terminal {
		t.Fatalf("turn/completed should be terminal")
	}
	evt := <-s.events
	if evt.Type != runtime.EventCompleted || evt.Text != "final answer" {
		t.Fatalf("completed event = %#v", evt)
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
