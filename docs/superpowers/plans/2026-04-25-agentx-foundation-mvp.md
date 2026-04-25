# AgentX Foundation MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first runnable AgentX slice: Go server, SQLite store, lightweight auth, organization/channel/message domain, in-process event bus, WebSocket updates, React web chat, and a fake agent runtime.

**Architecture:** The backend is a Go monolith with clear boundaries: `internal/app` owns domain workflows, `internal/store` defines repository interfaces, `internal/store/sqlite` implements them, `internal/eventbus` fans out typed events, `internal/runtime` defines agent runtime contracts, and `internal/httpapi` exposes REST/WebSocket. The web app is a Vite React client that uses REST for state and WebSocket for realtime conversation events.

**Tech Stack:** Go, chi, nhooyr WebSocket, modernc SQLite, goose-style embedded SQL migrations, slog, Vite, React, TypeScript, TanStack Query, Tailwind.

---

## Scope

This plan intentionally implements the foundation slice only. It produces working, testable software where an admin can bootstrap the app, open the web UI, select the default channel, send a message, and see a fake bot response stream back through WebSocket.

Codex runtime, Claude Code runtime, Discord integration, advanced permission controls, and a complete agent/workspace settings UI are separate follow-up plans. This keeps the first implementation small enough to finish and verify.

## File Structure

Create these backend files:

- `go.mod`: Go module and backend dependencies.
- `cmd/agentx/main.go`: process entrypoint.
- `internal/config/config.go`: boot-time config from env with defaults.
- `internal/id/id.go`: opaque ID and token generation.
- `internal/domain/types.go`: shared domain structs and constants.
- `internal/domain/events.go`: typed event payloads.
- `internal/eventbus/bus.go`: in-process pub/sub.
- `internal/store/store.go`: store interfaces used by app services.
- `internal/store/sqlite/db.go`: SQLite open, pragmas, migrations.
- `internal/store/sqlite/migrations/0001_init.sql`: initial schema.
- `internal/store/sqlite/store.go`: SQLite repository implementation.
- `internal/app/app.go`: service container.
- `internal/app/auth.go`: bootstrap/admin token and session auth.
- `internal/app/conversations.go`: organizations, channels, bindings, messages.
- `internal/app/runtime.go`: fake runtime orchestration for the foundation slice.
- `internal/runtime/runtime.go`: runtime interfaces.
- `internal/runtime/fake/fake.go`: deterministic fake agent.
- `internal/httpapi/router.go`: chi router assembly.
- `internal/httpapi/middleware.go`: auth middleware and JSON helpers.
- `internal/httpapi/routes_auth.go`: auth endpoints.
- `internal/httpapi/routes_conversations.go`: org/channel/message endpoints.
- `internal/httpapi/websocket.go`: WebSocket subscription gateway.

Create these backend test files:

- `internal/config/config_test.go`
- `internal/id/id_test.go`
- `internal/eventbus/bus_test.go`
- `internal/store/sqlite/store_test.go`
- `internal/app/auth_test.go`
- `internal/app/conversations_test.go`
- `internal/httpapi/httpapi_test.go`

Create these frontend files:

- `web/package.json`
- `web/vite.config.ts`
- `web/tsconfig.json`
- `web/index.html`
- `web/src/main.tsx`
- `web/src/App.tsx`
- `web/src/index.css`
- `web/src/api/client.ts`
- `web/src/api/types.ts`
- `web/src/ws/events.ts`
- `web/src/ws/useConversationSocket.ts`
- `web/src/components/LoginView.tsx`
- `web/src/components/Shell.tsx`
- `web/src/components/ChannelList.tsx`
- `web/src/components/MessagePane.tsx`
- `web/src/components/Composer.tsx`

Create these root files:

- `Makefile`: common build/test commands.
- `README.md`: local development instructions for the foundation slice.

## Task 1: Backend Scaffold And Config

**Files:**
- Create: `go.mod`
- Create: `cmd/agentx/main.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `Makefile`

- [ ] **Step 1: Initialize the Go module**

Run:

```bash
go mod init github.com/meteorsky/agentx
go get github.com/go-chi/chi/v5 nhooyr.io/websocket modernc.org/sqlite github.com/pressly/goose/v3
```

Expected: `go.mod` exists and contains module `github.com/meteorsky/agentx`.

- [ ] **Step 2: Write config tests**

Create `internal/config/config_test.go`:

```go
package config

import "testing"

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("AGENTX_ADDR", "")
	t.Setenv("AGENTX_DATA_DIR", "")
	t.Setenv("AGENTX_SQLITE_PATH", "")
	t.Setenv("AGENTX_ADMIN_TOKEN", "")

	cfg := FromEnv()

	if cfg.Addr != "127.0.0.1:8080" {
		t.Fatalf("Addr = %q, want 127.0.0.1:8080", cfg.Addr)
	}
	if cfg.DataDir != ".agentx" {
		t.Fatalf("DataDir = %q, want .agentx", cfg.DataDir)
	}
	if cfg.SQLitePath != ".agentx/agentx.db" {
		t.Fatalf("SQLitePath = %q, want .agentx/agentx.db", cfg.SQLitePath)
	}
	if cfg.AdminToken == "" {
		t.Fatal("AdminToken should have a generated token when unset")
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("AGENTX_ADDR", "0.0.0.0:9000")
	t.Setenv("AGENTX_DATA_DIR", "/tmp/agentx")
	t.Setenv("AGENTX_SQLITE_PATH", "/tmp/agentx/custom.db")
	t.Setenv("AGENTX_ADMIN_TOKEN", "dev-token")

	cfg := FromEnv()

	if cfg.Addr != "0.0.0.0:9000" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
	if cfg.DataDir != "/tmp/agentx" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.SQLitePath != "/tmp/agentx/custom.db" {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
	if cfg.AdminToken != "dev-token" {
		t.Fatalf("AdminToken = %q", cfg.AdminToken)
	}
}
```

- [ ] **Step 3: Run the failing config tests**

Run:

```bash
go test ./internal/config
```

Expected: FAIL because package `internal/config` does not exist.

- [ ] **Step 4: Implement env config**

Create `internal/config/config.go`:

Random admin token generation must fail closed: if `crypto/rand` fails, `randomToken()` panics so startup cannot continue with a predictable bootstrap secret.

```go
package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

type Config struct {
	Addr       string
	DataDir    string
	SQLitePath string
	AdminToken string
}

func FromEnv() Config {
	dataDir := getenv("AGENTX_DATA_DIR", ".agentx")
	return Config{
		Addr:       getenv("AGENTX_ADDR", "127.0.0.1:8080"),
		DataDir:    dataDir,
		SQLitePath: getenv("AGENTX_SQLITE_PATH", filepath.Join(dataDir, "agentx.db")),
		AdminToken: getenv("AGENTX_ADMIN_TOKEN", randomToken()),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("generate admin token: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
```

- [ ] **Step 5: Add a minimal server entrypoint**

Create `cmd/agentx/main.go`:

```go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/meteorsky/agentx/internal/config"
)

func main() {
	cfg := config.FromEnv()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("create data dir", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	slog.Info("agentx listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Add common commands**

Create `Makefile`:

```makefile
.PHONY: test build run

test:
	go test ./...

build:
	go build ./cmd/agentx

run:
	go run ./cmd/agentx
```

- [ ] **Step 7: Verify scaffold**

Run:

```bash
go test ./...
go build ./cmd/agentx
```

Expected: both commands exit 0.

- [ ] **Step 8: Commit**

Run:

```bash
git add go.mod go.sum Makefile cmd/agentx/main.go internal/config/config.go internal/config/config_test.go
git commit -m "chore: scaffold agentx server"
```

## Task 2: IDs, Domain Types, And Event Bus

**Files:**
- Create: `internal/id/id.go`
- Create: `internal/id/id_test.go`
- Create: `internal/domain/types.go`
- Create: `internal/domain/events.go`
- Create: `internal/eventbus/bus.go`
- Create: `internal/eventbus/bus_test.go`

- [ ] **Step 1: Write ID tests**

Create `internal/id/id_test.go`:

```go
package id

import "testing"

func TestNewReturnsPrefixedID(t *testing.T) {
	got := New("usr")
	if len(got) <= len("usr_") {
		t.Fatalf("id too short: %q", got)
	}
	if got[:4] != "usr_" {
		t.Fatalf("id prefix = %q, want usr_", got[:4])
	}
}

func TestNewTokenIsLong(t *testing.T) {
	got := NewToken()
	if len(got) < 32 {
		t.Fatalf("token length = %d, want >= 32", len(got))
	}
}
```

- [ ] **Step 2: Run the failing ID tests**

Run:

```bash
go test ./internal/id
```

Expected: FAIL because package `internal/id` does not exist.

- [ ] **Step 3: Implement ID helpers**

Create `internal/id/id.go`:

```go
package id

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

func New(prefix string) string {
	return prefix + "_" + randomBase32(16)
}

func NewToken() string {
	return randomBase32(32)
}

func randomBase32(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return strings.ToLower(enc)
}
```

- [ ] **Step 4: Define domain types**

Create `internal/domain/types.go`:

```go
package domain

import "time"

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type ConversationType string

const (
	ConversationChannel ConversationType = "channel"
	ConversationThread  ConversationType = "thread"
	ConversationDM      ConversationType = "dm"
)

type SenderType string

const (
	SenderUser   SenderType = "user"
	SenderBot    SenderType = "bot"
	SenderSystem SenderType = "system"
)

type MessageKind string

const (
	MessageText  MessageKind = "text"
	MessageEvent MessageKind = "event"
)

type User struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Channel struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
}

type BotUser struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	DisplayName    string    `json:"display_name"`
	CreatedAt      time.Time `json:"created_at"`
}

type Agent struct {
	ID                 string    `json:"id"`
	OrganizationID     string    `json:"organization_id"`
	BotUserID          string    `json:"bot_user_id"`
	Kind               string    `json:"kind"`
	Name               string    `json:"name"`
	Model              string    `json:"model"`
	DefaultWorkspaceID string    `json:"default_workspace_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Workspace struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Type           string    `json:"type"`
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ConversationBinding struct {
	ID               string           `json:"id"`
	OrganizationID   string           `json:"organization_id"`
	ConversationType ConversationType `json:"conversation_type"`
	ConversationID   string           `json:"conversation_id"`
	AgentID          string           `json:"agent_id"`
	WorkspaceID      string           `json:"workspace_id"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type Message struct {
	ID               string           `json:"id"`
	OrganizationID   string           `json:"organization_id"`
	ConversationType ConversationType `json:"conversation_type"`
	ConversationID   string           `json:"conversation_id"`
	SenderType       SenderType       `json:"sender_type"`
	SenderID         string           `json:"sender_id"`
	Kind             MessageKind      `json:"kind"`
	Body             string           `json:"body"`
	CreatedAt        time.Time        `json:"created_at"`
}
```

- [ ] **Step 5: Define typed events**

Create `internal/domain/events.go`:

```go
package domain

import "time"

type EventType string

const (
	EventMessageCreated             EventType = "MessageCreated"
	EventConversationBindingUpdated EventType = "ConversationBindingUpdated"
	EventAgentRunStarted            EventType = "AgentRunStarted"
	EventAgentOutputDelta           EventType = "AgentOutputDelta"
	EventAgentRunCompleted          EventType = "AgentRunCompleted"
	EventAgentRunFailed             EventType = "AgentRunFailed"
)

type Event struct {
	ID               string           `json:"id"`
	Type             EventType        `json:"type"`
	OrganizationID   string           `json:"organization_id"`
	ConversationType ConversationType `json:"conversation_type,omitempty"`
	ConversationID   string           `json:"conversation_id,omitempty"`
	Payload          any              `json:"payload"`
	CreatedAt        time.Time        `json:"created_at"`
}

type MessageCreatedPayload struct {
	Message Message `json:"message"`
}

type AgentOutputDeltaPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}

type AgentRunPayload struct {
	RunID   string `json:"run_id"`
	AgentID string `json:"agent_id"`
}

type AgentRunFailedPayload struct {
	RunID string `json:"run_id"`
	Error string `json:"error"`
}
```

- [ ] **Step 6: Write event bus tests**

Create `internal/eventbus/bus_test.go`:

```go
package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

func TestPublishDeliversToMatchingSubscriber(t *testing.T) {
	bus := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsubscribe := bus.Subscribe(ctx, Filter{OrganizationID: "org_1", ConversationID: "ch_1"})
	defer unsubscribe()

	bus.Publish(domain.Event{
		Type:           domain.EventMessageCreated,
		OrganizationID: "org_1",
		ConversationID: "ch_1",
	})

	select {
	case got := <-ch:
		if got.Type != domain.EventMessageCreated {
			t.Fatalf("event type = %q", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestPublishSkipsDifferentConversation(t *testing.T) {
	bus := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsubscribe := bus.Subscribe(ctx, Filter{OrganizationID: "org_1", ConversationID: "ch_1"})
	defer unsubscribe()

	bus.Publish(domain.Event{
		Type:           domain.EventMessageCreated,
		OrganizationID: "org_1",
		ConversationID: "ch_2",
	})

	select {
	case got := <-ch:
		t.Fatalf("unexpected event: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
}
```

- [ ] **Step 7: Run the failing event bus tests**

Run:

```bash
go test ./internal/id ./internal/eventbus
```

Expected: ID tests pass, event bus tests fail because `eventbus.New` is missing.

- [ ] **Step 8: Implement event bus**

Create `internal/eventbus/bus.go`:

```go
package eventbus

import (
	"context"
	"sync"

	"github.com/meteorsky/agentx/internal/domain"
)

type Filter struct {
	OrganizationID string
	ConversationID string
}

type Bus struct {
	mu   sync.RWMutex
	next int
	subs map[int]subscription
}

type subscription struct {
	filter Filter
	ch     chan domain.Event
}

func New() *Bus {
	return &Bus{subs: make(map[int]subscription)}
}

func (b *Bus) Subscribe(ctx context.Context, filter Filter) (<-chan domain.Event, func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	ch := make(chan domain.Event, 32)
	b.subs[id] = subscription{filter: filter, ch: ch}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		if sub, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(sub.ch)
		}
		b.mu.Unlock()
	}

	go func() {
		<-ctx.Done()
		unsubscribe()
	}()

	return ch, unsubscribe
}

func (b *Bus) Publish(evt domain.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subs {
		if !matches(sub.filter, evt) {
			continue
		}
		select {
		case sub.ch <- evt:
		default:
		}
	}
}

func matches(filter Filter, evt domain.Event) bool {
	if filter.OrganizationID != "" && filter.OrganizationID != evt.OrganizationID {
		return false
	}
	if filter.ConversationID != "" && filter.ConversationID != evt.ConversationID {
		return false
	}
	return true
}
```

- [ ] **Step 9: Verify and commit**

Run:

```bash
go test ./internal/id ./internal/eventbus
git add internal/id internal/domain internal/eventbus
git commit -m "feat: add domain events and event bus"
```

Expected: tests pass and commit succeeds.

## Task 3: Store Interfaces And SQLite Schema

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/sqlite/db.go`
- Create: `internal/store/sqlite/migrations/0001_init.sql`
- Create: `internal/store/sqlite/store.go`
- Create: `internal/store/sqlite/store_test.go`

- [ ] **Step 1: Define store contract**

Create `internal/store/store.go`:

```go
package store

import (
	"context"

	"github.com/meteorsky/agentx/internal/domain"
)

type Store interface {
	Tx(ctx context.Context, fn func(Tx) error) error
	Users() UserStore
	Organizations() OrganizationStore
	Channels() ChannelStore
	Messages() MessageStore
	BotUsers() BotUserStore
	Agents() AgentStore
	Workspaces() WorkspaceStore
	Bindings() BindingStore
	Sessions() SessionStore
}

type Tx interface {
	Users() UserStore
	Organizations() OrganizationStore
	Channels() ChannelStore
	Messages() MessageStore
	BotUsers() BotUserStore
	Agents() AgentStore
	Workspaces() WorkspaceStore
	Bindings() BindingStore
	Sessions() SessionStore
}

type UserStore interface {
	Create(ctx context.Context, user domain.User) error
	ByID(ctx context.Context, id string) (domain.User, error)
	CreateAPISession(ctx context.Context, token string, userID string) error
	UserIDByAPISession(ctx context.Context, token string) (string, error)
}

type OrganizationStore interface {
	Create(ctx context.Context, org domain.Organization) error
	ListForUser(ctx context.Context, userID string) ([]domain.Organization, error)
	AddMember(ctx context.Context, orgID string, userID string, role domain.Role) error
}

type ChannelStore interface {
	Create(ctx context.Context, channel domain.Channel) error
	ListByOrganization(ctx context.Context, orgID string) ([]domain.Channel, error)
	ByID(ctx context.Context, id string) (domain.Channel, error)
}

type MessageStore interface {
	Create(ctx context.Context, message domain.Message) error
	List(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error)
}

type BotUserStore interface {
	Create(ctx context.Context, bot domain.BotUser) error
}

type AgentStore interface {
	Create(ctx context.Context, agent domain.Agent) error
	ByID(ctx context.Context, id string) (domain.Agent, error)
	DefaultForOrganization(ctx context.Context, orgID string) (domain.Agent, error)
}

type WorkspaceStore interface {
	Create(ctx context.Context, workspace domain.Workspace) error
	ByID(ctx context.Context, id string) (domain.Workspace, error)
}

type BindingStore interface {
	Upsert(ctx context.Context, binding domain.ConversationBinding) error
	ByConversation(ctx context.Context, conversationType domain.ConversationType, conversationID string) (domain.ConversationBinding, error)
}

type SessionStore interface {
	SetAgentSession(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, providerSessionID string, status string) error
}
```

- [ ] **Step 2: Write SQLite schema**

Create `internal/store/sqlite/migrations/0001_init.sql`:

```sql
-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE users (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE api_sessions (
  token TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL
);

CREATE TABLE organizations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE memberships (
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (org_id, user_id)
);

CREATE TABLE channels (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE bot_users (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE agents (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  bot_user_id TEXT NOT NULL REFERENCES bot_users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  model TEXT NOT NULL,
  default_workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  env_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE conversation_bindings (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  conversation_type TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  agent_id TEXT NOT NULL REFERENCES agents(id),
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (conversation_type, conversation_id)
);

CREATE TABLE messages (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  conversation_type TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  sender_type TEXT NOT NULL,
  sender_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  body TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX messages_conversation_created_idx
  ON messages(conversation_type, conversation_id, created_at);

CREATE TABLE agent_sessions (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  conversation_type TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  provider_session_id TEXT NOT NULL,
  status TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(agent_id, conversation_type, conversation_id)
);

-- +goose Down
DROP TABLE IF EXISTS agent_sessions;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversation_bindings;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS bot_users;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS api_sessions;
DROP TABLE IF EXISTS users;
```

- [ ] **Step 3: Write store smoke test**

Create `internal/store/sqlite/store_test.go`:

```go
package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
)

func TestStoreCreatesOrganizationChannelAndMessage(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_1", DisplayName: "Admin", CreatedAt: now}
	org := domain.Organization{ID: "org_1", Name: "Default", CreatedAt: now}
	channel := domain.Channel{ID: "chn_1", OrganizationID: org.ID, Name: "general", CreatedAt: now}
	message := domain.Message{
		ID: "msg_1", OrganizationID: org.ID, ConversationType: domain.ConversationChannel,
		ConversationID: channel.ID, SenderType: domain.SenderUser, SenderID: user.ID,
		Kind: domain.MessageText, Body: "hello", CreatedAt: now,
	}

	err = st.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}
		if err := tx.Organizations().Create(ctx, org); err != nil {
			return err
		}
		if err := tx.Organizations().AddMember(ctx, org.ID, user.ID, domain.RoleOwner); err != nil {
			return err
		}
		if err := tx.Channels().Create(ctx, channel); err != nil {
			return err
		}
		return tx.Messages().Create(ctx, message)
	})
	if err != nil {
		t.Fatal(err)
	}

	channels, err := st.Channels().ListByOrganization(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0].Name != "general" {
		t.Fatalf("channels = %#v", channels)
	}

	messages, err := st.Messages().List(ctx, domain.ConversationChannel, channel.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Body != "hello" {
		t.Fatalf("messages = %#v", messages)
	}
}
```

- [ ] **Step 4: Run the failing store test**

Run:

```bash
go test ./internal/store/sqlite
```

Expected: FAIL because `sqlite.Open` is missing.

- [ ] **Step 5: Implement SQLite open and migrations**

Create `internal/store/sqlite/db.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("sqlite mkdir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite pragmas: %w", err)
	}
	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
```

- [ ] **Step 6: Implement repositories**

Create `internal/store/sqlite/store.go` with repository methods used by `store_test.go`. Use `time.RFC3339Nano` for timestamps and convert rows into `domain` structs. Implement `Store.Tx` with `db.BeginTx`, call `Commit` on nil error, and call `Rollback` on non-nil error.

The `Store` and transaction receiver must expose these methods:

```go
func (s *Store) Tx(ctx context.Context, fn func(store.Tx) error) error
func (s *Store) Users() userRepo
func (s *Store) Organizations() orgRepo
func (s *Store) Channels() channelRepo
func (s *Store) Messages() messageRepo
func (s *Store) BotUsers() botUserRepo
func (s *Store) Agents() agentRepo
func (s *Store) Workspaces() workspaceRepo
func (s *Store) Bindings() bindingRepo
func (s *Store) Sessions() sessionRepo
```

Define a concrete transaction wrapper named `txStore` and implement the same repository accessors on it.

For create methods, use parameterized `INSERT` statements. For list methods, order oldest to newest:

```sql
SELECT id, org_id, conversation_type, conversation_id, sender_type, sender_id, kind, body, created_at
FROM messages
WHERE conversation_type = ? AND conversation_id = ?
ORDER BY created_at ASC
LIMIT ?
```

- [ ] **Step 7: Verify and commit**

Run:

```bash
go test ./internal/store/sqlite
go test ./...
git add internal/store
git commit -m "feat: add sqlite store foundation"
```

Expected: tests pass and commit succeeds.

## Task 4: Bootstrap Auth And Default Workspace

**Files:**
- Create: `internal/app/app.go`
- Create: `internal/app/auth.go`
- Create: `internal/app/auth_test.go`

- [ ] **Step 1: Write bootstrap service test**

Create `internal/app/auth_test.go`:

```go
package app

import (
	"context"
	"testing"

	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
	"github.com/meteorsky/agentx/internal/eventbus"
)

func TestBootstrapCreatesAdminOrgChannelAgentAndWorkspace(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})
	result, err := app.Bootstrap(ctx, BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionToken == "" {
		t.Fatal("SessionToken is empty")
	}
	if result.User.DisplayName != "Meteorsky" {
		t.Fatalf("user = %#v", result.User)
	}
	if result.Organization.Name != "Default" {
		t.Fatalf("organization = %#v", result.Organization)
	}
	if result.Channel.Name != "general" {
		t.Fatalf("channel = %#v", result.Channel)
	}
	if result.Agent.Kind != "fake" {
		t.Fatalf("agent = %#v", result.Agent)
	}
	if result.Workspace.Type != "agent_default" {
		t.Fatalf("workspace = %#v", result.Workspace)
	}
}

func TestBootstrapRejectsWrongAdminToken(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})
	_, err = app.Bootstrap(ctx, BootstrapRequest{AdminToken: "wrong", DisplayName: "Bad"})
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run the failing bootstrap tests**

Run:

```bash
go test ./internal/app -run Bootstrap
```

Expected: FAIL because package `internal/app` does not exist.

- [ ] **Step 3: Implement app container**

Create `internal/app/app.go`:

```go
package app

import (
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/store"
)

type Options struct {
	AdminToken string
	DataDir    string
}

type App struct {
	store store.Store
	bus   *eventbus.Bus
	opts  Options
}

func New(st store.Store, bus *eventbus.Bus, opts Options) *App {
	return &App{store: st, bus: bus, opts: opts}
}
```

- [ ] **Step 4: Implement bootstrap workflow**

Create `internal/app/auth.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/store"
)

var ErrUnauthorized = errors.New("unauthorized")

type BootstrapRequest struct {
	AdminToken  string `json:"admin_token"`
	DisplayName string `json:"display_name"`
}

type BootstrapResult struct {
	SessionToken string              `json:"session_token"`
	User         domain.User         `json:"user"`
	Organization domain.Organization `json:"organization"`
	Channel      domain.Channel      `json:"channel"`
	BotUser      domain.BotUser      `json:"bot_user"`
	Agent        domain.Agent        `json:"agent"`
	Workspace    domain.Workspace    `json:"workspace"`
}

func (a *App) Bootstrap(ctx context.Context, req BootstrapRequest) (BootstrapResult, error) {
	if req.AdminToken != a.opts.AdminToken {
		return BootstrapResult{}, ErrUnauthorized
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = "Admin"
	}

	now := time.Now().UTC()
	user := domain.User{ID: id.New("usr"), DisplayName: name, CreatedAt: now}
	org := domain.Organization{ID: id.New("org"), Name: "Default", CreatedAt: now}
	channel := domain.Channel{ID: id.New("chn"), OrganizationID: org.ID, Name: "general", CreatedAt: now}
	bot := domain.BotUser{ID: id.New("bot"), OrganizationID: org.ID, DisplayName: "Fake Agent", CreatedAt: now}
	workspace := domain.Workspace{
		ID: id.New("wks"), OrganizationID: org.ID, Type: "agent_default",
		Name: "Fake Agent Workspace", Path: filepath.Join(a.opts.DataDir, "agents", "fake-default"),
		CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
	}
	agent := domain.Agent{
		ID: id.New("agt"), OrganizationID: org.ID, BotUserID: bot.ID,
		Kind: "fake", Name: "Fake Agent", Model: "fake-echo",
		DefaultWorkspaceID: workspace.ID, CreatedAt: now, UpdatedAt: now,
	}
	binding := domain.ConversationBinding{
		ID: id.New("bnd"), OrganizationID: org.ID, ConversationType: domain.ConversationChannel,
		ConversationID: channel.ID, AgentID: agent.ID, WorkspaceID: workspace.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	token := id.NewToken()

	err := a.store.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Users().Create(ctx, user); err != nil { return err }
		if err := tx.Users().CreateAPISession(ctx, token, user.ID); err != nil { return err }
		if err := tx.Organizations().Create(ctx, org); err != nil { return err }
		if err := tx.Organizations().AddMember(ctx, org.ID, user.ID, domain.RoleOwner); err != nil { return err }
		if err := tx.Channels().Create(ctx, channel); err != nil { return err }
		if err := tx.BotUsers().Create(ctx, bot); err != nil { return err }
		if err := tx.Workspaces().Create(ctx, workspace); err != nil { return err }
		if err := tx.Agents().Create(ctx, agent); err != nil { return err }
		return tx.Bindings().Upsert(ctx, binding)
	})
	if err != nil {
		return BootstrapResult{}, err
	}

	return BootstrapResult{
		SessionToken: token, User: user, Organization: org, Channel: channel,
		BotUser: bot, Agent: agent, Workspace: workspace,
	}, nil
}

func (a *App) UserForToken(ctx context.Context, token string) (domain.User, error) {
	userID, err := a.store.Users().UserIDByAPISession(ctx, token)
	if err != nil {
		return domain.User{}, ErrUnauthorized
	}
	return a.store.Users().ByID(ctx, userID)
}
```

- [ ] **Step 5: Add store interface assertion**

Confirm `*sqlite.Store` satisfies `store.Store` with this compile-time assertion in `internal/store/sqlite/store.go`:

```go
var _ store.Store = (*Store)(nil)
```

- [ ] **Step 6: Verify and commit**

Run:

```bash
go test ./internal/app ./internal/store/sqlite
go test ./...
git add internal/app internal/store/sqlite
git commit -m "feat: add bootstrap auth flow"
```

Expected: tests pass and commit succeeds.

## Task 5: Conversation Service And Fake Runtime

**Files:**
- Create: `internal/runtime/runtime.go`
- Create: `internal/runtime/fake/fake.go`
- Create: `internal/app/conversations.go`
- Create: `internal/app/runtime.go`
- Create: `internal/app/conversations_test.go`

- [ ] **Step 1: Write conversation service test**

Create `internal/app/conversations_test.go`:

```go
package app

import (
	"context"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

func TestSendMessagePersistsUserAndAgentMessages(t *testing.T) {
	ctx := context.Background()
	bus := eventbus.New()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, bus, Options{AdminToken: "secret", DataDir: t.TempDir()})
	boot, err := app.Bootstrap(ctx, BootstrapRequest{AdminToken: "secret", DisplayName: "Admin"})
	if err != nil {
		t.Fatal(err)
	}

	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID: boot.Organization.ID,
		ConversationID: boot.Channel.ID,
	})
	defer unsubscribe()

	msg, err := app.SendMessage(ctx, SendMessageRequest{
		UserID: boot.User.ID, OrganizationID: boot.Organization.ID,
		ConversationType: domain.ConversationChannel, ConversationID: boot.Channel.ID,
		Body: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Body != "hello" {
		t.Fatalf("message = %#v", msg)
	}

	deadline := time.After(2 * time.Second)
	sawDelta := false
	sawAgentMessage := false
	for !sawDelta || !sawAgentMessage {
		select {
		case evt := <-events:
			if evt.Type == domain.EventAgentOutputDelta {
				sawDelta = true
			}
			if evt.Type == domain.EventMessageCreated {
				payload := evt.Payload.(domain.MessageCreatedPayload)
				if payload.Message.SenderType == domain.SenderBot {
					sawAgentMessage = true
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for fake runtime events: sawDelta=%v sawAgentMessage=%v", sawDelta, sawAgentMessage)
		}
	}

	messages, err := app.ListMessages(ctx, domain.ConversationChannel, boot.Channel.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2: %#v", len(messages), messages)
	}
	if messages[1].Body != "Echo: hello" {
		t.Fatalf("agent message body = %q", messages[1].Body)
	}
}
```

- [ ] **Step 2: Run the failing conversation test**

Run:

```bash
go test ./internal/app -run SendMessage
```

Expected: FAIL because `SendMessage` and runtime interfaces do not exist.

- [ ] **Step 3: Define runtime interfaces**

Create `internal/runtime/runtime.go`:

```go
package runtime

import (
	"context"
)

type StartSessionRequest struct {
	AgentID     string
	Workspace  string
	Model      string
	Env        map[string]string
	SessionKey string
}

type Input struct {
	Prompt string
}

type EventType string

const (
	EventDelta     EventType = "delta"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
)

type Event struct {
	Type  EventType
	Text  string
	Error string
}

type Runtime interface {
	StartSession(ctx context.Context, req StartSessionRequest) (Session, error)
}

type Session interface {
	Send(ctx context.Context, input Input) error
	Events() <-chan Event
	CurrentSessionID() string
	Alive() bool
	Close(ctx context.Context) error
}
```

- [ ] **Step 4: Implement fake runtime**

Create `internal/runtime/fake/fake.go`:

```go
package fake

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/runtime"
)

type Runtime struct{}

func New() *Runtime {
	return &Runtime{}
}

func (r *Runtime) StartSession(context.Context, runtime.StartSessionRequest) (runtime.Session, error) {
	return &Session{id: id.New("fake_session"), events: make(chan runtime.Event, 16), alive: atomic.Bool{}}, nil
}

type Session struct {
	id     string
	events chan runtime.Event
	alive  atomic.Bool
}

func (s *Session) Send(ctx context.Context, input runtime.Input) error {
	s.alive.Store(true)
	go func() {
		text := "Echo: " + strings.TrimSpace(input.Prompt)
		if text == "Echo:" {
			text = "Echo: empty message"
		}
		select {
		case s.events <- runtime.Event{Type: runtime.EventDelta, Text: text}:
		case <-ctx.Done():
			return
		}
		select {
		case s.events <- runtime.Event{Type: runtime.EventCompleted, Text: text}:
		case <-ctx.Done():
			return
		}
	}()
	return nil
}

func (s *Session) Events() <-chan runtime.Event {
	return s.events
}

func (s *Session) CurrentSessionID() string {
	return s.id
}

func (s *Session) Alive() bool {
	return s.alive.Load()
}

func (s *Session) Close(context.Context) error {
	if s.alive.Swap(false) {
		close(s.events)
	}
	return nil
}
```

- [ ] **Step 5: Implement conversation service**

Create `internal/app/conversations.go` and `internal/app/runtime.go`.

`conversations.go` must expose:

```go
type SendMessageRequest struct {
	UserID           string
	OrganizationID   string
	ConversationType domain.ConversationType
	ConversationID   string
	Body             string
}

func (a *App) ListOrganizations(ctx context.Context, userID string) ([]domain.Organization, error)
func (a *App) ListChannels(ctx context.Context, orgID string) ([]domain.Channel, error)
func (a *App) ListMessages(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error)
func (a *App) SendMessage(ctx context.Context, req SendMessageRequest) (domain.Message, error)
```

`SendMessage` must trim whitespace, reject an empty body with an error, persist a user message, publish `MessageCreated`, then start the fake agent in a goroutine through `a.runAgentForMessage`.

`runtime.go` must:

- Look up the conversation binding.
- Look up the bound agent and workspace.
- Start `fake.New().StartSession`.
- Publish `AgentRunStarted`.
- Forward `runtime.EventDelta` as `AgentOutputDelta`.
- On `runtime.EventCompleted`, persist a bot message with the final text and publish `MessageCreated` plus `AgentRunCompleted`.
- On `runtime.EventFailed`, publish `AgentRunFailed`.

- [ ] **Step 6: Verify and commit**

Run:

```bash
go test ./internal/app ./internal/runtime/...
go test ./...
git add internal/runtime internal/app
git commit -m "feat: add conversation service and fake runtime"
```

Expected: tests pass and commit succeeds.

## Task 6: REST API

**Files:**
- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/middleware.go`
- Create: `internal/httpapi/routes_auth.go`
- Create: `internal/httpapi/routes_conversations.go`
- Create: `internal/httpapi/httpapi_test.go`
- Modify: `cmd/agentx/main.go`

- [ ] **Step 1: Write HTTP API test**

Create `internal/httpapi/httpapi_test.go`:

```go
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/eventbus"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

func TestBootstrapAndMessageHTTPFlow(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := app.New(st, eventbus.New(), app.Options{AdminToken: "secret", DataDir: t.TempDir()})
	srv := httptest.NewServer(NewRouter(a, eventbus.New()))
	defer srv.Close()

	bootReq := []byte(`{"admin_token":"secret","display_name":"Admin"}`)
	resp, err := http.Post(srv.URL+"/api/auth/bootstrap", "application/json", bytes.NewReader(bootReq))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status = %d", resp.StatusCode)
	}

	var boot struct {
		SessionToken string `json:"session_token"`
		Organization struct{ ID string `json:"id"` } `json:"organization"`
		Channel struct{ ID string `json:"id"` } `json:"channel"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&boot); err != nil {
		t.Fatal(err)
	}

	msgReq := []byte(`{"body":"hello from http"}`)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/conversations/channel/"+boot.Channel.ID+"/messages", bytes.NewReader(msgReq))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+boot.SessionToken)
	req.Header.Set("Content-Type", "application/json")
	msgResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer msgResp.Body.Close()
	if msgResp.StatusCode != http.StatusOK {
		t.Fatalf("message status = %d", msgResp.StatusCode)
	}
}
```

- [ ] **Step 2: Run the failing HTTP test**

Run:

```bash
go test ./internal/httpapi
```

Expected: FAIL because package `internal/httpapi` does not exist.

- [ ] **Step 3: Implement router and helpers**

Create `internal/httpapi/router.go`:

```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/eventbus"
)

type Server struct {
	app *app.App
	bus *eventbus.Bus
}

func NewRouter(a *app.App, bus *eventbus.Bus) http.Handler {
	s := &Server{app: a, bus: bus}
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/bootstrap", s.handleBootstrap)
		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Get("/me", s.handleMe)
			r.Get("/organizations", s.handleOrganizations)
			r.Get("/organizations/{orgID}/channels", s.handleChannels)
			r.Get("/conversations/{type}/{id}/messages", s.handleListMessages)
			r.Post("/conversations/{type}/{id}/messages", s.handleSendMessage)
		})
	})
	return r
}
```

Create `internal/httpapi/middleware.go` with `writeJSON`, `readJSON`, `writeError`, `authMiddleware`, and `userIDFromContext`. The auth middleware must require `Authorization: Bearer <token>` and call `app.UserForToken`.

- [ ] **Step 4: Implement auth routes**

Create `internal/httpapi/routes_auth.go` with:

```go
func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request)
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request)
```

`handleBootstrap` must decode `app.BootstrapRequest`, call `s.app.Bootstrap`, and return the `app.BootstrapResult` JSON directly.

- [ ] **Step 5: Implement conversation routes**

Create `internal/httpapi/routes_conversations.go` with:

```go
func (s *Server) handleOrganizations(w http.ResponseWriter, r *http.Request)
func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request)
func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request)
func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request)
```

`handleSendMessage` must map URL `{type}` to `domain.ConversationType`, read `{"body":"..."}`, and call `app.SendMessageRequest` with the authenticated user ID.

- [ ] **Step 6: Wire real router in main**

Modify `cmd/agentx/main.go` so it opens SQLite, creates one `eventbus.Bus`, creates `app.App`, and serves `httpapi.NewRouter`.

- [ ] **Step 7: Verify and commit**

Run:

```bash
go test ./internal/httpapi
go test ./...
git add cmd/agentx/main.go internal/httpapi
git commit -m "feat: expose foundation rest api"
```

Expected: tests pass and commit succeeds.

## Task 7: WebSocket Gateway

**Files:**
- Create: `internal/httpapi/websocket.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/httpapi_test.go`

- [ ] **Step 1: Add WebSocket test**

Append this test to `internal/httpapi/httpapi_test.go`:

```go
func TestWebSocketReceivesMessageCreated(t *testing.T) {
	ctx := context.Background()
	bus := eventbus.New()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := app.New(st, bus, app.Options{AdminToken: "secret", DataDir: t.TempDir()})
	boot, err := a.Bootstrap(ctx, app.BootstrapRequest{AdminToken: "secret", DisplayName: "Admin"})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewRouter(a, bus))
	defer srv.Close()

	wsURL := "ws" + srv.URL[len("http"):] + "/api/ws?token=" + boot.SessionToken
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	subscribe := `{"type":"subscribe","organization_id":"` + boot.Organization.ID + `","conversation_id":"` + boot.Channel.ID + `"}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(subscribe)); err != nil {
		t.Fatal(err)
	}

	_, err = a.SendMessage(ctx, app.SendMessageRequest{
		UserID: boot.User.ID, OrganizationID: boot.Organization.ID,
		ConversationType: domain.ConversationChannel, ConversationID: boot.Channel.ID,
		Body: "hello ws",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("MessageCreated")) {
		t.Fatalf("websocket data = %s", data)
	}
}
```

Also add imports for:

```go
"nhooyr.io/websocket"
"github.com/meteorsky/agentx/internal/domain"
```

- [ ] **Step 2: Run the failing WebSocket test**

Run:

```bash
go test ./internal/httpapi -run WebSocket
```

Expected: FAIL because `/api/ws` is not registered.

- [ ] **Step 3: Register WebSocket route**

Modify `internal/httpapi/router.go` directly under `/api`, before the authenticated REST group:

```go
r.Get("/ws", s.handleWebSocket)
```

Because browsers cannot set Authorization headers on a native WebSocket constructor, `handleWebSocket` must authenticate itself and accept either `Authorization: Bearer <token>` or `?token=<token>`.

- [ ] **Step 4: Implement WebSocket gateway**

Create `internal/httpapi/websocket.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/meteorsky/agentx/internal/eventbus"
	"nhooyr.io/websocket"
)

type subscribeMessage struct {
	Type           string `json:"type"`
	OrganizationID string `json:"organization_id"`
	ConversationID string `json:"conversation_id"`
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if _, err := s.app.UserForToken(r.Context(), token); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	_, data, err := conn.Read(r.Context())
	if err != nil {
		return
	}
	var sub subscribeMessage
	if err := json.Unmarshal(data, &sub); err != nil || sub.Type != "subscribe" {
		_ = conn.Close(websocket.StatusUnsupportedData, "expected subscribe")
		return
	}

	events, unsubscribe := s.bus.Subscribe(r.Context(), eventbus.Filter{
		OrganizationID: sub.OrganizationID,
		ConversationID: sub.ConversationID,
	})
	defer unsubscribe()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return
			}
			payload, _ := json.Marshal(evt)
			if err := conn.Write(r.Context(), websocket.MessageText, payload); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && header[:len(prefix)] == prefix {
		return header[len(prefix):]
	}
	return ""
}
```

- [ ] **Step 5: Verify and commit**

Run:

```bash
go test ./internal/httpapi -run WebSocket
go test ./...
git add internal/httpapi
git commit -m "feat: add websocket event gateway"
```

Expected: tests pass and commit succeeds.

## Task 8: React Web Client

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Create: `web/src/index.css`
- Create: `web/src/api/client.ts`
- Create: `web/src/api/types.ts`
- Create: `web/src/ws/events.ts`
- Create: `web/src/ws/useConversationSocket.ts`
- Create: `web/src/components/LoginView.tsx`
- Create: `web/src/components/Shell.tsx`
- Create: `web/src/components/ChannelList.tsx`
- Create: `web/src/components/MessagePane.tsx`
- Create: `web/src/components/Composer.tsx`

- [ ] **Step 1: Create frontend package**

Create `web/package.json`:

```json
{
  "scripts": {
    "dev": "vite --host 127.0.0.1",
    "build": "tsc -b && vite build",
    "typecheck": "tsc -b"
  },
  "dependencies": {
    "@tanstack/react-query": "^5.0.0",
    "@vitejs/plugin-react": "^5.0.0",
    "vite": "^7.0.0",
    "typescript": "^5.0.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "lucide-react": "^0.468.0"
  },
  "devDependencies": {}
}
```

- [ ] **Step 2: Add TypeScript and Vite config**

Create `web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx"
  },
  "include": ["src"]
}
```

Create `web/vite.config.ts`:

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8080",
    },
  },
});
```

- [ ] **Step 3: Define API types and client**

Create `web/src/api/types.ts`:

```ts
export type Organization = { id: string; name: string };
export type Channel = { id: string; organization_id: string; name: string };
export type User = { id: string; display_name: string };
export type Message = {
  id: string;
  organization_id: string;
  conversation_type: "channel" | "thread" | "dm";
  conversation_id: string;
  sender_type: "user" | "bot" | "system";
  sender_id: string;
  kind: "text" | "event";
  body: string;
  created_at: string;
};

export type BootstrapResponse = {
  session_token: string;
  user: User;
  organization: Organization;
  channel: Channel;
};
```

Create `web/src/api/client.ts`:

```ts
import type { BootstrapResponse, Channel, Message, Organization, User } from "./types";

const tokenKey = "agentx.session_token";

export function getToken(): string {
  return localStorage.getItem(tokenKey) ?? "";
}

export function setToken(token: string): void {
  localStorage.setItem(tokenKey, token);
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const resp = await fetch(path, { ...init, headers });
  if (!resp.ok) throw new Error(await resp.text());
  return resp.json() as Promise<T>;
}

export function bootstrap(adminToken: string, displayName: string): Promise<BootstrapResponse> {
  return request<BootstrapResponse>("/api/auth/bootstrap", {
    method: "POST",
    body: JSON.stringify({ admin_token: adminToken, display_name: displayName }),
  });
}

export function me(): Promise<User> {
  return request<User>("/api/me");
}

export function organizations(): Promise<Organization[]> {
  return request<Organization[]>("/api/organizations");
}

export function channels(orgID: string): Promise<Channel[]> {
  return request<Channel[]>(`/api/organizations/${orgID}/channels`);
}

export function messages(conversationType: string, conversationID: string): Promise<Message[]> {
  return request<Message[]>(`/api/conversations/${conversationType}/${conversationID}/messages`);
}

export function sendMessage(conversationType: string, conversationID: string, body: string): Promise<Message> {
  return request<Message>(`/api/conversations/${conversationType}/${conversationID}/messages`, {
    method: "POST",
    body: JSON.stringify({ body }),
  });
}
```

- [ ] **Step 4: Implement WebSocket hook**

Create `web/src/ws/events.ts`:

```ts
import type { Message } from "../api/types";

export type AgentXEvent =
  | { type: "MessageCreated"; payload: { message: Message } }
  | { type: "AgentRunStarted"; payload: { run_id: string; agent_id: string } }
  | { type: "AgentOutputDelta"; payload: { run_id: string; text: string } }
  | { type: "AgentRunCompleted"; payload: { run_id: string; agent_id: string } }
  | { type: "AgentRunFailed"; payload: { run_id: string; error: string } };
```

Create `web/src/ws/useConversationSocket.ts`:

```ts
import { useEffect } from "react";
import { getToken } from "../api/client";
import type { AgentXEvent } from "./events";

export function useConversationSocket(
  organizationID: string,
  conversationID: string,
  onEvent: (event: AgentXEvent) => void,
): void {
  useEffect(() => {
    if (!organizationID || !conversationID) return;
    const proto = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${window.location.host}/api/ws?token=${encodeURIComponent(getToken())}`);
    ws.onopen = () => {
      ws.send(JSON.stringify({ type: "subscribe", organization_id: organizationID, conversation_id: conversationID }));
    };
    ws.onmessage = (message) => {
      onEvent(JSON.parse(message.data) as AgentXEvent);
    };
    return () => ws.close();
  }, [organizationID, conversationID, onEvent]);
}
```

- [ ] **Step 5: Build UI components**

Create a dense app layout with a left channel list and central message pane.

`LoginView` must render admin token and display name inputs and call `bootstrap`.

`ChannelList` must render channels and call `onSelect(channel)`.

`MessagePane` must render messages, bot/user styling, streaming text from `AgentOutputDelta`, and empty state text only inside the message area.

`Composer` must render a single-line input and submit button; it must call `sendMessage("channel", channel.id, body)` and clear the input after a successful send.

`Shell` must compose channel list, message pane, and composer.

`App.tsx` must:

- Check for an existing token.
- Fetch `me`, `organizations`, and channels.
- If no token exists, show `LoginView`.
- Select the first organization and first channel after data loads.
- Fetch initial messages.
- Subscribe to WebSocket events.
- Append `MessageCreated` messages to local state.
- Show `AgentOutputDelta` as transient streaming text.

- [ ] **Step 6: Add frontend entry files**

Create `web/index.html`, `web/src/main.tsx`, and `web/src/index.css`.

`main.tsx` must wrap `App` with `QueryClientProvider`.

`index.css` must define a restrained dark/light neutral layout, fixed sidebar width, message pane scrolling, and stable composer height.

- [ ] **Step 7: Verify and commit**

Run:

```bash
cd web
npm install
npm run build
cd ..
git add web
git commit -m "feat: add web foundation client"
```

Expected: frontend build exits 0 and commit succeeds.

## Task 9: Full Integration And Development Docs

**Files:**
- Modify: `cmd/agentx/main.go`
- Create: `README.md`
- Modify: `Makefile`

- [ ] **Step 1: Serve built web assets in production mode**

Modify `cmd/agentx/main.go` to serve `web/dist` when it exists:

```go
if _, err := os.Stat("web/dist/index.html"); err == nil {
	mux.Handle("/", http.FileServer(http.Dir("web/dist")))
}
```

If `web/dist` is missing, the API server should still run; developers can use Vite proxy in that case.

- [ ] **Step 2: Expand Makefile**

Modify `Makefile`:

```makefile
.PHONY: test build run web-build

test:
	go test ./...

build:
	go build ./cmd/agentx

run:
	go run ./cmd/agentx

web-build:
	cd web && npm install && npm run build
```

- [ ] **Step 3: Write README**

Create `README.md`:

````markdown
# AgentX

Self-hosted management for AI coding agents.

## Foundation MVP

This slice includes:

- Go API server
- SQLite persistence
- Bootstrap admin token login
- Organization and channel model
- Message history
- WebSocket event stream
- React web client
- Fake echo agent runtime

## Run Backend

```bash
AGENTX_ADMIN_TOKEN=dev-token go run ./cmd/agentx
```

The API listens on `127.0.0.1:8080`.

## Run Web

```bash
cd web
npm install
npm run dev
```

Open `http://127.0.0.1:5173` and bootstrap with admin token `dev-token`.

## Test

```bash
go test ./...
cd web && npm run build
```
````

- [ ] **Step 4: Run full backend tests**

Run:

```bash
go test ./...
```

Expected: all Go tests pass.

- [ ] **Step 5: Run frontend build**

Run:

```bash
cd web
npm run build
```

Expected: TypeScript and Vite build exit 0.

- [ ] **Step 6: Manual smoke test**

Run backend:

```bash
AGENTX_ADMIN_TOKEN=dev-token go run ./cmd/agentx
```

Run web in another terminal:

```bash
cd web
npm run dev
```

Open `http://127.0.0.1:5173`, bootstrap with `dev-token`, send `hello`, and verify the message list shows:

```text
hello
Echo: hello
```

- [ ] **Step 7: Commit**

Run:

```bash
git add README.md Makefile cmd/agentx/main.go
git commit -m "docs: add foundation development workflow"
```

## Self-Review Checklist

- Spec coverage: this plan covers Go backend scaffold, SQLite store abstraction and implementation, lightweight auth, organization/channel/message model, event bus, REST, WebSocket, React web client, message history, and fake runtime. It intentionally excludes Codex, Claude Code, Discord, and advanced settings UI so those can be implemented as independent plans after the foundation is verified.
- Red-flag scan: run the writing-plans forbidden-token search against this file and expect no matches.
- Type consistency: `domain.ConversationType`, `domain.Message`, `domain.Event`, `eventbus.Filter`, `app.BootstrapRequest`, `app.SendMessageRequest`, and `runtime.Event` are introduced before use in implementation steps.
- Verification commands: every task ends with concrete `go test`, build, or manual smoke commands.
- Commit cadence: each task ends with one focused commit.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-25-agentx-foundation-mvp.md`. Two execution options:

1. **Subagent-Driven (recommended)** - dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** - execute tasks in this session using executing-plans, batch execution with checkpoints.
