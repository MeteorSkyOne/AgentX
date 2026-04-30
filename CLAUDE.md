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

**Data model**: Organization → Projects → Channels → Threads → Messages. Agents bind to channels via ChannelAgentBinding and run in Workspaces. Agents have configurable `kind`, `model`, `effort`, `fast_mode`, `yolo_mode`, `env`, and `description` fields.

**Agent runtimes**: Five runtime kinds are supported:
- `fake` — echo agent for testing
- `claude` — ephemeral Claude Code CLI sessions (`claude --print --output-format stream-json`)
- `codex` — ephemeral Codex CLI sessions (`codex exec --json`)
- `claude-persistent` — long-lived Claude Code process with stream-json stdin/stdout protocol and permission-prompt-tool stdio for interactive tool calls
- `codex-persistent` — long-lived Codex app-server process with JSON-RPC 2.0 protocol (`codex app-server --listen stdio://`)

**Persistent runtimes**: `claudepersist/` and `codexpersist/` keep agent processes alive across conversation turns via `procpool.ProcessPool`. Each pool manages `ManagedProcess` instances with turn-based mutex coordination (`AcquireTurn`/`ReleaseTurn`), idle reaping (default 30 min), and graceful shutdown. Sessions are keyed by `agentID:conversationType:conversationID`.

**Interactive tool calls**: When agents invoke AskUserQuestion (Claude) or requestUserInput (Codex), the runtime emits `EventInputRequest`. The app layer registers a pending question, publishes it via WebSocket, and blocks the session goroutine. The frontend renders a QuestionPrompt with option buttons and a text input. The user's answer is routed back through `POST /api/conversations/{type}/{id}/input-response` → `App.RespondToInputRequest` → `Session.RespondToInputRequest` channel → subprocess stdin. For Claude, the response is a `control_response` with `updatedInput` containing the answers. For Codex, it's a JSON-RPC result with the answers structure.

**Slash commands**: Built-in commands (`/new`, `/compact`, `/plan`, `/init`, `/model`, `/effort`, `/commit`, `/push`, `/review`) defined in `internal/app/commands.go`. Targeted at specific agents via `@handle` syntax. The composer provides autocomplete for both commands and agent mentions.

**Team coordination**: Multi-agent collaboration with leader/worker phases, tracked via `TeamMetadata` (session ID, leader agent, phase, turn counter). Channels configure team budgets via `team_max_batches` and `team_max_runs`.

**Notifications**: Organization-level webhook notifications (`internal/app/notifications.go`) with HMAC-SHA256 signing and URL templating. Browser native notifications (`web/src/notifications/browser.ts`) fire when agents reply and the page is inactive.

**Event flow**: Agent subprocess → Runtime Session → App (publishes to EventBus) → WebSocket handler → Frontend. Events are scoped by organization + conversation type + conversation ID. Event types: `AgentRunStarted`, `AgentOutputDelta`, `AgentRunCompleted`, `AgentRunFailed`, `AgentInputRequest`, `MessageCreated`.

**Auth**: Bearer token middleware. First-run admin setup uses `AGENTX_ADMIN_TOKEN` as the setup token; normal login uses username/password and 30-day hashed API sessions.

## Key Configuration (env vars)

| Variable | Default | Notes |
|----------|---------|-------|
| `AGENTX_ADDR` | 127.0.0.1:8080 | Server bind address; overrides `config.toml` |
| `AGENTX_ADMIN_TOKEN` | random | First-run setup token; `dev-token` in dev mode |
| `AGENTX_DATA_DIR` | ~/.agentx | SQLite storage and `config.toml` location |
| `AGENTX_DEFAULT_AGENT_KIND` | fake | `fake`, `claude`, `codex`, `claude-persistent`, `codex-persistent` |
| `AGENTX_DEFAULT_AGENT_MODEL` | | Default model for new agents |
| `AGENTX_CLAUDE_COMMAND` | claude | Path to Claude CLI |
| `AGENTX_CLAUDE_PERMISSION_MODE` | acceptEdits | `acceptEdits`, `bypassPermissions`, or `plan` |
| `AGENTX_CLAUDE_ALLOWED_TOOLS` | | Comma-separated allowed tool names |
| `AGENTX_CLAUDE_DISALLOWED_TOOLS` | | Comma-separated disallowed tool names |
| `AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT` | | System prompt appended to all Claude sessions |
| `AGENTX_CLAUDE_PERSISTENT_IDLE_MINUTES` | 30 | Idle timeout for persistent Claude processes |
| `AGENTX_CODEX_COMMAND` | codex | Path to Codex CLI |
| `AGENTX_CODEX_FULL_AUTO` | true | Auto-approve Codex operations |
| `AGENTX_CODEX_BYPASS_SANDBOX` | false | Bypass Codex sandbox |
| `AGENTX_CODEX_SKIP_GIT_REPO_CHECK` | true | Skip git repo validation |
| `AGENTX_CODEX_PERSISTENT_IDLE_MINUTES` | 30 | Idle timeout for persistent Codex processes |
| `AGENTX_D2_COMMAND` | d2 | D2 CLI binary used for Markdown `d2` diagrams |
| `AGENTX_D2_TIMEOUT_SECONDS` | 10 | Per-render D2 CLI timeout |
| `AGENTX_D2_CACHE_TTL_MINUTES` | 1440 | D2 SVG cache TTL |
| `AGENTX_D2_CACHE_MAX_ENTRIES` | 256 | Maximum backend D2 SVG cache entries |

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
- If a commit mixes unrelated changes, split it before committing whenever practical.

Examples:

```text
feat(web): add agent details panel
fix(scripts): handle ctrl-c as clean dev shutdown
refactor(store): split sqlite store by resource
test(e2e): isolate playwright ports from dev server
```

## Frontend

React 19 app in `web/`. State management via TanStack Query for server state and a custom message state reducer (`web/src/messages/state.ts`) for real-time streaming. WebSocket hook in `web/src/ws/useConversationSocket.ts` handles subscribe/unsubscribe and event dispatch. UI uses Radix primitives with Tailwind 4. Composer supports slash command autocomplete and `@agent` mentions. Workspace file tree (`web/src/components/FileTree.tsx`) shows agent working directory contents. Code editor powered by Monaco Editor. Markdown rendering with KaTeX math support is lazy-loaded.
