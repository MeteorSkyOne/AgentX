package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/transcript"
)

func TestHTTPThreadExportReturnsMarkdownAndJSON(t *testing.T) {
	ts := newTestServer(t)
	bootstrap := setupHTTP(t, ts.URL)

	var forum domain.Channel
	postJSON(t, ts.URL+"/api/projects/"+bootstrap.Project.ID+"/channels", bootstrap.SessionToken, map[string]any{
		"name": "forum",
		"type": "thread",
	}, http.StatusOK, &forum)
	putJSON(t, ts.URL+"/api/channels/"+forum.ID+"/agents", bootstrap.SessionToken, map[string]any{
		"agents": []map[string]any{{"agent_id": bootstrap.Agent.ID}},
	}, http.StatusOK, nil)
	var created struct {
		Thread domain.Thread `json:"thread"`
	}
	postJSON(t, ts.URL+"/api/channels/"+forum.ID+"/threads", bootstrap.SessionToken, map[string]any{
		"title": "Export me",
		"body":  "hello export",
	}, http.StatusOK, &created)

	exportURL := ts.URL + "/api/threads/" + created.Thread.ID + "/export"
	var markdown string
	requireEventually(t, time.Second, func() bool {
		status, _, body := getRaw(t, exportURL, bootstrap.SessionToken)
		markdown = string(body)
		return status == http.StatusOK && strings.Contains(markdown, "Echo: hello export")
	})
	for _, want := range []string{"# Export me", "Post ID: `" + created.Thread.ID + "`", "Channel: #forum", "hello export", "(@" + bootstrap.Agent.Handle + ")"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}

	status, headers, body := getRaw(t, exportURL+"?format=json", bootstrap.SessionToken)
	if status != http.StatusOK {
		t.Fatalf("json export status = %d: %s", status, body)
	}
	if disposition := headers.Get("Content-Disposition"); !strings.Contains(disposition, created.Thread.ID+".json") {
		t.Fatalf("content-disposition = %q", disposition)
	}
	var post transcript.Post
	if err := json.Unmarshal(body, &post); err != nil {
		t.Fatal(err)
	}
	if post.ID != created.Thread.ID || len(post.Messages) != 2 || post.Messages[1].SenderType != domain.SenderBot {
		t.Fatalf("post = %#v", post)
	}

	if status, _, _ := getRaw(t, exportURL+"?format=pdf", bootstrap.SessionToken); status != http.StatusBadRequest {
		t.Fatalf("unsupported format status = %d, want 400", status)
	}
	if status, _, _ := getRaw(t, exportURL, ""); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", status)
	}
	if status, _, _ := getRaw(t, ts.URL+"/api/threads/thr_missing/export", bootstrap.SessionToken); status != http.StatusNotFound {
		t.Fatalf("missing thread status = %d, want 404", status)
	}
}

func TestHTTPMCPSetupQuotesDatabasePath(t *testing.T) {
	env := newTestEnvWithOptions(t, app.Options{MCPCommand: "/opt/agentx/agentx", SQLitePath: "/home/me/Agent X/agentx.db"})
	bootstrap := setupHTTP(t, env.server.URL)

	var setup app.MCPSetup
	getJSON(t, env.server.URL+"/api/mcp/setup", bootstrap.SessionToken, http.StatusOK, &setup)
	want := "claude mcp add --scope user agentx -- /opt/agentx/agentx mcp --db '/home/me/Agent X/agentx.db'"
	if !setup.Available || setup.ClaudeCommand != want {
		t.Fatalf("setup = %#v, want claude command %q", setup, want)
	}
	if wantPi := "pi mcp add --exposure direct agentx -- /opt/agentx/agentx mcp --db '/home/me/Agent X/agentx.db'"; setup.PiCommand != wantPi {
		t.Fatalf("pi command = %q, want %q", setup.PiCommand, wantPi)
	}
}
