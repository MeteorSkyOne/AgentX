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

## Development

Start the full local stack:

```sh
make dev
```

This starts the API on `127.0.0.1:8080`, the web client on `127.0.0.1:5173`, and uses the bootstrap token `dev-token`.

Run the backend only:

```sh
AGENTX_ADMIN_TOKEN=dev-token go run ./cmd/agentx
```

The API listens on `127.0.0.1:8080`.

Run the web client:

```sh
cd web && npm install && npm run dev
```

Open `http://127.0.0.1:5173` and bootstrap with `dev-token`.

## Tests

```sh
go test ./...
bash scripts/dev_test.sh
cd web && npm run build
```
