package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/meteorsky/agentx/internal/config"
)

func main() {
	cfg := config.FromEnv()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("create data dir", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	slog.Info("agentx listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
