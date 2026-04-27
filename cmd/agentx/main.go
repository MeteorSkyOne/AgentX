package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
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
const logDir = "logs"

func main() {
	logFile, logPath, err := configureLogging(time.Now())
	if err != nil {
		slog.Error("create log file", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close log file %s: %v\n", logPath, err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
	if err := runHTTPServer(ctx, server); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func runHTTPServer(ctx context.Context, server *http.Server) error {
	return serveHTTP(ctx, server, server.ListenAndServe)
}

func serveHTTP(ctx context.Context, server *http.Server, serve func() error) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- serve()
	}()

	select {
	case <-ctx.Done():
		slog.Info("agentx shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		slog.Info("agentx stopped")
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func configureLogging(startedAt time.Time) (*os.File, string, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, "", err
	}

	path := filepath.Join(logDir, "log-"+startedAt.Format("20060102-150405")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", err
	}

	writer := io.MultiWriter(os.Stderr, file)
	slog.SetDefault(slog.New(slog.NewTextHandler(writer, nil)))
	return file, path, nil
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
