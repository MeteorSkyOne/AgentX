# AgentX Technical Selection Design

Date: 2026-04-25
Status: Draft for review

## 1. Goal

AgentX is a self-hosted management tool for AI coding agents. It provides a Discord-like collaboration model with organizations, channels, threads, and DMs, while treating each coding agent as a bot user.

The MVP focuses on a local or small-team deployment. It must make Codex and Claude Code available through Web and Discord, support persistent agent configuration, and provide a clean architecture for later VSCode, mobile app, additional agents, and additional databases.

## 2. Non-Goals

The MVP will not include SaaS billing, horizontal worker scaling, Docker isolation, external message queues, VSCode extension delivery, mobile app delivery, advanced enterprise permissions, or additional database implementations beyond SQLite.

The architecture should leave room for these capabilities later, but they should not increase the initial implementation burden.

## 3. Product Model

AgentX follows a Discord-like organization model:

- `organization`: a server-like workspace for a team.
- `channel`: a named conversation space inside an organization.
- `thread`: a focused branch under a channel.
- `dm_conversation`: a direct conversation between users and/or bot users.
- `message`: a unified message record for user input, agent output, and system events.
- `user`: a real human user.
- `bot_user`: the visible bot identity for an agent.
- `agent`: the runtime configuration behind a bot user.
- `workspace`: either an agent default workspace or a user-configured project workspace.
- `conversation_binding`: the current agent and optional project workspace bound to a channel, thread, or DM.

Each agent has a default private workspace. This is where long-lived files such as `memory.md`, agent-local settings, and agent state live. Users can also create project workspaces that point at code directories. A channel, thread, or DM can bind to an `agent + project_workspace`; when no project workspace is bound, the agent runs in its default workspace.

## 4. Recommended Architecture

Use a Go monolith for the backend and a separate TypeScript React frontend.

```text
agentx server
├─ REST API
├─ WebSocket Gateway
├─ EventBus
├─ Domain Services
│  ├─ Auth
│  ├─ Organization / Channel / Thread / DM
│  ├─ Message
│  ├─ Agent / Bot
│  └─ Workspace
├─ Store Interface
│  └─ SQLite implementation
├─ Agent Runtime
│  ├─ Codex adapter
│  └─ Claude Code adapter
└─ External Adapters
   ├─ Web client
   └─ Discord connector
```

This mirrors the useful parts of `cc-connect`: agent adapters are isolated from platform adapters, core interfaces live in a shared domain layer, and the runtime can manage long-running or resumable CLI sessions. AgentX differs by making the product domain first-class: organizations, channels, threads, DMs, users, bot users, messages, and workspace bindings belong to the core system rather than to any single chat platform.

## 5. Backend Stack

Use Go as the main backend language.

Recommended libraries:

- HTTP router: `github.com/go-chi/chi/v5`
- WebSocket: `nhooyr.io/websocket`
- Database: SQLite through `modernc.org/sqlite`
- Migrations: `github.com/pressly/goose/v3`
- Discord: `github.com/bwmarrin/discordgo`
- Logging: Go `log/slog`
- Config file format: TOML for boot-time config, with runtime configuration stored in DB

Reasons:

- Go fits process supervision, CLI streaming, cancellation, and concurrent event fan-out well.
- A single binary keeps self-hosted deployment simple.
- `cc-connect` already validates the Go approach for Codex and Claude Code style adapters.
- `chi`, `nhooyr`, and `slog` keep the stack close to the standard library.
- `modernc.org/sqlite` avoids CGO friction for early distribution.

## 6. Frontend Stack

Use Vite, React, and TypeScript.

Recommended libraries:

- Data fetching and cache: TanStack Query
- Styling: Tailwind CSS
- Accessible primitives: Radix UI style primitives
- Realtime transport: browser WebSocket API wrapped by a typed client module

The first screen should be the working app, not a marketing page. The web UI should prioritize a dense Discord-like operational interface: organization/sidebar navigation, channel/thread list, message pane, agent status, and workspace/agent controls.

## 7. API Boundary

Use REST for resource operations and WebSocket for realtime events.

REST handles:

- Admin token and invite token login.
- User, organization, membership, channel, thread, and DM CRUD.
- Agent and bot user configuration.
- Workspace configuration.
- Conversation binding updates.
- Message history queries.
- Agent session lifecycle actions.

WebSocket handles:

- Subscribing to organizations, channels, threads, and DMs.
- Message and system event delivery.
- Agent streaming deltas.
- Agent run state.
- Permission requests and permission decisions.
- Typing/running presence.

This keeps management operations easy to debug while preserving a single realtime path for Web, Discord, and future clients.

## 8. Realtime and Event Model

Use a lightweight in-process typed event bus for the MVP. Do not introduce Kafka, NATS, Redis Streams, or another external queue in the first version.

All inputs should become commands. All externally visible changes should become events after persistence.

```text
Web / Discord
    -> Command
    -> Domain Service
    -> Store transaction
    -> EventBus
    -> WebSocket / Discord adapter

Agent Runtime
    -> AgentEvent
    -> Domain Service
    -> Store transaction
    -> EventBus
    -> WebSocket / Discord adapter
```

Initial event types:

- `MessageCreated`
- `ThreadCreated`
- `DMConversationCreated`
- `ConversationBindingUpdated`
- `AgentRunStarted`
- `AgentOutputDelta`
- `AgentToolUse`
- `AgentPermissionRequested`
- `AgentPermissionResolved`
- `AgentRunCompleted`
- `AgentRunFailed`
- `WorkspaceUpdated`

The event bus should be an internal abstraction, not a transport-specific API. WebSocket and Discord subscribe to the same semantic events and render them for their own clients.

## 9. Store Abstraction

The domain layer depends on store/repository interfaces. SQLite is the only MVP implementation.

Suggested shape:

```go
type Store interface {
    Tx(ctx context.Context, fn func(StoreTx) error) error
    Users() UserStore
    Organizations() OrganizationStore
    Channels() ChannelStore
    Conversations() ConversationStore
    Messages() MessageStore
    Agents() AgentStore
    Workspaces() WorkspaceStore
    Sessions() SessionStore
}

type StoreTx interface {
    Users() UserStore
    Organizations() OrganizationStore
    Channels() ChannelStore
    Conversations() ConversationStore
    Messages() MessageStore
    Agents() AgentStore
    Workspaces() WorkspaceStore
    Sessions() SessionStore
}
```

The abstraction should describe domain operations, not generic CRUD. Avoid leaking SQLite-specific SQL or driver behavior into domain services. Use relational schema and explicit transaction boundaries so another SQL implementation can be added later without changing core behavior.

## 10. Authentication and Permissions

Use a lightweight user system with admin and invite token flows.

Initial tables:

- `users`
- `organizations`
- `memberships`
- `invite_tokens`
- `external_identities`

Initial roles:

- `owner`
- `admin`
- `member`

The first admin can be bootstrapped through a local admin token. Organization membership can be created through invite tokens. External identities map Discord accounts and future VSCode/App identities to internal users.

This avoids building a full OAuth/password system in the MVP while still making user, membership, and permission data part of the real product model.

## 11. Agent Runtime

The agent runtime follows the adapter approach proven by `cc-connect`, but the interfaces should be owned by AgentX.

Suggested interface:

```go
type Runtime interface {
    StartSession(ctx context.Context, req StartSessionRequest) (Session, error)
    ListSessions(ctx context.Context, agentID string) ([]SessionInfo, error)
    StopAgent(ctx context.Context, agentID string) error
}

type Session interface {
    Send(ctx context.Context, input Input) error
    RespondPermission(ctx context.Context, decision PermissionDecision) error
    Events() <-chan Event
    CurrentSessionID() string
    Alive() bool
    Close(ctx context.Context) error
}
```

Initial adapters:

- Codex CLI
- Claude Code CLI

The execution model is host process isolation. AgentX launches the relevant CLI in the selected workspace directory, merges environment variables, parses structured output, persists agent session IDs, and publishes runtime events.

Environment variables merge in this order:

1. System/base environment.
2. Agent environment.
3. Workspace or conversation override environment.

The runtime should support model parameters and permission modes per agent. Adapter-specific options should remain under adapter-specific config, while shared concepts such as model, workspace, environment, and session persistence should stay in common structures.

## 12. Workspace Model

There are two workspace categories:

- Agent default workspace: owned by an agent and used when no project workspace is bound.
- Project workspace: user-created binding to a code directory.

A conversation can bind to:

- An agent only, using the agent default workspace.
- An agent plus a project workspace.

The binding can be applied to channels, threads, and DMs. A thread may inherit its channel binding at creation, then later override it.

The workspace record should store at least:

- ID
- Organization ID
- Name
- Type: `agent_default` or `project`
- Absolute path
- Created by
- Created/updated timestamps

## 13. Discord Adapter

Discord is an external adapter, not the internal source of truth.

Inbound Discord events should be normalized into AgentX commands:

- Discord guild -> organization mapping.
- Discord channel -> channel mapping.
- Discord thread -> thread mapping.
- Discord user -> external identity -> internal user.
- Discord message -> `SendMessage` command.

Outbound AgentX events should be rendered back into Discord:

- `MessageCreated` from an agent becomes a Discord message or thread reply.
- Streaming can be rendered as throttled message edits or compact progress messages.
- Permission requests can be rendered with buttons when supported.

The Web client uses the same domain model and event stream as Discord. Neither adapter should own conversation state.

## 14. Configuration

Separate boot-time configuration from runtime configuration.

Boot-time config can live in TOML/env:

- Listen address.
- Data directory.
- SQLite path.
- Admin bootstrap token.
- Discord bot token.
- Log level.

Runtime config should live in DB:

- Users and memberships.
- Organizations and channels.
- Agents and bot users.
- Agent model parameters.
- Agent env vars.
- Workspace definitions.
- Conversation bindings.

Secrets should be redacted in logs and API responses. For MVP, secrets may be stored locally in SQLite with file permission guidance. A later version can add OS keychain or external secret store support.

## 15. MVP Scope

Included:

- Go backend monolith.
- React/TypeScript web client.
- SQLite store implementation behind repository interfaces.
- Migration runner.
- REST API.
- WebSocket gateway.
- In-process typed event bus.
- Lightweight user, membership, and invite model.
- Organization, channel, thread, and DM model.
- Bot users and agents.
- Codex and Claude Code runtime adapters.
- Agent env/model parameter configuration.
- Agent default workspace and project workspace binding.
- Web adapter.
- Discord adapter.
- Message history and agent session state persistence.

Excluded:

- Docker or VM isolation.
- External queue.
- Distributed workers.
- SaaS billing.
- Full OAuth/password login system.
- VSCode extension.
- Mobile app.
- Additional database implementations.
- Advanced fine-grained permissions beyond owner/admin/member.

## 16. Implementation Order

Recommended first implementation slice:

1. Project scaffold: Go server, React web, SQLite migration runner.
2. Store interfaces and SQLite schema.
3. Auth bootstrap, users, organizations, memberships.
4. Channels, threads, DMs, messages, and conversation bindings.
5. In-process event bus and WebSocket subscriptions.
6. Web chat UI against REST and WebSocket.
7. Agent runtime interfaces and fake runtime for tests.
8. Codex adapter.
9. Claude Code adapter.
10. Agent/workspace configuration UI.
11. Discord adapter.

This order builds the product domain before attaching real agent and Discord complexity. A fake runtime should exist early so UI, message flow, persistence, and event fan-out can be tested before Codex/Claude-specific parsing is complete.

## 17. Key Risks

Agent CLI output and session behavior may change. Keep Codex and Claude Code parsing isolated inside adapters and add regression tests with captured output samples.

Host process execution is not a strong security boundary. The MVP should document this clearly and avoid presenting self-hosted host execution as safe for untrusted users.

The event bus is in-process. It is correct for the MVP but will not support multiple backend processes without replacement or bridging.

SQLite is suitable for a self-hosted MVP, but long-running agent output and large message histories need pagination, retention controls, and WAL mode.

Discord rate limits can affect streaming. The adapter should throttle edits and degrade to compact progress messages when needed.

## 18. Open Decisions For Later

- Whether to migrate from in-process event bus to Redis Streams, NATS, or another broker.
- Whether to add Docker or per-user OS account isolation.
- Whether to add Postgres as the second store implementation.
- Whether VSCode should use the same WebSocket protocol directly or a thin extension-specific gateway.
- Whether secrets should move from SQLite to OS keychain or an external secret manager.
