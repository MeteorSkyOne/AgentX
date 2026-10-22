package pi

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

// fakeRPC records commands and serves canned responses keyed by command type.
type fakeRPC struct {
	mu        sync.Mutex
	calls     []record
	sent      []record
	responses map[string]any
	errors    map[string]error
	events    chan record
}

func newFakeRPC() *fakeRPC {
	return &fakeRPC{
		responses: map[string]any{},
		errors:    map[string]error{},
		events:    make(chan record, 64),
	}
}

func (f *fakeRPC) Call(ctx context.Context, cmd record) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, cmd)
	typ := stringVal(cmd, "type")
	if err := f.errors[typ]; err != nil {
		return nil, err
	}
	return f.responses[typ], nil
}

func (f *fakeRPC) CallMap(ctx context.Context, cmd record) (record, error) {
	data, err := f.Call(ctx, cmd)
	if err != nil {
		return nil, err
	}
	m, _ := data.(record)
	return m, nil
}

func (f *fakeRPC) Send(msg record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakeRPC) Events() <-chan record { return f.events }

func (f *fakeRPC) callTypes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, stringVal(c, "type"))
	}
	return out
}

func (f *fakeRPC) callOfType(typ string) record {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if stringVal(c, "type") == typ {
			return c
		}
	}
	return nil
}

// idleProcess returns a pooled process that stays alive until the test ends.
func idleProcess(t *testing.T) *procpool.ManagedProcess {
	t.Helper()
	pool := procpool.New(procpool.Options{IdleTimeout: time.Hour})
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	proc, _, err := pool.GetOrCreate("pi-test-"+t.Name(), func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "while read _; do :; done")
	})
	if err != nil {
		t.Fatal(err)
	}
	return proc
}

func newTestSession(t *testing.T, rpc *fakeRPC) *persistentSession {
	t.Helper()
	proc := idleProcess(t)
	rt := New(Options{Command: "pi"})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	return newPersistentSession(proc, rpc, "key", rt, runtime.StartSessionRequest{})
}

func collectEvents(t *testing.T, s *persistentSession) []runtime.Event {
	t.Helper()
	var events []runtime.Event
	timeout := time.After(5 * time.Second)
	for {
		select {
		case evt, ok := <-s.events:
			if !ok {
				return events
			}
			events = append(events, evt)
			switch evt.Type {
			case runtime.EventCompleted, runtime.EventFailed, runtime.EventCanceled:
				return events
			}
		case <-timeout:
			t.Fatalf("timed out waiting for events; got %d", len(events))
		}
	}
}

func assistantMessage(stop string, usage record) record {
	msg := record{"role": "assistant", "model": "claude-sonnet-4", "stopReason": stop}
	if usage != nil {
		msg["usage"] = usage
	}
	return msg
}

func TestProcessEventsStreamsTextAndPromotesPreToolText(t *testing.T) {
	rpc := newFakeRPC()
	rpc.responses["get_session_stats"] = record{"contextUsage": record{"tokens": 1200.0, "contextWindow": 200000.0, "percent": 0.6}}
	s := newTestSession(t, rpc)

	rpc.events <- record{"type": "agent_start"}
	rpc.events <- record{"type": "message_start", "message": assistantMessage("pending", nil)}
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "text_delta", "delta": "Let me look."}}
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "thinking_delta", "delta": "hm"}}
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "thinking_delta", "delta": "m"}}
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "thinking_end", "content": "hmm"}}
	rpc.events <- record{"type": "message_end", "message": assistantMessage("toolUse", record{"input": 10.0, "output": 5.0, "cacheRead": 2.0, "cacheWrite": 1.0, "totalTokens": 18.0, "cost": record{"total": 0.01}})}
	rpc.events <- record{"type": "tool_execution_start", "toolCallId": "call_1", "toolName": "bash", "args": record{"command": "ls"}}
	rpc.events <- record{"type": "tool_execution_end", "toolCallId": "call_1", "toolName": "bash", "isError": false,
		"result": record{"content": []any{record{"type": "text", "text": "file.go"}}}}
	rpc.events <- record{"type": "message_start", "message": assistantMessage("pending", nil)}
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "text_delta", "delta": "Done."}}
	rpc.events <- record{"type": "message_end", "message": assistantMessage("stop", record{"input": 20.0, "output": 7.0, "cacheRead": 0.0, "cacheWrite": 0.0, "totalTokens": 27.0, "cost": record{"total": 0.02}})}
	rpc.events <- record{"type": "agent_end"}
	rpc.events <- record{"type": "agent_settled", "aborted": false}

	go s.processEvents(context.Background())
	events := collectEvents(t, s)

	final := events[len(events)-1]
	if final.Type != runtime.EventCompleted || final.Text != "Done." {
		t.Fatalf("final event = %+v, want completed with text Done.", final)
	}
	if final.Usage == nil || final.Usage.InputTokens == nil || *final.Usage.InputTokens != 30 || *final.Usage.OutputTokens != 12 || *final.Usage.TotalTokens != 45 {
		t.Fatalf("usage = %+v, want summed tokens", final.Usage)
	}
	if final.Usage.TotalCostUSD == nil || *final.Usage.TotalCostUSD < 0.0299 || *final.Usage.TotalCostUSD > 0.0301 {
		t.Fatalf("cost = %v, want 0.03", final.Usage.TotalCostUSD)
	}
	if final.Usage.Context == nil || final.Usage.Context.TotalTokens == nil || *final.Usage.Context.TotalTokens != 1200 || final.Usage.Context.Model != "" && final.Usage.Context.Model != "claude-sonnet-4" {
		t.Fatalf("context usage = %+v", final.Usage.Context)
	}

	var sawPromotion, sawThinking, sawToolCall, sawToolResult bool
	var streamed strings.Builder
	for _, evt := range events {
		if evt.Type != runtime.EventDelta {
			continue
		}
		if evt.ClearText {
			sawPromotion = true
			if len(evt.Process) != 1 || evt.Process[0].Type != "thinking" || evt.Process[0].Text != "Let me look." {
				t.Fatalf("promotion event = %+v", evt)
			}
		}
		if evt.Thinking == "hmm" {
			sawThinking = true
		}
		streamed.WriteString(evt.Text)
		for _, item := range evt.Process {
			switch item.Type {
			case "tool_call":
				sawToolCall = item.ToolName == "bash" && item.ToolCallID == "call_1" && item.Status == "started"
			case "tool_result":
				sawToolResult = item.ToolCallID == "call_1" && item.Status == "completed" && item.Output == "file.go"
			}
		}
	}
	if !sawPromotion || !sawThinking || !sawToolCall || !sawToolResult {
		t.Fatalf("promotion=%v thinking=%v toolCall=%v toolResult=%v", sawPromotion, sawThinking, sawToolCall, sawToolResult)
	}
	if streamed.String() != "Let me look.Done." {
		t.Fatalf("streamed text = %q", streamed.String())
	}
}

func TestProcessEventsAbortedSettlesAsCanceled(t *testing.T) {
	rpc := newFakeRPC()
	s := newTestSession(t, rpc)
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "text_delta", "delta": "partial"}}
	rpc.events <- record{"type": "agent_settled", "aborted": true}

	go s.processEvents(context.Background())
	events := collectEvents(t, s)
	final := events[len(events)-1]
	if final.Type != runtime.EventCanceled || final.Text != "partial" {
		t.Fatalf("final event = %+v, want canceled", final)
	}
}

func TestProcessEventsErrorWithoutTextFails(t *testing.T) {
	rpc := newFakeRPC()
	s := newTestSession(t, rpc)
	rpc.events <- record{"type": "message_end", "message": record{"role": "assistant", "stopReason": "error", "errorMessage": "401 unauthorized"}}
	rpc.events <- record{"type": "agent_settled", "aborted": false}

	go s.processEvents(context.Background())
	events := collectEvents(t, s)
	final := events[len(events)-1]
	if final.Type != runtime.EventFailed || final.Error != "401 unauthorized" {
		t.Fatalf("final event = %+v, want failed with pi error", final)
	}
}

func TestExtensionUIRequestBecomesInputRequest(t *testing.T) {
	rpc := newFakeRPC()
	s := newTestSession(t, rpc)
	rpc.events <- record{"type": "extension_ui_request", "id": "ui-1", "method": "select", "title": "Allow rm -rf?", "options": []any{"Allow", "Block"}}

	go s.processEvents(context.Background())

	var request *runtime.InputRequest
	select {
	case evt := <-s.events:
		if evt.Type != runtime.EventInputRequest || evt.InputRequest == nil {
			t.Fatalf("event = %+v, want input request", evt)
		}
		request = evt.InputRequest
	case <-time.After(5 * time.Second):
		t.Fatal("no input request")
	}
	if request.Question != "Allow rm -rf?" || len(request.Options) != 2 || request.Options[1].Label != "Block" || request.RequestID != "ui-1" {
		t.Fatalf("request = %+v", request)
	}
	if err := s.RespondToInputRequest(request.QuestionID, "Block"); err != nil {
		t.Fatal(err)
	}
	rpc.events <- record{"type": "agent_settled", "aborted": false}
	collectEvents(t, s)

	rpc.mu.Lock()
	defer rpc.mu.Unlock()
	if len(rpc.sent) != 1 || rpc.sent[0]["id"] != "ui-1" || rpc.sent[0]["value"] != "Block" || rpc.sent[0]["type"] != "extension_ui_response" {
		t.Fatalf("sent = %+v, want extension_ui_response with value Block", rpc.sent)
	}
}

func TestExtensionUIResponseShapes(t *testing.T) {
	if r := extensionUIResponse("x", "confirm", "Yes"); r["confirmed"] != true {
		t.Fatalf("confirm yes = %v", r)
	}
	if r := extensionUIResponse("x", "confirm", "No"); r["confirmed"] != false {
		t.Fatalf("confirm no = %v", r)
	}
	if r := extensionUIResponse("x", "input", ""); r["cancelled"] != true {
		t.Fatalf("empty input = %v", r)
	}
	if r := extensionUIResponse("x", "editor", "text"); r["value"] != "text" {
		t.Fatalf("editor = %v", r)
	}
}

func TestPrepareAppliesModelAndThinkingLevel(t *testing.T) {
	rpc := newFakeRPC()
	rpc.responses["get_state"] = record{
		"sessionId":     "sess-1",
		"thinkingLevel": "medium",
		"model":         record{"provider": "anthropic", "id": "claude-sonnet-4", "name": "Claude Sonnet 4"},
	}
	rpc.responses["get_available_models"] = record{"models": []any{
		record{"provider": "anthropic", "id": "claude-sonnet-4", "name": "Claude Sonnet 4"},
		record{"provider": "openai", "id": "gpt-5", "name": "GPT-5"},
	}}
	rpc.responses["set_model"] = record{"provider": "openai", "id": "gpt-5", "name": "GPT-5"}
	rpc.responses["get_available_thinking_levels"] = record{"levels": []any{"off", "low", "medium", "high"}}
	s := newTestSession(t, rpc)
	s.req.Model = "gpt-5"
	s.req.Effort = "xhigh"
	s.needsNewSession = true

	if err := s.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	types := strings.Join(rpc.callTypes(), ",")
	if types != "new_session,get_state,get_available_models,set_model,get_available_thinking_levels,set_thinking_level" {
		t.Fatalf("calls = %s", types)
	}
	if set := rpc.callOfType("set_model"); set["provider"] != "openai" || set["modelId"] != "gpt-5" {
		t.Fatalf("set_model = %v", set)
	}
	if level := rpc.callOfType("set_thinking_level"); level["level"] != "high" {
		t.Fatalf("set_thinking_level = %v, want clamped high", level)
	}
	if s.CurrentSessionID() != "sess-1" || s.model != "gpt-5" {
		t.Fatalf("session id = %q model = %q", s.CurrentSessionID(), s.model)
	}
}

func TestPrepareSkipsWhenStateMatches(t *testing.T) {
	rpc := newFakeRPC()
	rpc.responses["get_state"] = record{
		"sessionId":     "sess-1",
		"thinkingLevel": "high",
		"model":         record{"provider": "anthropic", "id": "claude-sonnet-4"},
	}
	s := newTestSession(t, rpc)
	s.req.Model = "anthropic/claude-sonnet-4:high"

	if err := s.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if types := strings.Join(rpc.callTypes(), ","); types != "get_state" {
		t.Fatalf("calls = %s, want only get_state", types)
	}
}

func TestParseCompactPrompt(t *testing.T) {
	if instr, ok := parseCompactPrompt("/compact"); !ok || instr != "" {
		t.Fatalf("/compact = %q, %v", instr, ok)
	}
	if instr, ok := parseCompactPrompt("/compact keep the file list"); !ok || instr != "keep the file list" {
		t.Fatalf("/compact args = %q, %v", instr, ok)
	}
	if _, ok := parseCompactPrompt("/compaction"); ok {
		t.Fatal("/compaction should not match")
	}
	if _, ok := parseCompactPrompt("please /compact"); ok {
		t.Fatal("non-leading /compact should not match")
	}
}

func TestRunCompactReportsSummary(t *testing.T) {
	rpc := newFakeRPC()
	rpc.responses["compact"] = record{"summary": "We fixed the bug.", "tokensBefore": 150000.0, "estimatedTokensAfter": 32000.0}
	rpc.responses["get_session_stats"] = record{"contextUsage": record{"tokens": 32000.0, "contextWindow": 200000.0, "percent": 16.0}}
	s := newTestSession(t, rpc)

	go s.runCompact(context.Background(), "focus on code")
	events := collectEvents(t, s)
	final := events[len(events)-1]
	if final.Type != runtime.EventCompleted || !strings.Contains(final.Text, "150000 → ~32000") || !strings.Contains(final.Text, "We fixed the bug.") {
		t.Fatalf("final = %+v", final)
	}
	if cmd := rpc.callOfType("compact"); cmd["customInstructions"] != "focus on code" {
		t.Fatalf("compact cmd = %v", cmd)
	}
}

func TestSyncModeSendsCommandOnlyWhenChanged(t *testing.T) {
	rpc := newFakeRPC()
	rpc.responses["get_state"] = record{"sessionId": "s", "thinkingLevel": "medium"}
	rpc.responses["prompt"] = record{"disposition": "handled"}
	s := newTestSession(t, rpc)
	s.rt.extPath = "/tmp/agentx.ts"
	s.req.PermissionMode = "plan"
	s.req.YoloMode = false

	if err := s.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cmd := rpc.callOfType("prompt"); cmd == nil || cmd["message"] != "/agentx-mode plan=on yolo=off" {
		t.Fatalf("mode command = %v", cmd)
	}

	// Same mode again: no second command.
	rpc.calls = nil
	if err := s.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cmd := rpc.callOfType("prompt"); cmd != nil {
		t.Fatalf("unexpected mode command %v", cmd)
	}

	// Back to a normal yolo turn on the same process: command sent again.
	s.req.PermissionMode = ""
	s.req.YoloMode = true
	if err := s.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cmd := rpc.callOfType("prompt"); cmd == nil || cmd["message"] != "/agentx-mode plan=off yolo=on" {
		t.Fatalf("mode command = %v", cmd)
	}
}

func TestUpdatePlanToolCallBecomesPlanSnapshot(t *testing.T) {
	rpc := newFakeRPC()
	s := newTestSession(t, rpc)
	rpc.events <- record{"type": "tool_execution_start", "toolCallId": "c1", "toolName": "update_plan", "args": record{
		"explanation": "starting",
		"plan": []any{
			record{"step": "Read code", "status": "completed"},
			record{"step": "Write fix", "status": "in_progress"},
			record{"step": "", "status": "pending"},
			record{"step": "Test", "status": "weird"},
		},
	}}
	rpc.events <- record{"type": "agent_settled"}

	go s.processEvents(context.Background())
	events := collectEvents(t, s)
	var plan *runtime.PlanState
	for _, evt := range events {
		if evt.Plan != nil {
			plan = evt.Plan
		}
	}
	if plan == nil || plan.Explanation != "starting" || len(plan.Steps) != 3 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Steps[0].Status != runtime.PlanStepCompleted || plan.Steps[1].Status != runtime.PlanStepInProgress || plan.Steps[2].Status != runtime.PlanStepPending {
		t.Fatalf("plan steps = %+v", plan.Steps)
	}
}

func TestThinkingDeltasAreCoalescedPerBlock(t *testing.T) {
	rpc := newFakeRPC()
	s := newTestSession(t, rpc)
	for _, delta := range []string{"The ", "output ", "is ", "truncated."} {
		rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "thinking_delta", "delta": delta}}
	}
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "thinking_end", "content": "The output is truncated."}}
	// A second block without thinking_end is flushed when the message ends.
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "thinking_delta", "delta": "Let me "}}
	rpc.events <- record{"type": "message_update", "assistantMessageEvent": record{"type": "thinking_delta", "delta": "read."}}
	rpc.events <- record{"type": "message_end", "message": assistantMessage("stop", nil)}
	rpc.events <- record{"type": "agent_settled"}

	go s.processEvents(context.Background())
	events := collectEvents(t, s)
	var thinking []string
	for _, evt := range events {
		if evt.Thinking != "" {
			thinking = append(thinking, evt.Thinking)
		}
	}
	if len(thinking) != 2 || thinking[0] != "The output is truncated." || thinking[1] != "Let me read." {
		t.Fatalf("thinking events = %q, want two whole blocks", thinking)
	}
}
