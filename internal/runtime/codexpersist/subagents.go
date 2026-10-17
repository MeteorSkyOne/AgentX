package codexpersist

import "github.com/meteorsky/agentx/internal/runtime"

// subagentTracker turns codex's subagent lifecycle into the subagent_started /
// subagent_completed process signals the UI uses for a subagent's running
// state. A spawnAgent call completes as soon as the agent is spawned, so its
// own result says nothing about whether the agent is still working; the real
// state comes from the collab calls' agentsStates (spawnAgent, wait, ...) and,
// with multi-agent v2, subAgentActivity items.
type subagentTracker struct {
	spawnCalls map[string]string // agent thread id -> spawnAgent call id
	announced  map[string]bool   // spawn call ids with a started signal sent
	finished   map[string]bool   // spawn call ids with a completed signal sent
}

func newSubagentTracker() *subagentTracker {
	return &subagentTracker{
		spawnCalls: map[string]string{},
		announced:  map[string]bool{},
		finished:   map[string]bool{},
	}
}

// signals returns the lifecycle signals an item/started or item/completed
// item produces.
func (t *subagentTracker) signals(item map[string]any) []runtime.ProcessItem {
	switch normalizeItemType(stringVal(item, "type")) {
	case "collabagenttoolcall":
		if normalizeItemType(stringVal(item, "tool")) == "spawnagent" {
			if callID := stringVal(item, "id"); callID != "" {
				receivers, _ := item["receiverThreadIds"].([]any)
				for _, raw := range receivers {
					if threadID, _ := raw.(string); threadID != "" {
						t.spawnCalls[threadID] = callID
					}
				}
			}
		}
		states, _ := item["agentsStates"].(map[string]any)
		var signals []runtime.ProcessItem
		for threadID, raw := range states {
			state, _ := raw.(map[string]any)
			status := normalizeItemType(stringVal(state, "status"))
			switch {
			case status == "pendinginit" || status == "running":
				signals = t.appendStarted(signals, t.spawnCalls[threadID])
			case isTerminalSubagentStatus(status):
				signals = t.appendCompleted(signals, t.spawnCalls[threadID], subagentSignalStatus(status))
			}
		}
		return signals

	case "subagentactivity":
		threadID := stringVal(item, "agentThreadId")
		switch normalizeItemType(stringVal(item, "kind")) {
		case "started":
			// A v2 spawn reports its activity under the spawn call's id.
			if _, known := t.spawnCalls[threadID]; !known && threadID != "" {
				t.spawnCalls[threadID] = stringVal(item, "id")
			}
			return t.appendStarted(nil, t.spawnCalls[threadID])
		case "completed":
			return t.appendCompleted(nil, t.spawnCalls[threadID], "completed")
		case "interrupted":
			return t.appendCompleted(nil, t.spawnCalls[threadID], "stopped")
		}
	}
	return nil
}

func (t *subagentTracker) appendStarted(signals []runtime.ProcessItem, callID string) []runtime.ProcessItem {
	if callID == "" || t.announced[callID] || t.finished[callID] {
		return signals
	}
	t.announced[callID] = true
	return append(signals, runtime.ProcessItem{Type: "subagent_started", ToolCallID: callID})
}

func (t *subagentTracker) appendCompleted(signals []runtime.ProcessItem, callID string, status string) []runtime.ProcessItem {
	if callID == "" || t.finished[callID] {
		return signals
	}
	t.finished[callID] = true
	return append(signals, runtime.ProcessItem{Type: "subagent_completed", ToolCallID: callID, Status: status})
}

// isTerminalSubagentStatus reports whether a normalized CollabAgentStatus
// means the agent stopped working.
func isTerminalSubagentStatus(status string) bool {
	switch status {
	case "completed", "errored", "interrupted", "shutdown", "notfound":
		return true
	}
	return false
}

func subagentSignalStatus(status string) string {
	switch status {
	case "errored", "notfound":
		return "failed"
	case "interrupted", "shutdown":
		return "stopped"
	}
	return status
}

// subagentSignals feeds an item through the session's subagent tracker. Only
// the notification loop calls it, so the tracker needs no locking.
func (s *persistentSession) subagentSignals(item map[string]any) []runtime.ProcessItem {
	if s.subagents == nil {
		s.subagents = newSubagentTracker()
	}
	return s.subagents.signals(item)
}
