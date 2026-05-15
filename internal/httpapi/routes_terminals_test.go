package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	"golang.org/x/crypto/bcrypt"
	"nhooyr.io/websocket"
)

func TestWorkspaceTerminalsRequireAdminRole(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	boot := setupApp(t, ctx, env.app)
	memberToken := createTerminalTestUserSession(t, ctx, env, boot.Organization.ID, domain.RoleMember)

	getJSON(t, env.server.URL+"/api/workspaces/"+boot.ProjectWorkspace.ID+"/terminals", memberToken, http.StatusForbidden, nil)

	conn, resp, err := websocket.Dial(ctx, terminalWSURL(env.server.URL, boot.ProjectWorkspace.ID, memberToken), nil)
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		t.Fatal("terminal websocket dial succeeded for member role, want forbidden")
	}
	if resp == nil {
		t.Fatal("missing websocket HTTP response")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("terminal websocket status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestWorkspaceTerminalWebSocketReplaysServerHistory(t *testing.T) {
	env := newTestEnvWithOptions(t, app.Options{
		Terminal: app.TerminalOptions{
			Shell:       "/bin/sh",
			ReplayBytes: 1024 * 1024,
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	boot := setupApp(t, ctx, env.app)
	conn := dialTerminalWS(t, ctx, env.server.URL, boot.ProjectWorkspace.ID, boot.SessionToken)
	ready := attachTerminalWS(t, ctx, conn, "")
	if ready.Session == nil || ready.Session.ID == "" {
		t.Fatalf("ready frame = %#v", ready)
	}
	terminalID := ready.Session.ID
	t.Cleanup(func() {
		deleteJSON(t, env.server.URL+"/api/workspaces/"+boot.ProjectWorkspace.ID+"/terminals/"+terminalID, boot.SessionToken, http.StatusNoContent)
	})

	marker := "agentx_terminal_replay"
	writeTerminalInput(t, ctx, conn, "printf '"+marker+"\\n'\n")
	waitTerminalOutputContains(t, ctx, conn, marker)
	_ = conn.Close(websocket.StatusNormalClosure, "")

	var sessions []app.TerminalSessionSummary
	getJSON(t, env.server.URL+"/api/workspaces/"+boot.ProjectWorkspace.ID+"/terminals", boot.SessionToken, http.StatusOK, &sessions)
	if len(sessions) != 1 || sessions[0].ID != terminalID {
		t.Fatalf("terminal sessions = %#v, want cached session %q", sessions, terminalID)
	}

	var renamed app.TerminalSessionSummary
	patchJSON(t, env.server.URL+"/api/workspaces/"+boot.ProjectWorkspace.ID+"/terminals/"+terminalID, boot.SessionToken, map[string]string{
		"title": "build shell",
	}, http.StatusOK, &renamed)
	if renamed.Title != "build shell" {
		t.Fatalf("renamed terminal title = %q, want build shell", renamed.Title)
	}
	patchJSON(t, env.server.URL+"/api/workspaces/"+boot.ProjectWorkspace.ID+"/terminals/"+terminalID, boot.SessionToken, map[string]string{
		"title": " ",
	}, http.StatusBadRequest, nil)

	replayConn := dialTerminalWS(t, ctx, env.server.URL, boot.ProjectWorkspace.ID, boot.SessionToken)
	defer replayConn.Close(websocket.StatusNormalClosure, "")
	replayReady := attachTerminalWS(t, ctx, replayConn, terminalID)
	if replayReady.Session == nil || replayReady.Session.Title != "build shell" {
		t.Fatalf("replay ready session = %#v, want renamed title", replayReady.Session)
	}
	waitTerminalOutputContains(t, ctx, replayConn, marker)
}

func TestWorkspaceTerminalWebSocketRunsTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}

	env := newTestEnvWithOptions(t, app.Options{
		Terminal: app.TerminalOptions{Shell: "/bin/sh"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	boot := setupApp(t, ctx, env.app)
	conn := dialTerminalWS(t, ctx, env.server.URL, boot.ProjectWorkspace.ID, boot.SessionToken)
	defer conn.Close(websocket.StatusNormalClosure, "")
	ready := attachTerminalWS(t, ctx, conn, "")
	if ready.Session == nil || ready.Session.ID == "" {
		t.Fatalf("ready frame = %#v", ready)
	}
	terminalID := ready.Session.ID
	t.Cleanup(func() {
		deleteJSON(t, env.server.URL+"/api/workspaces/"+boot.ProjectWorkspace.ID+"/terminals/"+terminalID, boot.SessionToken, http.StatusNoContent)
	})

	socketName := strings.ReplaceAll(id.New("tmux"), "_", "-")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socketName, "kill-server").Run()
	})
	marker := "tmux_agentx_ok"
	writeTerminalInput(t, ctx, conn, fmt.Sprintf("tmux -L %s -f /dev/null new-session 'printf \"%s\\n\"; sleep 0.2'\n", socketName, marker))
	waitTerminalOutputContains(t, ctx, conn, marker)
}

func TestWorkspaceTerminalWebSocketReportsProtocolErrors(t *testing.T) {
	env := newTestEnvWithOptions(t, app.Options{
		Terminal: app.TerminalOptions{Shell: "/bin/sh"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	boot := setupApp(t, ctx, env.app)
	conn := dialTerminalWS(t, ctx, env.server.URL, boot.ProjectWorkspace.ID, boot.SessionToken)
	defer conn.Close(websocket.StatusNormalClosure, "")
	ready := attachTerminalWS(t, ctx, conn, "")
	if ready.Session == nil || ready.Session.ID == "" {
		t.Fatalf("ready frame = %#v", ready)
	}
	terminalID := ready.Session.ID
	t.Cleanup(func() {
		deleteJSON(t, env.server.URL+"/api/workspaces/"+boot.ProjectWorkspace.ID+"/terminals/"+terminalID, boot.SessionToken, http.StatusNoContent)
	})

	payload, err := json.Marshal(terminalClientMessage{Type: "input", Data: "not-base64"})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatal(err)
	}
	frame := readTerminalFrame(t, ctx, conn)
	if frame.Type != "error" || frame.Error != "invalid terminal input" {
		t.Fatalf("terminal error frame = %#v, want invalid terminal input", frame)
	}
}

func createTerminalTestUserSession(t *testing.T, ctx context.Context, env testEnv, orgID string, role domain.Role) string {
	t.Helper()
	password := "terminal-password-123"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	passwordUpdatedAt := now
	usernameSuffix := strings.TrimPrefix(id.New("usr"), "usr_")
	if len(usernameSuffix) > 8 {
		usernameSuffix = usernameSuffix[:8]
	}
	username := "terminal-" + usernameSuffix
	user := domain.User{
		ID:                id.New("usr"),
		Username:          username,
		DisplayName:       "Terminal User",
		PasswordHash:      string(hash),
		PasswordUpdatedAt: &passwordUpdatedAt,
		CreatedAt:         now,
	}
	if err := env.store.Users().Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := env.store.Organizations().AddMember(ctx, orgID, user.ID, role); err != nil {
		t.Fatal(err)
	}
	login, err := env.app.Login(ctx, app.LoginRequest{Username: username, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	return login.SessionToken
}

func terminalWSURL(baseURL string, workspaceID string, token string) string {
	return "ws" + strings.TrimPrefix(baseURL, "http") + "/api/workspaces/" + workspaceID + "/terminal/ws?token=" + token
}

func dialTerminalWS(t *testing.T, ctx context.Context, baseURL string, workspaceID string, token string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(ctx, terminalWSURL(baseURL, workspaceID, token), nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func attachTerminalWS(t *testing.T, ctx context.Context, conn *websocket.Conn, terminalID string) app.TerminalFrame {
	t.Helper()
	payload, err := json.Marshal(terminalClientMessage{
		Type:       "attach",
		TerminalID: terminalID,
		Cols:       80,
		Rows:       24,
		SinceSeq:   0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatal(err)
	}
	for {
		frame := readTerminalFrame(t, ctx, conn)
		switch frame.Type {
		case "ready":
			return frame
		case "error":
			t.Fatalf("terminal attach error: %s", frame.Error)
		}
	}
}

func writeTerminalInput(t *testing.T, ctx context.Context, conn *websocket.Conn, input string) {
	t.Helper()
	payload, err := json.Marshal(terminalClientMessage{
		Type: "input",
		Data: base64.StdEncoding.EncodeToString([]byte(input)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatal(err)
	}
}

func waitTerminalOutputContains(t *testing.T, ctx context.Context, conn *websocket.Conn, needle string) {
	t.Helper()
	var seen strings.Builder
	for !strings.Contains(seen.String(), needle) {
		frame := readTerminalFrame(t, ctx, conn)
		switch frame.Type {
		case "output":
			data, err := base64.StdEncoding.DecodeString(frame.Data)
			if err != nil {
				t.Fatal(err)
			}
			seen.Write(data)
		case "error":
			t.Fatalf("terminal error: %s", frame.Error)
		case "exit":
			t.Fatalf("terminal exited before %q appeared; output so far: %q", needle, seen.String())
		}
	}
}

func readTerminalFrame(t *testing.T, ctx context.Context, conn *websocket.Conn) app.TerminalFrame {
	t.Helper()
	typ, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("terminal frame type = %v, want text", typ)
	}
	var frame app.TerminalFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		t.Fatalf("terminal frame %s: %v", string(payload), err)
	}
	return frame
}
