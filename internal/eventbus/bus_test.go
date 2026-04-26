package eventbus

import (
	"context"
	"runtime"
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

func TestPublishSkipsDifferentConversationType(t *testing.T) {
	bus := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsubscribe := bus.Subscribe(ctx, Filter{
		OrganizationID:   "org_1",
		ConversationType: domain.ConversationChannel,
		ConversationID:   "conv_1",
	})
	defer unsubscribe()

	bus.Publish(domain.Event{
		Type:             domain.EventMessageCreated,
		OrganizationID:   "org_1",
		ConversationType: domain.ConversationThread,
		ConversationID:   "conv_1",
	})

	select {
	case got := <-ch:
		t.Fatalf("unexpected event: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestUnsubscribeStopsWatcherBeforeContextCancel(t *testing.T) {
	bus := New()
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsubscribe := bus.Subscribe(ctx, Filter{OrganizationID: "org_1", ConversationID: "ch_1"})
	unsubscribe()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("subscription channel is open after unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription channel to close")
	}

	bus.Publish(domain.Event{
		Type:           domain.EventMessageCreated,
		OrganizationID: "org_1",
		ConversationID: "ch_1",
	})

	select {
	case got, ok := <-ch:
		if ok {
			t.Fatalf("unexpected event after unsubscribe: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unsubscribed channel to remain closed")
	}

	if !eventuallyGoroutinesAtMost(before) {
		cancel()
		t.Fatalf("unsubscribe watcher still running before context cancel")
	}

	cancel()
}

func eventuallyGoroutinesAtMost(max int) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		runtime.Gosched()
		if runtime.NumGoroutine() <= max {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return runtime.NumGoroutine() <= max
}
