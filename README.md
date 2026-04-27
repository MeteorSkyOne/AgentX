# AgentX

AgentX is a self-hosted AI coding agent management service for coordinating organizations, channels, conversations, and agent activity from a local web UI.

## Foundation MVP

- Go API server
- SQLite persistence
- First-run admin setup and password login
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

This starts the API on `127.0.0.1:8080`, the web client on `127.0.0.1:5173`, and uses the setup token `dev-token` for the first admin account.

The frontend uses pnpm. If pnpm is not already available, enable Corepack once with `corepack enable`.

Run the backend only:

```sh
AGENTX_ADMIN_TOKEN=dev-token go run ./cmd/agentx
```

The API listens on `127.0.0.1:8080`.

Choose the default runtime before the first setup:

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

Open `http://127.0.0.1:5173`, set up the admin account with setup token `dev-token`, then sign in with the username and password you chose.

## Production

Build the web client, compile the Go server, and serve both from one process:

```sh
make prod
```

The production server listens on `127.0.0.1:8080` by default. Set `AGENTX_ADDR` to change it. Set `AGENTX_ADMIN_TOKEN` to use a stable first-run setup token; otherwise the script generates and prints a token for the current run.

Reset the local admin username and password directly against the configured SQLite database:

```sh
printf '%s\n' 'new-long-password' | agentx auth reset-admin --username admin --password-stdin
```

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

The e2e suite includes desktop and mobile viewport coverage. Optional diagnostic screenshots for AI-assisted UI review are available when needed and are not part of the default test command:

```sh
cd web
pnpm run e2e:screenshots
```

Screenshots are written under `.agentx-screenshot/`, which is ignored by git.

On Linux, Playwright may also need system browser libraries. Install them once with `pnpm exec playwright install-deps chromium` when the environment allows it.
