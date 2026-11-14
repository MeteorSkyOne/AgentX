package agy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
)

func writeFakeAgyScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-agy")
	body := `#!/usr/bin/env python3
import json, os, sys, time

state_file = os.environ.get("FAKE_AGY_STATE_FILE", "")
if state_file:
    with open(state_file, "a") as f:
        f.write("LAUNCH:" + json.dumps({"argv": sys.argv[1:], "cwd": os.getcwd(), "custom_env": os.environ.get("AGENTX_TEST_ENV", "")}) + "\n")

sessions = {}
counter = 0

def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()

def config_options(sess):
    return [
        {
            "id": "model",
            "name": "Model",
            "category": "model",
            "type": "select",
            "currentValue": sess["model"],
            "options": [
                {"value": "gemini-4-carbon", "name": "Gemini 4 Carbon"},
                {"value": "gemini-3.8-flash", "name": "Gemini 3.8 Flash"},
                {"value": "gpt-oss-120b-medium", "name": "GPT-OSS 120B"},
            ],
        },
        {
            "id": "thought_level",
            "name": "Thinking Level",
            "category": "thought_level",
            "type": "select",
            "currentValue": sess["thought"],
            "options": [
                {"value": "low", "name": "Low"},
                {"value": "medium", "name": "Medium"},
                {"value": "high", "name": "High"},
            ],
        },
        {
            "id": "mode",
            "name": "Mode",
            "category": "mode",
            "type": "select",
            "currentValue": sess["mode"],
            "options": [
                {"value": "default", "name": "Default"},
                {"value": "accept-edits", "name": "Accept Edits"},
                {"value": "auto", "name": "Auto"},
                {"value": "plan", "name": "Plan"},
            ],
        },
    ]

pending_perm = None

for raw in sys.stdin:
    raw = raw.strip()
    if not raw:
        continue
    msg = json.loads(raw)
    method = msg.get("method", "")
    req_id = msg.get("id")
    params = msg.get("params") or {}

    if not method and req_id is not None and pending_perm is not None:
        # Client answered session/request_permission (note string ID!)
        perm = pending_perm
        pending_perm = None
        outcome = (msg.get("result") or {}).get("outcome") or {}
        opt_id = outcome.get("optionId", "cancelled")
        sid = perm["sid"]
        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {
                "sessionId": sid,
                "update": {
                    "sessionUpdate": "agent_message_chunk",
                    "content": {"type": "text", "text": f"perm={opt_id} id_type={type(req_id).__name__}"},
                },
            },
        })
        send({
            "jsonrpc": "2.0",
            "id": perm["prompt_id"],
            "result": {
                "stopReason": "end_turn",
                "usage": {"inputTokens": 100, "outputTokens": 20, "cachedReadTokens": 50, "totalTokens": 170},
            },
        })
        continue

    if method == "initialize":
        send({
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "protocolVersion": 1,
                "agentCapabilities": {"loadSession": True, "promptCapabilities": {"image": True}},
            },
        })
    elif method == "session/new":
        counter += 1
        sid = f"sess-{counter}"
        meta_prompt = (((params.get("_meta") or {}).get("antigravity") or {}).get("systemPrompt") or {}).get("append", "")
        sessions[sid] = {
            "model": "gemini-4-carbon",
            "thought": "high",
            "mode": "default",
            "prompt": meta_prompt,
            "add_dirs": params.get("additionalDirectories") or [],
        }
        send({
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "sessionId": sid,
                "configOptions": config_options(sessions[sid]),
            },
        })
        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {
                "sessionId": sid,
                "update": {"sessionUpdate": "usage_update", "used": 4096, "size": 1048576},
            },
        })
    elif method in ("session/resume", "session/load"):
        sid = params.get("sessionId", "")
        if sid == "missing-session":
            send({
                "jsonrpc": "2.0",
                "id": req_id,
                "error": {"code": -32603, "message": f"Failed to read session {sid}; retry, or load it with session/load"},
            })
            continue
        if sid not in sessions:
            sessions[sid] = {"model": "gemini-4-carbon", "thought": "high", "mode": "default", "prompt": "", "add_dirs": []}
        send({
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "sessionId": sid,
                "configOptions": config_options(sessions[sid]),
            },
        })
        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {
                "sessionId": sid,
                "update": {"sessionUpdate": "usage_update", "used": 8192, "size": 1048576},
            },
        })
    elif method == "session/close":
        send({"jsonrpc": "2.0", "id": req_id, "result": {}})
    elif method == "session/set_config_option":
        sid = params["sessionId"]
        cid = params["configId"]
        val = params["value"]
        if cid == "model":
            sessions[sid]["model"] = val
        elif cid == "thought_level":
            sessions[sid]["thought"] = val
        elif cid == "mode":
            sessions[sid]["mode"] = val
        send({"jsonrpc": "2.0", "id": req_id, "result": {"configOptions": config_options(sessions[sid])}})
    elif method == "session/cancel":
        # Handled when waiting in slow_turn via next stdin message
        if pending_perm is not None:
            perm = pending_perm
            pending_perm = None
            send({"jsonrpc": "2.0", "id": perm["prompt_id"], "result": {"stopReason": "cancelled"}})
    elif method == "session/prompt":
        sid = params["sessionId"]
        sess = sessions[sid]
        blocks = params.get("prompt") or []
        text = blocks[0].get("text", "") if blocks else ""
        has_image = any(b.get("type") == "image" for b in blocks)
        if "ASK_PERM" in text:
            pending_perm = {"sid": sid, "prompt_id": req_id}
            # Note string request id "perm-str-1", matching real agy acp!
            send({
                "jsonrpc": "2.0",
                "id": "perm-str-1",
                "method": "session/request_permission",
                "params": {
                    "sessionId": sid,
                    "toolCall": {"toolCallId": "tc-1", "name": "run_command", "title": "Terminal", "args": {"CommandLine": "rm -rf build"}},
                    "options": [
                        {"optionId": "allow", "name": "Allow", "kind": "allow_once"},
                        {"optionId": "deny", "name": "Deny", "kind": "reject_once"},
                    ],
                },
            })
            continue
        if "WAIT_CANCEL" in text:
            pending_perm = {"sid": sid, "prompt_id": req_id}
            send({
                "jsonrpc": "2.0",
                "method": "session/update",
                "params": {"sessionId": sid, "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": "working..."}}},
            })
            continue

        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {"sessionId": sid, "update": {"sessionUpdate": "agent_thought_chunk", "content": {"type": "text", "text": "thinking step"}}},
        })
        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {
                "sessionId": sid,
                "update": {
                    "sessionUpdate": "tool_call",
                    "toolCallId": "call-read",
                    "name": "read_file",
                    "status": "in_progress",
                    "rawInput": {"Path": "README.md"},
                },
            },
        })
        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {
                "sessionId": sid,
                "update": {
                    "sessionUpdate": "tool_call_update",
                    "toolCallId": "call-read",
                    "name": "read_file",
                    "status": "completed",
                    "content": [{"type": "text", "text": "ok"}],
                },
            },
        })
        reply = f"sid={sid} model={sess['model']} thought={sess['thought']} mode={sess['mode']} img={has_image} sys={sess['prompt']}"
        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {"sessionId": sid, "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": reply}}},
        })
        send({
            "jsonrpc": "2.0",
            "method": "session/update",
            "params": {"sessionId": sid, "update": {"sessionUpdate": "usage_update", "used": 16384, "size": 1048576}},
        })
        send({
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "stopReason": "end_turn",
                "usage": {"inputTokens": 500, "outputTokens": 40, "cachedReadTokens": 200, "thoughtTokens": 15, "totalTokens": 740},
            },
        })
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestFakeAgyLifecyclePermissionsCancelAndStaleSession(t *testing.T) {
	script := writeFakeAgyScript(t)
	stateFile := filepath.Join(t.TempDir(), "launches.log")
	workspace := t.TempDir()
	instrDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(instrDir, "GEMINI.md"), []byte("Follow team style."), 0o644); err != nil {
		t.Fatal(err)
	}
	imgPath := filepath.Join(workspace, "sample.png")
	if err := os.WriteFile(imgPath, []byte("fakepng"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := New(Options{
		Command:            script,
		PermissionMode:     "accept-edits",
		AppendSystemPrompt: "Operator rule.",
		Env:                map[string]string{"FAKE_AGY_STATE_FILE": stateFile},
	})
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. First turn: new session, config reconciliation, image attachment, system prompt + GEMINI.md.
	sess1, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:              "a1",
		SessionKey:           "a1:channel:c1",
		Workspace:            workspace,
		InstructionWorkspace: instrDir,
		Model:                "gemini-3.8-flash-low",
		Effort:               "medium",
		SystemPrompt:         "Agent custom prompt.",
		Env:                  map[string]string{"AGENTX_TEST_ENV": "v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess1.Send(ctx, runtime.Input{
		Prompt: "Hello",
		Attachments: []runtime.Attachment{{
			ID:          "att1",
			Filename:    "sample.png",
			Kind:        "image",
			ContentType: "image/png",
			LocalPath:   imgPath,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	var final1 runtime.Event
	var sawToolCall, sawToolResult bool
	for evt := range sess1.Events() {
		if evt.Type == runtime.EventDelta {
			for _, p := range evt.Process {
				if p.Type == "tool_call" && p.ToolName == "read_file" {
					sawToolCall = true
				}
				if p.Type == "tool_result" && p.ToolName == "read_file" {
					sawToolResult = true
				}
			}
		} else {
			final1 = evt
		}
	}
	if final1.Type != runtime.EventCompleted {
		t.Fatalf("turn 1 final = %+v", final1)
	}
	if !sawToolCall || !sawToolResult {
		t.Fatalf("expected tool_call and tool_result, got call=%v result=%v", sawToolCall, sawToolResult)
	}
	if sess1.CurrentSessionID() != "sess-1" {
		t.Fatalf("sessionID = %q, want sess-1", sess1.CurrentSessionID())
	}
	for _, want := range []string{"model=gemini-3.8-flash", "thought=medium", "mode=accept-edits", "img=True", "Agent custom prompt.", "Operator rule.", "Follow team style."} {
		if !strings.Contains(final1.Text, want) {
			t.Fatalf("reply %q missing %q", final1.Text, want)
		}
	}
	if final1.Usage == nil || final1.Usage.Context == nil || *final1.Usage.Context.TotalTokens != 16384 {
		t.Fatalf("unexpected usage: %+v", final1.Usage)
	}

	// 2. ContextUsage probe on pooled process (/status).
	sessProbe, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:              "a1",
		SessionKey:           "a1:channel:c1",
		Workspace:            workspace,
		InstructionWorkspace: instrDir,
		SystemPrompt:         "Agent custom prompt.",
		PreviousSessionID:    "sess-1",
		Env:                  map[string]string{"AGENTX_TEST_ENV": "v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := sessProbe.(runtime.ContextUsageReader)
	if !ok {
		t.Fatal("session does not implement ContextUsageReader")
	}
	cu, err := reader.ContextUsage(ctx)
	if err != nil || cu == nil || *cu.TotalTokens != 16384 {
		t.Fatalf("ContextUsage = %+v, err=%v", cu, err)
	}
	_ = sessProbe.Close(ctx)

	// 3. Permission request with string JSON-RPC ID + plan mode reconciliation.
	sess2, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:              "a1",
		SessionKey:           "a1:channel:c1",
		Workspace:            workspace,
		InstructionWorkspace: instrDir,
		PermissionMode:       "plan",
		SystemPrompt:         "Agent custom prompt.",
		PreviousSessionID:    "sess-1",
		Env:                  map[string]string{"AGENTX_TEST_ENV": "v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess2.Send(ctx, runtime.Input{Prompt: "ASK_PERM"}); err != nil {
		t.Fatal(err)
	}
	var final2 runtime.Event
	for evt := range sess2.Events() {
		switch evt.Type {
		case runtime.EventInputRequest:
			if err := sess2.RespondToInputRequest(evt.InputRequest.QuestionID, "Allow"); err != nil {
				t.Fatal(err)
			}
		case runtime.EventDelta:
		default:
			final2 = evt
		}
	}
	if final2.Type != runtime.EventCompleted || !strings.Contains(final2.Text, "perm=allow id_type=str") {
		t.Fatalf("turn 2 final = %+v", final2)
	}

	// 4. Fresh session after /new (empty PreviousSessionID on reused process) + Interrupt.
	sess3, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:              "a1",
		SessionKey:           "a1:channel:c1",
		Workspace:            workspace,
		InstructionWorkspace: instrDir,
		SystemPrompt:         "Agent custom prompt.",
		PreviousSessionID:    "",
		Env:                  map[string]string{"AGENTX_TEST_ENV": "v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess3.Send(ctx, runtime.Input{Prompt: "WAIT_CANCEL"}); err != nil {
		t.Fatal(err)
	}
	var final3 runtime.Event
	interrupted := false
	for evt := range sess3.Events() {
		if evt.Type == runtime.EventDelta && evt.Text != "" && !interrupted {
			interrupted = true
			if err := sess3.(runtime.Interrupter).Interrupt(ctx); err != nil {
				t.Fatal(err)
			}
		} else if evt.Type != runtime.EventDelta {
			final3 = evt
		}
	}
	if final3.Type != runtime.EventCanceled {
		t.Fatalf("expected EventCanceled after Interrupt, got %+v", final3)
	}
	if sess3.CurrentSessionID() != "sess-2" {
		t.Fatalf("expected fresh session sess-2 after /new, got %q", sess3.CurrentSessionID())
	}

	// 5. Stale session emits StaleSession=true.
	sess4, err := rt.StartSession(ctx, runtime.StartSessionRequest{
		AgentID:              "a1",
		SessionKey:           "a1:channel:c1",
		Workspace:            workspace,
		InstructionWorkspace: instrDir,
		SystemPrompt:         "Agent custom prompt.",
		PreviousSessionID:    "missing-session",
		Env:                  map[string]string{"AGENTX_TEST_ENV": "v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess4.Send(ctx, runtime.Input{Prompt: "Hello again"}); err != nil {
		t.Fatal(err)
	}
	var final4 runtime.Event
	for evt := range sess4.Events() {
		if evt.Type != runtime.EventDelta {
			final4 = evt
		}
	}
	if final4.Type != runtime.EventFailed || !final4.StaleSession {
		t.Fatalf("expected StaleSession failure, got %+v", final4)
	}

	// Verify only 1 process was spawned for identical launch signatures!
	launches, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(launches)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 pooled process launch, got %d:\n%s", len(lines), string(launches))
	}
}
