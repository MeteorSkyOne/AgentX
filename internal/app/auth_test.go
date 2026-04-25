package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/store"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
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

func TestBootstrapRejectsEmptyConfiguredAdminToken(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "", DataDir: t.TempDir()})
	_, err = app.Bootstrap(ctx, BootstrapRequest{AdminToken: "", DisplayName: "Bad"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestBootstrapRejectsRepeatedBootstrap(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})
	if _, err := app.Bootstrap(ctx, BootstrapRequest{AdminToken: "secret", DisplayName: "First"}); err != nil {
		t.Fatal(err)
	}
	_, err = app.Bootstrap(ctx, BootstrapRequest{AdminToken: "secret", DisplayName: "Second"})
	if !errors.Is(err, ErrAlreadyBootstrapped) {
		t.Fatalf("error = %v, want %v", err, ErrAlreadyBootstrapped)
	}
}

func TestUserForTokenReturnsBootstrappedUser(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := New(st, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})
	result, err := app.Bootstrap(ctx, BootstrapRequest{AdminToken: "secret", DisplayName: "Meteorsky"})
	if err != nil {
		t.Fatal(err)
	}

	user, err := app.UserForToken(ctx, result.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != result.User.ID || user.DisplayName != result.User.DisplayName {
		t.Fatalf("user = %#v, want %#v", user, result.User)
	}
}

func TestBootstrapRejectsConcurrentBootstrapAttempts(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const attempts = 8
	gatedStore := newConcurrentAnyStore(st, attempts)
	app := New(gatedStore, eventbus.New(), Options{AdminToken: "secret", DataDir: t.TempDir()})

	var wg sync.WaitGroup
	type bootstrapAttempt struct {
		result BootstrapResult
		err    error
	}
	results := make([]bootstrapAttempt, attempts)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := app.Bootstrap(ctx, BootstrapRequest{AdminToken: "secret", DisplayName: "Meteorsky"})
			results[i] = bootstrapAttempt{result: result, err: err}
		}(i)
	}
	wg.Wait()

	successes := 0
	var successfulUserID string
	for _, result := range results {
		switch {
		case result.err == nil:
			successes++
			successfulUserID = result.result.User.ID
		case errors.Is(result.err, ErrAlreadyBootstrapped):
		default:
			t.Fatalf("unexpected error: %v", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want 1; results = %#v", successes, results)
	}

	orgs, err := st.Organizations().ListForUser(ctx, successfulUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 {
		t.Fatalf("organizations for successful user = %#v, want exactly one", orgs)
	}
}

type concurrentAnyStore struct {
	store.Store
	waitFor int
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	count   int
	any     bool
	err     error
}

func newConcurrentAnyStore(st store.Store, waitFor int) *concurrentAnyStore {
	return &concurrentAnyStore{
		Store:   st,
		waitFor: waitFor,
		release: make(chan struct{}),
	}
}

func (s *concurrentAnyStore) Organizations() store.OrganizationStore {
	return concurrentAnyOrganizationStore{
		OrganizationStore: s.Store.Organizations(),
		parent:            s,
	}
}

func (s *concurrentAnyStore) waitUntilAllReady(ctx context.Context) (bool, error) {
	s.mu.Lock()
	s.count++
	if s.count == s.waitFor {
		s.any, s.err = s.Store.Organizations().Any(ctx)
		s.once.Do(func() {
			close(s.release)
		})
	}
	s.mu.Unlock()
	<-s.release
	return s.any, s.err
}

type concurrentAnyOrganizationStore struct {
	store.OrganizationStore
	parent *concurrentAnyStore
}

func (s concurrentAnyOrganizationStore) Any(ctx context.Context) (bool, error) {
	return s.parent.waitUntilAllReady(ctx)
}

var _ store.OrganizationStore = concurrentAnyOrganizationStore{}
var _ store.Store = (*concurrentAnyStore)(nil)
