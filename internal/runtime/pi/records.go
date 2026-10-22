package pi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/runtime"
)

// turnState tracks one AgentX turn while pi streams session events.
type turnState struct {
	// text is the assistant text streamed since the last promotion. Text that
	// precedes a tool call is promoted into a thinking item (and cleared from
	// the reply), so the reply ends up being the final assistant message only.
	text      strings.Builder
	usage     usageTotals
	lastStop  string
	lastError string
	startedAt time.Time
	subagents *subagentTracker
	// openTools maps tool call ids to tool names for tool_execution_end items
	// that omit the name.
	openTools map[string]string
}

func newTurnState() *turnState {
	return &turnState{
		startedAt: time.Now(),
		subagents: newSubagentTracker(),
		openTools: map[string]string{},
	}
}

func (s *persistentSession) processEvents(ctx context.Context) {
	state := newTurnState()
	for {
		select {
		case <-ctx.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: ctx.Err().Error()})
			return
		case <-s.process.Done():
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: piExitedError(s.process.Stderr())})
			return
		case rec, ok := <-s.rpc.Events():
			if !ok {
				s.emit(runtime.Event{Type: runtime.EventFailed, Error: piExitedError(s.process.Stderr())})
				return
			}
			if s.handleRecord(ctx, rec, state) {
				return
			}
		}
	}
}

// handleRecord applies one pi stdout record to the turn. It returns true once
// the turn reached a terminal event.
func (s *persistentSession) handleRecord(ctx context.Context, rec record, state *turnState) bool {
	switch stringVal(rec, "type") {
	case "message_start":
		message := mapVal(rec, "message")
		if stringVal(message, "role") == "assistant" {
			s.promoteText(state)
		}

	case "message_update":
		s.handleAssistantUpdate(mapVal(rec, "assistantMessageEvent"), state)

	case "message_end":
		message := mapVal(rec, "message")
		switch stringVal(message, "role") {
		case "assistant":
			state.usage.add(message)
			state.lastStop = stringVal(message, "stopReason")
			state.lastError = stringVal(message, "errorMessage")
		case "custom":
			if boolVal(message, "display") {
				if text := contentText(message["content"]); text != "" {
					s.emitNote(fmt.Sprintf("[%s] %s", stringVal(message, "customType"), text))
				}
			}
		}

	case "tool_execution_start":
		s.promoteText(state)
		callID := stringVal(rec, "toolCallId")
		toolName := stringVal(rec, "toolName")
		if callID != "" {
			state.openTools[callID] = toolName
		}
		item := runtime.ProcessItem{
			Type:             "tool_call",
			ToolName:         toolName,
			ToolCallID:       callID,
			Status:           "started",
			Input:            rec["args"],
			Raw:              rec,
			ParentToolCallID: stringVal(rec, "parentToolCallId"),
		}
		items := []runtime.ProcessItem{item}
		items = append(items, state.subagents.started(rec)...)
		evt := runtime.Event{Type: runtime.EventDelta, Process: items}
		if toolName == updatePlanToolName {
			evt.Plan = planStateFromArgs(mapVal(rec, "args"))
		}
		s.emit(evt)

	case "tool_execution_update":
		if items := state.subagents.updated(rec); len(items) > 0 {
			s.emit(runtime.Event{Type: runtime.EventDelta, Process: items})
		}

	case "tool_execution_end":
		callID := stringVal(rec, "toolCallId")
		toolName := stringVal(rec, "toolName")
		if toolName == "" {
			toolName = state.openTools[callID]
		}
		delete(state.openTools, callID)
		status := "completed"
		if boolVal(rec, "isError") {
			status = "error"
		}
		result := mapVal(rec, "result")
		var output any = contentText(result["content"])
		if output == "" {
			output = result
		}
		item := runtime.ProcessItem{
			Type:             "tool_result",
			ToolName:         toolName,
			ToolCallID:       callID,
			Status:           status,
			Output:           output,
			Raw:              rec,
			ParentToolCallID: stringVal(rec, "parentToolCallId"),
		}
		items := []runtime.ProcessItem{item}
		items = append(items, state.subagents.ended(rec)...)
		s.emit(runtime.Event{Type: runtime.EventDelta, Process: items})

	case "compaction_start":
		s.emitNote(fmt.Sprintf("Compacting context (%s).", stringVal(rec, "reason")))

	case "compaction_end":
		if msg := stringVal(rec, "errorMessage"); msg != "" {
			s.emitNote("Compaction failed: " + msg)
		} else if boolVal(rec, "aborted") {
			s.emitNote("Compaction aborted.")
		} else if result := mapVal(rec, "result"); result != nil {
			before := int64Ptr(result, "tokensBefore")
			after := int64Ptr(result, "estimatedTokensAfter")
			if before != nil && after != nil {
				s.emitNote(fmt.Sprintf("Context compacted (%d → ~%d tokens).", *before, *after))
			} else {
				s.emitNote("Context compacted.")
			}
		}

	case "auto_retry_start":
		s.emitNote(fmt.Sprintf("Retrying after error (attempt %d/%d): %s",
			int64Val(rec, "attempt"), int64Val(rec, "maxAttempts"), stringVal(rec, "errorMessage")))

	case "auto_retry_end":
		if !boolVal(rec, "success") {
			if msg := stringVal(rec, "finalError"); msg != "" {
				state.lastError = msg
				state.lastStop = "error"
			}
		}

	case "extension_error":
		s.emitNote(fmt.Sprintf("Extension error in %s (%s): %s",
			stringVal(rec, "extensionPath"), stringVal(rec, "event"), stringVal(rec, "error")))

	case "extension_ui_request":
		s.handleExtensionUIRequest(ctx, rec)

	case "agent_settled":
		return s.finishTurn(ctx, state, boolVal(rec, "aborted"))
	}
	return false
}

func (s *persistentSession) handleAssistantUpdate(evt record, state *turnState) {
	switch stringVal(evt, "type") {
	case "text_delta":
		delta := stringVal(evt, "delta")
		if delta == "" {
			return
		}
		state.text.WriteString(delta)
		s.emit(runtime.Event{Type: runtime.EventDelta, Text: delta})
	case "thinking_delta":
		if delta := stringVal(evt, "delta"); delta != "" {
			s.emit(runtime.Event{Type: runtime.EventDelta, Thinking: delta})
		}
	}
}

// promoteText moves the text streamed so far out of the reply and into the
// process timeline, mirroring how the Claude runtime treats text that turns out
// to be commentary before a tool call rather than the final answer.
func (s *persistentSession) promoteText(state *turnState) {
	if state.text.Len() == 0 {
		return
	}
	text := state.text.String()
	state.text.Reset()
	s.emit(runtime.Event{
		Type:      runtime.EventDelta,
		ClearText: true,
		Process:   []runtime.ProcessItem{{Type: "thinking", Text: text}},
	})
}

func (s *persistentSession) finishTurn(ctx context.Context, state *turnState, aborted bool) bool {
	s.setRunning(false)
	text := state.text.String()
	usage := state.usage.toRuntime(time.Since(state.startedAt).Milliseconds(), s.fetchContextUsage(ctx))
	switch {
	case aborted || s.interrupted() || state.lastStop == "aborted":
		s.emit(runtime.Event{Type: runtime.EventCanceled, Text: text, Usage: usage})
	case state.lastStop == "error" && strings.TrimSpace(text) == "":
		errText := state.lastError
		if errText == "" {
			errText = "pi reported an error"
		}
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText, Usage: usage})
	default:
		s.emit(runtime.Event{Type: runtime.EventCompleted, Text: text, Usage: usage})
	}
	return true
}

// handleExtensionUIRequest services pi's extension UI subprotocol. Dialog
// methods become AgentX input requests and block the turn until answered;
// fire-and-forget methods are surfaced as notes or ignored.
func (s *persistentSession) handleExtensionUIRequest(ctx context.Context, rec record) {
	requestID := stringVal(rec, "id")
	method := stringVal(rec, "method")
	switch method {
	case "select", "confirm", "input", "editor":
	case "notify":
		prefix := ""
		if level := stringVal(rec, "notifyType"); level != "" && level != "info" {
			prefix = strings.ToUpper(level[:1]) + level[1:] + ": "
		}
		s.emitNote(prefix + stringVal(rec, "message"))
		return
	default:
		// setStatus, setWidget, setTitle, set_editor_text have no equivalent here.
		return
	}

	question := strings.TrimSpace(stringVal(rec, "title"))
	if msg := strings.TrimSpace(stringVal(rec, "message")); msg != "" {
		if question != "" {
			question += "\n\n"
		}
		question += msg
	}
	if question == "" {
		question = "The agent is requesting input"
	}
	var options []runtime.InputRequestOption
	switch method {
	case "select":
		for _, raw := range sliceVal(rec, "options") {
			if label, ok := raw.(string); ok {
				options = append(options, runtime.InputRequestOption{Label: label})
			}
		}
	case "confirm":
		options = []runtime.InputRequestOption{{Label: "Yes"}, {Label: "No"}}
	case "input":
		if placeholder := stringVal(rec, "placeholder"); placeholder != "" {
			question += "\n\n(" + placeholder + ")"
		}
	case "editor":
		if prefill := stringVal(rec, "prefill"); prefill != "" {
			question += "\n\nCurrent text:\n" + prefill
		}
	}

	s.emit(runtime.Event{
		Type: runtime.EventInputRequest,
		InputRequest: &runtime.InputRequest{
			QuestionID: id.New("qst"),
			Question:   question,
			RequestID:  requestID,
			Options:    options,
		},
	})

	var response record
	select {
	case <-ctx.Done():
		response = record{"type": "extension_ui_response", "id": requestID, "cancelled": true}
	case <-s.process.Done():
		return
	case <-s.interruptCh:
		response = record{"type": "extension_ui_response", "id": requestID, "cancelled": true}
	case answer := <-s.pendingInput:
		response = extensionUIResponse(requestID, method, answer.answer)
	}
	if err := s.rpc.Send(response); err != nil {
		logWarn("extension_ui_response failed", "error", err)
	}
}

func extensionUIResponse(requestID string, method string, answer string) record {
	answer = strings.TrimSpace(answer)
	if method == "confirm" {
		switch strings.ToLower(answer) {
		case "yes", "y", "true", "ok", "confirm", "allow":
			return record{"type": "extension_ui_response", "id": requestID, "confirmed": true}
		default:
			return record{"type": "extension_ui_response", "id": requestID, "confirmed": false}
		}
	}
	if answer == "" {
		return record{"type": "extension_ui_response", "id": requestID, "cancelled": true}
	}
	return record{"type": "extension_ui_response", "id": requestID, "value": answer}
}

// updatePlanToolName is the step-list tool registered by the bundled AgentX
// extension; its arguments are the plan snapshot.
const updatePlanToolName = "update_plan"

func planStateFromArgs(args record) *runtime.PlanState {
	if args == nil {
		return nil
	}
	plan := &runtime.PlanState{Explanation: stringVal(args, "explanation")}
	for _, raw := range sliceVal(args, "plan") {
		item, _ := raw.(record)
		step := strings.TrimSpace(stringVal(item, "step"))
		if step == "" {
			continue
		}
		plan.Steps = append(plan.Steps, runtime.PlanStep{
			Step:   step,
			Status: runtime.NormalizePlanStepStatus(stringVal(item, "status")),
		})
	}
	return plan
}

// contentText flattens a pi content value (a string or a list of text blocks)
// into plain text.
func contentText(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		var b strings.Builder
		for _, raw := range typed {
			block, _ := raw.(record)
			if stringVal(block, "type") != "text" {
				continue
			}
			text := stringVal(block, "text")
			if text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(text)
		}
		return b.String()
	}
	return ""
}
