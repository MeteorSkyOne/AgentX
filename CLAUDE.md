# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is AgentX

A self-hosted AI coding agent management service. It coordinates organizations, channels, conversations, and agent activity from a local web UI. Agents (Claude Code CLI, Codex CLI, or a fake echo agent) are spawned as subprocesses and their streaming output is relayed to the frontend via WebSocket. Supports slash commands, workspace file browsing, agent-level configuration (effort, fast mode, descriptions), and webhook/browser notifications for agent activity.

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
  → internal/runtime/     (interface) → claude/ | codex/ | fake/ (CLI adapters)
  → internal/config/      (env var + TOML config)
```

**Data model**: Organization → Projects → Channels → Threads → Messages. Agents bind to conversations via ConversationBinding and run in Workspaces. Agents have configurable `effort`, `fast_mode`, and `description` fields.

**Slash commands**: Built-in commands (`/new`, `/compact`, `/plan`, `/init`, `/model`, `/effort`, `/commit`, `/push`, `/review`) defined in `internal/app/commands.go`. Targeted at specific agents via `@handle` syntax. The composer provides autocomplete for both commands and agent mentions.

**Runtime pattern**: `runtime.Runtime` creates a `Session` (spawns a CLI subprocess). The session emits streaming `Event`s (output deltas, run started/completed/failed) over a Go channel. The app layer publishes these to the event bus, which fans them out to WebSocket subscribers. The runtime passes up to 40 messages of conversation history as context, respecting context boundaries set by `/new`.

**Notifications**: Organization-level webhook notifications (`internal/app/notifications.go`) with HMAC-SHA256 signing and URL templating. Browser native notifications (`web/src/notifications/browser.ts`) fire when agents reply and the page is inactive. Settings managed via `notification_settings` table.

**Event flow**: Agent subprocess → Runtime Session → App (publishes to EventBus) → WebSocket handler → Frontend. Events are scoped by organization + conversation type + conversation ID.

**Auth**: Bearer token middleware. First-run admin setup uses `AGENTX_ADMIN_TOKEN` as the setup token; normal login uses username/password and 30-day hashed API sessions.

## Key Configuration (env vars)

| Variable | Default | Notes |
|----------|---------|-------|
| `AGENTX_ADDR` | 127.0.0.1:8080 | Server bind address; overrides `config.toml` |
| `AGENTX_ADMIN_TOKEN` | random | First-run setup token; `dev-token` in dev mode |
| `AGENTX_DATA_DIR` | ~/.agentx | SQLite storage and `config.toml` location |
| `AGENTX_DEFAULT_AGENT_KIND` | fake | `fake`, `claude`, or `codex` |
| `AGENTX_CLAUDE_COMMAND` | claude | Path to Claude CLI |
| `AGENTX_CODEX_COMMAND` | codex | Path to Codex CLI |

## Database

SQLite with Goose migrations in `internal/store/sqlite/migrations/`. The store interface is split into sub-stores (UserStore, AgentStore, ConversationStore, etc.) with transaction support via `Tx`.

## Design Rules

- Reuse components as much as possible, avoid reinventing the wheel.

## Commit Messages

Use Conventional Commits for all commits:

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

React 19 app in `web/`. State management via TanStack Query for server state and a custom message state reducer (`web/src/messages/state.ts`) for real-time streaming. WebSocket hook in `web/src/ws/useConversationSocket.ts` handles subscribe/unsubscribe and event dispatch. UI uses Radix primitives with Tailwind 4. Composer supports slash command autocomplete and `@agent` mentions. Workspace file tree (`web/src/components/FileTree.tsx`) shows agent working directory contents. Markdown rendering is lazy-loaded.
