package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/eventbus"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

func TestServeHTTPShutsDownWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}
	t.Cleanup(func() {
		_ = server.Close()
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, server, func() error {
			return server.Serve(listener)
		})
	}()

	resp, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("GET test server: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Fatalf("close response body: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("response body = %q, want ok", string(body))
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serveHTTP() did not stop after context cancellation")
	}
}

func TestConfigureLoggingCreatesStartupLogFile(t *testing.T) {
	t.Chdir(t.TempDir())
	previousLogger := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	startedAt := time.Date(2026, 4, 27, 15, 4, 5, 0, time.Local)
	file, path, err := configureLogging(startedAt)
	if err != nil {
		t.Fatalf("configureLogging() error = %v", err)
	}

	wantPath := filepath.Join(logDir, "log-20260427-150405.log")
	if path != wantPath {
		t.Fatalf("log path = %q, want %q", path, wantPath)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat log file: %v", err)
	}

	slog.Info("slog entry")
	log.Print("standard log entry")
	if err := file.Close(); err != nil {
		t.Fatalf("close log file: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "slog entry") {
		t.Fatalf("log file does not contain slog entry: %q", got)
	}
	if !strings.Contains(got, "standard log entry") {
		t.Fatalf("log file does not contain standard log entry: %q", got)
	}
}

func TestPrintSetupTokenIfNeededWritesOnlyWhenSetupIsPending(t *testing.T) {
	ctx := context.Background()
	var stdout bytes.Buffer
	users := fakePasswordStatusStore{hasPassword: false}

	if err := printSetupTokenIfNeeded(ctx, &stdout, users, " secret-token "); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "Setup token: secret-token\n" {
		t.Fatalf("stdout = %q, want setup token", got)
	}

	stdout.Reset()
	users.hasPassword = true
	if err := printSetupTokenIfNeeded(ctx, &stdout, users, "secret-token"); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want no setup token", got)
	}
}

func TestPrintSetupTokenIfNeededDoesNotWriteTokenToLogFile(t *testing.T) {
	t.Chdir(t.TempDir())
	previousLogger := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	file, path, err := configureLogging(time.Date(2026, 4, 27, 15, 4, 5, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := printSetupTokenIfNeeded(context.Background(), &stdout, fakePasswordStatusStore{}, "secret-token"); err != nil {
		t.Fatal(err)
	}
	slog.Info("normal startup log")
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(stdout.String(), "secret-token") {
		t.Fatalf("stdout = %q, want setup token", stdout.String())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret-token") {
		t.Fatalf("log file contains setup token: %q", string(body))
	}
}

type fakePasswordStatusStore struct {
	hasPassword bool
	err         error
}

func (s fakePasswordStatusStore) HasPassword(context.Context) (bool, error) {
	return s.hasPassword, s.err
}

func TestNewHTTPHandlerServesWebDistAndKeepsAPI(t *testing.T) {
	distDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("app shell"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "assets", "app.js"), []byte("console.log('agentx')"), 0o644); err != nil {
		t.Fatal(err)
	}

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ws" && r.URL.Path != "/healthz" {
			t.Fatalf("unexpected API path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := newHTTPHandler(api, nil, distDir)

	indexRecorder := httptest.NewRecorder()
	handler.ServeHTTP(indexRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRecorder.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", indexRecorder.Code, http.StatusOK)
	}
	if body := indexRecorder.Body.String(); body != "app shell" {
		t.Fatalf("GET / body = %q, want %q", body, "app shell")
	}

	assetRecorder := httptest.NewRecorder()
	handler.ServeHTTP(assetRecorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if assetRecorder.Code != http.StatusOK {
		t.Fatalf("GET /assets/app.js status = %d, want %d", assetRecorder.Code, http.StatusOK)
	}
	if body := assetRecorder.Body.String(); body != "console.log('agentx')" {
		t.Fatalf("GET /assets/app.js body = %q", body)
	}

	apiRecorder := httptest.NewRecorder()
	handler.ServeHTTP(apiRecorder, httptest.NewRequest(http.MethodGet, "/api/ws", nil))
	if apiRecorder.Code != http.StatusNoContent {
		t.Fatalf("GET /api/ws status = %d, want %d", apiRecorder.Code, http.StatusNoContent)
	}

	healthRecorder := httptest.NewRecorder()
	handler.ServeHTTP(healthRecorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthRecorder.Code != http.StatusNoContent {
		t.Fatalf("GET /healthz status = %d, want %d", healthRecorder.Code, http.StatusNoContent)
	}
}

func TestNewHTTPHandlerUsesAPIWhenWebDistMissing(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(r.URL.Path))
	})
	handler := newHTTPHandler(api, nil, filepath.Join(t.TempDir(), "missing"))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/any-path", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("GET /any-path status = %d, want %d", recorder.Code, http.StatusAccepted)
	}
	if body := recorder.Body.String(); body != "/any-path" {
		t.Fatalf("GET /any-path body = %q, want %q", body, "/any-path")
	}
}

func TestNewHTTPHandlerServesEmbeddedWebDist(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected API fallback path: %s", r.URL.Path)
	})
	webFS := fstest.MapFS{
		"index.html":    {Data: []byte("embedded app shell")},
		"assets/app.js": {Data: []byte("console.log('embedded agentx')")},
	}
	handler := newHTTPHandler(api, webFS, filepath.Join(t.TempDir(), "missing"))

	indexRecorder := httptest.NewRecorder()
	handler.ServeHTTP(indexRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRecorder.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", indexRecorder.Code, http.StatusOK)
	}
	if body := indexRecorder.Body.String(); body != "embedded app shell" {
		t.Fatalf("GET / body = %q, want %q", body, "embedded app shell")
	}

	assetRecorder := httptest.NewRecorder()
	handler.ServeHTTP(assetRecorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if assetRecorder.Code != http.StatusOK {
		t.Fatalf("GET /assets/app.js status = %d, want %d", assetRecorder.Code, http.StatusOK)
	}
	if body := assetRecorder.Body.String(); body != "console.log('embedded agentx')" {
		t.Fatalf("GET /assets/app.js body = %q", body)
	}
}

func TestResolveWebDistDirPrefersWorkingDirectory(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	mustWriteIndex(t, filepath.Join(cwd, webDistDir))

	executable := filepath.Join(t.TempDir(), "bin", "agentx")
	got := resolveWebDistDirFromExecutable(webDistDir, func() (string, error) {
		return executable, nil
	})

	if got != webDistDir {
		t.Fatalf("resolveWebDistDirFromExecutable() = %q, want %q", got, webDistDir)
	}
}

func TestResolveWebDistDirFindsDistBesideBinaryParent(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	repoRoot := t.TempDir()
	mustWriteIndex(t, filepath.Join(repoRoot, webDistDir))
	executable := filepath.Join(repoRoot, "bin", "agentx")

	got := resolveWebDistDirFromExecutable(webDistDir, func() (string, error) {
		return executable, nil
	})
	want := filepath.Join(repoRoot, webDistDir)
	if got != want {
		t.Fatalf("resolveWebDistDirFromExecutable() = %q, want %q", got, want)
	}
}

func mustWriteIndex(t *testing.T, distDir string) {
	t.Helper()
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("app shell"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCLIResetAdminUpdatesPasswordAndClearsSessions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agentx.db")
	t.Setenv("AGENTX_DATA_DIR", dir)
	t.Setenv("AGENTX_SQLITE_PATH", dbPath)

	st, err := sqlitestore.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	a := app.New(st, eventbus.New(), app.Options{AdminToken: "secret", DataDir: dir})
	setup, err := a.SetupAdmin(ctx, app.SetupAdminRequest{
		SetupToken:  "secret",
		Username:    "admin",
		Password:    "old-password-1234",
		DisplayName: "Admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handled, code := runCLI(
		ctx,
		[]string{"auth", "reset-admin", "--username", "reset_admin", "--password-stdin"},
		strings.NewReader("new-password-1234\n"),
		&stdout,
		&stderr,
	)
	if !handled || code != 0 {
		t.Fatalf("runCLI handled=%v code=%d stdout=%q stderr=%q", handled, code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "reset_admin") {
		t.Fatalf("stdout = %q, want reset username", stdout.String())
	}

	st, err = sqlitestore.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a = app.New(st, eventbus.New(), app.Options{DataDir: dir})
	if _, err := a.UserForToken(ctx, setup.SessionToken); !errors.Is(err, app.ErrUnauthorized) {
		t.Fatalf("old session error = %v, want %v", err, app.ErrUnauthorized)
	}
	login, err := a.Login(ctx, app.LoginRequest{Username: "reset_admin", Password: "new-password-1234"})
	if err != nil {
		t.Fatal(err)
	}
	if login.User.ID != setup.User.ID {
		t.Fatalf("login user id = %q, want %q", login.User.ID, setup.User.ID)
	}
}
