package claudepersist

import (
	"encoding/json"
	"log/slog"

	"github.com/meteorsky/agentx/internal/runtime/claude"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

// newBackgroundFallback answers control requests that arrive while no turn is
// reading the process.
//
// Claude Code runs Agent/Task subagents in the background: the main turn emits
// its result as soon as the subagent is launched, so the subagent keeps working
// — and keeps asking for tool permission over the stdio permission prompt —
// long after the turn's reader has gone away. Nobody would answer those
// requests, and the subagent would block until the idle reaper killed the whole
// process.
func newBackgroundFallback(proc *procpool.ManagedProcess, key string) func([]byte) {
	return func(line []byte) {
		var payload map[string]any
		if err := json.Unmarshal(line, &payload); err != nil {
			return
		}
		slog.Debug("claudepersist: line arrived with no reader attached", "key", key,
			"type", claude.StringValue(payload, "type"),
			"subtype", claude.StringValue(payload, "subtype"))
		if claude.StringValue(payload, "type") != "control_request" {
			return
		}
		requestID := claude.StringValue(payload, "request_id")
		if requestID == "" {
			return
		}

		request, _ := payload["request"].(map[string]any)
		toolName := claude.StringValue(request, "tool_name")

		// There is no user to prompt outside a turn, so decline the question
		// rather than fabricate an answer the agent would treat as the user's.
		inner := map[string]any{"behavior": "allow"}
		if toolName == "AskUserQuestion" {
			inner = map[string]any{
				"behavior": "deny",
				"message":  "No interactive user is available for a background subagent. Continue with your best judgement.",
			}
		}

		response := map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "success",
				"request_id": requestID,
				"response":   inner,
			},
		}
		if err := proc.WriteJSON(response); err != nil {
			slog.Warn("claudepersist: failed to answer background control request",
				"key", key, "tool", toolName, "error", err)
			return
		}
		slog.Debug("claudepersist: answered background control request",
			"key", key, "tool", toolName, "behavior", inner["behavior"])
	}
}
