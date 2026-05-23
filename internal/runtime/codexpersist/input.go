package codexpersist

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

func buildUserInput(input runtime.Input) []map[string]any {
	items := []map[string]any{
		{"type": "text", "text": input.RenderedPrompt()},
	}
	for _, att := range input.Attachments {
		if att.Kind == "image" {
			if att.LocalPath != "" {
				items = append(items, map[string]any{"type": "localImage", "path": att.LocalPath})
			}
		}
	}
	return items
}

func usablePreviousSessionID(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "codex:") {
		return ""
	}
	return id
}

func rpcFailureEvent(prefix string, err error) runtime.Event {
	return runtime.Event{
		Type:         runtime.EventFailed,
		Error:        fmt.Sprintf("%s: %v", prefix, err),
		StaleSession: isStaleThreadError(err),
	}
}

func isStaleThreadError(err error) bool {
	if err == nil {
		return false
	}
	var rpcErr *jsonRPCError
	if errors.As(err, &rpcErr) {
		return isStaleThreadErrorMessage(rpcErr.Message)
	}
	return isStaleThreadErrorMessage(err.Error())
}

func isStaleThreadErrorMessage(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "thread not found") || strings.Contains(text, "no thread found")
}

func notificationParams(msg jsonRPCMessage) map[string]any {
	if msg.Params == nil {
		return nil
	}
	switch p := msg.Params.(type) {
	case map[string]any:
		return p
	default:
		data, err := json.Marshal(msg.Params)
		if err != nil {
			return nil
		}
		var result map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			return nil
		}
		return result
	}
}
