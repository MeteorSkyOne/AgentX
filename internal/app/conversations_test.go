package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/store"
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

func TestAgentRunStreamsAndPersistsProcessMetadata(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	bus := eventbus.New()
	app := New(st, bus, Options{
		AdminToken: "secret",
		DataDir:    t.TempDir(),
		Runtimes: map[string]agentruntime.Runtime{
			domain.AgentKindFake: scriptedRuntime{events: []agentruntime.Event{
				{
					Type:     agentruntime.EventDelta,
					Thinking: "checking workspace",
					Process: []agentruntime.ProcessItem{{
						Type: "thinking",
						Text: "checking workspace",
						Raw:  map[string]any{"type": "thinking"},
					}},
				},
				{
					Type: agentruntime.EventDelta,
					Process: []agentruntime.ProcessItem{{
						Type:       "tool_call",
						ToolName:   "Read",
						ToolCallID: "call_1",
						Input:      map[string]any{"path": "README.md"},
						Raw:        map[string]any{"type": "tool_use"},
					}},
				},
				{Type: agentruntime.EventDelta, Text: "done"},
				{Type: agentruntime.EventCompleted, Text: "done"},
			}},
		},
	})
	bootstrap, err := app.Bootstrap(ctx, BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID: bootstrap.Organization.ID,
		ConversationID: bootstrap.Channel.ID,
	})
	defer unsubscribe()

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "inspect",
	}); err != nil {
		t.Fatal(err)
	}

	var sawProcessDelta bool
	var sawBotMessage bool
	timeout := time.After(2 * time.Second)
	for !sawProcessDelta || !sawBotMessage {
		select {
		case evt := <-events:
			switch evt.Type {
			case domain.EventAgentOutputDelta:
				payload, ok := evt.Payload.(domain.AgentOutputDeltaPayload)
				if !ok {
					t.Fatalf("delta payload type = %T, want domain.AgentOutputDeltaPayload", evt.Payload)
				}
				if len(payload.Process) > 0 && payload.Process[0].Type == "tool_call" {
					if payload.Process[0].ToolName != "Read" {
						t.Fatalf("process delta = %#v", payload.Process[0])
					}
					sawProcessDelta = true
				}
			case domain.EventMessageCreated:
				payload, ok := evt.Payload.(domain.MessageCreatedPayload)
				if !ok {
					t.Fatalf("message payload type = %T, want domain.MessageCreatedPayload", evt.Payload)
				}
				if payload.Message.SenderType == domain.SenderBot && payload.Message.Body == "done" {
					sawBotMessage = true
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for process events")
		}
	}

	messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var botMessage domain.Message
	for _, message := range messages {
		if message.SenderType == domain.SenderBot {
			botMessage = message
			break
		}
	}
	if botMessage.ID == "" {
		t.Fatalf("messages = %#v", messages)
	}
	if botMessage.Metadata["thinking"] != "checking workspace" {
		t.Fatalf("metadata = %#v", botMessage.Metadata)
	}
	process, ok := botMessage.Metadata["process"].([]any)
	if !ok || len(process) != 2 {
		t.Fatalf("process metadata = %#v", botMessage.Metadata["process"])
	}
	toolCall, ok := process[1].(map[string]any)
	if !ok || toolCall["type"] != "tool_call" || toolCall["tool_name"] != "Read" {
		t.Fatalf("tool process metadata = %#v", process[1])
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

func TestSendMessageDispatchesAllAgentsOrOnlyMentionedHandles(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	second, err := app.CreateAgent(ctx, AgentCreateRequest{
		UserID:         bootstrap.User.ID,
		OrganizationID: bootstrap.Organization.ID,
		Name:           "Agent Two",
		Handle:         "agent_two",
		Kind:           domain.AgentKindFake,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetChannelAgents(ctx, bootstrap.Channel.ID, []domain.ChannelAgent{
		{AgentID: bootstrap.Agent.ID},
		{AgentID: second.ID},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "ping all",
	}); err != nil {
		t.Fatal(err)
	}
	requireEventuallyApp(t, time.Second, func() bool {
		messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 20)
		if err != nil {
			t.Fatal(err)
		}
		return countBotMessagesFrom(messages, "Echo: ping all", bootstrap.Agent.BotUserID, second.BotUserID) == 2
	})

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "@agent_two ping",
	}); err != nil {
		t.Fatal(err)
	}
	requireEventuallyApp(t, time.Second, func() bool {
		messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 50)
		if err != nil {
			t.Fatal(err)
		}
		var secondReplies int
		var firstReplies int
		for _, message := range messages {
			if message.Body != "Echo: @agent_two ping" || message.SenderType != domain.SenderBot {
				continue
			}
			if message.SenderID == second.BotUserID {
				secondReplies++
			}
			if message.SenderID == bootstrap.Agent.BotUserID {
				firstReplies++
			}
		}
		return secondReplies == 1 && firstReplies == 0
	})
}

func TestDirectedMessageIsIncludedInLaterAgentContext(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	capture := &capturingRuntime{sends: make(chan capturedInput, 8)}
	app := New(st, eventbus.New(), Options{
		AdminToken:       "secret",
		DataDir:          t.TempDir(),
		DefaultAgentKind: "capture",
		Runtimes: map[string]agentruntime.Runtime{
			"capture": capture,
		},
	})
	bootstrap, err := app.Bootstrap(ctx, BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := app.CreateAgent(ctx, AgentCreateRequest{
		UserID:         bootstrap.User.ID,
		OrganizationID: bootstrap.Organization.ID,
		Name:           "Agent Two",
		Handle:         "agent_two",
		Kind:           "capture",
		YoloMode:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetChannelAgents(ctx, bootstrap.Channel.ID, []domain.ChannelAgent{
		{AgentID: bootstrap.Agent.ID},
		{AgentID: second.ID},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "@agent_two directed",
	}); err != nil {
		t.Fatal(err)
	}
	directed := readCapturedInput(t, capture.sends)
	if directed.agentID != second.ID {
		t.Fatalf("directed agentID = %q, want %q", directed.agentID, second.ID)
	}
	if !directed.yoloMode {
		t.Fatal("directed yoloMode = false, want true")
	}
	select {
	case extra := <-capture.sends:
		t.Fatalf("unexpected extra directed run for agent %q", extra.agentID)
	case <-time.After(50 * time.Millisecond):
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "what did you see?",
	}); err != nil {
		t.Fatal(err)
	}

	inputs := []capturedInput{readCapturedInput(t, capture.sends), readCapturedInput(t, capture.sends)}
	var firstAgentContext string
	for _, input := range inputs {
		if input.agentID == bootstrap.Agent.ID {
			firstAgentContext = input.input.Context
		}
	}
	if !strings.Contains(firstAgentContext, "@agent_two directed") {
		t.Fatalf("first agent context = %q, want directed message", firstAgentContext)
	}
}

func TestSlashCommandRequiresTargetWhenConversationHasMultipleAgents(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	second, err := app.CreateAgent(ctx, AgentCreateRequest{
		UserID:         bootstrap.User.ID,
		OrganizationID: bootstrap.Organization.ID,
		Name:           "Agent Two",
		Handle:         "agent_two",
		Kind:           domain.AgentKindFake,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetChannelAgents(ctx, bootstrap.Channel.ID, []domain.ChannelAgent{
		{AgentID: bootstrap.Agent.ID},
		{AgentID: second.ID},
	}); err != nil {
		t.Fatal(err)
	}

	_, err = app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/new",
	})
	if !IsCommandInputError(err) {
		t.Fatalf("error = %v, want command input error", err)
	}
}

func TestSlashModelAndEffortPersistAgentConfig(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/model gpt-5.2",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/effort high",
	}); err != nil {
		t.Fatal(err)
	}

	agent, err := app.Agent(ctx, bootstrap.Agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Model != "gpt-5.2" || agent.Effort != "high" {
		t.Fatalf("agent model/effort = %q/%q, want gpt-5.2/high", agent.Model, agent.Effort)
	}
}

func TestSlashNewResetsProviderSessionAndFiltersOldContext(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	capture := &capturingRuntime{sends: make(chan capturedInput, 4)}
	app := New(st, eventbus.New(), Options{
		AdminToken:       "secret",
		DataDir:          t.TempDir(),
		DefaultAgentKind: "capture",
		Runtimes: map[string]agentruntime.Runtime{
			"capture": capture,
		},
	})
	bootstrap, err := app.Bootstrap(ctx, BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "old context",
	}); err != nil {
		t.Fatal(err)
	}
	first := readCapturedInput(t, capture.sends)
	requireEventuallyApp(t, time.Second, func() bool {
		session, err := st.Sessions().ByConversation(ctx, bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID)
		if err != nil {
			return false
		}
		return session.ProviderSessionID != ""
	})
	if first.previousSessionID != "" {
		t.Fatalf("first previous session = %q, want empty", first.previousSessionID)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/new",
	}); err != nil {
		t.Fatal(err)
	}

	session, err := st.Sessions().ByConversation(ctx, bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ProviderSessionID != "" || session.ContextStartedAt == nil {
		t.Fatalf("session after /new = %#v, want empty provider and boundary", session)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "after reset",
	}); err != nil {
		t.Fatal(err)
	}
	second := readCapturedInput(t, capture.sends)
	if second.previousSessionID != "" {
		t.Fatalf("second previous session = %q, want empty after /new", second.previousSessionID)
	}
	if strings.Contains(second.input.Context, "old context") {
		t.Fatalf("context after /new = %q, want old context filtered", second.input.Context)
	}
}

func TestSlashPlanUsesClaudePlanPermissionAndStripsTarget(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	capture := &capturingRuntime{sends: make(chan capturedInput, 4)}
	app := New(st, eventbus.New(), Options{
		AdminToken:       "secret",
		DataDir:          t.TempDir(),
		DefaultAgentKind: domain.AgentKindClaude,
		Runtimes: map[string]agentruntime.Runtime{
			domain.AgentKindClaude: capture,
		},
	})
	bootstrap, err := app.Bootstrap(ctx, BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/plan @claude-default build the feature",
	}); err != nil {
		t.Fatal(err)
	}
	input := readCapturedInput(t, capture.sends)
	if input.permissionMode != "plan" {
		t.Fatalf("permission mode = %q, want plan", input.permissionMode)
	}
	if strings.Contains(input.input.Prompt, "@claude-default") || !strings.Contains(input.input.Prompt, "build the feature") {
		t.Fatalf("prompt = %q, want stripped target and task", input.input.Prompt)
	}
}

func TestSlashCompactUnsupportedForCodexWritesSystemMessage(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	agent, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{Kind: stringPtr(domain.AgentKindCodex)})
	if err != nil {
		t.Fatal(err)
	}

	message, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/compact",
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.SenderType != domain.SenderSystem || !strings.Contains(message.Body, "not supported") || !strings.Contains(message.Body, agent.Handle) {
		t.Fatalf("compact message = %#v", message)
	}
}

func TestUpdateThreadTitlePreservesCatalogOrder(t *testing.T) {
	ctx := context.Background()
	application, _, bootstrap := newConversationTestApp(t, ctx)

	forum, err := application.CreateChannel(ctx, bootstrap.Project.ID, "forum", domain.ChannelTypeThread)
	if err != nil {
		t.Fatal(err)
	}
	older, _, err := application.CreateThread(ctx, bootstrap.User.ID, forum.ID, "older title", "older body")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	newer, _, err := application.CreateThread(ctx, bootstrap.User.ID, forum.ID, "newer title", "newer body")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := application.UpdateThread(ctx, older.ID, "renamed older title")
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UpdatedAt.Equal(older.UpdatedAt) {
		t.Fatalf("updated_at = %s, want preserved %s", updated.UpdatedAt, older.UpdatedAt)
	}

	threads, err := application.ListThreads(ctx, forum.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) < 2 || threads[0].ID != newer.ID || threads[1].ID != older.ID {
		t.Fatalf("threads order = %#v, want newer then older after rename", threads)
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

func countBotMessagesFrom(messages []domain.Message, body string, senderIDs ...string) int {
	wanted := make(map[string]bool, len(senderIDs))
	for _, senderID := range senderIDs {
		wanted[senderID] = true
	}
	var count int
	for _, message := range messages {
		if message.SenderType == domain.SenderBot && message.Body == body && wanted[message.SenderID] {
			count++
		}
	}
	return count
}

func requireEventuallyApp(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if check() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("condition not met before timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunAgentRecordsFailedSessionAfterBindingWhenAgentLookupFails(t *testing.T) {
	ctx := context.Background()
	baseApp, bus, bootstrap := newConversationTestApp(t, ctx)
	sessionStore := &recordingSessionStore{}
	app := New(&agentLookupFailureStore{
		Store:    baseApp.store,
		sessions: sessionStore,
		err:      errors.New("agent lookup failed"),
	}, bus, baseApp.opts)

	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID: bootstrap.Organization.ID,
		ConversationID: bootstrap.Channel.ID,
	})
	defer unsubscribe()

	app.runAgentForMessage(ctx, domain.Message{
		ID:               "msg_test",
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		SenderType:       domain.SenderUser,
		SenderID:         bootstrap.User.ID,
		Kind:             domain.MessageText,
		Body:             "hello",
		CreatedAt:        time.Now().UTC(),
	})

	if sessionStore.status != "failed" {
		t.Fatalf("session status = %q, want failed", sessionStore.status)
	}
	if sessionStore.agentID != bootstrap.Agent.ID {
		t.Fatalf("session agentID = %q, want %q", sessionStore.agentID, bootstrap.Agent.ID)
	}
	if sessionStore.conversationID != bootstrap.Channel.ID {
		t.Fatalf("session conversationID = %q, want %q", sessionStore.conversationID, bootstrap.Channel.ID)
	}

	select {
	case evt := <-events:
		if evt.Type != domain.EventAgentRunFailed {
			t.Fatalf("event type = %s, want %s", evt.Type, domain.EventAgentRunFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for AgentRunFailed")
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

type agentLookupFailureStore struct {
	store.Store
	sessions *recordingSessionStore
	err      error
}

func (s *agentLookupFailureStore) Agents() store.AgentStore {
	return failingAgentStore{err: s.err}
}

func (s *agentLookupFailureStore) Sessions() store.SessionStore {
	return s.sessions
}

type failingAgentStore struct {
	err error
}

func (s failingAgentStore) Create(ctx context.Context, agent domain.Agent) error {
	return s.err
}

func (s failingAgentStore) ByID(ctx context.Context, id string) (domain.Agent, error) {
	return domain.Agent{}, s.err
}

func (s failingAgentStore) DefaultForOrganization(ctx context.Context, orgID string) (domain.Agent, error) {
	return domain.Agent{}, s.err
}

func (s failingAgentStore) ListByOrganization(ctx context.Context, orgID string) ([]domain.Agent, error) {
	return nil, s.err
}

func (s failingAgentStore) ByHandle(ctx context.Context, orgID string, handle string) (domain.Agent, error) {
	return domain.Agent{}, s.err
}

func (s failingAgentStore) Update(ctx context.Context, agent domain.Agent) error {
	return s.err
}

type recordingSessionStore struct {
	agentID          string
	conversationType domain.ConversationType
	conversationID   string
	providerID       string
	status           string
}

func (s *recordingSessionStore) SetAgentSession(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, providerSessionID string, status string) error {
	s.agentID = agentID
	s.conversationType = conversationType
	s.conversationID = conversationID
	s.providerID = providerSessionID
	s.status = status
	return nil
}

func (s *recordingSessionStore) ResetAgentSessionContext(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, contextStartedAt time.Time) error {
	s.agentID = agentID
	s.conversationType = conversationType
	s.conversationID = conversationID
	s.providerID = ""
	s.status = "reset"
	return nil
}

func (s *recordingSessionStore) SetAgentSessionContextStartedAt(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string, contextStartedAt time.Time) error {
	s.agentID = agentID
	s.conversationType = conversationType
	s.conversationID = conversationID
	return nil
}

func (s *recordingSessionStore) ByConversation(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string) (domain.AgentSession, error) {
	return domain.AgentSession{}, sql.ErrNoRows
}

type scriptedRuntime struct {
	events []agentruntime.Event
}

func (r scriptedRuntime) StartSession(ctx context.Context, req agentruntime.StartSessionRequest) (agentruntime.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &scriptedSession{
		id:     "scripted:" + req.SessionKey,
		script: r.events,
		events: make(chan agentruntime.Event, len(r.events)),
	}, nil
}

type scriptedSession struct {
	id     string
	script []agentruntime.Event
	events chan agentruntime.Event
}

func (s *scriptedSession) Send(ctx context.Context, input agentruntime.Input) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, evt := range s.script {
		s.events <- evt
	}
	return nil
}

func (s *scriptedSession) Events() <-chan agentruntime.Event {
	return s.events
}

func (s *scriptedSession) CurrentSessionID() string {
	return s.id
}

func (s *scriptedSession) Alive() bool {
	return true
}

func (s *scriptedSession) Close(ctx context.Context) error {
	close(s.events)
	return nil
}

type capturedInput struct {
	agentID           string
	yoloMode          bool
	effort            string
	permissionMode    string
	previousSessionID string
	input             agentruntime.Input
}

type capturingRuntime struct {
	sends chan capturedInput
}

func (r *capturingRuntime) StartSession(ctx context.Context, req agentruntime.StartSessionRequest) (agentruntime.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &capturingSession{
		agentID:           req.AgentID,
		yoloMode:          req.YoloMode,
		effort:            req.Effort,
		permissionMode:    req.PermissionMode,
		previousSessionID: req.PreviousSessionID,
		id:                "capture:" + req.SessionKey,
		sends:             r.sends,
		events:            make(chan agentruntime.Event, 1),
	}, nil
}

type capturingSession struct {
	agentID           string
	yoloMode          bool
	effort            string
	permissionMode    string
	previousSessionID string
	id                string
	sends             chan capturedInput
	events            chan agentruntime.Event
}

func (s *capturingSession) Send(ctx context.Context, input agentruntime.Input) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.sends <- capturedInput{
		agentID:           s.agentID,
		yoloMode:          s.yoloMode,
		effort:            s.effort,
		permissionMode:    s.permissionMode,
		previousSessionID: s.previousSessionID,
		input:             input,
	}
	s.events <- agentruntime.Event{Type: agentruntime.EventCompleted, Text: "capture ok"}
	return nil
}

func (s *capturingSession) Events() <-chan agentruntime.Event {
	return s.events
}

func stringPtr(value string) *string {
	return &value
}

func (s *capturingSession) CurrentSessionID() string {
	return s.id
}

func (s *capturingSession) Alive() bool {
	return true
}

func (s *capturingSession) Close(ctx context.Context) error {
	close(s.events)
	return nil
}

func readCapturedInput(t *testing.T, sends <-chan capturedInput) capturedInput {
	t.Helper()

	select {
	case input := <-sends:
		return input
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for captured runtime input")
		return capturedInput{}
	}
}
