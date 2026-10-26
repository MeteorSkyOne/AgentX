package claudepersist

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/meteorsky/agentx/internal/runtime/claude"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

// backgroundTurnClaimTimeout bounds how long a turn Claude Code started on its
// own waits for the app to adopt it before its output is discarded.
const backgroundTurnClaimTimeout = 30 * time.Second

// newBackgroundFallback handles lines that arrive while no turn is reading the
// process.
//
// When the agent starts a turn on its own — Claude Code wakes the main agent to
// report on a background command that finished after its reply was over — the
// turn is claimed and offered to startTurn, so the app can read it as a reply
// of its own. Without a taker the output is discarded.
//
// Claude Code runs Agent/Task subagents in the background: the main turn emits
// its result as soon as the subagent is launched, so the subagent keeps working
// — and keeps asking for tool permission over the stdio permission prompt —
// long after the turn's reader has gone away. Nobody would answer those
// requests, and the subagent would block until the idle reaper killed the whole
// process.
func newBackgroundFallback(proc *procpool.ManagedProcess, key string, startTurn func(sessionKey string) bool) func([]byte) {
	return func(line []byte) {
		var payload map[string]any
		if err := json.Unmarshal(line, &payload); err != nil {
			return
		}
		payloadType := claude.StringValue(payload, "type")
		registryOf(proc).observe(payload)
		if startsBackgroundTurn(payload) && startTurn != nil && proc.ClaimTurn(backgroundTurnClaimTimeout) {
			if startTurn(key) {
				slog.Info("claudepersist: agent started a turn on its own", "key", key, "type", payloadType)
				return
			}
			proc.AbandonClaim()
		}
		if payloadType != "control_request" {
			// Agent output nobody adopted as a turn is lost. That is real
			// output disappearing, so make it visible in the logs.
			switch payloadType {
			case "assistant", "user", "result":
				slog.Warn("claudepersist: discarding agent output that arrived outside any turn",
					"key", key, "type", payloadType,
					"parent_tool_use_id", claude.StringValue(payload, "parent_tool_use_id"))
			default:
				slog.Debug("claudepersist: line arrived with no reader attached", "key", key,
					"type", payloadType, "subtype", claude.StringValue(payload, "subtype"))
			}
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

// startsBackgroundTurn reports whether an unread line means the agent is
// running a turn: the session init that opens each turn, or agent output.
// Task lifecycle messages and stray control traffic do not.
func startsBackgroundTurn(payload map[string]any) bool {
	switch claude.StringValue(payload, "type") {
	case "assistant", "user", "result", "stream_event":
		return true
	case "system":
		return claude.StringValue(payload, "subtype") == "init"
	}
	return false
}
