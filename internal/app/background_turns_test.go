package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

// backgroundTurnRuntime stands in for a runtime whose agent starts turns on
// its own: offer plays the runtime claiming such a turn.
type backgroundTurnRuntime struct {
	mu      sync.Mutex
	handler func(sessionKey string) bool
	claimed bool
	events  []agentruntime.Event
}

func (r *backgroundTurnRuntime) SetBackgroundTurnHandler(handler func(sessionKey string) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handler = handler
}

func (r *backgroundTurnRuntime) offer(sessionKey string, claim bool) bool {
	r.mu.Lock()
	handler := r.handler
	r.claimed = claim
	r.mu.Unlock()
	return handler != nil && handler(sessionKey)
}

func (r *backgroundTurnRuntime) StartSession(ctx context.Context, req agentruntime.StartSessionRequest) (agentruntime.Session, error) {
	return &backgroundTurnSession{
		scriptedSession: &scriptedSession{id: "bg:" + req.SessionKey, script: r.events, events: make(chan agentruntime.Event, len(r.events))},
		rt:              r,
	}, nil
}

type backgroundTurnSession struct {
	*scriptedSession
	rt *backgroundTurnRuntime
}

func (s *backgroundTurnSession) AdoptBackgroundTurn(ctx context.Context) error {
	s.rt.mu.Lock()
	claimed := s.rt.claimed
	s.rt.claimed = false
	s.rt.mu.Unlock()
	if !claimed {
		return errors.New("no claimed turn")
	}
	for _, evt := range s.script {
		s.events <- evt
	}
	return nil
}

func newBackgroundTurnApp(t *testing.T, rt *backgroundTurnRuntime) (*App, *eventbus.Bus, BootstrapResult) {
	t.Helper()
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	bus := eventbus.New()
	app := New(st, bus, Options{
		AdminToken: "secret",
		DataDir:    t.TempDir(),
		Runtimes:   map[string]agentruntime.Runtime{domain.AgentKindFake: rt},
	})
	t.Cleanup(func() { shutdownAppForTest(t, app) })
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	return app, bus, bootstrap
}

func TestBackgroundTurnBecomesItsOwnReply(t *testing.T) {
	rt := &backgroundTurnRuntime{events: []agentruntime.Event{
		{Type: agentruntime.EventDelta, Text: "the build "},
		{Type: agentruntime.EventCompleted, Text: "the build finished"},
	}}
	app, bus, bootstrap := newBackgroundTurnApp(t, rt)
	ctx := context.Background()
	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID: bootstrap.Organization.ID,
		ConversationID: bootstrap.Channel.ID,
	})
	defer unsubscribe()

	key := agentSessionKey(bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID)
	if !rt.offer(key, true) {
		t.Fatal("app declined the background turn")
	}

	var started bool
	timeout := time.After(3 * time.Second)
	for {
		select {
		case evt := <-events:
			switch evt.Type {
			case domain.EventAgentRunStarted:
				started = true
			case domain.EventMessageCreated:
				message := evt.Payload.(domain.MessageCreatedPayload).Message
				if message.SenderType != domain.SenderBot || message.Body != "the build finished" {
					t.Fatalf("unexpected message %#v", message)
				}
				if !started {
					t.Fatal("reply landed without a run start")
				}
				stored, err := app.store.Messages().ByID(ctx, message.ID)
				if err != nil || stored.Body != "the build finished" {
					t.Fatalf("reply was not stored: %#v %v", stored, err)
				}
				return
			}
		case <-timeout:
			t.Fatal("background turn never produced a reply")
		}
	}
}

func TestLapsedBackgroundTurnLeavesNoTrace(t *testing.T) {
	rt := &backgroundTurnRuntime{events: []agentruntime.Event{{Type: agentruntime.EventCompleted, Text: "never"}}}
	app, bus, bootstrap := newBackgroundTurnApp(t, rt)
	ctx := context.Background()
	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID: bootstrap.Organization.ID,
		ConversationID: bootstrap.Channel.ID,
	})
	defer unsubscribe()

	key := agentSessionKey(bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID)
	if !rt.offer(key, false) {
		t.Fatal("app declined the background turn")
	}
	activeKey := activeRunKey{conversationType: domain.ConversationChannel, conversationID: bootstrap.Channel.ID, agentID: bootstrap.Agent.ID}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case evt := <-events:
			t.Fatalf("a lapsed background turn must not publish anything, got %s", evt.Type)
		case <-deadline:
			t.Fatal("lapsed background turn kept its run active")
		case <-time.After(50 * time.Millisecond):
			if !app.hasActiveAgentRun(activeKey) {
				return
			}
		}
	}
}

func TestBackgroundTurnRejectsMalformedSessionKey(t *testing.T) {
	rt := &backgroundTurnRuntime{}
	newBackgroundTurnApp(t, rt)
	for _, key := range []string{"", "agent-only", "agent:channel", "agent::conv"} {
		if rt.offer(key, true) {
			t.Errorf("key %q must be declined", key)
		}
	}
}
