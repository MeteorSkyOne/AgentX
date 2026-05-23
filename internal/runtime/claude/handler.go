package claude

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/runtime"
)

type lineHandler struct {
	mu          sync.Mutex
	sessionID   string
	finalText   strings.Builder
	pendingText strings.Builder
	stageText   strings.Builder
}

func newLineHandler(fallbackID string) *lineHandler {
	return &lineHandler{sessionID: fallbackID}
}

func (h *lineHandler) HandleLine(line []byte) ([]runtime.Event, error) {
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		text := strings.TrimSpace(string(line))
		if text == "" {
			return nil, nil
		}
		h.appendText(text)
		return []runtime.Event{{Type: runtime.EventDelta, Text: text}}, nil
	}

	h.setSessionID(stringValue(payload, "session_id"))
	switch stringValue(payload, "type") {
	case "assistant", "user":
		text, thinking, process := assistantContent(payload)
		if stringValue(payload, "type") != "assistant" {
			text = ""
		}
		if text == "" && thinking == "" && len(process) == 0 {
			return nil, nil
		}
		if text != "" {
			h.appendText(text)
		}
		var clearText bool
		if thinking != "" || len(process) > 0 {
			h.mu.Lock()
			promoted := h.promotePendingTextImmediateLocked()
			h.mu.Unlock()
			if promoted != "" {
				process = append([]runtime.ProcessItem{{Type: "thinking", Text: promoted}}, process...)
				clearText = true
			}
		}
		if text != "" {
			h.appendPendingText(text)
		}
		return []runtime.Event{{Type: runtime.EventDelta, Text: text, Thinking: thinking, Process: process, ClearText: clearText}}, nil
	case "result":
		if isErrorResult(payload) {
			errText := resultError(payload)
			return []runtime.Event{{Type: runtime.EventFailed, Error: errText, StaleSession: isStaleSessionError(errText)}}, nil
		}
		text := stringValue(payload, "result")
		if text == "" {
			text = h.text()
		}
		evt := runtime.Event{Type: runtime.EventCompleted, Text: text, Usage: claudeUsage(payload)}
		if stage := h.stageThinkingForResult(text); stage != "" {
			evt.Thinking = stage
			evt.Process = []runtime.ProcessItem{{Type: "thinking", Text: stage}}
		}
		return []runtime.Event{evt}, nil
	default:
		return nil, nil
	}
}

func (h *lineHandler) Finish(stderr string, waitErr error) (runtime.Event, bool) {
	if waitErr != nil {
		errText := commandError(stderr, waitErr)
		return runtime.Event{Type: runtime.EventFailed, Error: errText, StaleSession: isStaleSessionError(errText)}, true
	}
	return runtime.Event{Type: runtime.EventCompleted, Text: h.text()}, true
}

func (h *lineHandler) CurrentSessionID() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessionID
}

func (h *lineHandler) setSessionID(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	h.mu.Lock()
	h.sessionID = id
	h.mu.Unlock()
}

func (h *lineHandler) appendText(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.finalText.Len() > 0 {
		h.finalText.WriteByte('\n')
	}
	h.finalText.WriteString(text)
}

func (h *lineHandler) appendPendingText(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	appendLine(&h.pendingText, text)
}

func (h *lineHandler) text() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finalText.String()
}

func (h *lineHandler) stageThinkingForResult(result string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if strings.TrimSpace(result) != "" && strings.TrimSpace(h.pendingText.String()) != "" && !sameNormalizedText(h.pendingText.String(), result) {
		h.promotePendingTextToStageLocked()
	}
	stage := strings.TrimSpace(h.stageText.String())
	if strings.TrimSpace(stage) == "" {
		return ""
	}
	if strings.TrimSpace(result) == "" {
		return ""
	}
	return stage
}

func (h *lineHandler) promotePendingTextToStageLocked() {
	text := strings.TrimSpace(h.pendingText.String())
	if text == "" {
		return
	}
	appendLine(&h.stageText, text)
	h.pendingText.Reset()
}

func (h *lineHandler) promotePendingTextImmediateLocked() string {
	text := strings.TrimSpace(h.pendingText.String())
	if text == "" {
		return ""
	}
	h.pendingText.Reset()
	return text
}

func appendLine(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(text)
}

func sameNormalizedText(left string, right string) bool {
	return strings.Join(strings.Fields(left), " ") == strings.Join(strings.Fields(right), " ")
}
