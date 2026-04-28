package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestSendReplyMessageResolvesReferenceAndDeletedPlaceholder(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	original, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "original message",
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "reply message",
		ReplyToMessageID: original.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.ReplyToMessageID != original.ID {
		t.Fatalf("reply_to_message_id = %q, want %q", reply.ReplyToMessageID, original.ID)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.Deleted || reply.ReplyTo.Body != original.Body || reply.ReplyTo.SenderType != domain.SenderUser {
		t.Fatalf("reply reference = %#v, want live original", reply.ReplyTo)
	}

	if err := app.DeleteMessage(ctx, original.ID); err != nil {
		t.Fatal(err)
	}
	messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if message.ID == reply.ID {
			if message.ReplyTo == nil || !message.ReplyTo.Deleted || message.ReplyTo.MessageID != original.ID {
				t.Fatalf("reply reference after delete = %#v, want deleted placeholder", message.ReplyTo)
			}
			return
		}
	}
	t.Fatalf("reply message %s missing from %#v", reply.ID, messages)
}

func TestSendReplyMessageRejectsInvalidReference(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "reply",
		ReplyToMessageID: "msg_missing",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing reference error = %v, want ErrInvalidInput", err)
	}

	otherChannel, err := app.CreateChannel(ctx, bootstrap.Project.ID, "other", domain.ChannelTypeText)
	if err != nil {
		t.Fatal(err)
	}
	otherMessage := domain.Message{
		ID:               "msg_other_channel",
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   otherChannel.ID,
		SenderType:       domain.SenderUser,
		SenderID:         bootstrap.User.ID,
		Kind:             domain.MessageText,
		Body:             "other",
		CreatedAt:        time.Now().UTC(),
	}
	if err := app.store.Messages().Create(ctx, otherMessage); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "reply",
		ReplyToMessageID: otherMessage.ID,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-conversation reference error = %v, want ErrInvalidInput", err)
	}
}

func TestSendAttachmentOnlyMessagePersistsAndPassesFilesToRuntime(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	capture := &capturingRuntime{sends: make(chan capturedInput, 2)}
	application := New(st, eventbus.New(), Options{
		AdminToken:       "secret",
		DataDir:          t.TempDir(),
		DefaultAgentKind: "capture",
		Runtimes: map[string]agentruntime.Runtime{
			"capture": capture,
		},
	})
	bootstrap, err := application.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}

	message, err := application.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Attachments: []AttachmentUpload{{
			Filename:    "notes.txt",
			ContentType: "text/plain",
			Data:        []byte("important context\n"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.Body != "" || len(message.Attachments) != 1 {
		t.Fatalf("message = %#v, want attachment-only message", message)
	}

	captured := readCapturedInput(t, capture.sends)
	if len(captured.input.Attachments) != 1 {
		t.Fatalf("runtime attachments = %#v, want one", captured.input.Attachments)
	}
	attachment := captured.input.Attachments[0]
	if attachment.Filename != "notes.txt" || attachment.Kind != string(domain.MessageAttachmentText) || attachment.LocalPath == "" {
		t.Fatalf("runtime attachment = %#v", attachment)
	}
	if _, err := os.Stat(attachment.LocalPath); err != nil {
		t.Fatalf("runtime attachment path missing: %v", err)
	}
	if rendered := captured.input.RenderedPrompt(); !strings.Contains(rendered, "notes.txt") || !strings.Contains(rendered, attachment.LocalPath) {
		t.Fatalf("rendered prompt = %q, want attachment filename and path", rendered)
	}

	if err := application.DeleteMessage(ctx, message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(attachment.LocalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attachment file after delete stat err = %v, want not exist", err)
	}
}

func TestSendMessageRejectsInvalidAttachmentsAndSlashCommandAttachments(t *testing.T) {
	ctx := context.Background()
	application, _, bootstrap := newConversationTestApp(t, ctx)

	if _, err := application.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "binary",
		Attachments: []AttachmentUpload{{
			Filename:    "blob.bin",
			ContentType: "application/octet-stream",
			Data:        []byte{0x00, 0x01, 0x02},
		}},
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("binary attachment error = %v, want ErrInvalidInput", err)
	}

	if _, err := application.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/effort high",
		Attachments: []AttachmentUpload{{
			Filename:    "notes.txt",
			ContentType: "text/plain",
			Data:        []byte("text"),
		}},
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("slash attachment error = %v, want ErrInvalidInput", err)
	}
}

func TestDeleteMessagePublishesEventWhenAttachmentFileCleanupFails(t *testing.T) {
	ctx := context.Background()
	application, bus, bootstrap := newConversationTestApp(t, ctx)

	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
	})
	defer unsubscribe()

	now := time.Now().UTC()
	message := domain.Message{
		ID:               "msg_cleanup_failure",
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		SenderType:       domain.SenderUser,
		SenderID:         bootstrap.User.ID,
		Kind:             domain.MessageText,
		Body:             "delete me",
		CreatedAt:        now,
	}
	nonEmptyDir := filepath.Join(t.TempDir(), "stored-attachment")
	if err := os.MkdirAll(nonEmptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "child"), []byte("keep directory non-empty"), 0o600); err != nil {
		t.Fatal(err)
	}
	attachment := domain.MessageAttachment{
		ID:               "att_cleanup_failure",
		MessageID:        message.ID,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Filename:         "broken.txt",
		ContentType:      "text/plain",
		Kind:             domain.MessageAttachmentText,
		SizeBytes:        4,
		StoragePath:      nonEmptyDir,
		CreatedAt:        now,
	}
	if err := application.store.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Messages().Create(ctx, message); err != nil {
			return err
		}
		return tx.MessageAttachments().Create(ctx, attachment)
	}); err != nil {
		t.Fatal(err)
	}

	if err := application.DeleteMessage(ctx, message.ID); err != nil {
		t.Fatalf("DeleteMessage returned cleanup error: %v", err)
	}
	select {
	case evt := <-events:
		if evt.Type != domain.EventMessageDeleted {
			t.Fatalf("event type = %s, want %s", evt.Type, domain.EventMessageDeleted)
		}
		payload := evt.Payload.(domain.MessageDeletedPayload)
		if payload.MessageID != message.ID {
			t.Fatalf("deleted message id = %q, want %q", payload.MessageID, message.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message deleted event")
	}
	if _, err := application.store.Messages().ByID(ctx, message.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("message lookup after delete error = %v, want sql.ErrNoRows", err)
	}
	if _, err := application.store.MessageAttachments().ByID(ctx, attachment.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("attachment lookup after delete error = %v, want sql.ErrNoRows", err)
	}
}

func TestRunAgentReloadsStoredAttachmentsForEmptyAttachmentSlice(t *testing.T) {
	ctx := context.Background()
	application, _, bootstrap := newConversationTestApp(t, ctx)
	capture := &capturingRuntime{sends: make(chan capturedInput, 2)}
	application.opts.Runtimes[domain.AgentKindFake] = capture

	now := time.Now().UTC()
	message := domain.Message{
		ID:               "msg_reload_attachments",
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		SenderType:       domain.SenderUser,
		SenderID:         bootstrap.User.ID,
		Kind:             domain.MessageText,
		Body:             "use stored attachment",
		Attachments:      []domain.MessageAttachment{},
		CreatedAt:        now,
	}
	storedFile := filepath.Join(t.TempDir(), "stored.txt")
	if err := os.WriteFile(storedFile, []byte("stored attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	attachment := domain.MessageAttachment{
		ID:               "att_reload_attachments",
		MessageID:        message.ID,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Filename:         "stored.txt",
		ContentType:      "text/plain",
		Kind:             domain.MessageAttachmentText,
		SizeBytes:        17,
		StoragePath:      storedFile,
		CreatedAt:        now,
	}
	if err := application.store.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Messages().Create(ctx, message); err != nil {
			return err
		}
		return tx.MessageAttachments().Create(ctx, attachment)
	}); err != nil {
		t.Fatal(err)
	}

	application.runAgentForMessage(ctx, message)
	captured := readCapturedInput(t, capture.sends)
	if len(captured.input.Attachments) != 1 || captured.input.Attachments[0].LocalPath != storedFile {
		t.Fatalf("captured attachments = %#v, want stored attachment path %q", captured.input.Attachments, storedFile)
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
	inputTokens := int64(42)
	cachedTokens := int64(12)
	outputTokens := int64(7)
	totalTokens := int64(49)
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
				{Type: agentruntime.EventCompleted, Text: "done", Usage: &agentruntime.Usage{
					Model:             "fake-test",
					InputTokens:       &inputTokens,
					CachedInputTokens: &cachedTokens,
					OutputTokens:      &outputTokens,
					TotalTokens:       &totalTokens,
					Raw:               map[string]any{"input_tokens": 42},
				}},
			}},
		},
	})
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
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
	metricsMeta, ok := botMessage.Metadata["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics metadata = %#v", botMessage.Metadata["metrics"])
	}
	if metricsMeta["provider"] != domain.AgentKindFake || metricsMeta["input_tokens"] != float64(42) || metricsMeta["output_tokens"] != float64(7) {
		t.Fatalf("metrics metadata = %#v", metricsMeta)
	}
	var rows []domain.AgentRunMetric
	requireEventuallyApp(t, time.Second, func() bool {
		var err error
		rows, err = app.ConversationMetrics(ctx, domain.ConversationChannel, bootstrap.Channel.ID, store.MetricsFilter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		return len(rows) == 1
	})
	if rows[0].Provider != domain.AgentKindFake || rows[0].InputTokens == nil || *rows[0].InputTokens != 42 || rows[0].ResponseMessageID != botMessage.ID {
		t.Fatalf("metrics rows = %#v", rows)
	}
}

func TestAgentRunPersistsFailedMetric(t *testing.T) {
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
				{Type: agentruntime.EventFailed, Error: "runtime failed"},
			}},
		},
	})
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "fail",
	}); err != nil {
		t.Fatal(err)
	}

	requireEventuallyApp(t, time.Second, func() bool {
		rows, err := app.ConversationMetrics(ctx, domain.ConversationChannel, bootstrap.Channel.ID, store.MetricsFilter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		return len(rows) == 1 && rows[0].Status == "failed" && rows[0].ResponseMessageID == ""
	})
}

func TestAgentRunMetricTPSUsesDurationForBatchedOutput(t *testing.T) {
	startedAt := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(10 * time.Second)
	firstTokenAt := completedAt.Add(-time.Millisecond)
	outputTokens := int64(100)
	tracker := agentRunTracker{
		RunID: "run_test",
		UserMessage: domain.Message{
			ID:               "msg_test",
			OrganizationID:   "org_test",
			ConversationType: domain.ConversationChannel,
			ConversationID:   "chn_test",
			CreatedAt:        startedAt,
		},
		Agent:     domain.Agent{ID: "agt_test", Name: "Codex", Kind: domain.AgentKindCodex},
		StartedAt: startedAt,
	}
	metric := tracker.metric(agentRunMetricInput{
		Status:       "completed",
		FirstTokenAt: &firstTokenAt,
		CompletedAt:  completedAt,
		Usage:        &agentruntime.Usage{OutputTokens: &outputTokens},
	})
	if metric.TPS == nil || *metric.TPS != 10 {
		t.Fatalf("tps = %#v, want 10", metric.TPS)
	}
	if metric.TTFTMS == nil || *metric.TTFTMS != 9999 {
		t.Fatalf("ttft = %#v, want 9999", metric.TTFTMS)
	}
}

func TestAgentRunMetricCacheHitUsesTotalInputSideTokens(t *testing.T) {
	startedAt := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	inputTokens := int64(3)
	cacheReadTokens := int64(11639)
	outputTokens := int64(3880)
	tracker := agentRunTracker{
		RunID: "run_cache",
		UserMessage: domain.Message{
			ID:               "msg_cache",
			OrganizationID:   "org_test",
			ConversationType: domain.ConversationChannel,
			ConversationID:   "chn_test",
			CreatedAt:        startedAt,
		},
		Agent:     domain.Agent{ID: "agt_test", Name: "Claude", Kind: domain.AgentKindClaude},
		StartedAt: startedAt,
	}
	metric := tracker.metric(agentRunMetricInput{
		Status:      "completed",
		CompletedAt: startedAt.Add(10 * time.Second),
		Usage: &agentruntime.Usage{
			InputTokens:          &inputTokens,
			CacheReadInputTokens: &cacheReadTokens,
			OutputTokens:         &outputTokens,
		},
	})
	if metric.CacheHitRate == nil {
		t.Fatal("cache hit rate is nil")
	}
	if *metric.CacheHitRate > 1 || *metric.CacheHitRate < 0.99 {
		t.Fatalf("cache hit rate = %v, want <= 1 and near 1", *metric.CacheHitRate)
	}
}

func TestAgentMessageWebhookPostsSignedPayload(t *testing.T) {
	ctx := context.Background()
	requests := make(chan webhookRequest, 1)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		requests <- webhookRequest{header: r.Header.Clone(), requestURI: r.RequestURI, body: body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhookServer.Close()

	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bus := eventbus.New()
	app := New(st, bus, Options{
		AdminToken:     "secret",
		DataDir:        t.TempDir(),
		WebhookTimeout: time.Second,
		Runtimes: map[string]agentruntime.Runtime{
			domain.AgentKindFake: scriptedRuntime{events: []agentruntime.Event{
				{Type: agentruntime.EventCompleted, Text: "signed reply"},
			}},
		},
	})
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	secret := "top-secret"
	if _, err := app.UpdateNotificationSettings(ctx, bootstrap.Organization.ID, NotificationSettingsUpdateRequest{
		WebhookEnabled: true,
		WebhookURL:     webhookServer.URL + "/${title}/${body}",
		WebhookSecret:  &secret,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "call webhook",
	}); err != nil {
		t.Fatal(err)
	}

	var got webhookRequest
	select {
	case got = <-requests:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook request")
	}
	if got.header.Get("X-AgentX-Event") != AgentMessageCreatedWebhookEvent {
		t.Fatalf("X-AgentX-Event = %q", got.header.Get("X-AgentX-Event"))
	}
	if got.header.Get("X-AgentX-Delivery") == "" {
		t.Fatal("X-AgentX-Delivery is empty")
	}
	timestamp := got.header.Get("X-AgentX-Timestamp")
	if timestamp == "" {
		t.Fatal("X-AgentX-Timestamp is empty")
	}
	if got.header.Get("X-AgentX-Signature") != testWebhookSignature(secret, timestamp, got.body) {
		t.Fatalf("X-AgentX-Signature = %q", got.header.Get("X-AgentX-Signature"))
	}
	if !strings.Contains(got.requestURI, "/Fake%20Agent/signed%20reply") {
		t.Fatalf("requestURI = %q, want encoded title/body placeholders", got.requestURI)
	}

	var payload AgentMessageWebhookPayload
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Event != AgentMessageCreatedWebhookEvent || payload.Delivery != got.header.Get("X-AgentX-Delivery") || payload.Title != "Fake Agent" {
		t.Fatalf("payload event/delivery = %#v", payload)
	}
	if payload.Message.Body != "signed reply" || payload.Message.SenderType != domain.SenderBot {
		t.Fatalf("payload message = %#v", payload.Message)
	}
}

func TestRenderWebhookURLSubstitutesAndTruncatesPlaceholders(t *testing.T) {
	longBody := strings.Repeat("x", webhookURLBodyLimit+10)
	got, err := renderWebhookURL("https://example.com/${title}/${body}", AgentMessageWebhookPayload{
		Title: "Agent One",
		Message: domain.Message{
			Body: longBody,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "/Agent%20One/") {
		t.Fatalf("rendered URL = %q, want encoded title", got)
	}
	if !strings.Contains(got, strings.Repeat("x", webhookURLBodyLimit-3)+"...") {
		t.Fatalf("rendered URL = %q, want truncated body with ellipsis", got)
	}
	if strings.Contains(got, "${title}") || strings.Contains(got, "${body}") {
		t.Fatalf("rendered URL still contains placeholders: %q", got)
	}
}

func TestRenderWebhookURLSubstitutesEncodedPlaceholders(t *testing.T) {
	got, err := renderWebhookURL("https://example.com/$%7Btitle%7D/$%7Bbody%7D", AgentMessageWebhookPayload{
		Title: "Agent One",
		Message: domain.Message{
			Body: "reply text",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got != "https://example.com/Agent%20One/reply%20text" {
		t.Fatalf("rendered URL = %q", got)
	}
}

func TestWebhookTimeoutDoesNotBreakBotMessageCreation(t *testing.T) {
	ctx := context.Background()
	started := make(chan struct{}, 1)

	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bus := eventbus.New()
	app := New(st, bus, Options{
		AdminToken:        "secret",
		DataDir:           t.TempDir(),
		WebhookHTTPClient: &http.Client{Transport: blockingWebhookTransport{started: started}},
		WebhookTimeout:    10 * time.Millisecond,
		Runtimes: map[string]agentruntime.Runtime{
			domain.AgentKindFake: scriptedRuntime{events: []agentruntime.Event{
				{Type: agentruntime.EventCompleted, Text: "still delivered"},
			}},
		},
	})
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateNotificationSettings(ctx, bootstrap.Organization.ID, NotificationSettingsUpdateRequest{
		WebhookEnabled: true,
		WebhookURL:     "https://example.test/agentx",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "timeout",
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook attempt")
	}

	requireEventuallyApp(t, time.Second, func() bool {
		messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 20)
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range messages {
			if message.SenderType == domain.SenderBot && message.Body == "still delivered" {
				return true
			}
		}
		return false
	})
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

func TestAgentChannelsListsJoinedChannels(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)

	support, err := app.CreateChannel(ctx, bootstrap.Project.ID, "support", domain.ChannelTypeThread)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetChannelAgents(ctx, support.ID, []domain.ChannelAgent{{
		AgentID:        bootstrap.Agent.ID,
		RunWorkspaceID: bootstrap.Workspace.ID,
	}}); err != nil {
		t.Fatal(err)
	}

	channels, err := app.AgentChannels(ctx, bootstrap.Agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 2 {
		t.Fatalf("channels = %#v, want two joined channels", channels)
	}
	for _, item := range channels {
		switch item.Channel.ID {
		case bootstrap.Channel.ID:
			if item.RunWorkspace.ID != bootstrap.ProjectWorkspace.ID {
				t.Fatalf("general run workspace = %q, want %q", item.RunWorkspace.ID, bootstrap.ProjectWorkspace.ID)
			}
		case support.ID:
			if item.RunWorkspace.ID != bootstrap.Workspace.ID {
				t.Fatalf("support run workspace = %q, want %q", item.RunWorkspace.ID, bootstrap.Workspace.ID)
			}
		default:
			t.Fatalf("unexpected channel = %#v", item.Channel)
		}
	}

	if err := app.ArchiveChannel(ctx, support.ID); err != nil {
		t.Fatal(err)
	}
	channels, err = app.AgentChannels(ctx, bootstrap.Agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0].Channel.ID != bootstrap.Channel.ID {
		t.Fatalf("channels after archive = %#v, want only bootstrap channel", channels)
	}
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
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}

	second, err := app.CreateAgent(ctx, AgentCreateRequest{
		UserID:         bootstrap.User.ID,
		OrganizationID: bootstrap.Organization.ID,
		Name:           "Agent Two",
		Handle:         "agent_two",
		Kind:           "capture",
		FastMode:       true,
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
	if !directed.fastMode {
		t.Fatal("directed fastMode = false, want true")
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

func TestAgentRunUsesProjectWorkspaceByDefaultAndAgentWorkspaceWhenPinned(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)
	capture := &capturingRuntime{sends: make(chan capturedInput, 4)}
	app.opts.Runtimes[domain.AgentKindFake] = capture

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "default workspace",
	}); err != nil {
		t.Fatal(err)
	}
	first := readCapturedInput(t, capture.sends)
	if first.workspace != bootstrap.ProjectWorkspace.Path {
		t.Fatalf("default run workspace = %q, want %q", first.workspace, bootstrap.ProjectWorkspace.Path)
	}
	if first.instructionWorkspace != bootstrap.Workspace.Path {
		t.Fatalf("instruction workspace = %q, want %q", first.instructionWorkspace, bootstrap.Workspace.Path)
	}

	if _, err := app.SetChannelAgents(ctx, bootstrap.Channel.ID, []domain.ChannelAgent{{
		AgentID:        bootstrap.Agent.ID,
		RunWorkspaceID: bootstrap.Agent.ConfigWorkspaceID,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "agent workspace",
	}); err != nil {
		t.Fatal(err)
	}
	second := readCapturedInput(t, capture.sends)
	if second.workspace != bootstrap.Workspace.Path {
		t.Fatalf("pinned run workspace = %q, want %q", second.workspace, bootstrap.Workspace.Path)
	}
	if second.instructionWorkspace != bootstrap.Workspace.Path {
		t.Fatalf("pinned instruction workspace = %q, want %q", second.instructionWorkspace, bootstrap.Workspace.Path)
	}
}

func TestAgentRunMigratesLegacyMemoryFile(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)
	capture := &capturingRuntime{sends: make(chan capturedInput, 1)}
	app.opts.Runtimes[domain.AgentKindFake] = capture

	if err := os.Remove(bootstrap.Workspace.Path + "/AGENTS.md"); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Remove(bootstrap.Workspace.Path + "/CLAUDE.md"); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.WriteFile(bootstrap.Workspace.Path+"/memory.md", []byte("legacy memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "migrate memory",
	}); err != nil {
		t.Fatal(err)
	}
	_ = readCapturedInput(t, capture.sends)

	agentsContent, err := os.ReadFile(bootstrap.Workspace.Path + "/AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(agentsContent)) != "legacy memory" {
		t.Fatalf("AGENTS.md = %q, want legacy memory", string(agentsContent))
	}
	claudeContent, err := os.ReadFile(bootstrap.Workspace.Path + "/CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(claudeContent)) != "@AGENTS.md" {
		t.Fatalf("CLAUDE.md = %q, want @AGENTS.md", string(claudeContent))
	}
}

func TestSlashNewWithoutTargetResetsAllAgentsWhenConversationHasMultipleAgents(t *testing.T) {
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
	if err := app.store.Sessions().SetAgentSession(ctx, bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID, "provider_1", "completed"); err != nil {
		t.Fatal(err)
	}
	if err := app.store.Sessions().SetAgentSession(ctx, second.ID, domain.ConversationChannel, bootstrap.Channel.ID, "provider_2", "completed"); err != nil {
		t.Fatal(err)
	}

	message, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.Body != "New context for all agents" || message.Metadata["scope"] != "all" {
		t.Fatalf("/new message = %#v", message)
	}
	for _, agentID := range []string{bootstrap.Agent.ID, second.ID} {
		session, err := app.store.Sessions().ByConversation(ctx, agentID, domain.ConversationChannel, bootstrap.Channel.ID)
		if err != nil {
			t.Fatal(err)
		}
		if session.ProviderSessionID != "" || session.ContextStartedAt == nil {
			t.Fatalf("session for %s = %#v, want reset context", agentID, session)
		}
	}
}

func TestSlashNonNewCommandRequiresTargetWhenConversationHasMultipleAgents(t *testing.T) {
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
		Body:             "/model gpt-5.2",
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

func TestSlashSkillsListsSkillsForSingleAndTargetedMultiAgentConversation(t *testing.T) {
	ctx := context.Background()
	app, _, bootstrap := newConversationTestApp(t, ctx)
	emptyHome := t.TempDir()
	if _, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{
		Kind:   stringPtr(domain.AgentKindCodex),
		EnvSet: true,
		Env:    map[string]string{"CODEX_HOME": filepath.Join(emptyHome, "codex"), "HOME": emptyHome},
	}); err != nil {
		t.Fatal(err)
	}
	writeAppSkill(t, filepath.Join(bootstrap.Workspace.Path, ".codex", "skills", "reviewer", "SKILL.md"), `---
name: reviewer
description: Review code
---
Review code carefully.
`)

	message, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/skills",
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.SenderType != domain.SenderSystem || !strings.Contains(message.Body, "Skills for @"+bootstrap.Agent.Handle) || !strings.Contains(message.Body, "/reviewer - Review code") {
		t.Fatalf("/skills message = %#v", message)
	}
	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/skill",
	}); !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("/skill error = %v, want ErrUnknownCommand", err)
	}

	second, err := app.CreateAgent(ctx, AgentCreateRequest{
		UserID:         bootstrap.User.ID,
		OrganizationID: bootstrap.Organization.ID,
		Name:           "Agent Two",
		Handle:         "agent_two",
		Kind:           domain.AgentKindCodex,
		Env:            map[string]string{"CODEX_HOME": filepath.Join(emptyHome, "codex-two"), "HOME": emptyHome},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondWorkspace, err := app.store.Workspaces().ByID(ctx, second.ConfigWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	writeAppSkill(t, filepath.Join(secondWorkspace.Path, ".codex", "skills", "tester", "SKILL.md"), `---
name: tester
description: Run focused tests
---
Run tests.
`)
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
		Body:             "/skills",
	}); !IsCommandInputError(err) {
		t.Fatalf("/skills without target error = %v, want command input error", err)
	}

	message, err = app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/skills @agent_two",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Body, "Skills for @agent_two") || !strings.Contains(message.Body, "/tester - Run focused tests") {
		t.Fatalf("targeted /skills message = %#v", message)
	}
}

func TestSlashSkillInvocationBuildsPromptAndTargetsAgent(t *testing.T) {
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
		DefaultAgentKind: domain.AgentKindCodex,
		Runtimes: map[string]agentruntime.Runtime{
			domain.AgentKindCodex: capture,
		},
	})
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	emptyHome := t.TempDir()
	if _, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{
		EnvSet: true,
		Env:    map[string]string{"CODEX_HOME": filepath.Join(emptyHome, "codex-one"), "HOME": emptyHome},
	}); err != nil {
		t.Fatal(err)
	}
	second, err := app.CreateAgent(ctx, AgentCreateRequest{
		UserID:         bootstrap.User.ID,
		OrganizationID: bootstrap.Organization.ID,
		Name:           "Agent Two",
		Handle:         "agent_two",
		Kind:           domain.AgentKindCodex,
		Env:            map[string]string{"CODEX_HOME": filepath.Join(emptyHome, "codex-two"), "HOME": emptyHome},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondWorkspace, err := app.store.Workspaces().ByID(ctx, second.ConfigWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	writeAppSkill(t, filepath.Join(secondWorkspace.Path, ".codex", "skills", "reviewer", "SKILL.md"), `---
name: reviewer
description: Review code
---
Review code carefully and cite files.
`)
	if _, err := app.SetChannelAgents(ctx, bootstrap.Channel.ID, []domain.ChannelAgent{
		{AgentID: bootstrap.Agent.ID},
		{AgentID: second.ID},
	}); err != nil {
		t.Fatal(err)
	}

	message, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/reviewer @agent_two check the API",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := readCapturedInput(t, capture.sends)
	if input.agentID != second.ID {
		t.Fatalf("captured agentID = %q, want %q", input.agentID, second.ID)
	}
	if !strings.Contains(input.input.Prompt, "Review code carefully and cite files.") || !strings.Contains(input.input.Prompt, "check the API") {
		t.Fatalf("skill prompt = %q", input.input.Prompt)
	}
	if strings.Contains(input.input.Prompt, "@agent_two") {
		t.Fatalf("skill prompt = %q, want target stripped from request args", input.input.Prompt)
	}

	messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var saved bool
	for _, item := range messages {
		if item.ID == message.ID && item.Body == "/reviewer @agent_two check the API" {
			saved = true
		}
	}
	if !saved {
		t.Fatalf("saved original user message missing from %#v", messages)
	}
}

func TestSlashSkillUnknownAndBuiltinPrecedence(t *testing.T) {
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
		DefaultAgentKind: domain.AgentKindCodex,
		Runtimes: map[string]agentruntime.Runtime{
			domain.AgentKindCodex: capture,
		},
	})
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	emptyHome := t.TempDir()
	if _, err := app.UpdateAgent(ctx, bootstrap.Agent.ID, AgentUpdateRequest{
		EnvSet: true,
		Env:    map[string]string{"CODEX_HOME": filepath.Join(emptyHome, "codex"), "HOME": emptyHome},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/does-not-exist",
	}); !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("unknown slash error = %v, want ErrUnknownCommand", err)
	}

	writeAppSkill(t, filepath.Join(bootstrap.Workspace.Path, ".codex", "skills", "review", "SKILL.md"), `---
name: review
description: Conflicting review skill
---
Use this skill body only if dynamic skills beat built-ins.
`)
	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/review check the diff",
	}); err != nil {
		t.Fatal(err)
	}
	input := readCapturedInput(t, capture.sends)
	if !strings.Contains(input.input.Prompt, "Review the current workspace changes.") || strings.Contains(input.input.Prompt, "Use this skill body") {
		t.Fatalf("review prompt = %q, want built-in command prompt", input.input.Prompt)
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
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
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
	messages, err := st.Messages().List(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var separator *domain.Message
	for i := range messages {
		message := messages[i]
		if message.SenderType == domain.SenderSystem && message.Metadata["command_name"] == "new" {
			separator = &message
			break
		}
	}
	if separator == nil || separator.Metadata["separator"] != true {
		t.Fatalf("/new separator message missing from %#v", messages)
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

func TestRunAgentRetriesWithoutResumeWhenProviderSessionIsStale(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	rt := &staleSessionRetryRuntime{}
	app := New(st, eventbus.New(), Options{
		AdminToken:       "secret",
		DataDir:          t.TempDir(),
		DefaultAgentKind: "retry-stale",
		Runtimes: map[string]agentruntime.Runtime{
			"retry-stale": rt,
		},
	})
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}

	if err := st.Sessions().SetAgentSession(ctx, bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID, "stale-session", "completed"); err != nil {
		t.Fatal(err)
	}

	app.runAgentForMessage(ctx, domain.Message{
		ID:               "msg_stale_resume",
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		SenderType:       domain.SenderUser,
		SenderID:         bootstrap.User.ID,
		Kind:             domain.MessageText,
		Body:             "hello",
		CreatedAt:        time.Now().UTC(),
	})

	if got, want := rt.previousSessionIDs, []string{"stale-session", ""}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("previous session ids = %#v, want %#v", got, want)
	}
	session, err := st.Sessions().ByConversation(ctx, bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ProviderSessionID != "fresh-session" || session.Status != "completed" {
		t.Fatalf("session = %#v, want fresh completed session", session)
	}
	messages, err := st.Messages().List(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if countBotMessagesFrom(messages, "recovered", bootstrap.Agent.BotUserID) != 1 {
		t.Fatalf("messages = %#v, want one recovered bot message", messages)
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
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}
	referenced := domain.Message{
		ID:               "msg_plan_reference",
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		SenderType:       domain.SenderUser,
		SenderID:         bootstrap.User.ID,
		Kind:             domain.MessageText,
		Body:             "the existing bug report",
		CreatedAt:        time.Now().UTC().Add(-time.Minute),
	}
	if err := st.Messages().Create(ctx, referenced); err != nil {
		t.Fatal(err)
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "/plan @claude-default build the feature",
		ReplyToMessageID: referenced.ID,
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
	if !strings.Contains(input.input.Prompt, referenced.Body) {
		t.Fatalf("prompt = %q, want referenced message body", input.input.Prompt)
	}
}

func TestAgentPromptIncludesReplyReferenceOutsideRecentContext(t *testing.T) {
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
		DefaultAgentKind: domain.AgentKindFake,
		Runtimes: map[string]agentruntime.Runtime{
			domain.AgentKindFake: capture,
		},
	})
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
	if err != nil {
		t.Fatal(err)
	}

	baseTime := time.Now().UTC().Add(-2 * time.Hour)
	referenced := domain.Message{
		ID:               "msg_old_reference",
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		SenderType:       domain.SenderUser,
		SenderID:         bootstrap.User.ID,
		Kind:             domain.MessageText,
		Body:             "old reference outside recent history",
		CreatedAt:        baseTime,
	}
	if err := st.Messages().Create(ctx, referenced); err != nil {
		t.Fatal(err)
	}
	if err := st.Sessions().SetAgentSessionContextStartedAt(ctx, bootstrap.Agent.ID, domain.ConversationChannel, bootstrap.Channel.ID, baseTime.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < runtimeContextMessageLimit+5; i++ {
		if err := st.Messages().Create(ctx, domain.Message{
			ID:               fmt.Sprintf("msg_filler_%02d", i),
			OrganizationID:   bootstrap.Organization.ID,
			ConversationType: domain.ConversationChannel,
			ConversationID:   bootstrap.Channel.ID,
			SenderType:       domain.SenderUser,
			SenderID:         bootstrap.User.ID,
			Kind:             domain.MessageText,
			Body:             fmt.Sprintf("filler %02d", i),
			CreatedAt:        baseTime.Add(time.Duration(i+2) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "please handle this",
		ReplyToMessageID: referenced.ID,
	}); err != nil {
		t.Fatal(err)
	}
	input := readCapturedInput(t, capture.sends)
	if !strings.Contains(input.input.Prompt, referenced.Body) {
		t.Fatalf("prompt = %q, want referenced body outside recent context", input.input.Prompt)
	}
	if strings.Contains(input.input.Context, referenced.Body) {
		t.Fatalf("context = %q, referenced body should not depend on recent history", input.input.Context)
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
	bootstrap, err := app.Bootstrap(ctx, testSetupRequest("Meteorsky"))
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

type staleSessionRetryRuntime struct {
	previousSessionIDs []string
}

func (r *staleSessionRetryRuntime) StartSession(ctx context.Context, req agentruntime.StartSessionRequest) (agentruntime.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.previousSessionIDs = append(r.previousSessionIDs, req.PreviousSessionID)
	if req.PreviousSessionID != "" {
		return &scriptedSession{
			id: "stale-attempt",
			script: []agentruntime.Event{{
				Type:         agentruntime.EventFailed,
				Error:        "No conversation found with session ID: " + req.PreviousSessionID,
				StaleSession: true,
			}},
			events: make(chan agentruntime.Event, 1),
		}, nil
	}
	return &scriptedSession{
		id:     "fresh-session",
		script: []agentruntime.Event{{Type: agentruntime.EventCompleted, Text: "recovered"}},
		events: make(chan agentruntime.Event, 1),
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
	agentID              string
	workspace            string
	instructionWorkspace string
	fastMode             bool
	yoloMode             bool
	effort               string
	permissionMode       string
	previousSessionID    string
	input                agentruntime.Input
}

type capturingRuntime struct {
	sends chan capturedInput
}

func (r *capturingRuntime) StartSession(ctx context.Context, req agentruntime.StartSessionRequest) (agentruntime.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &capturingSession{
		agentID:              req.AgentID,
		workspace:            req.Workspace,
		instructionWorkspace: req.InstructionWorkspace,
		fastMode:             req.FastMode,
		yoloMode:             req.YoloMode,
		effort:               req.Effort,
		permissionMode:       req.PermissionMode,
		previousSessionID:    req.PreviousSessionID,
		id:                   "capture:" + req.SessionKey,
		sends:                r.sends,
		events:               make(chan agentruntime.Event, 1),
	}, nil
}

type capturingSession struct {
	agentID              string
	workspace            string
	instructionWorkspace string
	fastMode             bool
	yoloMode             bool
	effort               string
	permissionMode       string
	previousSessionID    string
	id                   string
	sends                chan capturedInput
	events               chan agentruntime.Event
}

func (s *capturingSession) Send(ctx context.Context, input agentruntime.Input) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.sends <- capturedInput{
		agentID:              s.agentID,
		workspace:            s.workspace,
		instructionWorkspace: s.instructionWorkspace,
		fastMode:             s.fastMode,
		yoloMode:             s.yoloMode,
		effort:               s.effort,
		permissionMode:       s.permissionMode,
		previousSessionID:    s.previousSessionID,
		input:                input,
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

func testSetupRequest(displayName string) SetupAdminRequest {
	return SetupAdminRequest{
		SetupToken:  "secret",
		Username:    "meteorsky",
		Password:    "correct-password-123",
		DisplayName: displayName,
	}
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

func writeAppSkill(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

type webhookRequest struct {
	header     http.Header
	requestURI string
	body       []byte
}

type blockingWebhookTransport struct {
	started chan<- struct{}
}

func (t blockingWebhookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case t.started <- struct{}{}:
	default:
	}
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func testWebhookSignature(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
