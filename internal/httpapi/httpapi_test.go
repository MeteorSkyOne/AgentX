package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/id"
	"github.com/meteorsky/agentx/internal/store"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
	"golang.org/x/crypto/bcrypt"
	"nhooyr.io/websocket"
)

func TestHTTPBootstrapAuthAndMessagesFlow(t *testing.T) {
	ts := newTestServer(t)

	bootstrap := setupHTTP(t, ts.URL)
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

	bootstrap := setupHTTP(t, ts.URL)

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

	bootstrap := setupHTTP(t, ts.URL)

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

	bootstrap := setupHTTP(t, ts.URL)

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

	bootstrap := setupHTTP(t, ts.URL)

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

func TestHTTPMessageProcessDetailsAreLazyLoaded(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bootstrap := setupApp(t, ctx, env.app)

	message := lazyProcessTestMessage(bootstrap.Organization.ID, bootstrap.Channel.ID)
	if err := env.store.Messages().Create(ctx, message); err != nil {
		t.Fatal(err)
	}

	var messages []domain.Message
	getJSON(t, env.server.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, http.StatusOK, &messages)
	var listed domain.Message
	for _, item := range messages {
		if item.ID == message.ID {
			listed = item
			break
		}
	}
	if listed.ID == "" {
		t.Fatalf("messages = %#v, want %q", messages, message.ID)
	}
	process, ok := listed.Metadata["process"].([]any)
	if !ok || len(process) != 2 {
		t.Fatalf("listed process = %#v", listed.Metadata["process"])
	}
	call, ok := process[0].(map[string]any)
	if !ok {
		t.Fatalf("listed call = %#v", process[0])
	}
	if call["process_index"] != float64(0) || call["has_detail"] != true || call["tool_name"] != "Bash" {
		t.Fatalf("listed call meta = %#v", call)
	}
	if _, ok := call["input"]; ok {
		t.Fatalf("listed call leaked input: %#v", call)
	}
	if _, ok := call["raw"]; ok {
		t.Fatalf("listed call leaked raw: %#v", call)
	}
	result, ok := process[1].(map[string]any)
	if !ok {
		t.Fatalf("listed result = %#v", process[1])
	}
	if result["process_index"] != float64(1) || result["has_detail"] != true || result["status"] != "completed" {
		t.Fatalf("listed result meta = %#v", result)
	}
	if _, ok := result["output"]; ok {
		t.Fatalf("listed result leaked output: %#v", result)
	}

	var detail messageProcessItemDetail
	getJSON(t, env.server.URL+"/api/messages/"+message.ID+"/process-items/0", bootstrap.SessionToken, http.StatusOK, &detail)
	input, ok := detail.Item["input"].(map[string]any)
	if !ok || input["command"] != "pnpm test" {
		t.Fatalf("detail input = %#v", detail.Item["input"])
	}
	if detail.Result == nil || detail.Result["output"] != "tests passed" {
		t.Fatalf("detail result = %#v", detail.Result)
	}

	getJSON(t, env.server.URL+"/api/messages/"+message.ID+"/process-items/not-a-number", bootstrap.SessionToken, http.StatusBadRequest, nil)
	getJSON(t, env.server.URL+"/api/messages/"+message.ID+"/process-items/99", bootstrap.SessionToken, http.StatusNotFound, nil)
	getJSON(t, env.server.URL+"/api/messages/"+message.ID+"/process-items/0", "", http.StatusUnauthorized, nil)
}

func TestHTTPSetupLoginAndLogout(t *testing.T) {
	ts := newTestServer(t)

	var status app.AuthStatus
	getJSON(t, ts.URL+"/api/auth/status", "", http.StatusOK, &status)
	if !status.SetupRequired || !status.SetupTokenRequired {
		t.Fatalf("initial status = %#v", status)
	}

	var setupError errorResponse
	postJSON(t, ts.URL+"/api/auth/setup", "", app.SetupAdminRequest{
		SetupToken:  "secret",
		Username:    "meteorsky",
		Password:    "qwe123",
		DisplayName: "Meteorsky",
	}, http.StatusBadRequest, &setupError)
	if setupError.Error != "password must be at least 12 bytes" {
		t.Fatalf("setup error = %q, want password length message", setupError.Error)
	}

	var first app.AuthResult
	postJSON(t, ts.URL+"/api/auth/setup", "", app.SetupAdminRequest{
		SetupToken:  "secret",
		Username:    "meteorsky",
		Password:    "correct-password-123",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &first)
	if first.SessionToken == "" {
		t.Fatal("first session_token is empty")
	}
	getJSON(t, ts.URL+"/api/auth/status", "", http.StatusOK, &status)
	if status.SetupRequired || status.SetupTokenRequired {
		t.Fatalf("post-setup status = %#v", status)
	}
	postJSON(t, ts.URL+"/api/auth/setup", "", app.SetupAdminRequest{
		SetupToken:  "secret",
		Username:    "second",
		Password:    "correct-password-123",
		DisplayName: "Second",
	}, http.StatusConflict, nil)
	postJSON(t, ts.URL+"/api/auth/login", "", app.LoginRequest{
		Username: "meteorsky",
		Password: "wrong-password",
	}, http.StatusUnauthorized, nil)

	var second app.AuthResult
	postJSON(t, ts.URL+"/api/auth/login", "", app.LoginRequest{
		Username: "METEORSKY",
		Password: "correct-password-123",
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

	postJSON(t, ts.URL+"/api/auth/logout", second.SessionToken, map[string]any{}, http.StatusNoContent, nil)
	getJSON(t, ts.URL+"/api/me", second.SessionToken, http.StatusUnauthorized, nil)
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

func TestHTTPPreferencesAndMetricsEndpoints(t *testing.T) {
	ts := newTestServer(t)

	bootstrap := setupHTTP(t, ts.URL)

	var preferences map[string]bool
	getJSON(t, ts.URL+"/api/me/preferences", bootstrap.SessionToken, http.StatusOK, &preferences)
	if !preferences["show_ttft"] || !preferences["show_tps"] {
		t.Fatalf("default preferences = %#v", preferences)
	}
	putJSON(t, ts.URL+"/api/me/preferences", bootstrap.SessionToken, map[string]bool{
		"show_ttft": false,
		"show_tps":  true,
	}, http.StatusOK, &preferences)
	if preferences["show_ttft"] || !preferences["show_tps"] {
		t.Fatalf("updated preferences = %#v", preferences)
	}

	var sent domain.Message
	postJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body": "metrics",
	}, http.StatusOK, &sent)

	var rows []domain.AgentRunMetric
	requireEventually(t, time.Second, func() bool {
		getJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/metrics?provider=fake", bootstrap.SessionToken, http.StatusOK, &rows)
		return len(rows) == 1 && rows[0].Provider == domain.AgentKindFake && rows[0].MessageID == sent.ID
	})
	if rows[0].ProjectName != bootstrap.Project.Name || rows[0].ChannelName != bootstrap.Channel.Name {
		t.Fatalf("metrics scope names = %#v, want project %q channel %q", rows[0], bootstrap.Project.Name, bootstrap.Channel.Name)
	}
	getJSON(t, ts.URL+"/api/channels/"+bootstrap.Channel.ID+"/metrics", bootstrap.SessionToken, http.StatusOK, &rows)
	if len(rows) != 1 {
		t.Fatalf("channel metrics rows = %#v", rows)
	}
	getJSON(t, ts.URL+"/api/projects/"+bootstrap.Project.ID+"/metrics", bootstrap.SessionToken, http.StatusOK, &rows)
	if len(rows) != 1 {
		t.Fatalf("project metrics rows = %#v", rows)
	}
	getJSON(t, ts.URL+"/api/projects/"+bootstrap.Project.ID+"/metrics?group=agent", bootstrap.SessionToken, http.StatusOK, &rows)
	if len(rows) != 1 || rows[0].RunCount != 1 || rows[0].AgentID != bootstrap.Agent.ID {
		t.Fatalf("project metric summaries = %#v", rows)
	}
	getJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/metrics?provider=bad", bootstrap.SessionToken, http.StatusBadRequest, nil)
	getJSON(t, ts.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/metrics?group=bad", bootstrap.SessionToken, http.StatusBadRequest, nil)
	getJSON(t, ts.URL+"/api/me/preferences", "", http.StatusUnauthorized, nil)
}

func TestHTTPSendMessageRejectsUnknownChannelWithoutCreatingOrphan(t *testing.T) {
	ts := newTestServer(t)

	bootstrap := setupHTTP(t, ts.URL)

	postJSON(t, ts.URL+"/api/conversations/channel/not-a-real-channel/messages", bootstrap.SessionToken, map[string]string{
		"body": "orphan",
	}, http.StatusNotFound, nil)

	getJSON(t, ts.URL+"/api/conversations/channel/not-a-real-channel/messages", bootstrap.SessionToken, http.StatusNotFound, nil)
}

func TestHTTPChannelsRejectsOrganizationOutsideAuthenticatedMemberships(t *testing.T) {
	ts := newTestServer(t)

	bootstrap := setupHTTP(t, ts.URL)

	getJSON(t, ts.URL+"/api/organizations/not-a-real-org/channels", bootstrap.SessionToken, http.StatusNotFound, nil)
}

func TestHTTPServerSettingsAuthorizeAndPersistTLS(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	bootstrap := setupApp(t, ctx, env.app)

	settingsURL := env.server.URL + "/api/organizations/" + bootstrap.Organization.ID + "/server-settings"
	getJSON(t, settingsURL, "", http.StatusUnauthorized, nil)
	getJSON(t, env.server.URL+"/api/organizations/not-a-real-org/server-settings", bootstrap.SessionToken, http.StatusNotFound, nil)

	memberPassword := "member-password-123"
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(memberPassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	passwordUpdatedAt := now
	member := domain.User{
		ID:                "usr_member",
		Username:          "member",
		DisplayName:       "Member",
		PasswordHash:      string(passwordHash),
		PasswordUpdatedAt: &passwordUpdatedAt,
		CreatedAt:         now,
	}
	if err := env.store.Users().Create(ctx, member); err != nil {
		t.Fatal(err)
	}
	if err := env.store.Organizations().AddMember(ctx, bootstrap.Organization.ID, member.ID, domain.RoleMember); err != nil {
		t.Fatal(err)
	}
	memberLogin, err := env.app.Login(ctx, app.LoginRequest{Username: "member", Password: memberPassword})
	if err != nil {
		t.Fatal(err)
	}
	getJSON(t, settingsURL, memberLogin.SessionToken, http.StatusForbidden, nil)

	var settings app.ServerSettings
	getJSON(t, settingsURL, bootstrap.SessionToken, http.StatusOK, &settings)
	if settings.ListenIP != "127.0.0.1" || settings.ListenPort != 8080 || settings.TLS.Enabled || settings.TLS.ListenPort != 8443 {
		t.Fatalf("default server settings = %#v", settings)
	}

	putJSON(t, settingsURL, bootstrap.SessionToken, map[string]any{
		"listen_ip":   "127.0.0.1",
		"listen_port": 70000,
		"tls":         map[string]any{"enabled": false, "cert_file": "", "key_file": ""},
	}, http.StatusBadRequest, nil)

	putJSON(t, settingsURL, bootstrap.SessionToken, map[string]any{
		"listen_ip":   "0.0.0.0",
		"listen_port": 8080,
		"tls": map[string]any{
			"enabled":     true,
			"listen_port": 9443,
			"cert_file":   "",
			"key_file":    "",
			"cert_pem":    "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----",
			"key_pem":     "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----",
		},
	}, http.StatusOK, &settings)

	if settings.ListenIP != "0.0.0.0" || settings.ListenPort != 8080 || !settings.TLS.Enabled || settings.TLS.ListenPort != 9443 || !settings.RestartRequired {
		t.Fatalf("updated server settings = %#v", settings)
	}
	if filepath.Base(settings.TLS.CertFile) != "cert" || filepath.Base(settings.TLS.KeyFile) != "privkey" {
		t.Fatalf("TLS files = %#v", settings.TLS)
	}
	keyInfo, err := os.Stat(settings.TLS.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %v, want 0600", keyInfo.Mode().Perm())
	}
}

func TestHTTPNotificationSettingsAuthorizeValidateAndRedactSecret(t *testing.T) {
	ts := newTestServer(t)
	webhookCalls := make(chan http.Header, 1)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalls <- r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhookServer.Close()

	bootstrap := setupHTTP(t, ts.URL)

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

	bootstrap := setupHTTP(t, ts.URL)

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

	bootstrap := setupHTTP(t, ts.URL)

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

	bootstrap := setupHTTP(t, env.server.URL)

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

	bootstrap := setupHTTP(t, env.server.URL)

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

func TestHTTPMultipartAttachmentsCanBeSentDownloadedAndAuthorized(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bootstrap := setupHTTP(t, env.server.URL)

	var sent domain.Message
	postMultipartMessage(t, env.server.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body": "",
	}, []multipartTestFile{{
		Field:       "files[]",
		Filename:    "notes.txt",
		ContentType: "text/plain",
		Body:        []byte("hello attachment"),
	}}, http.StatusOK, &sent)
	if sent.Body != "" || len(sent.Attachments) != 1 {
		t.Fatalf("sent message = %#v, want attachment-only message", sent)
	}
	attachment := sent.Attachments[0]
	if attachment.Filename != "notes.txt" || attachment.Kind != domain.MessageAttachmentText || attachment.StoragePath != "" {
		t.Fatalf("attachment response = %#v, want redacted text attachment", attachment)
	}

	status, headers, body := getRaw(t, env.server.URL+"/api/attachments/"+attachment.ID+"/content", bootstrap.SessionToken)
	if status != http.StatusOK {
		t.Fatalf("attachment content status = %d, body = %s", status, string(body))
	}
	if string(body) != "hello attachment" {
		t.Fatalf("attachment body = %q", string(body))
	}
	if contentType := headers.Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", contentType)
	}
	if disposition := headers.Get("Content-Disposition"); !strings.Contains(disposition, "notes.txt") {
		t.Fatalf("content-disposition = %q, want filename", disposition)
	}
	if csp := headers.Get("Content-Security-Policy"); csp != "default-src 'none'; sandbox" {
		t.Fatalf("content-security-policy = %q, want locked down attachment policy", csp)
	}
	if cacheControl := headers.Get("Cache-Control"); cacheControl != "private, no-store" {
		t.Fatalf("cache-control = %q, want private, no-store", cacheControl)
	}

	var htmlMessage domain.Message
	postMultipartMessage(t, env.server.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages", bootstrap.SessionToken, map[string]string{
		"body": "html attachment",
	}, []multipartTestFile{{
		Field:       "files[]",
		Filename:    "page.html",
		ContentType: "text/html",
		Body:        []byte(`<script>window.evil = true</script>`),
	}}, http.StatusOK, &htmlMessage)
	if len(htmlMessage.Attachments) != 1 || htmlMessage.Attachments[0].ContentType != "text/html" {
		t.Fatalf("html attachment message = %#v, want text/html attachment", htmlMessage)
	}
	status, headers, _ = getRaw(t, env.server.URL+"/api/attachments/"+htmlMessage.Attachments[0].ID+"/content", bootstrap.SessionToken)
	if status != http.StatusOK {
		t.Fatalf("html attachment content status = %d", status)
	}
	if csp := headers.Get("Content-Security-Policy"); csp != "default-src 'none'; sandbox" {
		t.Fatalf("html content-security-policy = %q, want locked down attachment policy", csp)
	}

	otherOrg := domain.Organization{ID: "org_other_attachment", Name: "Other", CreatedAt: time.Now().UTC()}
	otherWorkspace := domain.Workspace{
		ID: "wks_other_attachment", OrganizationID: otherOrg.ID, Type: "project", Name: "Other Workspace",
		Path: filepath.Join(t.TempDir(), "other"), CreatedBy: bootstrap.User.ID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	otherProject := domain.Project{
		ID: "prj_other_attachment", OrganizationID: otherOrg.ID, Name: "Other", WorkspaceID: otherWorkspace.ID,
		CreatedBy: bootstrap.User.ID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	otherChannel := domain.Channel{
		ID: "chn_other_attachment", OrganizationID: otherOrg.ID, ProjectID: otherProject.ID,
		Type: domain.ChannelTypeText, Name: "Other", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	otherMessage := domain.Message{
		ID: "msg_other_attachment", OrganizationID: otherOrg.ID, ConversationType: domain.ConversationChannel,
		ConversationID: otherChannel.ID, SenderType: domain.SenderUser, SenderID: bootstrap.User.ID,
		Kind: domain.MessageText, Body: "other", CreatedAt: time.Now().UTC(),
	}
	otherPath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(otherPath, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	otherAttachment := domain.MessageAttachment{
		ID: "att_other_attachment", MessageID: otherMessage.ID, OrganizationID: otherOrg.ID,
		ConversationType: domain.ConversationChannel, ConversationID: otherChannel.ID,
		Filename: "secret.txt", ContentType: "text/plain", Kind: domain.MessageAttachmentText,
		SizeBytes: 6, StoragePath: otherPath, CreatedAt: time.Now().UTC(),
	}
	if err := env.store.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Organizations().Create(ctx, otherOrg); err != nil {
			return err
		}
		if err := tx.Workspaces().Create(ctx, otherWorkspace); err != nil {
			return err
		}
		if err := tx.Projects().Create(ctx, otherProject); err != nil {
			return err
		}
		if err := tx.Channels().Create(ctx, otherChannel); err != nil {
			return err
		}
		if err := tx.Messages().Create(ctx, otherMessage); err != nil {
			return err
		}
		return tx.MessageAttachments().Create(ctx, otherAttachment)
	}); err != nil {
		t.Fatal(err)
	}
	status, _, _ = getRaw(t, env.server.URL+"/api/attachments/"+otherAttachment.ID+"/content", bootstrap.SessionToken)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org attachment status = %d, want 404", status)
	}
}

func TestHTTPMalformedMultipartMessageReturnsStableError(t *testing.T) {
	env := newTestEnv(t)
	bootstrap := setupHTTP(t, env.server.URL)

	req, err := http.NewRequest(
		http.MethodPost,
		env.server.URL+"/api/conversations/channel/"+bootstrap.Channel.ID+"/messages",
		strings.NewReader("not a valid multipart body"),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+bootstrap.SessionToken)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=agentx")
	var response errorResponse
	doJSON(t, req, http.StatusBadRequest, &response)
	if response.Error != "malformed multipart form" {
		t.Fatalf("multipart error = %q, want stable malformed multipart form", response.Error)
	}
}

func TestWebSocketReceivesMessageCreated(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot := setupApp(t, ctx, env.app)

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

func TestWebSocketRedactsMessageCreatedProcessDetails(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot := setupApp(t, ctx, env.app)

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
	requireWebSocketHistoryCompleted(t, ctx, conn, false)

	message := lazyProcessTestMessage(boot.Organization.ID, boot.Channel.ID)
	env.bus.Publish(domain.Event{
		ID:               id.New("evt"),
		Type:             domain.EventMessageCreated,
		OrganizationID:   boot.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   boot.Channel.ID,
		Payload:          domain.MessageCreatedPayload{Message: message},
		CreatedAt:        message.CreatedAt,
	})

	frame := requireWebSocketEventType(t, ctx, conn, domain.EventMessageCreated)
	var payload domain.MessageCreatedPayload
	unmarshalWebSocketPayload(t, frame, &payload)
	process, ok := payload.Message.Metadata["process"].([]any)
	if !ok || len(process) != 2 {
		t.Fatalf("created process = %#v", payload.Message.Metadata["process"])
	}
	call := process[0].(map[string]any)
	if call["process_index"] != float64(0) || call["has_detail"] != true {
		t.Fatalf("created call meta = %#v", call)
	}
	if _, ok := call["input"]; ok {
		t.Fatalf("created event leaked input: %#v", call)
	}
	result := process[1].(map[string]any)
	if _, ok := result["output"]; ok {
		t.Fatalf("created event leaked output: %#v", result)
	}
}

func TestWebSocketHistoryRedactsProcessDetails(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot := setupApp(t, ctx, env.app)
	message := lazyProcessTestMessage(boot.Organization.ID, boot.Channel.ID)
	if err := env.store.Messages().Create(ctx, message); err != nil {
		t.Fatal(err)
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
	frame := requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryChunk)
	var payload domain.MessageHistoryChunkPayload
	unmarshalWebSocketPayload(t, frame, &payload)
	if len(payload.Messages) != 1 {
		t.Fatalf("history messages = %#v", payload.Messages)
	}
	process, ok := payload.Messages[0].Metadata["process"].([]any)
	if !ok || len(process) != 2 {
		t.Fatalf("history process = %#v", payload.Messages[0].Metadata["process"])
	}
	call := process[0].(map[string]any)
	if _, ok := call["input"]; ok {
		t.Fatalf("history leaked input: %#v", call)
	}
	result := process[1].(map[string]any)
	if _, ok := result["output"]; ok {
		t.Fatalf("history leaked output: %#v", result)
	}
	requireWebSocketEventType(t, ctx, conn, domain.EventMessageHistoryCompleted)
}

func TestWebSocketStreamsMessageHistoryBeforeLiveEvents(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	boot := setupApp(t, ctx, env.app)

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

	boot := setupApp(t, ctx, env.app)

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

	boot := setupApp(t, ctx, env.app)

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

	boot := setupApp(t, ctx, env.app)

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

func setupHTTP(t *testing.T, baseURL string) app.BootstrapResult {
	t.Helper()

	var auth app.AuthResult
	postJSON(t, baseURL+"/api/auth/setup", "", app.SetupAdminRequest{
		SetupToken:  "secret",
		Username:    "meteorsky",
		Password:    "correct-password-123",
		DisplayName: "Meteorsky",
	}, http.StatusOK, &auth)

	var orgs []domain.Organization
	getJSON(t, baseURL+"/api/organizations", auth.SessionToken, http.StatusOK, &orgs)
	if len(orgs) == 0 {
		t.Fatal("setup did not create an organization")
	}

	var projects []domain.Project
	getJSON(t, baseURL+"/api/organizations/"+orgs[0].ID+"/projects", auth.SessionToken, http.StatusOK, &projects)
	if len(projects) == 0 {
		t.Fatal("setup did not create a project")
	}

	var channels []domain.Channel
	getJSON(t, baseURL+"/api/projects/"+projects[0].ID+"/channels", auth.SessionToken, http.StatusOK, &channels)
	if len(channels) == 0 {
		t.Fatal("setup did not create a channel")
	}

	var agents []domain.Agent
	getJSON(t, baseURL+"/api/organizations/"+orgs[0].ID+"/agents", auth.SessionToken, http.StatusOK, &agents)
	if len(agents) == 0 {
		t.Fatal("setup did not create an agent")
	}

	var agentWorkspace domain.Workspace
	getJSON(t, baseURL+"/api/workspaces/"+agents[0].ConfigWorkspaceID, auth.SessionToken, http.StatusOK, &agentWorkspace)
	var projectWorkspace domain.Workspace
	getJSON(t, baseURL+"/api/workspaces/"+projects[0].WorkspaceID, auth.SessionToken, http.StatusOK, &projectWorkspace)

	return app.BootstrapResult{
		SessionToken:     auth.SessionToken,
		User:             auth.User,
		Organization:     orgs[0],
		Project:          projects[0],
		Channel:          channels[0],
		Agent:            agents[0],
		Workspace:        agentWorkspace,
		ProjectWorkspace: projectWorkspace,
	}
}

func setupApp(t *testing.T, ctx context.Context, a *app.App) app.BootstrapResult {
	t.Helper()
	boot, err := a.Bootstrap(ctx, app.SetupAdminRequest{
		SetupToken:  "secret",
		Username:    "meteorsky",
		Password:    "correct-password-123",
		DisplayName: "Meteorsky",
	})
	if err != nil {
		t.Fatal(err)
	}
	return boot
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

func lazyProcessTestMessage(organizationID string, conversationID string) domain.Message {
	return domain.Message{
		ID:               "msg_lazy_process",
		OrganizationID:   organizationID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   conversationID,
		SenderType:       domain.SenderBot,
		SenderID:         "bot_lazy_process",
		Kind:             domain.MessageText,
		Body:             "done",
		Metadata: map[string]any{
			"process": []domain.ProcessItem{
				{
					Type:       "tool_call",
					ToolName:   "Bash",
					ToolCallID: "call_1",
					Input:      map[string]any{"command": "pnpm test"},
					Raw:        map[string]any{"type": "tool_use"},
				},
				{
					Type:       "tool_result",
					ToolCallID: "call_1",
					Status:     "completed",
					Output:     "tests passed",
					Raw:        map[string]any{"type": "tool_result"},
				},
			},
		},
		CreatedAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC),
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

type multipartTestFile struct {
	Field       string
	Filename    string
	ContentType string
	Body        []byte
}

func postMultipartMessage(t *testing.T, url string, token string, fields map[string]string, files []multipartTestFile, wantStatus int, dst any) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
			"name":     file.Field,
			"filename": file.Filename,
		}))
		if file.ContentType != "" {
			header.Set("Content-Type", file.ContentType)
		}
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file.Body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
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

func getRaw(t *testing.T, url string, token string) (int, http.Header, []byte) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header.Clone(), body
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
