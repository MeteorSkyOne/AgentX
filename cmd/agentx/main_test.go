package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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
