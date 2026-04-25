package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/config"
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/httpapi"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

const webDistDir = "web/dist"

func main() {
	ctx := context.Background()
	cfg := config.FromEnv()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("create data dir", "error", err)
		os.Exit(1)
	}

	st, err := sqlitestore.Open(ctx, cfg.SQLitePath)
	if err != nil {
		slog.Error("open sqlite", "path", cfg.SQLitePath, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := st.Close(); err != nil {
			slog.Error("close sqlite", "error", err)
		}
	}()

	bus := eventbus.New()
	a := app.New(st, bus, app.Options{
		AdminToken: cfg.AdminToken,
		DataDir:    cfg.DataDir,
	})

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newHTTPHandler(httpapi.NewRouter(a, bus), webDistDir),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("agentx listening", "addr", cfg.Addr)
	if err := server.ListenAndServe(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func newHTTPHandler(apiHandler http.Handler, distDir string) http.Handler {
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		return apiHandler
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/healthz", apiHandler)
	mux.Handle("/", http.FileServer(http.Dir(distDir)))
	return mux
}
