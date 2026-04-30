# AgentX

A self-hosted AI coding agent management service. Coordinate multiple AI agents from a single web UI — organize work into projects and channels, route conversations to agents, and stream their output in real time.

## Features

- **Multi-agent management** — run Claude Code, Codex, or custom agents side-by-side in the same workspace
- **Persistent agent sessions** — keep agent processes alive across turns to reduce startup overhead and preserve context
- **Interactive tool calls** — agents can ask the user questions with selectable options (AskUserQuestion) surfaced directly in the chat UI
- **Real-time streaming** — agent output, thinking, and tool calls streamed over WebSocket
- **Workspace file browsing** — browse and edit agent working directories from the UI
- **Markdown diagrams** — render Mermaid blocks in the browser and D2 blocks through a cached CLI-backed SVG renderer
- **Slash commands** — `/new`, `/compact`, `/plan`, `/commit`, `/push`, `/review` and more, with `@agent` mention targeting
- **Team coordination** — multi-agent collaboration with leader/worker phases and turn budgets
- **Notifications** — webhook notifications with HMAC-SHA256 signing, plus browser native notifications
- **TLS support** — optional HTTPS with cert/key configuration via UI or config file

## Agent Runtimes

| Kind | Mode | Description |
|------|------|-------------|
| `fake` | Ephemeral | Echo agent for testing |
| `claude` | Ephemeral | Claude Code CLI (`claude --print --output-format stream-json`) |
| `codex` | Ephemeral | Codex CLI (`codex exec --json`) |
| `claude-persistent` | Persistent | Long-lived Claude Code process with stream-json stdin/stdout and permission-prompt-tool |
| `codex-persistent` | Persistent | Long-lived Codex app-server process with JSON-RPC 2.0 protocol |

Persistent runtimes are managed by a process pool with turn-based coordination and configurable idle timeout (default 30 minutes).

## Quick Start

```sh
make dev
```

Starts the API on `127.0.0.1:8080` and the web UI on `127.0.0.1:5173` with setup token `dev-token`.

Open `http://127.0.0.1:5173`, create an admin account using `dev-token`, then sign in.

### Choosing an agent runtime

```sh
AGENTX_DEFAULT_AGENT_KIND=claude make dev
AGENTX_DEFAULT_AGENT_KIND=claude-persistent make dev
AGENTX_DEFAULT_AGENT_KIND=codex-persistent make dev
```

The CLI commands (`claude`, `codex`) must already be installed and authenticated.

## Production

```sh
make prod
```

Builds the frontend, embeds it into the Go binary, and starts the server. The compiled binary is self-contained.

The server creates `~/.agentx/config.toml` on first run:

```toml
[server]
listen_ip = "127.0.0.1"
listen_port = 8080

[server.tls]
enabled = false
listen_port = 8443
cert_file = ""
key_file = ""
```

Set `AGENTX_ADDR` to override the listen address. Set `AGENTX_ADMIN_TOKEN` to use a stable setup token.

### Reset admin password

```sh
printf '%s\n' 'new-password' | agentx auth reset-admin --username admin --password-stdin
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENTX_ADDR` | 127.0.0.1:8080 | Server bind address (overrides config.toml) |
| `AGENTX_ADMIN_TOKEN` | random | First-run setup token |
| `AGENTX_DATA_DIR` | ~/.agentx | SQLite database and config location |
| `AGENTX_DEFAULT_AGENT_KIND` | fake | Default runtime: `fake`, `claude`, `codex`, `claude-persistent`, `codex-persistent` |
| `AGENTX_DEFAULT_AGENT_MODEL` | | Default model for new agents |
| `AGENTX_CLAUDE_COMMAND` | claude | Claude CLI binary |
| `AGENTX_CLAUDE_PERMISSION_MODE` | acceptEdits | Permission mode: `acceptEdits`, `bypassPermissions`, `plan` |
| `AGENTX_CLAUDE_ALLOWED_TOOLS` | | Comma-separated allowed tool names |
| `AGENTX_CLAUDE_DISALLOWED_TOOLS` | | Comma-separated disallowed tool names |
| `AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT` | | System prompt appended to all Claude sessions |
| `AGENTX_CLAUDE_PERSISTENT_IDLE_MINUTES` | 30 | Idle timeout for persistent Claude processes |
| `AGENTX_CODEX_COMMAND` | codex | Codex CLI binary |
| `AGENTX_CODEX_FULL_AUTO` | true | Auto-approve Codex operations |
| `AGENTX_CODEX_BYPASS_SANDBOX` | false | Bypass Codex sandbox |
| `AGENTX_CODEX_SKIP_GIT_REPO_CHECK` | true | Skip git repo validation |
| `AGENTX_CODEX_PERSISTENT_IDLE_MINUTES` | 30 | Idle timeout for persistent Codex processes |
| `AGENTX_D2_COMMAND` | d2 | D2 CLI binary used for Markdown `d2` diagrams |
| `AGENTX_D2_TIMEOUT_SECONDS` | 10 | Per-render D2 CLI timeout |
| `AGENTX_D2_CACHE_TTL_MINUTES` | 1440 | D2 SVG cache TTL |
| `AGENTX_D2_CACHE_MAX_ENTRIES` | 256 | Maximum backend D2 SVG cache entries |

## Tests

```sh
go test ./...                    # Go unit tests
bash scripts/dev_test.sh         # Integration tests
cd web && pnpm test              # Vitest frontend tests
cd web && pnpm run e2e           # Playwright E2E (needs: pnpm exec playwright install chromium)
```
