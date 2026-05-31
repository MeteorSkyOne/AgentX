package codexpersist

import (
	"fmt"
	"strings"
	"testing"

	"github.com/meteorsky/agentx/internal/runtime"
)

func turnCompletedMsg(status string) jsonRPCMessage {
	return jsonRPCMessage{
		Method: "turn/completed",
		Params: map[string]any{"turn": map[string]any{"status": status}},
	}
}

// goalChunk is the text a single turn contributes to the goal transcript: a
// process-break marker (offset = process items delivered so far) followed by the
// turn's answer, mirroring flush().
func goalChunk(processCount int, text string) string {
	return fmt.Sprintf("\n\n<!-- process-break:%d -->\n\n%s", processCount, text)
}

func TestGoalIntermediateTurnCompletedIsNotTerminal(t *testing.T) {
	tr := newGoalTurnTracker()
	tr.state.writePendingAgentText("round one answer")

	events, ctrl, handled := tr.handleControl(turnCompletedMsg("completed"), 0)

	if !handled {
		t.Fatalf("turn/completed should be handled by the goal tracker")
	}
	if ctrl != goalCtrlArmIdle {
		t.Fatalf("ctrl = %v, want goalCtrlArmIdle (wait for continuation)", ctrl)
	}
	if tr.turnActive {
		t.Fatalf("turnActive should be false after turn/completed")
	}
	want := goalChunk(0, "round one answer")
	if len(events) != 1 || events[0].Type != runtime.EventDelta || events[0].Text != want {
		t.Fatalf("events = %#v, want a single delta streaming the round answer", events)
	}
	if got := tr.transcript.String(); got != want {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
}

func TestGoalEachTurnIsAProcessBreakSeparatedBlock(t *testing.T) {
	tr := newGoalTurnTracker()

	// Round one ran 2 tool calls (process items) before answering.
	tr.state.writePendingAgentText("round one")
	tr.handleControl(turnCompletedMsg("completed"), 2)

	// A continuation turn starts (the loop sets turnActive on turn/started),
	// runs 3 more tool calls, then answers.
	tr.turnActive = true
	tr.state.writePendingAgentText("round two")
	events, ctrl, _ := tr.handleControl(turnCompletedMsg("completed"), 5)

	if ctrl != goalCtrlArmIdle {
		t.Fatalf("ctrl = %v, want goalCtrlArmIdle", ctrl)
	}
	wantSecond := goalChunk(5, "round two")
	if len(events) != 1 || events[0].Text != wantSecond {
		t.Fatalf("events = %#v, want delta %q", events, wantSecond)
	}
	want := goalChunk(2, "round one") + goalChunk(5, "round two")
	got := tr.transcript.String()
	if got != want {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
	// Both rounds' answers and their process-break offsets are present so the
	// frontend can interleave each turn's text with its tool calls.
	for _, sub := range []string{"<!-- process-break:2 -->", "round one", "<!-- process-break:5 -->", "round two"} {
		if !strings.Contains(got, sub) {
			t.Fatalf("transcript %q missing %q", got, sub)
		}
	}
}

func TestGoalTerminalStatusDuringActiveTurnWaitsForTurnCompleted(t *testing.T) {
	tr := newGoalTurnTracker()

	events, ctrl, handled := tr.handleControl(jsonRPCMessage{
		Method: "thread/goal/updated",
		Params: map[string]any{"status": "completed"},
	}, 0)
	if !handled {
		t.Fatalf("thread/goal/updated should be handled")
	}
	// Terminal status mid-turn must not stop yet (wait for turn/completed), but it
	// arms the idle backstop so the run can't hang if the turn is abandoned.
	if ctrl != goalCtrlArmIdle || len(events) != 0 {
		t.Fatalf("terminal status during an active turn should arm idle, not stop: ctrl=%v events=%#v", ctrl, events)
	}
	if !tr.goalDone {
		t.Fatalf("goalDone should be set")
	}

	tr.state.writePendingAgentText("final summary")
	events, ctrl, _ = tr.handleControl(turnCompletedMsg("completed"), 4)
	if ctrl != goalCtrlStop {
		t.Fatalf("ctrl = %v, want goalCtrlStop once the final turn completes", ctrl)
	}
	if len(events) != 2 || events[0].Type != runtime.EventDelta || events[1].Type != runtime.EventCompleted {
		t.Fatalf("events = %#v, want [delta, completed]", events)
	}
	if want := goalChunk(4, "final summary"); events[1].Text != want {
		t.Fatalf("completed text = %q, want %q", events[1].Text, want)
	}
}

func TestGoalTerminalStatusBetweenTurnsCompletesImmediately(t *testing.T) {
	tr := newGoalTurnTracker()
	tr.state.writePendingAgentText("the answer")
	tr.handleControl(turnCompletedMsg("completed"), 1) // turnActive -> false, arms idle

	events, ctrl, _ := tr.handleControl(jsonRPCMessage{
		Method: "thread/goal/updated",
		Params: map[string]any{"goal": map[string]any{"status": "achieved"}},
	}, 1)
	if ctrl != goalCtrlStop {
		t.Fatalf("ctrl = %v, want goalCtrlStop between turns", ctrl)
	}
	if want := goalChunk(1, "the answer"); len(events) != 1 || events[0].Type != runtime.EventCompleted || events[0].Text != want {
		t.Fatalf("events = %#v, want a single completed with the transcript", events)
	}
}

func turnCompletedWithInputTokens(status string, inputTokens int) jsonRPCMessage {
	return jsonRPCMessage{
		Method: "turn/completed",
		Params: map[string]any{"turn": map[string]any{
			"status": status,
			"tokenUsage": map[string]any{
				"last": map[string]any{"inputTokens": float64(inputTokens)},
			},
		}},
	}
}

func TestGoalAccumulatesUsageAcrossTurns(t *testing.T) {
	tr := newGoalTurnTracker()

	tr.state.writePendingAgentText("r1")
	tr.handleControl(turnCompletedWithInputTokens("completed", 10), 0) // round one: 10 input tokens

	tr.turnActive = true
	tr.state.writePendingAgentText("r2")
	tr.handleControl(jsonRPCMessage{Method: "thread/goal/updated", Params: map[string]any{"status": "completed"}}, 0)
	events, ctrl, _ := tr.handleControl(turnCompletedWithInputTokens("completed", 7), 0) // round two: 7 input tokens

	if ctrl != goalCtrlStop {
		t.Fatalf("ctrl = %v, want goalCtrlStop", ctrl)
	}
	var completed *runtime.Event
	for i := range events {
		if events[i].Type == runtime.EventCompleted {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Usage == nil || completed.Usage.InputTokens == nil {
		t.Fatalf("events = %#v, want a completed event carrying summed usage", events)
	}
	if *completed.Usage.InputTokens != 17 {
		t.Fatalf("input tokens = %d, want 17 (10+7 summed across turns)", *completed.Usage.InputTokens)
	}
}

func TestClearGoalBestEffortSendsClear(t *testing.T) {
	rpc := &recordingSessionRPC{}
	s := &persistentSession{rpc: rpc, threadID: "thread_1"}

	s.clearGoalBestEffort()

	if rpc.method != "thread/goal/clear" {
		t.Fatalf("method = %q, want thread/goal/clear", rpc.method)
	}
	params, _ := rpc.params.(map[string]any)
	if params["threadId"] != "thread_1" {
		t.Fatalf("threadId = %v, want thread_1", params["threadId"])
	}
}

func TestClearGoalBestEffortSkipsWithoutThread(t *testing.T) {
	rpc := &recordingSessionRPC{}
	s := &persistentSession{rpc: rpc}

	s.clearGoalBestEffort()

	if rpc.method != "" {
		t.Fatalf("should not send an RPC without a thread id, got %q", rpc.method)
	}
}

func TestRPCClientDrainDiscardsBufferedNotifications(t *testing.T) {
	c := &rpcClient{notifyCh: make(chan jsonRPCMessage, 8)}
	c.notifyCh <- jsonRPCMessage{Method: "item/started"}
	c.notifyCh <- jsonRPCMessage{Method: "turn/completed"}

	c.drain()

	if len(c.notifyCh) != 0 {
		t.Fatalf("notifyCh len = %d, want 0 after drain", len(c.notifyCh))
	}
	c.drain() // draining an empty channel is a no-op and must not block
}

func TestGoalProgressUpdateKeepsRunning(t *testing.T) {
	tr := newGoalTurnTracker()
	tr.state.writePendingAgentText("partial")
	tr.handleControl(turnCompletedMsg("completed"), 0)

	events, ctrl, handled := tr.handleControl(jsonRPCMessage{
		Method: "thread/goal/updated",
		Params: map[string]any{"status": "inProgress"},
	}, 0)
	if !handled || ctrl != goalCtrlNone || len(events) != 0 {
		t.Fatalf("in-progress goal update must not finish: ctrl=%v events=%#v handled=%v", ctrl, events, handled)
	}
	if tr.goalDone {
		t.Fatalf("goalDone should remain false for an in-progress update")
	}
}

func TestGoalClearedCompletesRun(t *testing.T) {
	tr := newGoalTurnTracker()
	tr.state.writePendingAgentText("done")
	tr.handleControl(turnCompletedMsg("completed"), 0)

	events, ctrl, _ := tr.handleControl(jsonRPCMessage{Method: "thread/goal/cleared"}, 0)
	if ctrl != goalCtrlStop {
		t.Fatalf("ctrl = %v, want goalCtrlStop on cleared", ctrl)
	}
	if want := goalChunk(0, "done"); len(events) != 1 || events[0].Type != runtime.EventCompleted || events[0].Text != want {
		t.Fatalf("events = %#v, want completed with transcript", events)
	}
}

func TestGoalInterruptedTurnEmitsCanceled(t *testing.T) {
	tr := newGoalTurnTracker()
	tr.state.writePendingAgentText("partial work")

	events, ctrl, _ := tr.handleControl(turnCompletedMsg("interrupted"), 0)
	if ctrl != goalCtrlStop {
		t.Fatalf("ctrl = %v, want goalCtrlStop on interrupt", ctrl)
	}
	var canceled *runtime.Event
	for i := range events {
		if events[i].Type == runtime.EventCanceled {
			canceled = &events[i]
		}
	}
	if want := goalChunk(0, "partial work"); canceled == nil || canceled.Text != want {
		t.Fatalf("events = %#v, want a canceled event carrying partial work", events)
	}
}

func TestGoalStreamingNotificationsAreDelegated(t *testing.T) {
	tr := newGoalTurnTracker()
	for _, method := range []string{"item/agentMessage/delta", "item/started", "item/completed", "thread/tokenUsage/updated", "item/reasoning/textDelta"} {
		if _, _, handled := tr.handleControl(jsonRPCMessage{Method: method}, 0); handled {
			t.Fatalf("%s should be delegated to handleNotification, not consumed by the goal tracker", method)
		}
	}
}

func TestGoalFinishGoalUsesTranscript(t *testing.T) {
	tr := newGoalTurnTracker()
	tr.state.writePendingAgentText("answer")
	tr.handleControl(turnCompletedMsg("completed"), 0)

	events := tr.finishGoal(0)
	if want := goalChunk(0, "answer"); len(events) != 1 || events[0].Type != runtime.EventCompleted || events[0].Text != want {
		t.Fatalf("finishGoal events = %#v, want completed with transcript", events)
	}
}

func TestEventProcessItemCount(t *testing.T) {
	cases := []struct {
		name string
		evt  runtime.Event
		want int
	}{
		{"process items", runtime.Event{Process: []runtime.ProcessItem{{}, {}}}, 2},
		{"thinking only", runtime.Event{Thinking: "reasoning"}, 1},
		{"process beats thinking", runtime.Event{Process: []runtime.ProcessItem{{}}, Thinking: "x"}, 1},
		{"text only", runtime.Event{Text: "hello"}, 0},
		{"empty", runtime.Event{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := eventProcessItemCount(tc.evt); got != tc.want {
				t.Fatalf("eventProcessItemCount = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGoalUpdateStatusExtractsAcrossShapes(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"top level string", map[string]any{"status": "completed"}, "completed"},
		{"nested status object", map[string]any{"status": map[string]any{"state": "paused"}}, "paused"},
		{"goal object", map[string]any{"goal": map[string]any{"status": "failed"}}, "failed"},
		{"thread goal nested type", map[string]any{"threadGoal": map[string]any{"status": map[string]any{"type": "abandoned"}}}, "abandoned"},
		{"state key", map[string]any{"state": "budgetLimited"}, "budgetLimited"},
		{"missing", map[string]any{"objective": "do it"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := goalUpdateStatus(tc.params); got != tc.want {
				t.Fatalf("goalUpdateStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsTerminalGoalStatus(t *testing.T) {
	terminal := []string{"completed", "achieved", "failed", "abandoned", "paused", "blocked", "usage_limited", "budget-limited", "Cancelled", "DONE"}
	for _, s := range terminal {
		if !isTerminalGoalStatus(s) {
			t.Fatalf("isTerminalGoalStatus(%q) = false, want true", s)
		}
	}
	running := []string{"", "active", "in_progress", "inProgress", "running", "pending", "new", "unknown_future_state"}
	for _, s := range running {
		if isTerminalGoalStatus(s) {
			t.Fatalf("isTerminalGoalStatus(%q) = true, want false", s)
		}
	}
}
