package mcpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

// Minimal valid 1x1 PNG image bytes.
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
	0x00, 0x03, 0x01, 0x01, 0x00, 0x18, 0xdd, 0x8d,
	0xb0, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
	0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestServerReadsAndSearchesPostFromReadOnlyDatabase(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "agentx.db")
	writer, err := sqlitestore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	seedPost(t, ctx, writer, dataDir)

	reader, err := sqlitestore.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	server := New(func(context.Context) (store.Store, error) { return reader, nil }, "test").WithDataDir(dataDir)

	responses := serveLines(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_post","arguments":{"post_id":"thr_1","limit":2}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_post","arguments":{"post_id":"thr_1","offset":2,"include_process":true}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_post","arguments":{"post_id":"thr_1","query":"ROLLBACK plan"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_post","arguments":{"post_id":"thr_missing"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"search_post","arguments":{"post_id":"thr_1","query":"make deploy","include_process":true}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_1","attachment_id":"att_img"}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_1","attachment_id":"att_1"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"unknown/method"}`,
		`not json`,
	)
	if len(responses) != 11 {
		t.Fatalf("responses = %d, want 11 (notification must not be answered)", len(responses))
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
	if len(tools.Tools) != 3 || tools.Tools[0].Name != "get_post" || tools.Tools[1].Name != "search_post" || tools.Tools[2].Name != "get_attachment" {
		t.Fatalf("tools = %#v", tools)
	}

	firstPage := toolResultText(t, responses[2], false)
	for _, want := range []string{"# Release planning", "Post ID: `thr_1`", "Project: Demo", "Channel: #forum", "## [1] Alice (user)", "`att_img`", "neurons.png", "## [2] Agent (bot)", "offset=2"} {
		if !strings.Contains(firstPage, want) {
			t.Fatalf("first page missing %q:\n%s", want, firstPage)
		}
	}
	if strings.Contains(firstPage, "[3]") {
		t.Fatalf("first page should stop at limit 2:\n%s", firstPage)
	}

	secondPage := toolResultText(t, responses[3], false)
	for _, want := range []string{"## [3] Alice (user)", "_Reply to [2]_", "notes.txt", "`att_1`", "Thinking", "tool `Bash`"} {
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

	var imageResult struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Data     string `json:"data"`
			MimeType string `json:"mimeType"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	decodeResult(t, responses[7], &imageResult)
	if imageResult.IsError || len(imageResult.Content) != 2 {
		t.Fatalf("image tool result = %#v", imageResult)
	}
	if imageResult.Content[0].Type != "text" || !strings.Contains(imageResult.Content[0].Text, "neurons.png") {
		t.Fatalf("image text block = %#v", imageResult.Content[0])
	}
	if imageResult.Content[1].Type != "image" || imageResult.Content[1].MimeType != "image/png" {
		t.Fatalf("image content block = %#v", imageResult.Content[1])
	}
	decodedPNG, err := base64.StdEncoding.DecodeString(imageResult.Content[1].Data)
	if err != nil || !bytes.Equal(decodedPNG, tinyPNG) {
		t.Fatalf("decoded image bytes mismatch: err=%v len=%d", err, len(decodedPNG))
	}

	noteText := toolResultText(t, responses[8], false)
	if !strings.Contains(noteText, "notes.txt") || !strings.Contains(noteText, "rollback ok\n") {
		t.Fatalf("text attachment result = %q", noteText)
	}

	if responses[9].Error == nil || responses[9].Error.Code != codeMethodNotFound {
		t.Fatalf("unknown method response = %#v", responses[9])
	}
	if responses[10].Error == nil || responses[10].Error.Code != codeParseError {
		t.Fatalf("parse error response = %#v", responses[10])
	}
}

func TestGetAttachmentScopesToPostAndPagesOversizedText(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataDir := filepath.Join(root, ".agentx")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dataDir, "agentx.db")
	writer, err := sqlitestore.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	seedPost(t, ctx, writer, dataDir)

	now := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	// Relative .agentx/attachments/... path like `make dev` writes when AGENTX_DATA_DIR=.agentx.
	relDir := filepath.Join(dataDir, "attachments", "org_1", "thread", "thr_1", "msg_1")
	if err := os.MkdirAll(relDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relDir, "att_rel.png"), tinyPNG, 0o600); err != nil {
		t.Fatal(err)
	}
	oversizedBody := strings.Repeat("神经元发现αβγ\n", 3000) // > maxPageBytes, multi-byte UTF-8
	oversizedPath := filepath.Join(relDir, "att_big.txt")
	if err := os.WriteFile(oversizedPath, []byte(oversizedBody), 0o600); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(relDir, "att_bin.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	chanPath := filepath.Join(dataDir, "att_chan.png")
	if err := os.WriteFile(chanPath, tinyPNG, 0o600); err != nil {
		t.Fatal(err)
	}

	err = writer.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Threads().Create(ctx, domain.Thread{
			ID: "thr_2", OrganizationID: "org_1", ProjectID: "prj_1", ChannelID: "chn_1",
			Title: "Other post", CreatedBy: "usr_1", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		if err := tx.Channels().Create(ctx, domain.Channel{
			ID: "chn_chat", OrganizationID: "org_1", ProjectID: "prj_1", Type: domain.ChannelTypeText,
			Name: "general", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		if err := tx.Messages().Create(ctx, domain.Message{
			ID: "msg_chan", OrganizationID: "org_1", ConversationType: domain.ConversationChannel,
			ConversationID: "chn_chat", SenderType: domain.SenderUser, SenderID: "usr_1",
			Kind: domain.MessageText, Body: "channel image", CreatedAt: now,
		}); err != nil {
			return err
		}
		for _, att := range []domain.MessageAttachment{
			{
				ID: "att_rel", MessageID: "msg_1", OrganizationID: "org_1", ConversationType: domain.ConversationThread,
				ConversationID: "thr_1", Filename: "mislabeled.jpg", ContentType: "image/jpeg; charset=binary",
				Kind: domain.MessageAttachmentImage, SizeBytes: int64(len(tinyPNG)),
				StoragePath: filepath.Join(".agentx", "attachments", "org_1", "thread", "thr_1", "msg_1", "att_rel.png"),
				CreatedAt:   now,
			},
			{
				ID: "att_big", MessageID: "msg_1", OrganizationID: "org_1", ConversationType: domain.ConversationThread,
				ConversationID: "thr_1", Filename: "neurons.md", ContentType: "text/markdown; charset=utf-8",
				Kind: domain.MessageAttachmentText, SizeBytes: int64(len(oversizedBody)),
				StoragePath: oversizedPath, CreatedAt: now,
			},
			{
				ID: "att_bin", MessageID: "msg_1", OrganizationID: "org_1", ConversationType: domain.ConversationThread,
				ConversationID: "thr_1", Filename: "weights.bin", ContentType: "application/octet-stream",
				Kind: domain.MessageAttachmentFile, SizeBytes: 4,
				StoragePath: binPath, CreatedAt: now,
			},
			{
				ID: "att_missing_file", MessageID: "msg_1", OrganizationID: "org_1", ConversationType: domain.ConversationThread,
				ConversationID: "thr_1", Filename: "gone.png", ContentType: "image/png",
				Kind: domain.MessageAttachmentImage, SizeBytes: 68,
				StoragePath: filepath.Join(relDir, "does_not_exist.png"), CreatedAt: now,
			},
			{
				ID: "att_chan", MessageID: "msg_chan", OrganizationID: "org_1", ConversationType: domain.ConversationChannel,
				ConversationID: "chn_chat", Filename: "secret.png", ContentType: "image/png",
				Kind: domain.MessageAttachmentImage, SizeBytes: int64(len(tinyPNG)),
				StoragePath: chanPath, CreatedAt: now,
			},
		} {
			if err := tx.MessageAttachments().Create(ctx, att); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run from an unrelated working directory so relative StoragePath resolution uses dataDir.
	t.Chdir(t.TempDir())

	reader, err := sqlitestore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	server := New(func(context.Context) (store.Store, error) { return reader, nil }, "test").WithDataDir(dataDir)

	responses := serveLines(t, server,
		// 0: relative .agentx path + sniffed PNG magic bytes override image/jpeg header
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_1","attachment_id":"att_rel"}}}`,
		// 1: wrong post_id rejected even when attachment exists in thr_1
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_2","attachment_id":"att_img"}}}`,
		// 2: channel attachment rejected
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_1","attachment_id":"att_chan"}}}`,
		// 3: binary file rejected
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_1","attachment_id":"att_bin"}}}`,
		// 4: missing file on disk reported cleanly
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_1","attachment_id":"att_missing_file"}}}`,
		// 5: missing attachment_id rejected
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_1"}}}`,
	)

	var relImg struct {
		Content []struct {
			Type     string `json:"type"`
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	decodeResult(t, responses[0], &relImg)
	if relImg.IsError || len(relImg.Content) != 2 || relImg.Content[1].MimeType != "image/png" {
		t.Fatalf("relative/mislabeled image = %#v, want sniffed image/png", relImg)
	}

	if wrongPost := toolResultText(t, responses[1], true); !strings.Contains(wrongPost, "no attachment") {
		t.Fatalf("wrong post error = %q", wrongPost)
	}
	if chanErr := toolResultText(t, responses[2], true); !strings.Contains(chanErr, "no attachment") {
		t.Fatalf("channel attachment error = %q", chanErr)
	}
	if binErr := toolResultText(t, responses[3], true); !strings.Contains(binErr, "binary file") {
		t.Fatalf("binary attachment error = %q", binErr)
	}
	if missingFile := toolResultText(t, responses[4], true); !strings.Contains(missingFile, "missing") {
		t.Fatalf("missing file error = %q", missingFile)
	}
	if missingArg := toolResultText(t, responses[5], true); !strings.Contains(missingArg, "attachment_id is required") {
		t.Fatalf("missing arg error = %q", missingArg)
	}

	// Page through the entire oversized UTF-8 text attachment and verify the
	// concatenated chunks match the original file byte-for-byte without splitting runes.
	var reconstructed strings.Builder
	offset := 0
	for page := 0; page < 10; page++ {
		call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_attachment","arguments":{"post_id":"thr_1","attachment_id":"att_big","offset":` + strconv.Itoa(offset) + `}}}`
		text := toolResultText(t, serveLines(t, server, call)[0], false)
		if strings.ContainsRune(text, '\ufffd') {
			t.Fatalf("corrupted UTF-8 rune at offset %d", offset)
		}
		_, body, ok := strings.Cut(text, "\n\n")
		if !ok {
			t.Fatalf("missing header separator in:\n%s", text)
		}
		chunk, footer, truncated := strings.Cut(body, "\n\n---\n\n(Attachment truncated at byte ")
		if !truncated {
			reconstructed.WriteString(body)
			break
		}
		// When a chunk did not end with '\n', readTextAttachment inserted one before the footer separator.
		expectedLen := 0
		end := strings.IndexFunc(footer, func(r rune) bool { return r < '0' || r > '9' })
		nextOffset, err := strconv.Atoi(footer[:end])
		if err != nil || nextOffset <= offset {
			t.Fatalf("invalid continuation offset from %q", footer)
		}
		expectedLen = nextOffset - offset
		if len(chunk) == expectedLen+1 && strings.HasSuffix(chunk, "\n") {
			chunk = chunk[:expectedLen]
		}
		reconstructed.WriteString(chunk)
		offset = nextOffset
	}
	if reconstructed.String() != oversizedBody {
		t.Fatalf("reconstructed len = %d, want %d", reconstructed.Len(), len(oversizedBody))
	}
}

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "agentx.db")
	writer, err := sqlitestore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	seedPost(t, ctx, writer, dataDir)
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

func seedPost(t *testing.T, ctx context.Context, st *sqlitestore.Store, dataDir string) {
	t.Helper()
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	imgPath := filepath.Join(dataDir, "attachments", "org_1", "thread", "thr_1", "msg_1", "att_img.png")
	if err := os.MkdirAll(filepath.Dir(imgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imgPath, tinyPNG, 0o600); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(dataDir, "att_1")
	if err := os.WriteFile(notePath, []byte("rollback ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}

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
		if err := tx.MessageAttachments().Create(ctx, domain.MessageAttachment{
			ID: "att_img", MessageID: "msg_1", OrganizationID: "org_1", ConversationType: domain.ConversationThread,
			ConversationID: "thr_1", Filename: "neurons.png", ContentType: "image/png", Kind: domain.MessageAttachmentImage,
			SizeBytes: int64(len(tinyPNG)), StoragePath: imgPath, CreatedAt: now,
		}); err != nil {
			return err
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
