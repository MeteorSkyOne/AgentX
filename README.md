# AgentX

[![Release](https://img.shields.io/github/v/release/MeteorSkyOne/AgentX?display_name=tag)](https://github.com/MeteorSkyOne/AgentX/releases/latest)
[![Build](https://github.com/MeteorSkyOne/AgentX/actions/workflows/release.yml/badge.svg)](https://github.com/MeteorSkyOne/AgentX/actions/workflows/release.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/MeteorSkyOne/AgentX)](go.mod)

**AgentX** is a self-hosted control plane for AI coding agents. It runs Claude Code and Codex CLI agents on your own machine, organizes their work into projects and channels, and streams everything to a single web UI in real time.

Think of it as a chat workspace where every teammate is an agent: give each one a working directory, talk to them in channels, `@mention` them in forum posts, let them ask you questions, and watch their tool calls, subagents, and diffs as they happen.

- **Single binary.** The Go server embeds the React frontend. Download, run, open a browser.
- **Bring your own CLI.** AgentX drives the official `claude` and `codex` command-line tools, so it uses the subscriptions and credentials you already have.
- **Local first.** SQLite storage under `~/.agentx`, no external services required.

## Contents

- [Features](#features)
- [Agent runtimes](#agent-runtimes)
- [Getting started](#getting-started)
- [Configuration](#configuration)
- [Keeping AgentX up to date](#keeping-agentx-up-to-date)
- [CLI reference](#cli-reference)
- [Development](#development)
- [Architecture](#architecture)
- [Contributing](#contributing)

## Features

### Agents and conversations

- **Multiple agents side by side.** Bind any number of Claude Code, Codex, or test agents to a channel. Each agent has its own kind, model, effort, fast mode, YOLO mode, environment variables, and description.
- **Text and forum channels.** Text channels are one running conversation. Forum channels hold posts, each with its own thread; mentioning `@agent` in a post scopes it to those agents only.
- **Persistent sessions.** Persistent runtimes keep the agent process alive between turns, so context survives and every turn starts instantly.
- **Interactive questions.** When an agent calls `AskUserQuestion` (Claude) or `requestUserInput` (Codex), the question appears in the chat with clickable options and a free-text field. The answer is routed straight back to the process.
- **Live streaming.** Assistant text, thinking, tool calls, and subagent lifecycles stream over WebSocket. Subagent work is grouped under the call that spawned it and can be stopped from the UI.
- **Message queue and steering.** Queue messages while an agent is busy, then steer or cancel them before they are sent.
- **Failure diagnostics and retry.** Failed runs show the underlying reason and can be retried in place.
- **Slash commands.** `/new`, `/stop`, `/compact`, `/plan`, `/init`, `/model`, `/effort`, `/commit`, `/push`, `/review`, and more, with `@handle` targeting and composer autocomplete.
- **Team runs.** Multi-agent collaboration with leader/worker phases and per-channel batch and run budgets.
- **Skills.** Agent skills discovered from `SKILL.md` files in the workspace and agent home are listed per conversation.
- **Provider usage.** Per-model usage windows for Claude and Codex are shown in the UI so you can see how much of a plan is left.

### Workspaces

- **File browser and editor.** Browse, search, create, rename, delete, and edit files in each agent's working directory with a Monaco editor. Hidden files can be toggled. Images are previewed inline.
- **Git view.** Status, diff, and history for the workspace repository.
- **Integrated terminals.** PTY-backed shells inside each workspace, with reconnect replay and per-workspace session limits.
- **Attachments.** Attach files and images to messages and posts with upload progress and no fixed size limit.
- **Diagrams and math.** Markdown rendering with KaTeX, Mermaid diagrams in the browser, and D2 diagrams rendered through the D2 CLI with a server-side SVG cache.

### Projects and automation

- **Roadmap.** Per-project stages and tasks with ordering and completion tracking.
- **Scheduled tasks.** Cron-driven tasks per project: post a prompt into an existing conversation, open a new forum post on every run, or run a shell command in a workspace. Each task can reset agent context before running and mute notifications. Every run is recorded with status, timing, output, and a link to the resulting post.
- **Notifications.** Organization-level webhooks with HMAC-SHA256 signing and URL templating, plus browser native notifications when an agent replies while the tab is inactive.
- **Metrics.** Token and run metrics at the conversation, channel, and project level.

### Operations

- **Self-update.** Check for and install new AgentX releases from Settings, on demand or on a daily schedule. Choose the `release` or `dev` channel.
- **Runtime updates.** Check for and install Claude Code and Codex CLI updates from Settings, optionally on a daily schedule.
- **TLS.** Optional HTTPS with certificate and key paths managed from the UI or `config.toml`. A restart button reloads certificates without touching the shell.
- **Auth.** Username and password login with 30-day hashed API sessions. First-run setup is protected by a one-time setup token.

## Agent runtimes

| Kind | Mode | Backed by |
|------|------|-----------|
| `fake` | Ephemeral | Built-in echo agent for testing the UI without any CLI |
| `claude` | Ephemeral | `claude --print --output-format stream-json`, one process per turn |
| `codex` | Ephemeral | `codex exec --json`, one process per turn |
| `claude-persistent` | Persistent | Long-lived Claude Code process speaking stream-json over stdin/stdout, with a permission-prompt tool for interactive questions |
| `codex-persistent` | Persistent | Long-lived `codex app-server` process speaking JSON-RPC 2.0 over stdio |

Persistent processes are managed by a process pool with turn-based coordination, idle reaping (30 minutes by default), and graceful shutdown. The persistent kinds are recommended for day-to-day use.

## Getting started

### Prerequisites

- A Linux, macOS, or Windows machine (amd64 or arm64).
- For real agents, the CLI you want to use, installed and authenticated:
  - [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`claude`)
  - [Codex CLI](https://github.com/openai/codex) (`codex`)
- Optional: the [D2](https://d2lang.com) CLI if you want `d2` code blocks rendered as diagrams.

You can start with the `fake` runtime and add real agents later.

### 1. Install

Download the archive for your platform from the [latest release](https://github.com/MeteorSkyOne/AgentX/releases/latest) and put the binary on your `PATH`.

```sh
# Linux amd64 example
curl -fsSLO https://github.com/MeteorSkyOne/AgentX/releases/latest/download/agentx-linux-amd64.tar.gz
curl -fsSLO https://github.com/MeteorSkyOne/AgentX/releases/latest/download/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
tar -xzf agentx-linux-amd64.tar.gz
sudo install -m 0755 agentx /usr/local/bin/agentx
```

Available archives: `agentx-linux-amd64.tar.gz`, `agentx-linux-arm64.tar.gz`, `agentx-darwin-amd64.tar.gz`, `agentx-darwin-arm64.tar.gz`, `agentx-windows-amd64.zip`, `agentx-windows-arm64.zip`.

To build from source instead, see [Development](#development).

### 2. Run the server

```sh
AGENTX_DEFAULT_AGENT_KIND=claude-persistent agentx
```

On first start AgentX creates `~/.agentx/` with the SQLite database and a `config.toml`, listens on `http://127.0.0.1:8080`, and prints a setup token:

```text
Setup token: 3f9a...
```

Set `AGENTX_ADMIN_TOKEN` if you want a stable token instead of a random one. Set `AGENTX_ADDR` (for example `0.0.0.0:8080`) to listen on another interface.

### 3. Create the admin account

Open `http://127.0.0.1:8080`, paste the setup token, and choose a username and password. The token is only accepted until the first admin exists.

### 4. Add a project, a channel, and an agent

1. **Create a project** from the sidebar.
2. **Create a channel** inside it. Pick *Text* for a continuous conversation or *Forum* for posts.
3. **Create an agent** under Agents. Choose its kind (`claude-persistent`, `codex-persistent`, ...), a model, and a **workspace directory** that the agent will work in.
4. **Bind the agent** to the channel from the channel's Members panel.
5. Send a message. Use `@handle` to address a specific agent and `/` for slash commands.

### 5. Optional: enable HTTPS

Open **Settings → Server / SSL**, enable HTTPS, point it at a certificate and key file, and restart. The same settings live under `[server.tls]` in `config.toml`.

## Configuration

AgentX is configured through environment variables and a small `config.toml`. Environment variables take precedence at startup; settings that can be changed from the UI are persisted to `config.toml`.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENTX_ADDR` | *(from config.toml)* | Listen address, e.g. `0.0.0.0:8080`. Overrides `[server]` in `config.toml` |
| `AGENTX_DATA_DIR` | `~/.agentx` | Directory for the SQLite database and `config.toml` |
| `AGENTX_SQLITE_PATH` | `$AGENTX_DATA_DIR/agentx.db` | SQLite database file |
| `AGENTX_ADMIN_TOKEN` | random | One-time setup token used to create the first admin |
| `AGENTX_DEFAULT_AGENT_KIND` | `fake` | Default runtime for new agents: `fake`, `claude`, `codex`, `claude-persistent`, `codex-persistent` |
| `AGENTX_DEFAULT_AGENT_MODEL` | | Default model for new agents |
| `AGENTX_GITHUB_REPO` | `MeteorSkyOne/AgentX` | Repository used for self-update checks |
| `AGENTX_SELF_UPDATE_CHANNEL` | `release` | Self-update channel: `release` or `dev` |
| `AGENTX_CLAUDE_COMMAND` | `claude` | Claude Code CLI binary |
| `AGENTX_CLAUDE_PERMISSION_MODE` | `acceptEdits` | `acceptEdits`, `bypassPermissions`, or `plan` |
| `AGENTX_CLAUDE_ALLOWED_TOOLS` | | Comma-separated allowed tool names |
| `AGENTX_CLAUDE_DISALLOWED_TOOLS` | | Comma-separated disallowed tool names |
| `AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT` | | Text appended to the system prompt of every Claude session |
| `AGENTX_CLAUDE_PERSISTENT_IDLE_MINUTES` | `30` | Idle timeout before a persistent Claude process is stopped |
| `AGENTX_CODEX_COMMAND` | `codex` | Codex CLI binary |
| `AGENTX_CODEX_FULL_AUTO` | `true` | Run Codex in full-auto mode |
| `AGENTX_CODEX_BYPASS_SANDBOX` | `false` | Bypass the Codex sandbox |
| `AGENTX_CODEX_SKIP_GIT_REPO_CHECK` | `true` | Skip the Codex git repository check |
| `AGENTX_CODEX_PERSISTENT_IDLE_MINUTES` | `30` | Idle timeout before a persistent Codex process is stopped |
| `AGENTX_TERMINAL_SHELL` | `$SHELL` | Shell used for workspace terminals |
| `AGENTX_TERMINAL_IDLE_MINUTES` | `30` | Idle timeout for workspace terminals |
| `AGENTX_TERMINAL_MAX_SESSIONS_PER_WORKSPACE` | `8` | Maximum concurrent terminals per workspace |
| `AGENTX_TERMINAL_REPLAY_BYTES` | `8388608` | Scrollback replayed when a terminal reconnects |
| `AGENTX_SCHEDULED_SHELL_ENABLED` | `false` | Allow `shell_command` scheduled tasks (owner/admin only) |
| `AGENTX_D2_COMMAND` | `d2` | D2 CLI binary for `d2` diagram blocks |
| `AGENTX_D2_TIMEOUT_SECONDS` | `10` | Per-render D2 timeout |
| `AGENTX_D2_CACHE_TTL_MINUTES` | `1440` | D2 SVG cache TTL |
| `AGENTX_D2_CACHE_MAX_ENTRIES` | `256` | Maximum cached D2 renders |

### config.toml

Created on first run at `$AGENTX_DATA_DIR/config.toml`. All of these sections can also be edited from **Settings** in the UI.

```toml
[server]
listen_ip = "127.0.0.1"
listen_port = 8080

[server.tls]
enabled = false
listen_port = 8443
cert_file = ""
key_file = ""

[tool_updates]        # Claude Code / Codex CLI updates
auto_enabled = false
time_of_day = "04:00"
timezone = "UTC"        # defaults to $TZ when set
claude_enabled = true
codex_enabled = true

[self_update]         # AgentX binary updates
auto_enabled = false
time_of_day = "04:00"
timezone = "UTC"
channel = "release"   # "release" or "dev"
```

### Data and logs

- Database and config: `~/.agentx/` (or `AGENTX_DATA_DIR`).
- Logs: a timestamped file under `logs/` in the directory where `agentx` was started, mirrored to stderr.

## Keeping AgentX up to date

**From the UI.** Open **Settings → AgentX self-update**, check for updates, then update. AgentX downloads the matching release archive, verifies its SHA-256 checksum, replaces its own binary, and restarts. Enable the daily schedule to automate this.

**Channels.** The `release` channel follows tagged releases. The `dev` channel follows the rolling pre-release built from every push to `master`.

**Manually.** Download a new archive and replace the binary as in [Install](#1-install). The database is migrated automatically on start.

## CLI reference

```text
agentx                     Start the server
agentx version             Print version, commit, and build date
agentx auth reset-admin    Reset the admin password
    --username <name>      Admin username
    --password-stdin       Read the new password from stdin
```

Reset a forgotten admin password:

```sh
printf '%s\n' 'new-password' | agentx auth reset-admin --username admin --password-stdin
```

## Development

### Requirements

- Go 1.25 or newer
- Node.js 24 and pnpm 10 (`corepack enable` will install the pinned pnpm)

### Run the full stack

```sh
make dev
```

This starts the Go API on `127.0.0.1:8080` and the Vite dev server on `127.0.0.1:5173` with setup token `dev-token` and data in `./.agentx`. Open `http://127.0.0.1:5173`.

Pick a runtime for the session:

```sh
AGENTX_DEFAULT_AGENT_KIND=claude-persistent make dev
```

To work on several branches at once, `scripts/dev-worktree.sh <branch>` creates an isolated git worktree with its own data directory and ports.

### Build a production binary

```sh
make build        # builds web/, embeds it, writes ./agentx
make prod         # builds and starts the server with a generated setup token
```

The build script stamps the binary with the version derived from `git describe`, the commit, and the build date.

### Tests

```sh
make test                        # Go tests + shell script checks
go test ./...                    # Go unit tests only
go test ./internal/app/... -run TestName
cd web && pnpm test              # Vitest unit tests
cd web && pnpm run typecheck     # TypeScript
cd web && pnpm run e2e           # Playwright E2E (first: pnpm exec playwright install chromium)
```

## Architecture

```text
web/                      React 19 · Vite · TypeScript · TanStack Query · Tailwind 4 · Monaco
   │  REST + WebSocket (/api/ws)
cmd/agentx/               Entrypoint, CLI subcommands, logging, TLS servers
internal/httpapi/         Chi router, REST handlers, WebSocket and terminal endpoints
internal/app/             Business logic: auth, conversations, teams, scheduled tasks,
                          roadmap, notifications, terminals, self-update, tool updates
internal/domain/          Types and events
internal/store/sqlite/    SQLite (modernc.org) with Goose migrations
internal/eventbus/        In-memory pub/sub scoped by organization and conversation
internal/runtime/         Runtime interface
   ├── claude/ codex/ fake/           Ephemeral CLI adapters
   ├── claudepersist/ codexpersist/   Persistent process runtimes
   └── procpool/                      Process pool with turn coordination
internal/skills/          SKILL.md discovery
internal/config/          Environment variables and config.toml
internal/webdist/         Embedded frontend assets
```

**Data model.** Organization → Projects → Channels → Threads → Messages. Agents bind to channels and run inside Workspaces.

**Event flow.** Agent process → runtime session → app layer publishes to the event bus → WebSocket handler → frontend reducer. Events include run started, output delta, run completed, run failed, input request, and message created.

**API.** All endpoints live under `/api` and require a bearer token except `/api/healthz`, `/api/auth/setup`, and `/api/auth/login`. The frontend is the reference client; the route list in `internal/httpapi` is the source of truth.

## Contributing

Issues and pull requests are welcome.

- Run `make test` and `cd web && pnpm test` before opening a pull request.
- Use [Conventional Commits](https://www.conventionalcommits.org): `type(scope): summary`, with types `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `build`, or `ci` and scopes such as `api`, `web`, `runtime`, `store`, `config`, or `scripts`.
- Keep unrelated changes in separate commits.

Every push to `master` publishes a rolling `dev` pre-release with binaries for all platforms. Tagging `vX.Y.Z` publishes a versioned release.
