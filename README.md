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

Run the backend:

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
cd web && npm run build
```
