package main

import (
	"context"
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
	"time"
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
	handler := newHTTPHandler(api, distDir)

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
	handler := newHTTPHandler(api, filepath.Join(t.TempDir(), "missing"))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/any-path", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("GET /any-path status = %d, want %d", recorder.Code, http.StatusAccepted)
	}
	if body := recorder.Body.String(); body != "/any-path" {
		t.Fatalf("GET /any-path body = %q, want %q", body, "/any-path")
	}
}
