package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

func TestSendMessagePersistsUserMessageAndFakeBotReply(t *testing.T) {
	ctx := context.Background()
	app, bus, bootstrap := newConversationTestApp(t, ctx)

	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID: bootstrap.Organization.ID,
		ConversationID: bootstrap.Channel.ID,
	})
	defer unsubscribe()

	userMessage, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if userMessage.Body != "hello" {
		t.Fatalf("user message body = %q, want hello", userMessage.Body)
	}

	var sawDelta bool
	var sawBotMessage bool
	timeout := time.After(2 * time.Second)
	for !sawDelta || !sawBotMessage {
		select {
		case evt := <-events:
			switch evt.Type {
			case domain.EventAgentOutputDelta:
				payload, ok := evt.Payload.(domain.AgentOutputDeltaPayload)
				if !ok {
					t.Fatalf("delta payload type = %T, want domain.AgentOutputDeltaPayload", evt.Payload)
				}
				if payload.Text != "Echo: hello" {
					t.Fatalf("delta text = %q, want Echo: hello", payload.Text)
				}
				sawDelta = true
			case domain.EventMessageCreated:
				payload, ok := evt.Payload.(domain.MessageCreatedPayload)
				if !ok {
					t.Fatalf("message payload type = %T, want domain.MessageCreatedPayload", evt.Payload)
				}
				if payload.Message.SenderType == domain.SenderBot && payload.Message.Body == "Echo: hello" {
					sawBotMessage = true
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for fake agent events")
		}
	}

	messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2: %#v", len(messages), messages)
	}
	if messages[0].Body != "hello" || messages[0].SenderType != domain.SenderUser {
		t.Fatalf("first message = %#v", messages[0])
	}
	if messages[1].Body != "Echo: hello" || messages[1].SenderType != domain.SenderBot {
		t.Fatalf("second message = %#v", messages[1])
	}
}

func TestSendMessageRejectsEmptyBody(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	_, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             " \n\t ",
	})
	if !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("error = %v, want %v", err, ErrEmptyMessage)
	}
}

func TestListOrganizationsAndChannels(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	orgs, err := app.ListOrganizations(ctx, bootstrap.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 || orgs[0].ID != bootstrap.Organization.ID {
		t.Fatalf("organizations = %#v", orgs)
	}

	channels, err := app.ListChannels(ctx, bootstrap.Organization.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0].ID != bootstrap.Channel.ID {
		t.Fatalf("channels = %#v", channels)
	}
}

func newConversationTestApp(t *testing.T, ctx context.Context) (*App, *eventbus.Bus, BootstrapResult) {
	t.Helper()

	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	})

	bus := eventbus.New()
	app := New(st, bus, Options{AdminToken: "secret", DataDir: t.TempDir()})
	bootstrap, err := app.Bootstrap(ctx, BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}
	return app, bus, bootstrap
}
