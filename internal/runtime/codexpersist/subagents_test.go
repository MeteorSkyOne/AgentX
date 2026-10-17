package codexpersist

import (
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func drainSubagentSignals(s *persistentSession) []runtime.ProcessItem {
	var signals []runtime.ProcessItem
	for {
		select {
		case evt := <-s.events:
			for _, item := range evt.Process {
				if item.Type == "subagent_started" || item.Type == "subagent_completed" {
					signals = append(signals, item)
				}
			}
		default:
			return signals
		}
	}
}

func itemNotification(method string, item map[string]any) jsonRPCMessage {
	return jsonRPCMessage{Method: method, Params: map[string]any{"threadId": "thr", "item": item}}
}

// spawnAgent completes as soon as the agent is spawned; the agent must stay
// running until a later collab call reports it finished.
func TestSpawnedCodexSubagentRunsUntilWaitReportsCompletion(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 16)}
	state := newNotificationState()

	s.handleNotification(itemNotification("item/started", map[string]any{
		"type": "collabAgentToolCall", "id": "call_spawn", "tool": "spawnAgent", "status": "inProgress",
		"receiverThreadIds": []any{}, "agentsStates": map[string]any{}, "prompt": "inspect",
	}), state)
	s.handleNotification(itemNotification("item/completed", map[string]any{
		"type": "collabAgentToolCall", "id": "call_spawn", "tool": "spawnAgent", "status": "completed",
		"receiverThreadIds": []any{"thr_child"},
		"agentsStates":      map[string]any{"thr_child": map[string]any{"status": "pendingInit"}},
	}), state)

	signals := drainSubagentSignals(s)
	if len(signals) != 1 || signals[0].Type != "subagent_started" || signals[0].ToolCallID != "call_spawn" {
		t.Fatalf("signals after spawn = %#v, want subagent_started for call_spawn", signals)
	}

	s.handleNotification(itemNotification("item/completed", map[string]any{
		"type": "collabAgentToolCall", "id": "call_wait", "tool": "wait", "status": "completed",
		"receiverThreadIds": []any{"thr_child"},
		"agentsStates":      map[string]any{"thr_child": map[string]any{"status": "completed", "message": "done"}},
	}), state)

	signals = drainSubagentSignals(s)
	if len(signals) != 1 || signals[0].Type != "subagent_completed" || signals[0].ToolCallID != "call_spawn" || signals[0].Status != "completed" {
		t.Fatalf("signals after wait = %#v, want subagent_completed for call_spawn", signals)
	}
}

func TestCodexSubAgentActivityDrivesLifecycle(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 16)}
	state := newNotificationState()

	s.handleNotification(itemNotification("item/completed", map[string]any{
		"type": "subAgentActivity", "id": "call_spawn", "kind": "started", "agentThreadId": "thr_child", "agentPath": "/root/worker",
	}), state)
	// The spawn's own completion repeats the running state; no second signal.
	s.handleNotification(itemNotification("item/completed", map[string]any{
		"type": "collabAgentToolCall", "id": "call_spawn", "tool": "spawnAgent", "status": "completed",
		"receiverThreadIds": []any{"thr_child"},
		"agentsStates":      map[string]any{"thr_child": map[string]any{"status": "running"}},
	}), state)
	s.handleNotification(itemNotification("item/completed", map[string]any{
		"type": "subAgentActivity", "id": "subagent-completed-turn2", "kind": "interrupted", "agentThreadId": "thr_child", "agentPath": "/root/worker",
	}), state)

	signals := drainSubagentSignals(s)
	if len(signals) != 2 ||
		signals[0].Type != "subagent_started" || signals[0].ToolCallID != "call_spawn" ||
		signals[1].Type != "subagent_completed" || signals[1].ToolCallID != "call_spawn" || signals[1].Status != "stopped" {
		t.Fatalf("signals = %#v, want started then stopped for call_spawn", signals)
	}
}

func TestCodexSubagentFailedSpawnFinishesImmediately(t *testing.T) {
	s := &persistentSession{events: make(chan runtime.Event, 16)}

	s.handleNotification(itemNotification("item/completed", map[string]any{
		"type": "collabAgentToolCall", "id": "call_spawn", "tool": "spawnAgent", "status": "failed",
		"receiverThreadIds": []any{"thr_child"},
		"agentsStates":      map[string]any{"thr_child": map[string]any{"status": "errored", "message": "boom"}},
	}), newNotificationState())

	signals := drainSubagentSignals(s)
	if len(signals) != 1 || signals[0].Type != "subagent_completed" || signals[0].Status != "failed" {
		t.Fatalf("signals = %#v, want a failed subagent_completed", signals)
	}
}
