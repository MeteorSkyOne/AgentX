package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

func TestRetryAgentRunReplacesLastReply(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "hello",
	}); err != nil {
		t.Fatal(err)
	}

	key := activeRunKey{
		conversationType: domain.ConversationChannel,
		conversationID:   bootstrap.Channel.ID,
		agentID:          bootstrap.Agent.ID,
	}
	var firstBotID string
	requireEventuallyApp(t, 2*time.Second, func() bool {
		messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		bot := findMessageFromSender(messages, bootstrap.Agent.BotUserID, "Echo: hello")
		if bot == nil || app.hasActiveAgentRun(key) {
			return false
		}
		firstBotID = bot.ID
		return true
	})

	if err := app.RetryAgentRun(ctx, domain.ConversationChannel, bootstrap.Channel.ID, bootstrap.Agent.ID); err != nil {
		t.Fatalf("RetryAgentRun: %v", err)
	}

	// The stale reply is removed and replaced by a fresh one (different ID),
	// leaving exactly one user message and one bot reply.
	requireEventuallyApp(t, 2*time.Second, func() bool {
		messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if countBotMessagesFrom(messages, "Echo: hello", bootstrap.Agent.BotUserID) != 1 {
			return false
		}
		bot := findMessageFromSender(messages, bootstrap.Agent.BotUserID, "Echo: hello")
		return bot != nil && bot.ID != firstBotID && !app.hasActiveAgentRun(key)
	})
}

func TestRetryAgentRunWithoutUserMessage(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	err := app.RetryAgentRun(ctx, domain.ConversationChannel, bootstrap.Channel.ID, bootstrap.Agent.ID)
	if !errors.Is(err, ErrNoMessageToRetry) {
		t.Fatalf("RetryAgentRun error = %v, want ErrNoMessageToRetry", err)
	}
}

func TestRetryAgentRunUnknownAgent(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	err := app.RetryAgentRun(ctx, domain.ConversationChannel, bootstrap.Channel.ID, "agt_does_not_exist")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("RetryAgentRun error = %v, want ErrInvalidInput", err)
	}
}
