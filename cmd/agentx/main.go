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
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/httpapi"
	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/claude"
	"github.com/meteorsky/agentx/internal/runtime/codex"
	"github.com/meteorsky/agentx/internal/runtime/fake"
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
		AdminToken:        cfg.AdminToken,
		DataDir:           cfg.DataDir,
		DefaultAgentKind:  cfg.DefaultAgentKind,
		DefaultAgentModel: cfg.DefaultAgentModel,
		Runtimes: map[string]runtime.Runtime{
			domain.AgentKindFake: fake.New(),
			domain.AgentKindCodex: codex.New(codex.Options{
				Command:          cfg.CodexCommand,
				FullAuto:         cfg.CodexFullAuto,
				BypassSandbox:    cfg.CodexBypassSandbox,
				SkipGitRepoCheck: cfg.CodexSkipGitRepoCheck,
			}),
			domain.AgentKindClaude: claude.New(claude.Options{
				Command:            cfg.ClaudeCommand,
				PermissionMode:     cfg.ClaudePermissionMode,
				AllowedTools:       cfg.ClaudeAllowedTools,
				DisallowedTools:    cfg.ClaudeDisallowedTools,
				AppendSystemPrompt: cfg.ClaudeAppendSystemText,
			}),
		},
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
