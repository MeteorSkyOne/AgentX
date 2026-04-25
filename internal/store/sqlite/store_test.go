package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
)

func TestStoreCreatesOrganizationChannelAndMessage(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
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

	err := st.Tx(ctx, func(tx store.Tx) error {
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

func TestTxCallbackErrorRollsBack(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	user := domain.User{ID: "usr_rollback", DisplayName: "Rollback", CreatedAt: time.Now().UTC()}
	errRollback := errors.New("rollback")
	err := st.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("Tx error = %v, want %v", err, errRollback)
	}

	_, err = st.Users().ByID(ctx, user.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ByID error = %v, want sql.ErrNoRows", err)
	}
}

func TestTxCallbackPanicRollsBackAndReleasesConnection(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	user := domain.User{ID: "usr_panic", DisplayName: "Panic", CreatedAt: time.Now().UTC()}
	panicked := false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				panicked = true
			}
		}()
		_ = st.Tx(ctx, func(tx store.Tx) error {
			if err := tx.Users().Create(ctx, user); err != nil {
				return err
			}
			panic("boom")
		})
	}()
	if !panicked {
		t.Fatal("Tx did not re-panic")
	}

	queryCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err := st.Users().ByID(queryCtx, user.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ByID error = %v, want sql.ErrNoRows", err)
	}
}

func TestForeignKeysRejectInvalidReferences(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	err := st.Channels().Create(ctx, domain.Channel{
		ID:             "chn_missing_org",
		OrganizationID: "org_missing",
		Name:           "missing-org",
		CreatedAt:      now,
	})
	if err == nil {
		t.Fatal("Create channel with missing organization succeeded")
	}

	org := domain.Organization{ID: "org_fk", Name: "FK", CreatedAt: now}
	if err := st.Organizations().Create(ctx, org); err != nil {
		t.Fatal(err)
	}
	err = st.Organizations().AddMember(ctx, org.ID, "usr_missing", domain.RoleMember)
	if err == nil {
		t.Fatal("AddMember with missing user succeeded")
	}
}

func TestBindingUpsertReplacesAgentAndWorkspace(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	fixture := seedBindingFixture(t, ctx, st)
	first := domain.ConversationBinding{
		ID:               "bind_1",
		OrganizationID:   fixture.org.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   "chn_bound",
		AgentID:          fixture.agent1.ID,
		WorkspaceID:      fixture.workspace1.ID,
		CreatedAt:        fixture.now,
		UpdatedAt:        fixture.now,
	}
	second := first
	second.AgentID = fixture.agent2.ID
	second.WorkspaceID = fixture.workspace2.ID
	second.UpdatedAt = fixture.now.Add(time.Minute)

	if err := st.Bindings().Upsert(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := st.Bindings().Upsert(ctx, second); err != nil {
		t.Fatal(err)
	}

	got, err := st.Bindings().ByConversation(ctx, first.ConversationType, first.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != fixture.agent2.ID || got.WorkspaceID != fixture.workspace2.ID || got.OrganizationID != fixture.org.ID {
		t.Fatalf("binding = %#v", got)
	}
	if !got.UpdatedAt.Equal(second.UpdatedAt) {
		t.Fatalf("UpdatedAt = %s, want %s", got.UpdatedAt, second.UpdatedAt)
	}
}

func TestSessionUpsertAllowsRepeatedAgentConversation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	fixture := seedBindingFixture(t, ctx, st)
	if err := st.Sessions().SetAgentSession(ctx, fixture.agent1.ID, domain.ConversationChannel, "chn_session", "provider_1", "running"); err != nil {
		t.Fatal(err)
	}
	if err := st.Sessions().SetAgentSession(ctx, fixture.agent1.ID, domain.ConversationChannel, "chn_session", "provider_2", "done"); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceTimestampRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Date(2026, 4, 25, 10, 11, 12, 123456789, time.UTC)
	user := domain.User{ID: "usr_time", DisplayName: "Time", CreatedAt: now}
	org := domain.Organization{ID: "org_time", Name: "Time", CreatedAt: now}
	workspace := domain.Workspace{
		ID:             "wsp_time",
		OrganizationID: org.ID,
		Type:           "local",
		Name:           "Time",
		Path:           "/tmp/time",
		CreatedBy:      user.ID,
		CreatedAt:      now,
		UpdatedAt:      now.Add(time.Second),
	}
	if err := st.Users().Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := st.Organizations().Create(ctx, org); err != nil {
		t.Fatal(err)
	}
	if err := st.Workspaces().Create(ctx, workspace); err != nil {
		t.Fatal(err)
	}

	got, err := st.Workspaces().ByID(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps must be non-zero: %#v", got)
	}
	if !got.CreatedAt.Equal(workspace.CreatedAt) || !got.UpdatedAt.Equal(workspace.UpdatedAt) {
		t.Fatalf("workspace timestamps = %s/%s, want %s/%s", got.CreatedAt, got.UpdatedAt, workspace.CreatedAt, workspace.UpdatedAt)
	}
}

func TestConcurrentOpenSeparateFiles(t *testing.T) {
	const stores = 8

	ctx := context.Background()
	dir := t.TempDir()
	errs := make(chan error, stores)

	var wg sync.WaitGroup
	for i := 0; i < stores; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			suffix := strconv.Itoa(i)

			st, err := Open(ctx, filepath.Join(dir, "store_"+suffix+".db"))
			if err != nil {
				errs <- err
				return
			}
			defer st.Close()

			user := domain.User{
				ID:          "usr_concurrent_" + suffix,
				DisplayName: "Concurrent",
				CreatedAt:   time.Now().UTC(),
			}
			if err := st.Users().Create(ctx, user); err != nil {
				errs <- err
				return
			}
			if _, err := st.Users().ByID(ctx, user.ID); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	st, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

type bindingFixture struct {
	now        time.Time
	user       domain.User
	org        domain.Organization
	bot1       domain.BotUser
	bot2       domain.BotUser
	workspace1 domain.Workspace
	workspace2 domain.Workspace
	agent1     domain.Agent
	agent2     domain.Agent
}

func seedBindingFixture(t *testing.T, ctx context.Context, st *Store) bindingFixture {
	t.Helper()

	now := time.Date(2026, 4, 25, 10, 0, 0, 123456789, time.UTC)
	fixture := bindingFixture{
		now:  now,
		user: domain.User{ID: "usr_binding", DisplayName: "Binding User", CreatedAt: now},
		org:  domain.Organization{ID: "org_binding", Name: "Binding Org", CreatedAt: now},
		bot1: domain.BotUser{ID: "bot_binding_1", OrganizationID: "org_binding", DisplayName: "Bot 1", CreatedAt: now},
		bot2: domain.BotUser{ID: "bot_binding_2", OrganizationID: "org_binding", DisplayName: "Bot 2", CreatedAt: now},
		workspace1: domain.Workspace{
			ID:             "wsp_binding_1",
			OrganizationID: "org_binding",
			Type:           "local",
			Name:           "Workspace 1",
			Path:           "/tmp/workspace-1",
			CreatedBy:      "usr_binding",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		workspace2: domain.Workspace{
			ID:             "wsp_binding_2",
			OrganizationID: "org_binding",
			Type:           "local",
			Name:           "Workspace 2",
			Path:           "/tmp/workspace-2",
			CreatedBy:      "usr_binding",
			CreatedAt:      now.Add(time.Second),
			UpdatedAt:      now.Add(time.Second),
		},
	}
	fixture.agent1 = domain.Agent{
		ID:                 "agt_binding_1",
		OrganizationID:     fixture.org.ID,
		BotUserID:          fixture.bot1.ID,
		Kind:               "assistant",
		Name:               "Agent 1",
		Model:              "model-1",
		DefaultWorkspaceID: fixture.workspace1.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	fixture.agent2 = domain.Agent{
		ID:                 "agt_binding_2",
		OrganizationID:     fixture.org.ID,
		BotUserID:          fixture.bot2.ID,
		Kind:               "assistant",
		Name:               "Agent 2",
		Model:              "model-2",
		DefaultWorkspaceID: fixture.workspace2.ID,
		CreatedAt:          now.Add(time.Second),
		UpdatedAt:          now.Add(time.Second),
	}

	if err := st.Users().Create(ctx, fixture.user); err != nil {
		t.Fatal(err)
	}
	if err := st.Organizations().Create(ctx, fixture.org); err != nil {
		t.Fatal(err)
	}
	if err := st.BotUsers().Create(ctx, fixture.bot1); err != nil {
		t.Fatal(err)
	}
	if err := st.BotUsers().Create(ctx, fixture.bot2); err != nil {
		t.Fatal(err)
	}
	if err := st.Workspaces().Create(ctx, fixture.workspace1); err != nil {
		t.Fatal(err)
	}
	if err := st.Workspaces().Create(ctx, fixture.workspace2); err != nil {
		t.Fatal(err)
	}
	if err := st.Agents().Create(ctx, fixture.agent1); err != nil {
		t.Fatal(err)
	}
	if err := st.Agents().Create(ctx, fixture.agent2); err != nil {
		t.Fatal(err)
	}
	return fixture
}
