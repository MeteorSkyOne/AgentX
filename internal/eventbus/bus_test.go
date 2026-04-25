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
