package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/id"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
	"nhooyr.io/websocket"
)

func TestHTTPBootstrapAuthAndMessagesFlow(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)
	if bootstrap.SessionToken == "" {
		t.Fatal("session_token is empty")
	}
	if bootstrap.Organization.ID == "" {
		t.Fatal("organization id is empty")
	}
	if bootstrap.Channel.ID == "" {
		t.Fatal("channel id is empty")
	}

	var me domain.User
	getJSON(t, ts.URL+"/api/me", bootstrap.SessionToken, http.StatusOK, &me)
	if me.ID != bootstrap.User.ID {
		t.Fatalf("me id = %q, want %q", me.ID, bootstrap.User.ID)
	}

	var conversationContext app.ConversationContext
	getJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/context", bootstrap.SessionToken, http.StatusOK, &conversationContext)
	if conversationContext.Agent.ID != bootstrap.Agent.ID || conversationContext.Workspace.ID != bootstrap.Workspace.ID {
		t.Fatalf("conversation context = %#v", conversationContext)
	}

	var sent domain.Message
	postJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body": "hello from http",
	}, http.StatusOK, &sent)
	if sent.Body != "hello from http" || sent.SenderType != domain.SenderUser {
		t.Fatalf("sent message = %#v", sent)
	}

	var messages []domain.Message
	requireEventually(t, time.Second, func() bool {
		getJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, http.StatusOK, &messages)
		var sawUser bool
		var sawBot bool
		for _, message := range messages {
			if message.ID == sent.ID && message.Body == "hello from http" {
				sawUser = true
			}
			if message.Body == "Echo: hello from http" && message.SenderType == domain.SenderBot {
				sawBot = true
			}
		}
		return sawUser && sawBot
	})
}

func TestHTTPAgentCreateAndUpdateRoundTripsEffort(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	var created domain.Agent
	postJSON(t, ts.URL+"/api/organizations/"+bootstrap.Organization.ID+"/agents", bootstrap.SessionToken, map[string]any{
		"name":        "Planner",
		"description": "Plans implementation work",
		"handle":      "planner",
		"kind":        "codex",
		"model":       "gpt-test",
		"effort":      "medium",
		"fast_mode":   true,
	}, http.StatusOK, &created)
	if created.Effort != "medium" || !created.FastMode || created.Description != "Plans implementation work" {
		t.Fatalf("created agent = %#v", created)
	}
	var memory struct {
		Body string `json:"body"`
	}
	getJSON(t, ts.URL+"/api/workspaces/"+created.ConfigWorkspaceID+"/files?path=memory.md", bootstrap.SessionToken, http.StatusOK, &memory)
	if !strings.Contains(memory.Body, "Name: Planner") || !strings.Contains(memory.Body, "Description: Plans implementation work") {
		t.Fatalf("memory.md = %q", memory.Body)
	}
	var agentsFile struct {
		Body string `json:"body"`
	}
	getJSON(t, ts.URL+"/api/workspaces/"+created.ConfigWorkspaceID+"/files?path=AGENTS.md", bootstrap.SessionToken, http.StatusOK, &agentsFile)
	if !strings.Contains(agentsFile.Body, "Name: Planner") || !strings.Contains(agentsFile.Body, "Description: Plans implementation work") {
		t.Fatalf("AGENTS.md = %q", agentsFile.Body)
	}
	var claudeFile struct {
		Body string `json:"body"`
	}
	getJSON(t, ts.URL+"/api/workspaces/"+created.ConfigWorkspaceID+"/files?path=CLAUDE.md", bootstrap.SessionToken, http.StatusOK, &claudeFile)
	if strings.TrimSpace(claudeFile.Body) != "@AGENTS.md" {
		t.Fatalf("CLAUDE.md = %q, want @AGENTS.md", claudeFile.Body)
	}

	var updated domain.Agent
	patchJSON(t, ts.URL+"/api/agents/"+created.ID, bootstrap.SessionToken, map[string]any{
		"description": "Updated planner",
		"effort":      "high",
		"fast_mode":   false,
	}, http.StatusOK, &updated)
	if updated.Effort != "high" || updated.Model != "gpt-test" || updated.FastMode || updated.Description != "Updated planner" {
		t.Fatalf("updated agent = %#v", updated)
	}

	deleteJSON(t, ts.URL+"/api/agents/"+created.ID, bootstrap.SessionToken, http.StatusNoContent)

	var recreated domain.Agent
	postJSON(t, ts.URL+"/api/organizations/"+bootstrap.Organization.ID+"/agents", bootstrap.SessionToken, map[string]any{
		"name":   "Planner",
		"handle": "planner",
		"kind":   "codex",
	}, http.StatusOK, &recreated)
	if recreated.ID == created.ID || recreated.Handle != "planner" {
		t.Fatalf("recreated agent = %#v, deleted agent = %#v", recreated, created)
	}
}

func TestHTTPAgentChannels(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	var channels []app.AgentChannelContext
	getJSON(t, ts.URL+"/api/agents/"+bootstrap.Agent.ID+"/channels", bootstrap.SessionToken, http.StatusOK, &channels)
	if len(channels) != 1 || channels[0].Channel.ID != bootstrap.Channel.ID {
		t.Fatalf("channels = %#v, want bootstrap channel", channels)
	}
	if channels[0].RunWorkspace.ID != bootstrap.ProjectWorkspace.ID {
		t.Fatalf("run workspace = %q, want %q", channels[0].RunWorkspace.ID, bootstrap.ProjectWorkspace.ID)
	}

	getJSON(t, ts.URL+"/api/agents/not-a-real-agent/channels", bootstrap.SessionToken, http.StatusNotFound, nil)
}

func TestHTTPSlashCommandErrorsAndSuccess(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	postJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body": "/does-not-exist",
	}, http.StatusBadRequest, nil)

	var message domain.Message
	postJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body": "/effort high",
	}, http.StatusOK, &message)
	if message.SenderType != domain.SenderSystem || !strings.Contains(message.Body, "effort") {
		t.Fatalf("slash command response = %#v", message)
	}
}

func TestHTTPMessagesCanBeUpdatedAndDeleted(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	var sent domain.Message
	postJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body": "draft text",
	}, http.StatusOK, &sent)

	var updated domain.Message
	patchJSON(t, ts.URL+"/api/messages/"+sent.ID, bootstrap.SessionToken, map[string]string{
		"body": "edited text",
	}, http.StatusOK, &updated)
	if updated.ID != sent.ID || updated.Body != "edited text" {
		t.Fatalf("updated message = %#v", updated)
	}

	var messages []domain.Message
	getJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, http.StatusOK, &messages)
	var foundEdited bool
	for _, message := range messages {
		if message.ID == sent.ID {
			foundEdited = message.Body == "edited text"
		}
	}
	if !foundEdited {
		t.Fatalf("messages after update = %#v", messages)
	}

	deleteJSON(t, ts.URL+"/api/messages/"+sent.ID, bootstrap.SessionToken, http.StatusNoContent)
	getJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, http.StatusOK, &messages)
	for _, message := range messages {
		if message.ID == sent.ID {
			t.Fatalf("deleted message still listed: %#v", messages)
		}
	}
}

func TestHTTPLoginCanResumeAfterBootstrap(t *testing.T) {
	ts := newTestServer(t)

	var first app.AuthResult
	postJSON(t, ts.URL+"/api/auth/login", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &first)
	if first.SessionToken == "" {
		t.Fatal("first session_token is empty")
	}

	var second app.AuthResult
	postJSON(t, ts.URL+"/api/auth/login", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Ignored",
	}, http.StatusOK, &second)
	if second.SessionToken == "" || second.SessionToken == first.SessionToken {
		t.Fatalf("second session_token = %q, first = %q", second.SessionToken, first.SessionToken)
	}
	if second.User.ID != first.User.ID {
		t.Fatalf("second user = %#v, want %#v", second.User, first.User)
	}

	var me domain.User
	getJSON(t, ts.URL+"/api/me", second.SessionToken, http.StatusOK, &me)
	if me.ID != first.User.ID {
		t.Fatalf("me = %#v, want user id %q", me, first.User.ID)
	}
}

func TestHTTPMeRequiresBearerToken(t *testing.T) {
	ts := newTestServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestHTTPSendMessageRejectsUnknownChannelWithoutCreatingOrphan(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	postJSON(t, ts.URL+"/api/conversations/channel/not-a-real-channel/messages", bootstrap.SessionToken, map[string]string{
		"body": "orphan",
	}, http.StatusNotFound, nil)

	getJSON(t, ts.URL+"/api/conversations/channel/not-a-real-channel/messages", bootstrap.SessionToken, http.StatusNotFound, nil)
}

func TestHTTPChannelsRejectsOrganizationOutsideAuthenticatedMemberships(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	getJSON(t, ts.URL+"/api/organizations/not-a-real-org/channels", bootstrap.SessionToken, http.StatusNotFound, nil)
}

func TestHTTPNotificationSettingsAuthorizeValidateAndRedactSecret(t *testing.T) {
	ts := newTestServer(t)
	webhookCalls := make(chan http.Header, 1)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalls <- r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhookServer.Close()

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	settingsURL := ts.URL + "/api/organizations/" + bootstrap.Organization.ID + "/notification-settings"
	getJSON(t, settingsURL, "", http.StatusUnauthorized, nil)
	getJSON(t, ts.URL+"/api/organizations/not-a-real-org/notification-settings", bootstrap.SessionToken, http.StatusNotFound, nil)

	var settings map[string]any
	getJSON(t, settingsURL, bootstrap.SessionToken, http.StatusOK, &settings)
	if settings["webhook_enabled"] != false || settings["webhook_secret_configured"] != false {
		t.Fatalf("default notification settings = %#v", settings)
	}

	putJSON(t, settingsURL, bootstrap.SessionToken, map[string]any{
		"webhook_enabled": true,
		"webhook_url":     "ftp://example.com/hook",
	}, http.StatusBadRequest, nil)

	putJSON(t, settingsURL, bootstrap.SessionToken, map[string]any{
		"webhook_enabled": true,
		"webhook_url":     webhookServer.URL,
		"webhook_secret":  "secret-value",
	}, http.StatusOK, &settings)
	if settings["webhook_url"] != webhookServer.URL || settings["webhook_secret_configured"] != true {
		t.Fatalf("saved notification settings = %#v", settings)
	}
	if _, ok := settings["webhook_secret"]; ok {
		t.Fatalf("response leaked webhook_secret: %#v", settings)
	}

	postJSON(t, settingsURL+"/test", bootstrap.SessionToken, map[string]any{}, http.StatusOK, nil)
	select {
	case headers := <-webhookCalls:
		if headers.Get("X-AgentX-Event") != app.AgentMessageCreatedWebhookEvent {
			t.Fatalf("X-AgentX-Event = %q", headers.Get("X-AgentX-Event"))
		}
		if headers.Get("X-AgentX-Signature") == "" {
			t.Fatal("X-AgentX-Signature is empty")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook test call")
	}
}

func TestHTTPWorkspaceFilesRejectTraversalAndRoundTripText(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	fileURL := ts.URL + "/api/workspaces/" + bootstrap.Workspace.ID + "/files?path=memory.md"
	putJSON(t, fileURL, bootstrap.SessionToken, map[string]string{"body": "remember this"}, http.StatusOK, nil)

	var file struct {
		Body string `json:"body"`
	}
	getJSON(t, fileURL, bootstrap.SessionToken, http.StatusOK, &file)
	if file.Body != "remember this" {
		t.Fatalf("file body = %q, want remembered text", file.Body)
	}

	getJSON(t, ts.URL+"/api/workspaces/"+bootstrap.Workspace.ID+"/files?path=../secret", bootstrap.SessionToken, http.StatusBadRequest, nil)
	putJSON(t, ts.URL+"/api/workspaces/"+bootstrap.Workspace.ID+"/files?path=/tmp/secret", bootstrap.SessionToken, map[string]string{"body": "bad"}, http.StatusBadRequest, nil)
	deleteJSON(t, ts.URL+"/api/workspaces/"+bootstrap.Workspace.ID+"/files?path=/tmp/secret", bootstrap.SessionToken, http.StatusBadRequest)
	deleteJSON(t, fileURL, bootstrap.SessionToken, http.StatusNoContent)
	getJSON(t, fileURL, bootstrap.SessionToken, http.StatusNotFound, nil)
}

func TestHTTPWorkspaceMetadataAndProjectWorkspacePathUpdate(t *testing.T) {
	ts := newTestServer(t)

	var bootstrap app.BootstrapResult
	postJSON(t, ts.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	var workspace domain.Workspace
	getJSON(t, ts.URL+"/api/workspaces/"+bootstrap.ProjectWorkspace.ID, bootstrap.SessionToken, http.StatusOK, &workspace)
	if workspace.ID != bootstrap.ProjectWorkspace.ID || workspace.Path != bootstrap.ProjectWorkspace.Path {
		t.Fatalf("workspace = %#v, want project workspace %#v", workspace, bootstrap.ProjectWorkspace)
	}

	nextPath := bootstrap.ProjectWorkspace.Path + "-next"
	var project domain.Project
	patchJSON(t, ts.URL+"/api/projects/"+bootstrap.Project.ID, bootstrap.SessionToken, map[string]string{
		"name":           "Renamed",
		"workspace_path": nextPath,
	}, http.StatusOK, &project)
	if project.Name != "Renamed" {
		t.Fatalf("project name = %q, want Renamed", project.Name)
	}

	getJSON(t, ts.URL+"/api/workspaces/"+project.WorkspaceID, bootstrap.SessionToken, http.StatusOK, &workspace)
	if workspace.Path != nextPath {
		t.Fatalf("workspace path = %q, want %q", workspace.Path, nextPath)
	}
}

func TestHTTPBoundNonChannelConversationsCanSendAndListMessages(t *testing.T) {
	env := newTestEnv(t)

	var bootstrap app.BootstrapResult
	postJSON(t, env.server.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	for _, conversationType := range []domain.ConversationType{domain.ConversationThread, domain.ConversationDM} {
		conversationID := string(conversationType) + "-conversation"
		now := time.Now().UTC()
		if err := env.store.Bindings().Upsert(context.Background(), domain.ConversationBinding{
			ID:               id.New("bnd"),
			OrganizationID:   bootstrap.Organization.ID,
			ConversationType: conversationType,
			ConversationID:   conversationID,
			AgentID:          bootstrap.Agent.ID,
			WorkspaceID:      bootstrap.Workspace.ID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			t.Fatal(err)
		}

		body := "hello " + string(conversationType)
		var sent domain.Message
		postJSON(t, env.server.URL+"/api/conversations/"+string(conversationType)+"/"+conversationID+"/messages", bootstrap.SessionToken, map[string]string{
			"body": body,
		}, http.StatusOK, &sent)
		if sent.OrganizationID != bootstrap.Organization.ID {
			t.Fatalf("%s sent organization_id = %q, want %q", conversationType, sent.OrganizationID, bootstrap.Organization.ID)
		}

		var messages []domain.Message
		requireEventually(t, time.Second, func() bool {
			getJSON(t, env.server.URL+"/api/conversations/"+string(conversationType)+"/"+conversationID+"/messages", bootstrap.SessionToken, http.StatusOK, &messages)
			var sawUser bool
			var sawBot bool
			for _, message := range messages {
				if message.ID == sent.ID && message.Body == body {
					sawUser = true
				}
				if message.Body == "Echo: "+body && message.SenderType == domain.SenderBot {
					sawBot = true
				}
			}
			return sawUser && sawBot
		})
	}
}

func TestHTTPSendReplyMessageReturnsResolvedReferenceAndRejectsInvalidTarget(t *testing.T) {
	env := newTestEnv(t)

	var bootstrap app.BootstrapResult
	postJSON(t, env.server.URL+"/api/auth/bootstrap", "", app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &bootstrap)

	var original domain.Message
	postJSON(t, env.server.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body": "original over http",
	}, http.StatusOK, &original)

	var reply domain.Message
	postJSON(t, env.server.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body":                "reply over http",
		"reply_to_message_id": original.ID,
	}, http.StatusOK, &reply)
	if reply.ReplyToMessageID != original.ID {
		t.Fatalf("reply_to_message_id = %q, want %q", reply.ReplyToMessageID, original.ID)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.Deleted || reply.ReplyTo.Body != original.Body {
		t.Fatalf("reply_to = %#v, want resolved original", reply.ReplyTo)
	}

	postJSON(t, env.server.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body":                "bad reply",
		"reply_to_message_id": "msg_missing",
	}, http.StatusBadRequest, nil)
}

func TestWebSocketReceivesMessageCreated(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot, err := env.app.Bootstrap(ctx, app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/ws?token=" + boot.SessionToken
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","organization_id":"`+boot.Organization.ID+`","conversation_id":"`+boot.Channel.ID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	requireWebSocketSubscribed(t, ctx, conn)
	requireWebSocketHistoryCompleted(t, ctx, conn, false)

	_, err = env.app.SendMessage(ctx, app.SendMessageRequest{
		UserID:           boot.User.ID,
		OrganizationID:   boot.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   boot.Channel.ID,
		Body:             "hello ws",
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), string(domain.EventMessageCreated)) && strings.Contains(string(payload), "Echo: hello ws") {
			return
		}
	}
}

func TestWebSocketStreamsMessageHistoryBeforeLiveEvents(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot, err := env.app.Bootstrap(ctx, app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	for seq := 1; seq <= 60; seq++ {
		createWebSocketHistoryMessage(t, env, boot.Organization.ID, domain.ConversationChannel, boot.Channel.ID, seq)
	}

	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/ws?token=" + boot.SessionToken
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","organization_id":"`+boot.Organization.ID+`","conversation_type":"channel","conversation_id":"`+boot.Channel.ID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	requireWebSocketSubscribed(t, ctx, conn)

	liveMessage := webSocketHistoryMessage(boot.Organization.ID, domain.ConversationChannel, boot.Channel.ID, 61)
	env.bus.Publish(domain.Event{
		ID:               id.New("evt"),
		Type:             domain.EventMessageCreated,
		OrganizationID:   boot.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   boot.Channel.ID,
		Payload:          domain.MessageCreatedPayload{Message: liveMessage},
		CreatedAt:        liveMessage.CreatedAt,
	})

	frame := requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryStarted)
	if frame.OrganizationID != boot.Organization.ID || frame.ConversationType != domain.ConversationChannel || frame.ConversationID != boot.Channel.ID {
		t.Fatalf("history started frame scope = %#v", frame)
	}

	var allHistory []domain.Message
	for _, wantSize := range []int{25, 25, 10} {
		frame = requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryChunk)
		var payload domain.MessageHistoryChunkPayload
		unmarshalWebSocketPayload(t, frame, &payload)
		if len(payload.Messages) != wantSize {
			t.Fatalf("history chunk size = %d, want %d", len(payload.Messages), wantSize)
		}
		allHistory = append(allHistory, payload.Messages...)
	}
	if len(allHistory) != 60 {
		t.Fatalf("history message count = %d, want 60", len(allHistory))
	}
	if allHistory[0].ID != "msg_history_001" || allHistory[len(allHistory)-1].ID != "msg_history_060" {
		t.Fatalf("history window = %s..%s, want msg_history_001..msg_history_060", allHistory[0].ID, allHistory[len(allHistory)-1].ID)
	}

	frame = requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryCompleted)
	var completed domain.MessageHistoryCompletedPayload
	unmarshalWebSocketPayload(t, frame, &completed)
	if completed.HasMore {
		t.Fatal("history completed has_more = true, want false")
	}

	frame = requireWebSocketEventType(t, ctx, conn, domain.EventMessageCreated)
	var created domain.MessageCreatedPayload
	unmarshalWebSocketPayload(t, frame, &created)
	if created.Message.ID != liveMessage.ID {
		t.Fatalf("live message id = %q, want %q", created.Message.ID, liveMessage.ID)
	}
}

func TestWebSocketHistoryUsesLatestRecentWindow(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot, err := env.app.Bootstrap(ctx, app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	for seq := 1; seq <= 105; seq++ {
		createWebSocketHistoryMessage(t, env, boot.Organization.ID, domain.ConversationChannel, boot.Channel.ID, seq)
	}

	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/ws?token=" + boot.SessionToken
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","organization_id":"`+boot.Organization.ID+`","conversation_type":"channel","conversation_id":"`+boot.Channel.ID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	requireWebSocketSubscribed(t, ctx, conn)
	requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryStarted)

	var allHistory []domain.Message
	for len(allHistory) < 100 {
		frame := requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryChunk)
		var payload domain.MessageHistoryChunkPayload
		unmarshalWebSocketPayload(t, frame, &payload)
		allHistory = append(allHistory, payload.Messages...)
	}
	if len(allHistory) != 100 {
		t.Fatalf("history message count = %d, want 100", len(allHistory))
	}
	if allHistory[0].ID != "msg_history_006" || allHistory[len(allHistory)-1].ID != "msg_history_105" {
		t.Fatalf("history window = %s..%s, want msg_history_006..msg_history_105", allHistory[0].ID, allHistory[len(allHistory)-1].ID)
	}

	frame := requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryCompleted)
	var completed domain.MessageHistoryCompletedPayload
	unmarshalWebSocketPayload(t, frame, &completed)
	if !completed.HasMore {
		t.Fatal("history completed has_more = false, want true")
	}

	err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"load_history","organization_id":"`+boot.Organization.ID+`","conversation_type":"channel","conversation_id":"`+boot.Channel.ID+`","before":"`+allHistory[0].CreatedAt.Format(time.RFC3339Nano)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	frame = requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryStarted)
	var started domain.MessageHistoryStartedPayload
	unmarshalWebSocketPayload(t, frame, &started)
	if started.Before != allHistory[0].CreatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("history started before = %q, want %q", started.Before, allHistory[0].CreatedAt.Format(time.RFC3339Nano))
	}
	frame = requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryChunk)
	var older domain.MessageHistoryChunkPayload
	unmarshalWebSocketPayload(t, frame, &older)
	if len(older.Messages) != 5 || older.Messages[0].ID != "msg_history_001" || older.Messages[4].ID != "msg_history_005" {
		t.Fatalf("older history messages = %#v", older.Messages)
	}
	frame = requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryCompleted)
	unmarshalWebSocketPayload(t, frame, &completed)
	if completed.HasMore || completed.Before != allHistory[0].CreatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("older history completion = %#v", completed)
	}
}

func TestWebSocketStreamsHistoryForBoundNonChannelConversations(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot, err := env.app.Bootstrap(ctx, app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	for index, conversationType := range []domain.ConversationType{domain.ConversationThread, domain.ConversationDM} {
		t.Run(string(conversationType), func(t *testing.T) {
			conversationID := string(conversationType) + "_ws_conversation"
			now := time.Now().UTC()
			if err := env.store.Bindings().Upsert(ctx, domain.ConversationBinding{
				ID:               id.New("bnd"),
				OrganizationID:   boot.Organization.ID,
				ConversationType: conversationType,
				ConversationID:   conversationID,
				AgentID:          boot.Agent.ID,
				WorkspaceID:      boot.Workspace.ID,
				CreatedAt:        now,
				UpdatedAt:        now,
			}); err != nil {
				t.Fatal(err)
			}
			message := createWebSocketHistoryMessage(t, env, boot.Organization.ID, conversationType, conversationID, index+1)

			wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/ws?token=" + boot.SessionToken
			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","organization_id":"`+boot.Organization.ID+`","conversation_type":"`+string(conversationType)+`","conversation_id":"`+conversationID+`"}`))
			if err != nil {
				t.Fatal(err)
			}
			requireWebSocketSubscribed(t, ctx, conn)
			requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryStarted)

			frame := requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryChunk)
			var payload domain.MessageHistoryChunkPayload
			unmarshalWebSocketPayload(t, frame, &payload)
			if len(payload.Messages) != 1 || payload.Messages[0].ID != message.ID || payload.Messages[0].ConversationType != conversationType {
				t.Fatalf("history messages = %#v, want %s message %q", payload.Messages, conversationType, message.ID)
			}

			requireWebSocketHistoryCompletionOnly(t, ctx, conn, false)
		})
	}
}

func TestWebSocketRejectsUnauthorizedSubscriptions(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot, err := env.app.Bootstrap(ctx, app.BootstrapRequest{
		AdminToken:  "secret",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name           string
		organizationID string
		conversationID string
	}{
		{name: "mismatched organization", organizationID: "org_unknown", conversationID: boot.Channel.ID},
		{name: "unknown conversation", organizationID: boot.Organization.ID, conversationID: "chn_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/ws?token=" + boot.SessionToken
			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","organization_id":"`+tc.organizationID+`","conversation_id":"`+tc.conversationID+`"}`))
			if err != nil {
				t.Fatal(err)
			}

			publishCtx, stopPublish := context.WithCancel(ctx)
			defer stopPublish()
			go publishMatchingWebSocketEvents(publishCtx, env.bus, tc.organizationID, tc.conversationID)

			_, payload, err := conn.Read(ctx)
			if err == nil {
				t.Fatalf("read payload %s, want unauthorized subscription close", string(payload))
			}
		})
	}
}

func TestWebSocketRejectsMissingOrInvalidToken(t *testing.T) {
	env := newTestEnv(t)
	baseURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/ws"

	for _, tc := range []struct {
		name string
		url  string
	}{
		{name: "missing", url: baseURL},
		{name: "invalid", url: baseURL + "?token=invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			conn, resp, err := websocket.Dial(ctx, tc.url, nil)
			if err == nil {
				conn.Close(websocket.StatusNormalClosure, "")
				t.Fatal("dial succeeded, want unauthorized")
			}
			if resp == nil {
				t.Fatal("missing HTTP response")
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

type testEnv struct {
	server *httptest.Server
	store  *sqlitestore.Store
	app    *app.App
	bus    *eventbus.Bus
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newTestEnv(t).server
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()

	ctx := context.Background()
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
	a := app.New(st, bus, app.Options{AdminToken: "secret", DataDir: t.TempDir()})
	ts := httptest.NewServer(NewRouter(a, bus))
	t.Cleanup(ts.Close)
	return testEnv{server: ts, store: st, app: a, bus: bus}
}

func requireWebSocketSubscribed(t *testing.T, ctx context.Context, conn *websocket.Conn) {
	t.Helper()

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ack struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Type != "subscribed" {
		t.Fatalf("ack type = %q, want subscribed", ack.Type)
	}
}

type webSocketEventFrame struct {
	Type             domain.EventType        `json:"type"`
	OrganizationID   string                  `json:"organization_id"`
	ConversationType domain.ConversationType `json:"conversation_type"`
	ConversationID   string                  `json:"conversation_id"`
	Payload          json.RawMessage         `json:"payload"`
}

func requireWebSocketEventType(t *testing.T, ctx context.Context, conn *websocket.Conn, eventType domain.EventType) webSocketEventFrame {
	t.Helper()

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var frame webSocketEventFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		t.Fatal(err)
	}
	if frame.Type != eventType {
		t.Fatalf("event type = %q, want %q; payload = %s", frame.Type, eventType, string(payload))
	}
	return frame
}

func requireWebSocketHistoryCompleted(t *testing.T, ctx context.Context, conn *websocket.Conn, hasMore bool) {
	t.Helper()

	requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryStarted)
	requireWebSocketHistoryCompletionOnly(t, ctx, conn, hasMore)
}

func requireWebSocketHistoryCompletionOnly(t *testing.T, ctx context.Context, conn *websocket.Conn, hasMore bool) {
	t.Helper()

	for {
		frame := requireWebSocketEventTypeAny(t, ctx, conn)
		switch frame.Type {
		case domain.EventMessageHistoryChunk:
			continue
		case domain.EventMessageHistoryCompleted:
			var payload domain.MessageHistoryCompletedPayload
			unmarshalWebSocketPayload(t, frame, &payload)
			if payload.HasMore != hasMore {
				t.Fatalf("history completed has_more = %v, want %v", payload.HasMore, hasMore)
			}
			return
		default:
			t.Fatalf("event type = %q, want history chunk or completion", frame.Type)
		}
	}
}

func requireWebSocketEventTypeAny(t *testing.T, ctx context.Context, conn *websocket.Conn) webSocketEventFrame {
	t.Helper()

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var frame webSocketEventFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		t.Fatal(err)
	}
	return frame
}

func unmarshalWebSocketPayload(t *testing.T, frame webSocketEventFrame, target any) {
	t.Helper()

	if err := json.Unmarshal(frame.Payload, target); err != nil {
		t.Fatal(err)
	}
}

func createWebSocketHistoryMessage(t *testing.T, env testEnv, organizationID string, conversationType domain.ConversationType, conversationID string, seq int) domain.Message {
	t.Helper()

	message := webSocketHistoryMessage(organizationID, conversationType, conversationID, seq)
	if err := env.store.Messages().Create(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	return message
}

func webSocketHistoryMessage(organizationID string, conversationType domain.ConversationType, conversationID string, seq int) domain.Message {
	return domain.Message{
		ID:               fmt.Sprintf("msg_history_%03d", seq),
		OrganizationID:   organizationID,
		ConversationType: conversationType,
		ConversationID:   conversationID,
		SenderType:       domain.SenderUser,
		SenderID:         "usr_history",
		Kind:             domain.MessageText,
		Body:             fmt.Sprintf("history %03d", seq),
		CreatedAt:        time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Second),
	}
}

func publishMatchingWebSocketEvents(ctx context.Context, bus *eventbus.Bus, organizationID string, conversationID string) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		bus.Publish(domain.Event{
			ID:               id.New("evt"),
			Type:             domain.EventMessageCreated,
			OrganizationID:   organizationID,
			ConversationType: domain.ConversationChannel,
			ConversationID:   conversationID,
			Payload: domain.MessageCreatedPayload{Message: domain.Message{
				ID:               id.New("msg"),
				OrganizationID:   organizationID,
				ConversationType: domain.ConversationChannel,
				ConversationID:   conversationID,
				SenderType:       domain.SenderUser,
				Kind:             domain.MessageText,
				Body:             "unauthorized event",
				CreatedAt:        time.Now().UTC(),
			}},
			CreatedAt: time.Now().UTC(),
		})

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func postJSON(t *testing.T, url string, token string, body any, wantStatus int, dst any) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	doJSON(t, req, wantStatus, dst)
}

func putJSON(t *testing.T, url string, token string, body any, wantStatus int, dst any) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	doJSON(t, req, wantStatus, dst)
}

func patchJSON(t *testing.T, url string, token string, body any, wantStatus int, dst any) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	doJSON(t, req, wantStatus, dst)
}

func deleteJSON(t *testing.T, url string, token string, wantStatus int) {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	doJSON(t, req, wantStatus, nil)
}

func getJSON(t *testing.T, url string, token string, wantStatus int, dst any) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	doJSON(t, req, wantStatus, dst)
}

func doJSON(t *testing.T, req *http.Request, wantStatus int, dst any) {
	t.Helper()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d", req.Method, req.URL.Path, resp.StatusCode, wantStatus)
	}
	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			t.Fatal(err)
		}
	}
}

func requireEventually(t *testing.T, timeout time.Duration, check func() bool) {
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
