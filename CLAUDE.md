# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is AgentX

A self-hosted AI coding agent management service. It coordinates multiple AI agents from a single web UI — organizing work into projects and channels, routing conversations to agents, and streaming their output in real time. Agents (Claude Code CLI, Codex CLI, or a fake echo agent) can run as ephemeral subprocesses or as persistent long-lived processes. The system supports interactive tool calls (AskUserQuestion), team coordination across multiple agents, workspace file browsing, slash commands, and webhook/browser notifications.

## Commands

```bash
make dev          # Full stack: API (127.0.0.1:8080) + Web UI (127.0.0.1:5173), setup token=dev-token
make build        # Build Go binary
make test         # All tests: Go + shell scripts + frontend build + frontend tests
make run          # Backend only (set AGENTX_ADMIN_TOKEN first)
make web-build    # Build frontend
scripts/prod.sh   # Build and run production server (auto-generates setup token)
scripts/dev-worktree.sh <branch>  # Isolated git worktree dev environment per branch

# Go tests
go test ./...                              # All Go tests
go test ./internal/app/...                 # Single package
go test ./internal/app/... -run TestName   # Single test

# Frontend
cd web && pnpm test                        # Vitest unit tests
cd web && pnpm run e2e                     # Playwright E2E (needs: pnpm exec playwright install chromium)
cd web && pnpm run dev                     # Dev server only
```

## Architecture

```
web/ (React 19 + Vite + TypeScript + TanStack Query + Tailwind 4)
  ↕ HTTP REST + WebSocket (/api/ws)
cmd/agentx/main.go (bootstrap)
  → internal/httpapi/     (Chi v5 router, REST routes, WebSocket handler)
  → internal/app/         (business logic: auth, conversations, management)
  → internal/domain/      (types + events)
  → internal/store/       (interface) → sqlite/ (SQLite via modernc.org, Goose migrations)
  → internal/eventbus/    (in-memory pub/sub, filters by org/conversation)
  → internal/runtime/     (interface) → claude/ | codex/ | fake/ (ephemeral CLI adapters)
                                      → claudepersist/ | codexpersist/ (persistent-process runtimes)
                                      → procpool/ (process pool with turn coordination)
  → internal/config/      (env var + TOML config)
```

**Data model**: Organization → Projects → Channels → Threads → Messages. Agents bind to channels via ChannelAgentBinding and run in Workspaces. Agents have configurable `kind`, `model`, `effort`, `fast_mode`, `yolo_mode`, `env`, `description`, and `system_prompt` fields. `system_prompt` applies on every session according to `system_prompt_mode`: `append` (default) adds it after the runtime's built-in prompt (Claude `--append-system-prompt`, Codex `developer_instructions`, pi `--append-system-prompt`); `replace` swaps the built-in prompt for it (Claude and pi `--system-prompt`, Codex `-c model_instructions_file=<content-addressed temp file>` written by `runtime.SystemPromptFile`). The operator-level `AGENTX_*_APPEND_SYSTEM_PROMPT` text and the agent workspace instruction files are still appended in both modes. The persistent Claude, Codex and pi runtimes restart an idle pooled process when the prompt, mode or agent `env` changes (`procpool.GetOrCreateSigned`, env via `StartSessionRequest.EnvSignature`) since those only apply at launch; the conversation resumes by session/thread id.

**Forum post members**: In a forum channel (`ChannelTypeThread`), mentioning `@agent` in the post body scopes the new post to the mentioned agents — they become its only members (`thread_agents` table), and other channel agents have to be added afterwards from the Members panel (`GET`/`PUT /api/threads/{threadID}/agents`). A post with no mention stores no membership rows and inherits every agent bound to its channel. Scheduled `forum_post` tasks apply the same rule via their `agent_id` or prompt mentions. `App.conversationAgents` enforces membership for every thread conversation, so replies, team runs, and notifications all follow it.

**Agent runtimes**: Six runtime kinds are supported:
- `fake` — echo agent for testing
- `claude` — ephemeral Claude Code CLI sessions (`claude --print --output-format stream-json`)
- `codex` — ephemeral Codex CLI sessions (`codex exec --json`)
- `claude-persistent` — long-lived Claude Code process with stream-json stdin/stdout protocol and permission-prompt-tool stdio for interactive tool calls
- `codex-persistent` — long-lived Codex app-server process with JSON-RPC 2.0 protocol (`codex app-server --listen stdio://`)
- `pi` — long-lived [pi](https://pi.dev) process in RPC mode (`pi --mode rpc --session-dir … --session-id …`), JSONL commands/events over stdio. Always persistent; there is no ephemeral variant.

**Persistent runtimes**: `claudepersist/`, `codexpersist/` and `pi/` keep agent processes alive across conversation turns via `procpool.ProcessPool`. Each pool manages `ManagedProcess` instances with turn-based mutex coordination (`AcquireTurn`/`ReleaseTurn`), idle reaping (default 30 min), and graceful shutdown. Sessions are keyed by `agentID:conversationType:conversationID`.

**Background turns**: A Claude Code background Bash command (`local_bash` task) does not hold the reply open (`claude.BackgroundTracker`); background subagents still do. When the command finishes after the reply is over, the CLI wakes the agent for a turn nobody prompted. Its output reaches the process's fallback handler (`claudepersist/fallback.go`), which claims the turn (`ManagedProcess.ClaimTurn`; dispatch holds lines for the adopting reader, abandoned after 30 s) and offers it to the app via `runtime.BackgroundTurnNotifier`. `App.runBackgroundTurn` (`internal/app/background_turns.go`) runs it as an agent run with no prompt (`agentRunOptions.BackgroundTurn`, session `AdoptBackgroundTurn` instead of `Send`), so the report streams and lands as a reply of its own, with notifications, and messages sent meanwhile queue behind it.

**pi runtime** (`internal/runtime/pi/`): the pi session id is the provider session id; `--session-id` creates or resumes it, so there are no stale sessions, and `/new` (empty `PreviousSessionID` on a live process) sends `new_session`. Each turn first reconciles the agent's `model`/`effort` with `get_state` (`set_model`, `set_thinking_level`, effort = pi thinking level `off…max`), then sends `prompt` (images as base64 `ImageContent`) and completes only on `agent_settled` (`agent_end` can be followed by retries/queued work). Assistant text streamed before a tool call is promoted into a `thinking` process item with `ClearText`, like the Claude runtime. `/compact` maps to pi's `compact` command, `/cancel` to `abort`, steering to `steer`; usage sums every assistant `message_end` and `get_session_stats` supplies the context window. pi has no permission prompts, plan mode, goal, fast mode or built-in subagents, so AgentX ships a bridge extension (`internal/runtime/pi/extension/agentx.ts`, embedded and written to `AGENTX_PI_EXTENSION_DIR`, loaded with `-e`): it registers `ask_user` (model asks the user via `ctx.ui.select`/`input`), `update_plan` (the runtime turns its arguments into `Event.Plan`) and the `/agentx-mode plan=on|off yolo=on|off` command, which the runtime sends before a turn whenever the agent's `PermissionMode`/`yolo_mode` differ from what the process last received (tracked in `ManagedProcess.UserData`). Plan mode blocks `edit`/`write` and non read-only shell commands in the extension's `tool_call` hook; yolo off asks for confirmation before dangerous shell commands (`rm -rf`, `sudo`, force pushes, …). `fast_mode` is a no-op and `/goal` is refused. The `subagent` tool from the third-party pi-subagents package is rendered as a subagent (`subagent_started`/`subagent_completed`, child tools from `tool_execution_update` progress); AgentX passes `--exclude-tools subagents_enable` so it is active from the first turn, and (unless `AGENTX_PI_SUBAGENTS_FOREGROUND=false`) merges `asyncByDefault: false` into `~/.pi/agent/extensions/subagent/config.json` when that key is unset, because background children only report back as a custom message on a later turn. pi's extension UI requests (`select`/`confirm`/`input`/`editor`) become `EventInputRequest`s answered with `extension_ui_response`; `notify` becomes a process note.

**Interactive tool calls**: When agents invoke AskUserQuestion (Claude) or requestUserInput (Codex), the runtime emits `EventInputRequest`. The app layer registers a pending question, publishes it via WebSocket, and blocks the session goroutine. The frontend renders a QuestionPrompt with option buttons and a text input. The user's answer is routed back through `POST /api/conversations/{type}/{id}/input-response` → `App.RespondToInputRequest` → `Session.RespondToInputRequest` channel → subprocess stdin. For Claude, the response is a `control_response` with `updatedInput` containing the answers. For Codex, it's a JSON-RPC result with the answers structure.

**Goal and step progress**: Runtimes report the agent's `/goal` and step list as `runtime.Event.Goal` / `Plan` snapshots (`internal/runtime/progress.go`). Codex app-server maps `thread/goal/updated` / `thread/goal/cleared` and `turn/plan/updated`, and `codex exec` maps `todo_list` items. Claude Code's `/goal` is a Stop-hook loop with no goal event, so `claude.ProgressTracker` infers it from the synthetic `Goal set:` reply, the `isSynthetic` "Stop hook feedback" messages (one per unmet check) and the final result; Claude steps come from `TodoWrite` and `TaskCreate`/`TaskUpdate`. The app publishes the run's full progress as `AgentRunProgress` (replayed to late subscribers) and stores the final state in message metadata (`goal`, `plan`); `web/src/components/message-pane/RunProgress.tsx` renders it under streaming and finished replies.

**Slash commands**: Built-in commands (`/new`, `/compact`, `/plan`, `/init`, `/model`, `/effort`, `/commit`, `/push`, `/review`) defined in `internal/app/commands.go`. Targeted at specific agents via `@handle` syntax. The composer provides autocomplete for both commands and agent mentions.

**Team coordination**: Multi-agent collaboration with leader/worker phases, tracked via `TeamMetadata` (session ID, leader agent, phase, turn counter). Channels configure team budgets via `team_max_batches` and `team_max_runs`.

**Scheduled tasks**: Cron-driven project tasks (`internal/app/scheduled_tasks.go`, robfig/cron with `CRON_TZ`). Three kinds: `agent_prompt` posts the prompt into an existing conversation, `forum_post` opens a new thread in a forum channel on every run (title from `post_title` with `${task}`/`${date}`/`${time}`/`${datetime}` placeholders), and `shell_command` runs a workspace command (gated by `AGENTX_SCHEDULED_SHELL_ENABLED` plus owner/admin role). `fresh_context` resets the agent session (`ResetAgentSessionContext`) before each run so no conversation history or resumed provider session is sent. `notify` (default true) gates notifications for the replies a task triggers: when off, the posted prompt carries a `suppress_notifications` metadata flag that agent replies inherit, which makes both the webhook (`notifyAgentMessageCreated`) and the browser notification (`web/src/notifications/browser.ts`) skip them. Every run is recorded in `scheduled_task_runs` with status, timing, output, and the resulting `message_id`/`thread_id`; the Tasks panel renders that history and links back to the produced post.

**Notifications**: Organization-level webhook notifications (`internal/app/notifications.go`) with HMAC-SHA256 signing and URL templating. Browser native notifications (`web/src/notifications/browser.ts`) fire when agents reply and the page is inactive.

**Event flow**: Agent subprocess → Runtime Session → App (publishes to EventBus) → WebSocket handler → Frontend. Events are scoped by organization + conversation type + conversation ID. Event types: `AgentRunStarted`, `AgentOutputDelta`, `AgentRunProgress`, `AgentRunCompleted`, `AgentRunFailed`, `AgentInputRequest`, `MessageCreated`.

**Auth**: Bearer token middleware. First-run admin setup uses `AGENTX_ADMIN_TOKEN` as the setup token; normal login uses username/password and 30-day hashed API sessions.

## Key Configuration (env vars)

| Variable | Default | Notes |
|----------|---------|-------|
| `AGENTX_ADDR` | 127.0.0.1:8080 | Server bind address; overrides `config.toml` |
| `AGENTX_ADMIN_TOKEN` | random | First-run setup token; `dev-token` in dev mode |
| `AGENTX_DATA_DIR` | ~/.agentx | SQLite storage and `config.toml` location |
| `AGENTX_DEFAULT_AGENT_KIND` | fake | `fake`, `claude`, `codex`, `claude-persistent`, `codex-persistent`, `pi` |
| `AGENTX_DEFAULT_AGENT_MODEL` | | Default model for new agents |
| `AGENTX_CLAUDE_COMMAND` | claude | Path to Claude CLI |
| `AGENTX_CLAUDE_PERMISSION_MODE` | acceptEdits | `acceptEdits`, `bypassPermissions`, or `plan` |
| `AGENTX_CLAUDE_ALLOWED_TOOLS` | | Comma-separated allowed tool names |
| `AGENTX_CLAUDE_DISALLOWED_TOOLS` | | Comma-separated disallowed tool names |
| `AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT` | | System prompt appended to all Claude sessions |
| `AGENTX_CLAUDE_THINKING_DISPLAY` | summarized | Claude `--thinking-display` mode (`summarized`, `omitted`, `highlights`) |
| `AGENTX_CLAUDE_PERSISTENT_IDLE_MINUTES` | 30 | Idle timeout for persistent Claude processes |
| `AGENTX_CODEX_COMMAND` | codex | Path to Codex CLI |
| `AGENTX_CODEX_FULL_AUTO` | true | Auto-approve Codex operations |
| `AGENTX_CODEX_BYPASS_SANDBOX` | false | Bypass Codex sandbox |
| `AGENTX_CODEX_SKIP_GIT_REPO_CHECK` | true | Skip git repo validation |
| `AGENTX_CODEX_PERSISTENT_IDLE_MINUTES` | 30 | Idle timeout for persistent Codex processes |
| `AGENTX_PI_COMMAND` | pi | Path to pi CLI |
| `AGENTX_PI_PERSISTENT_IDLE_MINUTES` | 30 | Idle timeout for pi processes |
| `AGENTX_PI_SESSION_DIR` | `$AGENTX_DATA_DIR/pi-sessions` | pi `--session-dir` |
| `AGENTX_PI_APPEND_SYSTEM_PROMPT` | | System prompt appended to all pi sessions |
| `AGENTX_PI_TOOLS` | | Comma-separated pi tool allowlist |
| `AGENTX_PI_EXCLUDE_TOOLS` | | Comma-separated pi tools to disable |
| `AGENTX_PI_PROJECT_TRUST` | approve | `approve`, `no-approve`, or empty (pi decides) |
| `AGENTX_PI_EXTENSION_DIR` | `$AGENTX_DATA_DIR/pi-extension` | Where the bundled AgentX pi extension is written; empty disables it |
| `AGENTX_PI_SUBAGENTS_FOREGROUND` | true | Write `asyncByDefault: false` into pi-subagents' `config.json` (if unset) so subagents stream into the reply |
| `AGENTX_D2_COMMAND` | d2 | D2 CLI binary used for Markdown `d2` diagrams |
| `AGENTX_D2_TIMEOUT_SECONDS` | 10 | Per-render D2 CLI timeout |
| `AGENTX_D2_CACHE_TTL_MINUTES` | 1440 | D2 SVG cache TTL |
| `AGENTX_D2_CACHE_MAX_ENTRIES` | 256 | Maximum backend D2 SVG cache entries |
| `AGENTX_SCHEDULED_SHELL_ENABLED` | false | Enable project scheduled shell command tasks |

## Database

SQLite with Goose migrations in `internal/store/sqlite/migrations/`. The store interface is split into sub-stores (UserStore, AgentStore, ConversationStore, etc.) with transaction support via `Tx`.

## Design Rules

- Reuse components as much as possible, avoid reinventing the wheel.

## Commit Messages

Use Conventional Commits for all commits. Never include Co-Authored-By lines.

```text
<type>(<scope>): <summary>
```

- `type`: one of `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `build`, or `ci`.
- `scope`: optional but preferred; use the main area touched, such as `api`, `store`, `web`, `e2e`, `scripts`, `runtime`, or `config`.
- `summary`: imperative, lowercase, no trailing period, and keep it under 72 characters.
- Use `feat` for user-visible capabilities, `fix` for behavior corrections, `refactor` for restructuring without intended behavior change, and `chore` only for maintenance that does not fit the other types.
- One commit is one complete feature or fix. Keep its backend, frontend, and tests together in a single commit instead of splitting by layer; pick the scope of the main area touched (e.g. `web` for a UI feature backed by a small API change).
- Only split a commit when it mixes genuinely unrelated changes.

Examples:

```text
feat(web): add agent details panel
fix(scripts): handle ctrl-c as clean dev shutdown
refactor(store): split sqlite store by resource
test(e2e): isolate playwright ports from dev server
```

## Frontend

React 19 app in `web/`. State management via TanStack Query for server state and a custom message state reducer (`web/src/messages/state.ts`) for real-time streaming. WebSocket hook in `web/src/ws/useConversationSocket.ts` handles subscribe/unsubscribe and event dispatch. UI uses Radix primitives with Tailwind 4. Composer supports slash command autocomplete and `@agent` mentions. Workspace file tree (`web/src/components/FileTree.tsx`) shows agent working directory contents. Code editor powered by Monaco Editor. Markdown rendering with KaTeX math support is lazy-loaded.
