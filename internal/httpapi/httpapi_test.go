package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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
		if strings.Contains(string(payload), string(domain.EventMessageCreated)) {
			return
		}
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
