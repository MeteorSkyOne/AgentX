# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is AgentX

A self-hosted AI coding agent management service. It coordinates organizations, channels, conversations, and agent activity from a local web UI. Agents (Claude Code CLI, Codex CLI, or a fake echo agent) are spawned as subprocesses and their streaming output is relayed to the frontend via WebSocket. Supports slash commands, workspace file browsing, agent-level configuration (effort, fast mode, descriptions), and webhook/browser notifications for agent activity.

## Commands

```bash
make dev          # Full stack: API (127.0.0.1:8080) + Web UI (127.0.0.1:5173), token=dev-token
make build        # Build Go binary
make test         # All tests: Go + shell scripts + frontend build + frontend tests
make run          # Backend only (set AGENTX_ADMIN_TOKEN first)
make web-build    # Build frontend
scripts/prod.sh   # Build and run production server (auto-generates bootstrap token)
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
  → internal/config/      (env var config)
```

**Data model**: Organization → Projects → Channels → Threads → Messages. Agents bind to conversations via ConversationBinding and run in Workspaces. Agents have configurable `effort`, `fast_mode`, and `description` fields.

**Slash commands**: Built-in commands (`/new`, `/compact`, `/plan`, `/init`, `/model`, `/effort`, `/commit`, `/push`, `/review`) defined in `internal/app/commands.go`. Targeted at specific agents via `@handle` syntax. The composer provides autocomplete for both commands and agent mentions.

**Runtime pattern**: `runtime.Runtime` creates a `Session` (spawns a CLI subprocess). The session emits streaming `Event`s (output deltas, run started/completed/failed) over a Go channel. The app layer publishes these to the event bus, which fans them out to WebSocket subscribers. The runtime passes up to 40 messages of conversation history as context, respecting context boundaries set by `/new`.

**Notifications**: Organization-level webhook notifications (`internal/app/notifications.go`) with HMAC-SHA256 signing and URL templating. Browser native notifications (`web/src/notifications/browser.ts`) fire when agents reply and the page is inactive. Settings managed via `notification_settings` table.

**Event flow**: Agent subprocess → Runtime Session → App (publishes to EventBus) → WebSocket handler → Frontend. Events are scoped by organization + conversation type + conversation ID.

**Auth**: Bearer token middleware. Bootstrap with `AGENTX_ADMIN_TOKEN` env var.

## Key Configuration (env vars)

| Variable | Default | Notes |
|----------|---------|-------|
| `AGENTX_ADDR` | 127.0.0.1:8080 | Server bind address |
| `AGENTX_ADMIN_TOKEN` | random | `dev-token` in dev mode |
| `AGENTX_DATA_DIR` | ~/.agentx | SQLite storage location |
| `AGENTX_DEFAULT_AGENT_KIND` | fake | `fake`, `claude`, or `codex` |
| `AGENTX_CLAUDE_COMMAND` | claude | Path to Claude CLI |
| `AGENTX_CODEX_COMMAND` | codex | Path to Codex CLI |

## Database

SQLite with Goose migrations in `internal/store/sqlite/migrations/`. The store interface is split into sub-stores (UserStore, AgentStore, ConversationStore, etc.) with transaction support via `Tx`.

## Design Rules

- Reuse components as much as possible, avoid reinventing the wheel.

## Frontend

React 19 app in `web/`. State management via TanStack Query for server state and a custom message state reducer (`web/src/messages/state.ts`) for real-time streaming. WebSocket hook in `web/src/ws/useConversationSocket.ts` handles subscribe/unsubscribe and event dispatch. UI uses Radix primitives with Tailwind 4. Composer supports slash command autocomplete and `@agent` mentions. Workspace file tree (`web/src/components/FileTree.tsx`) shows agent working directory contents. Markdown rendering is lazy-loaded.
