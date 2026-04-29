#!/usr/bin/env bash
# Temporary script: E2E test for AskUserQuestion in persist mode.
# Uses Python to spawn Claude CLI and handle stdin/stdout bidirectionally.
# Usage: ./scripts/test_ask_question.sh [claude-command]
set -euo pipefail

CLAUDE_CMD="${1:-claude}"

echo "=== AskUserQuestion E2E test ==="
echo "Command: $CLAUDE_CMD"
echo ""

python3 -u << 'PYEOF'
import subprocess, json, sys, threading, time, os

claude_cmd = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("CLAUDE_CMD", "claude")

PROMPT = (
    "You must use the AskUserQuestion tool right now to ask me which programming language "
    "I prefer for a new CLI project. Present exactly 3 options: Go, Rust, Python. "
    "Each option must have a label and description. "
    "Do NOT answer in plain text - you MUST call the AskUserQuestion tool."
)

user_msg = json.dumps({
    "type": "user",
    "message": {
        "role": "user",
        "content": [{"type": "text", "text": PROMPT}]
    }
})

args = [
    claude_cmd,
    "--output-format", "stream-json",
    "--input-format", "stream-json",
    "--permission-prompt-tool", "stdio",
    "--verbose",
    "--permission-mode", "bypassPermissions",
]

proc = subprocess.Popen(
    args,
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=open("/tmp/claude-askq-stderr.log", "w"),
    text=True,
    bufsize=1,
)

print(f"Claude PID: {proc.pid}")
print(f"Sending prompt: {PROMPT[:80]}...")
print()

# Send the user message
proc.stdin.write(user_msg + "\n")
proc.stdin.flush()

# Read stdout line by line, handle control_requests
try:
    for line in proc.stdout:
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            print(f"[unparsed] {line[:200]}")
            continue

        line_type = obj.get("type", "unknown")

        if line_type == "control_request":
            request = obj.get("request", {})
            tool_name = request.get("tool_name", "")
            request_id = obj.get("request_id", "")

            if tool_name == "AskUserQuestion":
                print("=" * 60)
                print(">>> INTERCEPTED AskUserQuestion control_request <<<")
                print(json.dumps(obj, indent=2, ensure_ascii=False))
                print("=" * 60)

                # Build control_response with "Go" as the answer
                original_input = request.get("input", {})
                questions = original_input.get("questions", [])
                question_text = questions[0]["question"] if questions else ""

                updated_input = dict(original_input)
                updated_input["answers"] = {question_text: "Go"}

                response = {
                    "type": "control_response",
                    "response": {
                        "subtype": "success",
                        "request_id": request_id,
                        "response": {
                            "behavior": "allow",
                            "updatedInput": updated_input,
                        },
                    },
                }
                print()
                print(">>> SENDING control_response <<<")
                print(json.dumps(response, indent=2, ensure_ascii=False))
                print("=" * 60)
                print()

                proc.stdin.write(json.dumps(response) + "\n")
                proc.stdin.flush()
            else:
                # Auto-approve other control requests
                response = {
                    "type": "control_response",
                    "response": {
                        "subtype": "success",
                        "request_id": request_id,
                        "response": {"behavior": "allow"},
                    },
                }
                proc.stdin.write(json.dumps(response) + "\n")
                proc.stdin.flush()
                print(f"[{line_type}] auto-approved: {tool_name}")

        elif line_type == "result":
            print("=" * 60)
            print(">>> RESULT <<<")
            print(json.dumps(obj, indent=2, ensure_ascii=False))
            print("=" * 60)
            break
        elif "AskUserQuestion" in line:
            print(f"[{line_type}] (AskUserQuestion) {line[:300]}")
        elif line_type in ("assistant", "user"):
            raw = json.dumps(obj, ensure_ascii=False)
            print(f"[{line_type}] {raw[:250]}")
        elif line_type in ("system",):
            subtype = obj.get("subtype", "")
            if subtype == "init":
                print(f"[system:init] session_id={obj.get('session_id','?')}")
            else:
                print(f"[system:{subtype}] ...")
        else:
            print(f"[{line_type}] ...")
except KeyboardInterrupt:
    pass
finally:
    try:
        proc.stdin.close()
    except:
        pass
    proc.wait(timeout=5)

print()
print("=== Done ===")
PYEOF
