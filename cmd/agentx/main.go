package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/config"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/httpapi"
	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/claude"
	"github.com/meteorsky/agentx/internal/runtime/claudepersist"
	"github.com/meteorsky/agentx/internal/runtime/codex"
	"github.com/meteorsky/agentx/internal/runtime/codexpersist"
	"github.com/meteorsky/agentx/internal/runtime/fake"
	sqlitestore "github.com/meteorsky/agentx/internal/store/sqlite"
	"github.com/meteorsky/agentx/internal/webdist"
)

const webDistDir = "web/dist"
const logDir = "logs"

func main() {
	if handled, code := runCLI(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}

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

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
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
	runtimes := map[string]runtime.Runtime{
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
		domain.AgentKindClaudePersistent: claudepersist.New(claudepersist.Options{
			Command:            cfg.ClaudeCommand,
			PermissionMode:     cfg.ClaudePermissionMode,
			AllowedTools:       cfg.ClaudeAllowedTools,
			DisallowedTools:    cfg.ClaudeDisallowedTools,
			AppendSystemPrompt: cfg.ClaudeAppendSystemText,
			IdleTimeout:        time.Duration(cfg.ClaudePersistentIdleMinutes) * time.Minute,
		}),
		domain.AgentKindCodexPersistent: codexpersist.New(codexpersist.Options{
			Command:     cfg.CodexCommand,
			IdleTimeout: time.Duration(cfg.CodexPersistentIdleMinutes) * time.Minute,
		}),
	}
	defer shutdownRuntimes(runtimes)

	a := app.New(st, bus, app.Options{
		AdminToken:        cfg.AdminToken,
		DataDir:           cfg.DataDir,
		ServerSettings:    cfg.Server,
		ServerAddr:        cfg.Addr,
		AddrOverride:      cfg.AddrOverrideActive,
		AddrOverrideValue: cfg.AddrOverrideValue,
		DefaultAgentKind:  cfg.DefaultAgentKind,
		DefaultAgentModel: cfg.DefaultAgentModel,
		ProviderLimits: app.ProviderLimitOptions{
			CodexCommand:  cfg.CodexCommand,
			ClaudeCommand: cfg.ClaudeCommand,
		},
		Runtimes: runtimes,
	})
	if err := printSetupTokenIfNeeded(ctx, os.Stdout, st.Users(), cfg.AdminToken); err != nil {
		slog.Error("check setup status", "error", err)
		os.Exit(1)
	}

	handler := newHTTPHandler(httpapi.NewRouter(a, bus), webdist.FS(), resolveWebDistDir(webDistDir))
	servers := []serverRunner{{
		name:   "http",
		server: newServer(cfg.Addr, handler),
	}}
	if cfg.Server.TLS.Enabled {
		tlsAddr := config.ServerTLSAddr(cfg.Server)
		servers = append(servers, serverRunner{
			name:   "https",
			server: newServer(tlsAddr, handler),
			serve: func(server *http.Server) func() error {
				return func() error {
					return server.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
				}
			},
		})
		slog.Info("agentx listening", "http_addr", cfg.Addr, "https_addr", tlsAddr)
	} else {
		slog.Info("agentx listening", "http_addr", cfg.Addr)
	}
	if err := runHTTPServers(ctx, servers); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func runCLI(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != "auth" {
		return false, 0
	}
	if len(args) < 2 || args[1] != "reset-admin" {
		_, _ = fmt.Fprintln(stderr, "usage: agentx auth reset-admin --username <name> --password-stdin")
		return true, 2
	}

	fs := flag.NewFlagSet("agentx auth reset-admin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	username := fs.String("username", "", "admin username")
	passwordStdin := fs.Bool("password-stdin", false, "read password from stdin")
	if err := fs.Parse(args[2:]); err != nil {
		return true, 2
	}
	if strings.TrimSpace(*username) == "" || !*passwordStdin || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: agentx auth reset-admin --username <name> --password-stdin")
		return true, 2
	}

	passwordBytes, err := io.ReadAll(stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "read password: %v\n", err)
		return true, 1
	}
	password := strings.TrimSuffix(string(passwordBytes), "\n")
	password = strings.TrimSuffix(password, "\r")

	cfg := config.FromEnv()
	st, err := sqlitestore.Open(ctx, cfg.SQLitePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "open sqlite: %v\n", err)
		return true, 1
	}
	defer func() {
		if err := st.Close(); err != nil {
			_, _ = fmt.Fprintf(stderr, "close sqlite: %v\n", err)
		}
	}()

	a := app.New(st, eventbus.New(), app.Options{DataDir: cfg.DataDir})
	user, err := a.ResetAdmin(ctx, app.ResetAdminRequest{Username: *username, Password: password})
	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidInput):
			_, _ = fmt.Fprintln(stderr, "invalid username or password")
		case errors.Is(err, sql.ErrNoRows):
			_, _ = fmt.Fprintln(stderr, "no admin user exists; run initial setup in the web UI")
		default:
			_, _ = fmt.Fprintf(stderr, "reset admin: %v\n", err)
		}
		return true, 1
	}
	_, _ = fmt.Fprintf(stdout, "admin credentials reset for %s\n", user.Username)
	return true, 0
}

type serverRunner struct {
	name   string
	server *http.Server
	serve  func(*http.Server) func() error
}

func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func runHTTPServers(ctx context.Context, servers []serverRunner) error {
	if len(servers) == 0 {
		return nil
	}

	errCh := make(chan error, len(servers))
	for _, runner := range servers {
		runner := runner
		serve := runner.server.ListenAndServe
		if runner.serve != nil {
			serve = runner.serve(runner.server)
		}
		go func() {
			if err := serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("%s server: %w", runner.name, err)
				return
			}
			errCh <- nil
		}()
	}

	select {
	case <-ctx.Done():
		return shutdownHTTPServers(servers)
	case err := <-errCh:
		if err != nil {
			_ = shutdownHTTPServers(servers)
			return err
		}
		return nil
	}
}

func shutdownHTTPServers(servers []serverRunner) error {
	slog.Info("agentx shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var shutdownErr error
	for _, runner := range servers {
		if err := runner.server.Shutdown(shutdownCtx); err != nil && shutdownErr == nil {
			shutdownErr = err
		}
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	slog.Info("agentx stopped")
	return nil
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

type passwordStatusStore interface {
	HasPassword(ctx context.Context) (bool, error)
}

func printSetupTokenIfNeeded(ctx context.Context, stdout io.Writer, users passwordStatusStore, setupToken string) error {
	hasPassword, err := users.HasPassword(ctx)
	if err != nil {
		return err
	}
	if hasPassword {
		return nil
	}
	_, err = fmt.Fprintf(stdout, "Setup token: %s\n", strings.TrimSpace(setupToken))
	return err
}

func newHTTPHandler(apiHandler http.Handler, webFS fs.FS, distDir string) http.Handler {
	webFS = resolveWebFS(webFS, distDir)
	if webFS == nil {
		return apiHandler
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/healthz", apiHandler)
	mux.Handle("/", http.FileServer(http.FS(webFS)))
	return mux
}

func resolveWebFS(webFS fs.FS, distDir string) fs.FS {
	if hasWebIndex(webFS) {
		return webFS
	}
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err == nil {
		return os.DirFS(distDir)
	}
	return nil
}

func hasWebIndex(webFS fs.FS) bool {
	if webFS == nil {
		return false
	}
	_, err := fs.Stat(webFS, "index.html")
	return err == nil
}

func resolveWebDistDir(distDir string) string {
	return resolveWebDistDirFromExecutable(distDir, os.Executable)
}

func resolveWebDistDirFromExecutable(distDir string, executablePath func() (string, error)) string {
	candidates := []string{distDir}
	if !filepath.IsAbs(distDir) {
		if executable, err := executablePath(); err == nil && executable != "" {
			executableDir := filepath.Dir(executable)
			candidates = append(candidates,
				filepath.Join(executableDir, distDir),
				filepath.Join(executableDir, "..", distDir),
			)
		}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil {
			return candidate
		}
	}
	return distDir
}

func shutdownRuntimes(runtimes map[string]runtime.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for name, rt := range runtimes {
		if s, ok := rt.(runtime.Shutdowner); ok {
			if err := s.Shutdown(ctx); err != nil {
				slog.Warn("runtime shutdown error", "runtime", name, "error", err)
			}
		}
	}
}
