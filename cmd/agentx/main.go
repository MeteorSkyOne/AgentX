package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/config"
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/httpapi"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
)

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

	slog.Info("agentx listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, httpapi.NewRouter(a, bus)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
