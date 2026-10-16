package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

func TestServerReadsAndSearchesPostFromReadOnlyDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agentx.db")
	writer, err := sqlitestore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	seedPost(t, ctx, writer)

	reader, err := sqlitestore.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	server := New(func(context.Context) (store.Store, error) { return reader, nil }, "test")

	responses := serveLines(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_post","arguments":{"post_id":"thr_1","limit":2}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_post","arguments":{"post_id":"thr_1","offset":2,"include_process":true}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_post","arguments":{"post_id":"thr_1","query":"ROLLBACK plan"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_post","arguments":{"post_id":"thr_missing"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"search_post","arguments":{"post_id":"thr_1","query":"make deploy","include_process":true}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"unknown/method"}`,
		`not json`,
	)
	if len(responses) != 9 {
		t.Fatalf("responses = %d, want 9 (notification must not be answered)", len(responses))
	}

	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	decodeResult(t, responses[0], &initResult)
	if initResult.ProtocolVersion != "2025-03-26" || initResult.ServerInfo.Name != "agentx" {
		t.Fatalf("initialize = %#v", initResult)
	}

	var tools struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	decodeResult(t, responses[1], &tools)
	if len(tools.Tools) != 2 || tools.Tools[0].Name != "get_post" || tools.Tools[1].Name != "search_post" {
		t.Fatalf("tools = %#v", tools)
	}

	firstPage := toolResultText(t, responses[2], false)
	for _, want := range []string{"# Release planning", "Post ID: `thr_1`", "Project: Demo", "Channel: #forum", "## [1] Alice (user)", "## [2] Agent (bot)", "offset=2"} {
		if !strings.Contains(firstPage, want) {
			t.Fatalf("first page missing %q:\n%s", want, firstPage)
		}
	}
	if strings.Contains(firstPage, "[3]") {
		t.Fatalf("first page should stop at limit 2:\n%s", firstPage)
	}

	secondPage := toolResultText(t, responses[3], false)
	for _, want := range []string{"## [3] Alice (user)", "_Reply to [2]_", "notes.txt", "Thinking", "tool `Bash`"} {
		if !strings.Contains(secondPage, want) {
			t.Fatalf("second page missing %q:\n%s", want, secondPage)
		}
	}
	if got := strings.Count(secondPage, "Check the deploy script first."); got != 1 {
		t.Fatalf("thinking rendered %d times, want 1:\n%s", got, secondPage)
	}

	search := toolResultText(t, responses[4], false)
	if !strings.Contains(search, "[2] Agent (bot)") || strings.Contains(search, "[1] Alice") {
		t.Fatalf("search = %s", search)
	}

	if missing := toolResultText(t, responses[5], true); !strings.Contains(missing, "thr_missing") {
		t.Fatalf("missing post error = %s", missing)
	}
	if processSearch := toolResultText(t, responses[6], false); !strings.Contains(processSearch, "[4] Agent (bot)") || !strings.Contains(processSearch, "in process") {
		t.Fatalf("process search = %s", processSearch)
	}
	if responses[7].Error == nil || responses[7].Error.Code != codeMethodNotFound {
		t.Fatalf("unknown method response = %#v", responses[7])
	}
	if responses[8].Error == nil || responses[8].Error.Code != codeParseError {
		t.Fatalf("parse error response = %#v", responses[8])
	}
}

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agentx.db")
	writer, err := sqlitestore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	seedPost(t, ctx, writer)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := sqlitestore.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.Threads().ByID(ctx, "thr_1"); err != nil {
		t.Fatalf("read thread: %v", err)
	}
	if err := reader.Threads().Update(ctx, domain.Thread{ID: "thr_1", Title: "changed"}); err == nil {
		t.Fatal("read-only store accepted a write")
	}
	if _, err := sqlitestore.OpenReadOnly(ctx, filepath.Join(t.TempDir(), "missing.db")); err == nil {
		t.Fatal("OpenReadOnly created a missing database")
	}
}

func seedPost(t *testing.T, ctx context.Context, st *sqlitestore.Store) {
	t.Helper()
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	err := st.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Users().Create(ctx, domain.User{ID: "usr_1", DisplayName: "Alice", CreatedAt: now}); err != nil {
			return err
		}
		if err := tx.Organizations().Create(ctx, domain.Organization{ID: "org_1", Name: "Default", CreatedAt: now}); err != nil {
			return err
		}
		if err := tx.Workspaces().Create(ctx, domain.Workspace{ID: "wks_1", OrganizationID: "org_1", Type: "project", Name: "demo", Path: "/tmp/demo", CreatedBy: "usr_1", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if err := tx.Projects().Create(ctx, domain.Project{ID: "prj_1", OrganizationID: "org_1", Name: "Demo", WorkspaceID: "wks_1", CreatedBy: "usr_1", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if err := tx.Channels().Create(ctx, domain.Channel{ID: "chn_1", OrganizationID: "org_1", ProjectID: "prj_1", Type: domain.ChannelTypeThread, Name: "forum", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if err := tx.Threads().Create(ctx, domain.Thread{ID: "thr_1", OrganizationID: "org_1", ProjectID: "prj_1", ChannelID: "chn_1", Title: "Release planning", CreatedBy: "usr_1", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		messages := []domain.Message{
			{ID: "msg_1", SenderType: domain.SenderUser, SenderID: "usr_1", Body: "How should we ship v2?"},
			{ID: "msg_2", SenderType: domain.SenderBot, SenderID: "bot_gone", Body: "Ship behind a flag and keep a rollback plan ready."},
			{ID: "msg_3", SenderType: domain.SenderUser, SenderID: "usr_1", Body: "Agreed, see attached notes.", ReplyToMessageID: "msg_2"},
			{ID: "msg_4", SenderType: domain.SenderBot, SenderID: "bot_gone", Body: "Done.", Metadata: map[string]any{
				"thinking": "Check the deploy script first.",
				"process": []map[string]any{
					{"type": "thinking", "text": "Check the deploy script first."},
					{"type": "tool_call", "tool_name": "Bash", "status": "completed", "input": map[string]any{"command": "make deploy"}},
				},
			}},
		}
		for i, message := range messages {
			message.OrganizationID = "org_1"
			message.ConversationType = domain.ConversationThread
			message.ConversationID = "thr_1"
			message.Kind = domain.MessageText
			message.CreatedAt = now.Add(time.Duration(i) * time.Minute)
			if err := tx.Messages().Create(ctx, message); err != nil {
				return err
			}
		}
		return tx.MessageAttachments().Create(ctx, domain.MessageAttachment{
			ID: "att_1", MessageID: "msg_3", OrganizationID: "org_1", ConversationType: domain.ConversationThread,
			ConversationID: "thr_1", Filename: "notes.txt", ContentType: "text/plain", Kind: domain.MessageAttachmentText,
			SizeBytes: 12, StoragePath: "att_1", CreatedAt: now,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func serveLines(t *testing.T, server *Server, lines ...string) []response {
	t.Helper()
	var out bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var responses []response
	decoder := json.NewDecoder(&out)
	for decoder.More() {
		var resp struct {
			response
			Result json.RawMessage `json:"result"`
		}
		if err := decoder.Decode(&resp); err != nil {
			t.Fatal(err)
		}
		resp.response.Result = resp.Result
		responses = append(responses, resp.response)
	}
	return responses
}

func decodeResult(t *testing.T, resp response, dst any) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	if err := json.Unmarshal(resp.Result.(json.RawMessage), dst); err != nil {
		t.Fatal(err)
	}
}

func toolResultText(t *testing.T, resp response, wantError bool) string {
	t.Helper()
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	decodeResult(t, resp, &result)
	if result.IsError != wantError || len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("tool result = %#v, want isError=%v", result, wantError)
	}
	return result.Content[0].Text
}
