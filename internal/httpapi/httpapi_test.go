package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
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

func newTestServer(t *testing.T) *httptest.Server {
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
	return ts
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
