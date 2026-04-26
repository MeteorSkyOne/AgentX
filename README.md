# AgentX

AgentX is a self-hosted AI coding agent management service for coordinating organizations, channels, conversations, and agent activity from a local web UI.

## Foundation MVP

- Go API server
- SQLite persistence
- Bootstrap admin token login
- Organization and channel model
- Message history
- WebSocket event stream
- React web client
- Fake echo agent runtime
- Codex CLI runtime adapter
- Claude Code CLI runtime adapter

## Development

Start the full local stack:

```sh
make dev
```

This starts the API on `127.0.0.1:8080`, the web client on `127.0.0.1:5173`, and uses the bootstrap token `dev-token`.

The frontend uses pnpm. If pnpm is not already available, enable Corepack once with `corepack enable`.

Run the backend only:

```sh
AGENTX_ADMIN_TOKEN=dev-token go run ./cmd/agentx
```

The API listens on `127.0.0.1:8080`.

Choose the default runtime before the first bootstrap:

```sh
AGENTX_ADMIN_TOKEN=dev-token AGENTX_DEFAULT_AGENT_KIND=codex go run ./cmd/agentx
AGENTX_ADMIN_TOKEN=dev-token AGENTX_DEFAULT_AGENT_KIND=claude go run ./cmd/agentx
```

Codex uses `codex exec --json` and Claude Code uses `claude --print --output-format stream-json`. The CLI commands must already be installed and authenticated. Optional knobs:

- `AGENTX_DEFAULT_AGENT_MODEL`
- `AGENTX_CODEX_COMMAND`, `AGENTX_CODEX_FULL_AUTO`, `AGENTX_CODEX_BYPASS_SANDBOX`, `AGENTX_CODEX_SKIP_GIT_REPO_CHECK`
- `AGENTX_CLAUDE_COMMAND`, `AGENTX_CLAUDE_PERMISSION_MODE`, `AGENTX_CLAUDE_ALLOWED_TOOLS`, `AGENTX_CLAUDE_DISALLOWED_TOOLS`, `AGENTX_CLAUDE_APPEND_SYSTEM_PROMPT`

Run the web client:

```sh
cd web && pnpm install && pnpm run dev
```

Open `http://127.0.0.1:5173` and bootstrap with `dev-token`.

## Tests

```sh
go test ./...
bash scripts/dev_test.sh
cd web && pnpm test
cd web && pnpm run build
```

Run the browser e2e smoke test:

```sh
cd web
pnpm exec playwright install chromium
pnpm run e2e
```

On Linux, Playwright may also need system browser libraries. Install them once with `pnpm exec playwright install-deps chromium` when the environment allows it.
