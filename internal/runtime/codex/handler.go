package codex

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/runtime"
)

type lineHandler struct {
	mu        sync.Mutex
	sessionID string
	finalText strings.Builder
	pending   []string
	usage     *runtime.Usage
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

	switch stringValue(payload, "type") {
	case "thread.started":
		h.setSessionID(stringValue(payload, "thread_id"))
	case "turn.started":
		h.clearPending()
	case "item.started":
		item, _ := payload["item"].(map[string]any)
		return h.handleItemStarted(item)
	case "item.completed":
		item, _ := payload["item"].(map[string]any)
		return h.handleItemCompleted(item)
	case "response_item":
		item, _ := payload["payload"].(map[string]any)
		return h.handleItem(item)
	case "event_msg":
		item, _ := payload["payload"].(map[string]any)
		return h.handleEventMessage(item)
	case "token_count.info":
		if usage := codexTokenCountUsage(payload); usage != nil {
			h.setUsage(usage)
		}
	case "turn.completed":
		h.flushPendingAsText()
		usage := codexTurnUsage(payload)
		if usage == nil {
			usage = h.currentUsage()
		} else {
			h.setUsage(usage)
		}
		return []runtime.Event{{Type: runtime.EventCompleted, Text: h.text(), Usage: usage}}, nil
	case "turn.failed":
		return []runtime.Event{{Type: runtime.EventFailed, Error: errorText(payload)}}, nil
	case "error":
		return []runtime.Event{{Type: runtime.EventFailed, Error: errorText(payload)}}, nil
	}
	return nil, nil
}

func (h *lineHandler) handleItemStarted(item map[string]any) ([]runtime.Event, error) {
	itemType := normalizedType(stringValue(item, "type"))
	switch itemType {
	case "", "agent_message", "message", "reasoning", "user_message", "plan", "hook_prompt", "context_compaction":
		return nil, nil
	}

	events := h.flushPendingAsThinking()
	if process := codexStartedToolProcessItems(item); len(process) > 0 {
		events = append(events, runtime.Event{Type: runtime.EventDelta, Process: process})
	}
	return events, nil
}

func (h *lineHandler) handleItemCompleted(item map[string]any) ([]runtime.Event, error) {
	switch normalizedType(stringValue(item, "type")) {
	case "agent_message", "message":
		text := codexMessageText(item)
		if text != "" {
			h.appendPending(text)
		}
		return nil, nil
	default:
		return h.handleItem(item)
	}
}

func (h *lineHandler) handleItem(item map[string]any) ([]runtime.Event, error) {
	switch normalizedType(stringValue(item, "type")) {
	case "agent_message":
		text := codexMessageText(item)
		if text != "" {
			events := h.flushPendingAsThinking()
			h.appendText(text)
			events = append(events, runtime.Event{Type: runtime.EventDelta, Text: text})
			return events, nil
		}
	case "message":
		if role := stringValue(item, "role"); role != "" && role != "assistant" {
			return nil, nil
		}
		text := codexMessageText(item)
		if text != "" {
			events := h.flushPendingAsThinking()
			h.appendText(text)
			events = append(events, runtime.Event{Type: runtime.EventDelta, Text: text})
			return events, nil
		}
	case "reasoning":
		text := reasoningSummaryText(item)
		if text != "" {
			return []runtime.Event{{Type: runtime.EventDelta, Thinking: text, Process: []runtime.ProcessItem{{
				Type: "thinking",
				Text: text,
				Raw:  item,
			}}}}, nil
		}
	default:
		if process := codexToolProcessItems(item); len(process) > 0 {
			events := h.flushPendingAsThinking()
			events = append(events, runtime.Event{Type: runtime.EventDelta, Process: process})
			return events, nil
		}
	}
	return nil, nil
}

func (h *lineHandler) handleEventMessage(item map[string]any) ([]runtime.Event, error) {
	switch normalizedType(stringValue(item, "type")) {
	case "agent_message", "agent_message_delta":
		text := firstText(item, "message", "text", "delta")
		if text != "" {
			h.appendPending(text)
		}
	case "agent_reasoning", "agent_reasoning_delta":
		text := firstText(item, "text", "message", "delta")
		if text != "" {
			return []runtime.Event{{Type: runtime.EventDelta, Thinking: text, Process: []runtime.ProcessItem{{
				Type: "thinking",
				Text: text,
				Raw:  item,
			}}}}, nil
		}
	case "collab_agent_spawn_begin":
		events := h.flushPendingAsThinking()
		events = append(events, runtime.Event{Type: runtime.EventDelta, Process: codexCollabSpawnStarted(item)})
		return events, nil
	case "collab_agent_spawn_end":
		return []runtime.Event{{Type: runtime.EventDelta, Process: codexCollabSpawnCompleted(item)}}, nil
	default:
		if process := codexToolProcessItems(item); len(process) > 0 {
			return []runtime.Event{{Type: runtime.EventDelta, Process: process}}, nil
		}
	}
	return nil, nil
}

func (h *lineHandler) Finish(stderr string, waitErr error) (runtime.Event, bool) {
	if waitErr != nil {
		return runtime.Event{Type: runtime.EventFailed, Error: commandError(stderr, waitErr)}, true
	}
	h.flushPendingAsText()
	return runtime.Event{Type: runtime.EventCompleted, Text: h.text(), Usage: h.currentUsage()}, true
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

func (h *lineHandler) appendPending(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending = append(h.pending, text)
}

func (h *lineHandler) clearPending() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending = h.pending[:0]
}

func (h *lineHandler) flushPendingAsThinking() []runtime.Event {
	h.mu.Lock()
	pending := append([]string(nil), h.pending...)
	h.pending = h.pending[:0]
	h.mu.Unlock()

	events := make([]runtime.Event, 0, len(pending))
	for _, text := range pending {
		if strings.TrimSpace(text) == "" {
			continue
		}
		events = append(events, runtime.Event{Type: runtime.EventDelta, Thinking: text, Process: []runtime.ProcessItem{{
			Type: "thinking",
			Text: text,
		}}})
	}
	return events
}

func (h *lineHandler) flushPendingAsText() {
	h.mu.Lock()
	pending := append([]string(nil), h.pending...)
	h.pending = h.pending[:0]
	h.mu.Unlock()

	for _, text := range pending {
		if strings.TrimSpace(text) != "" {
			h.appendText(text)
		}
	}
}

func (h *lineHandler) text() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finalText.String()
}

func (h *lineHandler) setUsage(usage *runtime.Usage) {
	h.mu.Lock()
	h.usage = usage
	h.mu.Unlock()
}

func (h *lineHandler) currentUsage() *runtime.Usage {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.usage == nil {
		return nil
	}
	copy := *h.usage
	return &copy
}
