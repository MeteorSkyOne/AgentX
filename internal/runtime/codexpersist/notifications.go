package codexpersist

import (
	"context"
	"log/slog"
	"strings"

	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/runtime"
)

type notificationState struct {
	text strings.Builder
	// pendingAgentText holds the text of the agent message item that is still
	// streaming. Codex can run tool calls while a message is being produced, so
	// this text must not be reclassified until the message item completes.
	pendingAgentText   strings.Builder
	pendingAgentItemID string
	// completedAgentText holds agent messages that finished but have not been
	// classified yet: they become "thinking" if another item follows them in
	// the same turn, or the final text otherwise.
	completedAgentText strings.Builder
	streamedPlanItems  map[string]bool
	completedPlanText  string
}

func newNotificationState() *notificationState {
	return &notificationState{streamedPlanItems: make(map[string]bool)}
}

func (st *notificationState) writeText(text string) {
	if text != "" {
		st.text.WriteString(text)
	}
}

func (st *notificationState) writePendingAgentText(itemID, text string) {
	if text == "" {
		return
	}
	if itemID != "" && st.pendingAgentItemID != "" && itemID != st.pendingAgentItemID {
		// A new message item started streaming before the previous one
		// reported completion; treat the previous one as completed.
		st.completeAgentMessage("")
	}
	if itemID != "" {
		st.pendingAgentItemID = itemID
	}
	st.pendingAgentText.WriteString(text)
}

// completeAgentMessage moves the streaming agent message into the completed
// bucket. fullText is the authoritative text from item/completed and is used
// when nothing was streamed for this item.
func (st *notificationState) completeAgentMessage(fullText string) {
	if st.pendingAgentText.Len() == 0 {
		st.completedAgentText.WriteString(fullText)
	} else {
		st.completedAgentText.WriteString(st.pendingAgentText.String())
	}
	st.pendingAgentText.Reset()
	st.pendingAgentItemID = ""
}

func (st *notificationState) hasCompletedAgentText() bool {
	return st.completedAgentText.Len() > 0
}

func (st *notificationState) flushCompletedAgentTextAsThinking() runtime.Event {
	text := st.completedAgentText.String()
	st.completedAgentText.Reset()
	return runtime.Event{Type: runtime.EventDelta, Thinking: text, Process: []runtime.ProcessItem{{
		Type: "thinking",
		Text: text,
	}}}
}

func (st *notificationState) flushAgentTextAsText() {
	st.writeText(st.completedAgentText.String())
	st.completedAgentText.Reset()
	st.writeText(st.pendingAgentText.String())
	st.pendingAgentText.Reset()
	st.pendingAgentItemID = ""
}

func (st *notificationState) textString() string {
	if st.completedPlanText != "" {
		return st.completedPlanText
	}
	st.flushAgentTextAsText()
	return st.text.String()
}

func (s *persistentSession) processNotifications(ctx context.Context) {
	state := newNotificationState()
	for {
		select {
		case <-ctx.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: ctx.Err().Error()})
			return
		case <-s.process.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: codexAppServerExitedError(s.process.Stderr())})
			return
		case msg, ok := <-s.rpc.Notifications():
			if !ok {
				if text := state.textString(); text != "" {
					s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text})
				} else {
					s.emit(runtime.Event{Type: runtime.EventFailed, Error: "notification channel closed"})
				}
				return
			}

			if msg.ID != nil {
				s.handleServerRequest(msg)
				continue
			}

			terminal := s.handleNotification(msg, state)
			if terminal {
				return
			}
		}
	}
}

func (s *persistentSession) handleNotification(msg jsonRPCMessage, state *notificationState) bool {
	params := notificationParams(msg)

	switch msg.Method {
	case "item/agentMessage/delta":
		delta, _ := params["delta"].(string)
		if delta != "" {
			state.writePendingAgentText(stringVal(params, "itemId"), delta)
		}

	case "item/plan/delta":
		delta, _ := params["delta"].(string)
		if delta != "" {
			if itemID := stringVal(params, "itemId"); itemID != "" {
				state.streamedPlanItems[itemID] = true
			}
			state.writeText(delta)
			s.emit(runtime.Event{Type: runtime.EventDelta, Text: delta})
		}

	case "item/reasoning/textDelta":
		delta, _ := params["delta"].(string)
		if delta != "" {
			s.emit(runtime.Event{Type: runtime.EventDelta, Thinking: delta})
		}

	case "item/started":
		item, _ := params["item"].(map[string]any)
		if pi := itemToProcessItem(item, "started"); pi != nil {
			if state.hasCompletedAgentText() {
				s.emit(state.flushCompletedAgentTextAsThinking())
			}
			s.emit(runtime.Event{Type: runtime.EventDelta, Process: []runtime.ProcessItem{*pi}})
		}

	case "item/completed":
		item, _ := params["item"].(map[string]any)
		if isAgentMessageItem(item) {
			state.completeAgentMessage(completedAgentMessageText(item))
		}
		if text, streamed := completedPlanText(item, state); text != "" {
			if streamed {
				state.completedPlanText = text
			} else {
				state.writeText(text)
				s.emit(runtime.Event{Type: runtime.EventDelta, Text: text})
			}
		}
		if pi := itemToProcessItem(item, "completed"); pi != nil {
			if state.hasCompletedAgentText() {
				s.emit(state.flushCompletedAgentTextAsThinking())
			}
			s.emit(runtime.Event{Type: runtime.EventDelta, Process: []runtime.ProcessItem{*pi}})
		}

	case "turn/started":
		turnID := turnIDFromResult(params)
		if turnID != "" {
			s.setActiveTurnID(turnID)
		}

	case "turn/completed":
		text := state.textString()
		usage := turnCompletedUsage(params)
		status := turnStatus(params)
		s.clearActiveTurn()
		if status == "interrupted" {
			s.emit(runtime.Event{Type: runtime.EventCanceled, Text: text, Usage: usage})
			return true
		}
		s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text, Usage: usage})
		return true

	case "thread/tokenUsage/updated":
		if usage := threadTokenUsageUpdatedUsage(params, s.turnModel()); usage != nil {
			s.emit(runtime.Event{Type: runtime.EventDelta, Usage: usage})
		}

	case "thread/goal/updated", "thread/goal/cleared":
		// goal lifecycle notifications — feedback is provided by the turn events

	case "error":
		errMsg := codexNotificationErrorMessage(msg)
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: errMsg, StaleSession: isStaleThreadErrorMessage(errMsg)})
		return true

	case "thread/closed":
		text := state.textString()
		if text != "" {
			s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text})
		} else {
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: "thread closed"})
		}
		return true
	}
	return false
}

func (s *persistentSession) handleServerRequest(msg jsonRPCMessage) {
	switch msg.Method {
	case "item/commandExecution/requestApproval",
		"item/fileChange/requestApproval",
		"item/permissions/requestApproval",
		"execCommandApproval",
		"applyPatchApproval":
		if err := s.rpc.RespondToRequest(msg.ID, map[string]any{"decision": "accept"}); err != nil {
			slog.Warn("codexpersist: failed to auto-approve", "method", msg.Method, "error", err)
		}
	case "item/tool/requestUserInput":
		s.handleUserInputRequest(msg)
	default:
		if err := s.rpc.RespondToRequest(msg.ID, map[string]any{}); err != nil {
			slog.Warn("codexpersist: failed to respond to server request", "method", msg.Method, "error", err)
		}
	}
}

func (s *persistentSession) handleUserInputRequest(msg jsonRPCMessage) {
	params := notificationParams(msg)

	var question string
	var questionRefID string
	var options []runtime.InputRequestOption

	// Codex uses a questions array: params.questions[0]
	if questions, ok := params["questions"].([]any); ok && len(questions) > 0 {
		q, _ := questions[0].(map[string]any)
		if q != nil {
			question = stringVal(q, "question")
			questionRefID = stringVal(q, "id")
			if rawOptions, ok := q["options"].([]any); ok {
				for _, opt := range rawOptions {
					optMap, _ := opt.(map[string]any)
					if optMap == nil {
						continue
					}
					options = append(options, runtime.InputRequestOption{
						Label:       stringVal(optMap, "label"),
						Description: stringVal(optMap, "description"),
					})
				}
			}
		}
	}

	// Fallback to flat fields
	if question == "" {
		question = stringVal(params, "question")
	}
	if question == "" {
		question = stringVal(params, "prompt")
	}
	if question == "" {
		question = "The agent is requesting input"
	}

	questionID := id.New("qst")
	s.emit(runtime.Event{
		Type: runtime.EventInputRequest,
		InputRequest: &runtime.InputRequest{
			QuestionID: questionID,
			Question:   question,
			RequestID:  msg.ID,
			Options:    options,
		},
	})

	select {
	case <-s.process.Done():
		return
	case <-s.interruptCh:
		return
	case answer := <-s.pendingInput:
		// Codex expects: {"answers": {"<question-ref-id>": {"answers": ["<answer>"]}}}
		refID := questionRefID
		if refID == "" {
			refID = "q0"
		}
		result := map[string]any{
			"answers": map[string]any{
				refID: map[string]any{
					"answers": []string{answer.answer},
				},
			},
		}
		if err := s.rpc.RespondToRequest(msg.ID, result); err != nil {
			slog.Warn("codexpersist: failed to respond to user input request", "error", err)
		}
	}
}
