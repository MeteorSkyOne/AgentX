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
	role, err := st.Organizations().MemberRole(ctx, org.ID, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if role != domain.RoleOwner {
		t.Fatalf("MemberRole = %q, want %q", role, domain.RoleOwner)
	}

	messages, err := st.Messages().List(ctx, domain.ConversationChannel, channel.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Body != "hello" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestMessageMetadataRoundTripsArbitraryJSON(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_meta", DisplayName: "Meta", CreatedAt: now}
	org := domain.Organization{ID: "org_meta", Name: "Meta", CreatedAt: now}
	channel := domain.Channel{ID: "chn_meta", OrganizationID: org.ID, Name: "meta", CreatedAt: now}
	message := domain.Message{
		ID: "msg_meta", OrganizationID: org.ID, ConversationType: domain.ConversationChannel,
		ConversationID: channel.ID, SenderType: domain.SenderBot, SenderID: "bot_meta",
		Kind: domain.MessageText, Body: "done", CreatedAt: now,
		Metadata: map[string]any{
			"thinking": "look up file",
			"process": []domain.ProcessItem{{
				Type:     "tool_call",
				ToolName: "Read",
				Input:    map[string]any{"path": "README.md"},
				Raw:      map[string]any{"type": "tool_use"},
			}},
		},
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

	messages, err := st.Messages().List(ctx, domain.ConversationChannel, channel.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Metadata["thinking"] != "look up file" {
		t.Fatalf("metadata = %#v", messages[0].Metadata)
	}
	process, ok := messages[0].Metadata["process"].([]any)
	if !ok || len(process) != 1 {
		t.Fatalf("process metadata = %#v", messages[0].Metadata["process"])
	}
	item, ok := process[0].(map[string]any)
	if !ok || item["type"] != "tool_call" || item["tool_name"] != "Read" {
		t.Fatalf("process item = %#v", process[0])
	}
	input, ok := item["input"].(map[string]any)
	if !ok || input["path"] != "README.md" {
		t.Fatalf("input = %#v", item["input"])
	}
}

func TestMessageReplyToMessageIDRoundTripsAndSurvivesUpdate(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_reply", DisplayName: "Reply", CreatedAt: now}
	org := domain.Organization{ID: "org_reply", Name: "Reply", CreatedAt: now}
	channel := domain.Channel{ID: "chn_reply", OrganizationID: org.ID, Name: "reply", CreatedAt: now}
	original := domain.Message{
		ID: "msg_reply_original", OrganizationID: org.ID, ConversationType: domain.ConversationChannel,
		ConversationID: channel.ID, SenderType: domain.SenderUser, SenderID: user.ID,
		Kind: domain.MessageText, Body: "original", CreatedAt: now,
	}
	reply := domain.Message{
		ID: "msg_reply_child", OrganizationID: org.ID, ConversationType: domain.ConversationChannel,
		ConversationID: channel.ID, SenderType: domain.SenderUser, SenderID: user.ID,
		Kind: domain.MessageText, Body: "reply", ReplyToMessageID: original.ID, CreatedAt: now.Add(time.Second),
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
		if err := tx.Messages().Create(ctx, original); err != nil {
			return err
		}
		return tx.Messages().Create(ctx, reply)
	})
	if err != nil {
		t.Fatal(err)
	}

	messages, err := st.Messages().List(ctx, domain.ConversationChannel, channel.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].ReplyToMessageID != original.ID {
		t.Fatalf("messages = %#v, want reply_to_message_id %q", messages, original.ID)
	}

	reply.Body = "updated reply"
	if err := st.Messages().Update(ctx, reply); err != nil {
		t.Fatal(err)
	}
	updated, err := st.Messages().ByID(ctx, reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Body != "updated reply" || updated.ReplyToMessageID != original.ID {
		t.Fatalf("updated message = %#v, want body update and preserved reply link", updated)
	}
}

func TestMessageAttachmentsRoundTripAndCascadeDelete(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_attachment", DisplayName: "Attachment", CreatedAt: now}
	org := domain.Organization{ID: "org_attachment", Name: "Attachment", CreatedAt: now}
	channel := domain.Channel{ID: "chn_attachment", OrganizationID: org.ID, Name: "attachment", CreatedAt: now}
	message := domain.Message{
		ID: "msg_attachment", OrganizationID: org.ID, ConversationType: domain.ConversationChannel,
		ConversationID: channel.ID, SenderType: domain.SenderUser, SenderID: user.ID,
		Kind: domain.MessageText, Body: "", CreatedAt: now,
	}
	attachment := domain.MessageAttachment{
		ID: "att_1", MessageID: message.ID, OrganizationID: org.ID,
		ConversationType: domain.ConversationChannel, ConversationID: channel.ID,
		Filename: "notes.txt", ContentType: "text/plain", Kind: domain.MessageAttachmentText,
		SizeBytes: 5, StoragePath: filepath.Join(t.TempDir(), "notes.txt"), CreatedAt: now,
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
		if err := tx.Messages().Create(ctx, message); err != nil {
			return err
		}
		return tx.MessageAttachments().Create(ctx, attachment)
	})
	if err != nil {
		t.Fatal(err)
	}

	byID, err := st.MessageAttachments().ByID(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byID.Filename != attachment.Filename || byID.StoragePath != attachment.StoragePath || byID.Kind != attachment.Kind {
		t.Fatalf("attachment by id = %#v, want %#v", byID, attachment)
	}

	attachments, err := st.MessageAttachments().ListByMessage(ctx, message.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].ID != attachment.ID {
		t.Fatalf("attachments = %#v, want one %s", attachments, attachment.ID)
	}

	if err := st.Messages().Delete(ctx, message.ID); err != nil {
		t.Fatal(err)
	}
	attachments, err = st.MessageAttachments().ListByMessage(ctx, message.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 0 {
		t.Fatalf("attachments after message delete = %#v, want none", attachments)
	}
}

func TestMessagesListRecentReturnsLatestMessagesChronologically(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_recent", DisplayName: "Recent", CreatedAt: now}
	org := domain.Organization{ID: "org_recent", Name: "Recent", CreatedAt: now}
	channel := domain.Channel{ID: "chn_recent", OrganizationID: org.ID, Name: "recent", CreatedAt: now}

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
		for i := 1; i <= 3; i++ {
			if err := tx.Messages().Create(ctx, domain.Message{
				ID:               "msg_recent_" + strconv.Itoa(i),
				OrganizationID:   org.ID,
				ConversationType: domain.ConversationChannel,
				ConversationID:   channel.ID,
				SenderType:       domain.SenderUser,
				SenderID:         user.ID,
				Kind:             domain.MessageText,
				Body:             "message " + strconv.Itoa(i),
				CreatedAt:        now.Add(time.Duration(i) * time.Second),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	messages, err := st.Messages().ListRecent(ctx, domain.ConversationChannel, channel.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != "msg_recent_2" || messages[1].ID != "msg_recent_3" {
		t.Fatalf("recent messages = %#v", messages)
	}

	messages, err = st.Messages().ListRecentBefore(ctx, domain.ConversationChannel, channel.ID, now.Add(3*time.Second), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != "msg_recent_1" || messages[1].ID != "msg_recent_2" {
		t.Fatalf("recent messages before cursor = %#v", messages)
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

func TestOrganizationsAnyReportsWhetherOrganizationExists(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	any, err := st.Organizations().Any(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if any {
		t.Fatal("Any = true before organization exists")
	}

	if err := st.Organizations().Create(ctx, domain.Organization{
		ID:        "org_any",
		Name:      "Any",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	any, err = st.Organizations().Any(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !any {
		t.Fatal("Any = false after organization exists")
	}
}

func TestNotificationSettingsUpsertRoundTripsRawSecret(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	org := domain.Organization{ID: "org_notify", Name: "Notify", CreatedAt: now}
	if err := st.Organizations().Create(ctx, org); err != nil {
		t.Fatal(err)
	}

	_, err := st.NotificationSettings().ByOrganization(ctx, org.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ByOrganization error = %v, want sql.ErrNoRows", err)
	}

	first := domain.NotificationSettings{
		OrganizationID: org.ID,
		WebhookEnabled: true,
		WebhookURL:     "https://example.com/webhook",
		WebhookSecret:  "first-secret",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := st.NotificationSettings().Upsert(ctx, first); err != nil {
		t.Fatal(err)
	}

	second := first
	second.WebhookURL = "https://example.com/next"
	second.WebhookSecret = ""
	second.WebhookEnabled = false
	second.UpdatedAt = now.Add(time.Minute)
	if err := st.NotificationSettings().Upsert(ctx, second); err != nil {
		t.Fatal(err)
	}

	got, err := st.NotificationSettings().ByOrganization(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WebhookEnabled || got.WebhookURL != second.WebhookURL || got.WebhookSecret != "" {
		t.Fatalf("settings = %#v", got)
	}
	if got.WebhookSecretConfigured {
		t.Fatalf("WebhookSecretConfigured = true, want false")
	}
	if !got.CreatedAt.Equal(first.CreatedAt) || !got.UpdatedAt.Equal(second.UpdatedAt) {
		t.Fatalf("timestamps = %s/%s, want %s/%s", got.CreatedAt, got.UpdatedAt, first.CreatedAt, second.UpdatedAt)
	}
}

func TestUserPreferencesDefaultTableUpsert(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	user := domain.User{ID: "usr_preferences", DisplayName: "Prefs", CreatedAt: now}
	if err := st.Users().Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	_, err := st.UserPreferences().ByUser(ctx, user.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ByUser error = %v, want sql.ErrNoRows", err)
	}
	if err := st.UserPreferences().Upsert(ctx, domain.UserPreferences{
		UserID:      user.ID,
		ShowTTFT:    false,
		ShowTPS:     true,
		HideAvatars: false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UserPreferences().Upsert(ctx, domain.UserPreferences{
		UserID:      user.ID,
		ShowTTFT:    true,
		ShowTPS:     false,
		HideAvatars: true,
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.UserPreferences().ByUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ShowTTFT || got.ShowTPS || !got.HideAvatars {
		t.Fatalf("preferences = %#v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("timestamps = %s/%s", got.CreatedAt, got.UpdatedAt)
	}
}

func TestUsersEnforceUniqueUsernameAndCredentialRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	passwordUpdatedAt := now.Add(time.Minute)
	user := domain.User{
		ID:                "usr_auth",
		Username:          "admin",
		DisplayName:       "Admin",
		PasswordHash:      "$2a$10$hash",
		PasswordUpdatedAt: &passwordUpdatedAt,
		CreatedAt:         now,
	}
	if err := st.Users().Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().Create(ctx, domain.User{
		ID:          "usr_auth_duplicate",
		Username:    "admin",
		DisplayName: "Duplicate",
		CreatedAt:   now,
	}); err == nil {
		t.Fatal("Create user with duplicate username succeeded")
	}

	got, err := st.Users().ByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID || got.PasswordHash != user.PasswordHash || got.PasswordUpdatedAt == nil || !got.PasswordUpdatedAt.Equal(passwordUpdatedAt) {
		t.Fatalf("user = %#v, want credential fields from %#v", got, user)
	}
	hasPassword, err := st.Users().HasPassword(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPassword {
		t.Fatal("HasPassword = false, want true")
	}
}

func TestAPISessionsUseTokenHashAndExpiration(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	user := domain.User{ID: "usr_session", DisplayName: "Session", CreatedAt: now}
	if err := st.Users().Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().CreateAPISession(ctx, "token-hash", user.ID, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Users().UserIDByAPISessionHash(ctx, "raw-token", now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("raw token lookup error = %v, want sql.ErrNoRows", err)
	}
	userID, err := st.Users().UserIDByAPISessionHash(ctx, "token-hash", now)
	if err != nil {
		t.Fatal(err)
	}
	if userID != user.ID {
		t.Fatalf("userID = %q, want %q", userID, user.ID)
	}
	if _, err := st.Users().UserIDByAPISessionHash(ctx, "token-hash", now.Add(2*time.Hour)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired lookup error = %v, want sql.ErrNoRows", err)
	}
	if err := st.Users().DeleteAPISession(ctx, "token-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Users().UserIDByAPISessionHash(ctx, "token-hash", now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted lookup error = %v, want sql.ErrNoRows", err)
	}
}

func TestMetricsQueriesFilterByScopeAndProvider(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	fixture := seedBindingFixture(t, ctx, st)
	firstTokenAt := fixture.now.Add(500 * time.Millisecond)
	completedAt := fixture.now.Add(2 * time.Second)
	inputTokens := int64(100)
	outputTokens := int64(20)
	cacheHitRate := 0.3
	if err := st.Metrics().Create(ctx, domain.AgentRunMetric{
		RunID:             "run_metrics_1",
		OrganizationID:    fixture.org.ID,
		ProjectID:         "prj_metrics",
		ChannelID:         "chn_metrics",
		ConversationType:  domain.ConversationChannel,
		ConversationID:    "chn_metrics",
		MessageID:         "msg_prompt_1",
		AgentID:           fixture.agent1.ID,
		AgentName:         fixture.agent1.Name,
		Provider:          domain.AgentKindCodex,
		Model:             "gpt-test",
		Status:            "completed",
		StartedAt:         fixture.now,
		FirstTokenAt:      &firstTokenAt,
		CompletedAt:       &completedAt,
		TTFTMS:            int64Ptr(500),
		DurationMS:        int64Ptr(2000),
		TPS:               float64Ptr(10),
		InputTokens:       &inputTokens,
		CachedInputTokens: int64Ptr(30),
		OutputTokens:      &outputTokens,
		TotalTokens:       int64Ptr(120),
		CacheHitRate:      &cacheHitRate,
		TotalCostUSD:      float64Ptr(0.001),
		RawUsageJSON:      `{"input_tokens":100}`,
		CreatedAt:         fixture.now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Metrics().Create(ctx, domain.AgentRunMetric{
		RunID:            "run_metrics_2",
		OrganizationID:   fixture.org.ID,
		ProjectID:        "prj_metrics",
		ChannelID:        "chn_metrics",
		ThreadID:         "thr_metrics",
		ConversationType: domain.ConversationThread,
		ConversationID:   "thr_metrics",
		MessageID:        "msg_prompt_2",
		AgentID:          fixture.agent2.ID,
		AgentName:        fixture.agent2.Name,
		Provider:         domain.AgentKindClaude,
		Status:           "failed",
		StartedAt:        fixture.now.Add(time.Second),
		CompletedAt:      &completedAt,
		DurationMS:       int64Ptr(1000),
		CreatedAt:        fixture.now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	channelRows, err := st.Metrics().ListByChannel(ctx, "chn_metrics", store.MetricsFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(channelRows) != 2 || channelRows[0].RunID != "run_metrics_2" || channelRows[1].RunID != "run_metrics_1" {
		t.Fatalf("channel rows = %#v", channelRows)
	}
	codexRows, err := st.Metrics().ListByProject(ctx, "prj_metrics", store.MetricsFilter{Limit: 10, Provider: domain.AgentKindCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(codexRows) != 1 || codexRows[0].Provider != domain.AgentKindCodex || codexRows[0].InputTokens == nil || *codexRows[0].InputTokens != 100 {
		t.Fatalf("codex rows = %#v", codexRows)
	}
	conversationRows, err := st.Metrics().ListByConversation(ctx, domain.ConversationThread, "thr_metrics", store.MetricsFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(conversationRows) != 1 || conversationRows[0].ThreadID != "thr_metrics" {
		t.Fatalf("conversation rows = %#v", conversationRows)
	}

	moreInputTokens := int64(50)
	moreOutputTokens := int64(10)
	if err := st.Metrics().Create(ctx, domain.AgentRunMetric{
		RunID:            "run_metrics_3",
		OrganizationID:   fixture.org.ID,
		ProjectID:        "prj_metrics",
		ChannelID:        "chn_metrics",
		ConversationType: domain.ConversationChannel,
		ConversationID:   "chn_metrics",
		MessageID:        "msg_prompt_3",
		AgentID:          fixture.agent1.ID,
		AgentName:        fixture.agent1.Name,
		Provider:         domain.AgentKindCodex,
		Model:            "gpt-test",
		Status:           "completed",
		StartedAt:        fixture.now.Add(2 * time.Second),
		CompletedAt:      &completedAt,
		DurationMS:       int64Ptr(1000),
		InputTokens:      &moreInputTokens,
		OutputTokens:     &moreOutputTokens,
		TotalTokens:      int64Ptr(60),
		CreatedAt:        fixture.now.Add(2 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	summaryRows, err := st.Metrics().ListAgentSummariesByProject(ctx, "prj_metrics", store.MetricsFilter{Limit: 10, Provider: domain.AgentKindCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaryRows) != 1 || summaryRows[0].AgentID != fixture.agent1.ID || summaryRows[0].RunCount != 2 || summaryRows[0].InputTokens == nil || *summaryRows[0].InputTokens != 150 {
		t.Fatalf("summary rows = %#v", summaryRows)
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
	session, err := st.Sessions().ByConversation(ctx, fixture.agent1.ID, domain.ConversationChannel, "chn_session")
	if err != nil {
		t.Fatal(err)
	}
	if session.ProviderSessionID != "provider_2" || session.Status != "done" {
		t.Fatalf("session = %#v", session)
	}
}

func TestAgentEnvRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	fixture := seedBindingFixture(t, ctx, st)
	fixture.agent1.Env = map[string]string{"CODEX_API_KEY": "secret", "CUSTOM": "value"}
	fixture.agent1.Description = "Handles code changes"
	fixture.agent1.FastMode = true
	fixture.agent1.YoloMode = true
	fixture.agent1.ID = "agt_env"
	if err := st.Agents().Create(ctx, fixture.agent1); err != nil {
		t.Fatal(err)
	}
	got, err := st.Agents().ByID(ctx, fixture.agent1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Env["CODEX_API_KEY"] != "secret" || got.Env["CUSTOM"] != "value" {
		t.Fatalf("Env = %#v", got.Env)
	}
	if got.Description != "Handles code changes" {
		t.Fatalf("Description = %q, want Handles code changes", got.Description)
	}
	if !got.YoloMode {
		t.Fatalf("YoloMode = false, want true")
	}
	if !got.FastMode {
		t.Fatalf("FastMode = false, want true")
	}
	got.FastMode = false
	got.YoloMode = false
	got.Description = "Updated description"
	got.UpdatedAt = got.UpdatedAt.Add(time.Second)
	if err := st.Agents().Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	updated, err := st.Agents().ByID(ctx, fixture.agent1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.YoloMode {
		t.Fatalf("updated YoloMode = true, want false")
	}
	if updated.FastMode {
		t.Fatalf("updated FastMode = true, want false")
	}
	if updated.Description != "Updated description" {
		t.Fatalf("updated Description = %q, want Updated description", updated.Description)
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

func TestScheduledTasksRoundTripTaskAndRuns(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	defer st.Close()

	fixture := seedBindingFixture(t, ctx, st)
	project := domain.Project{
		ID:             "prj_scheduled",
		OrganizationID: fixture.org.ID,
		Name:           "Scheduled",
		WorkspaceID:    fixture.workspace1.ID,
		CreatedBy:      fixture.user.ID,
		CreatedAt:      fixture.now,
		UpdatedAt:      fixture.now,
	}
	if err := st.Projects().Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	nextRunAt := fixture.now.Add(time.Hour)
	task := domain.ScheduledTask{
		ID:               "tsk_1",
		OrganizationID:   fixture.org.ID,
		ProjectID:        project.ID,
		Name:             "Daily check",
		Kind:             domain.ScheduledTaskKindAgentPrompt,
		Enabled:          true,
		Schedule:         "0 9 * * *",
		Timezone:         "UTC",
		ConversationType: domain.ConversationChannel,
		ConversationID:   "chn_scheduled",
		AgentID:          fixture.agent1.ID,
		WorkspaceID:      fixture.workspace1.ID,
		Prompt:           "status",
		TimeoutSeconds:   600,
		CreatedBy:        fixture.user.ID,
		NextRunAt:        &nextRunAt,
		CreatedAt:        fixture.now,
		UpdatedAt:        fixture.now,
	}
	if err := st.ScheduledTasks().Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	tasks, err := st.ScheduledTasks().ListByProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != task.ID || tasks[0].NextRunAt == nil {
		t.Fatalf("tasks = %#v", tasks)
	}

	startedAt := fixture.now.Add(2 * time.Hour)
	run := domain.ScheduledTaskRun{
		ID:             "trn_1",
		TaskID:         task.ID,
		OrganizationID: fixture.org.ID,
		ProjectID:      project.ID,
		Kind:           task.Kind,
		Trigger:        domain.ScheduledTaskTriggerManual,
		StartedAt:      startedAt,
		Status:         domain.ScheduledTaskRunStatusRunning,
	}
	if err := st.ScheduledTasks().CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	exitCode := 7
	finishedAt := startedAt.Add(time.Second)
	run.Status = domain.ScheduledTaskRunStatusFailed
	run.FinishedAt = &finishedAt
	run.ExitCode = &exitCode
	run.Stdout = "out"
	run.Stderr = "err"
	run.OutputTruncated = true
	if err := st.ScheduledTasks().UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := st.ScheduledTasks().UpdateScheduleState(ctx, task.ID, run.ID, string(run.Status), &run.StartedAt, run.FinishedAt, nil, finishedAt); err != nil {
		t.Fatal(err)
	}

	runs, err := st.ScheduledTasks().ListRunsByTask(ctx, task.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ExitCode == nil || *runs[0].ExitCode != exitCode || !runs[0].OutputTruncated {
		t.Fatalf("runs = %#v", runs)
	}
	updated, err := st.ScheduledTasks().ByID(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastRunID != run.ID || updated.LastRunStatus != string(run.Status) || updated.NextRunAt != nil {
		t.Fatalf("updated task = %#v", updated)
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

func int64Ptr(value int64) *int64 {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}
