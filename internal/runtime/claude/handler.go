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

	// Accessed only from the single goroutine that scans stdout.
	background        *BackgroundTracker
	pendingCompletion *runtime.Event
}

func newLineHandler(fallbackID string) *lineHandler {
	return &lineHandler{sessionID: fallbackID, background: NewBackgroundTracker()}
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
	case "system":
		if h.background.TrackSystemMessage(payload) {
			// The main agent is about to be woken up to report on the finished
			// task, so the result we are holding is not the last word.
			h.pendingCompletion = nil
		}
		return nil, nil
	case "assistant", "user":
		text, thinking, process := assistantContent(payload)
		if stringValue(payload, "type") != "assistant" {
			text = ""
		}
		if text == "" && thinking == "" && len(process) == 0 {
			return nil, nil
		}

		// Subagent output is streamed for visibility but kept out of the main
		// agent's reply, which belongs to the main agent alone.
		if parent := stringValue(payload, "parent_tool_use_id"); parent != "" {
			for i := range process {
				process[i].ParentToolCallID = parent
			}
			return []runtime.Event{{Type: runtime.EventDelta, Thinking: thinking, Process: process}}, nil
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
		// A result while background subagents are still running only ends the
		// main agent's current reply, not the turn: the CLI keeps the subagents
		// going and wakes the agent again with a further result. Hold this one
		// back so the run is completed once, with everything in it.
		// The ephemeral CLI buffers its results and flushes them as it exits, so
		// once a subagent has appeared this turn there is no way to tell from a
		// result alone whether another is coming. Hold every result and let the
		// process exiting end the turn, so the run completes once with all of
		// the agent's replies in it.
		if h.background.Seen() {
			h.pendingCompletion = mergeCompletion(h.pendingCompletion, evt)
			return nil, nil
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
	// A result held back for background subagents is emitted here, once the CLI
	// has exited and no further result is coming. The accumulated text spans
	// every reply the agent made across the turn, so prefer it.
	if h.pendingCompletion != nil {
		evt := *h.pendingCompletion
		if text := h.text(); text != "" {
			evt.Text = text
		}
		return evt, true
	}
	return runtime.Event{Type: runtime.EventCompleted, Text: h.text()}, true
}

// mergeCompletion folds a later result into the one being held, keeping the
// newest text while accumulating token usage across the turn's results.
func mergeCompletion(held *runtime.Event, next runtime.Event) *runtime.Event {
	if held == nil {
		return &next
	}
	next.Usage = mergeUsage(held.Usage, next.Usage)
	return &next
}

func mergeUsage(a *runtime.Usage, b *runtime.Usage) *runtime.Usage {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	merged := *b
	if merged.Model == "" {
		merged.Model = a.Model
	}
	merged.InputTokens = addTokens(a.InputTokens, b.InputTokens)
	merged.CachedInputTokens = addTokens(a.CachedInputTokens, b.CachedInputTokens)
	merged.CacheCreationInputTokens = addTokens(a.CacheCreationInputTokens, b.CacheCreationInputTokens)
	merged.CacheReadInputTokens = addTokens(a.CacheReadInputTokens, b.CacheReadInputTokens)
	merged.OutputTokens = addTokens(a.OutputTokens, b.OutputTokens)
	merged.ReasoningOutputTokens = addTokens(a.ReasoningOutputTokens, b.ReasoningOutputTokens)
	merged.TotalTokens = addTokens(a.TotalTokens, b.TotalTokens)
	merged.DurationMS = addTokens(a.DurationMS, b.DurationMS)
	merged.DurationAPIMS = addTokens(a.DurationAPIMS, b.DurationAPIMS)
	merged.TotalCostUSD = addCost(a.TotalCostUSD, b.TotalCostUSD)
	return &merged
}

func addTokens(a *int64, b *int64) *int64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	sum := *a + *b
	return &sum
}

func addCost(a *float64, b *float64) *float64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	sum := *a + *b
	return &sum
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
