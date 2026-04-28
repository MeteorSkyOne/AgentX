package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/runtime"
)

var ErrSessionClosed = errors.New("cli runtime session closed")
var ErrSessionAlreadyStarted = errors.New("cli runtime session already started")

type Command struct {
	Name  string
	Args  []string
	Dir   string
	Env   map[string]string
	Stdin []byte
}

type CommandBuilder func(input runtime.Input) (Command, error)

type LineHandler interface {
	HandleLine(line []byte) ([]runtime.Event, error)
	Finish(stderr string, waitErr error) (runtime.Event, bool)
	CurrentSessionID() string
}

type Session struct {
	fallbackID string
	build      CommandBuilder
	handler    LineHandler
	events     chan runtime.Event

	mu      sync.RWMutex
	alive   bool
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
	close   sync.Once
}

func NewSession(fallbackID string, build CommandBuilder, handler LineHandler) *Session {
	return &Session{
		fallbackID: fallbackID,
		build:      build,
		handler:    handler,
		events:     make(chan runtime.Event, 64),
		alive:      true,
		done:       make(chan struct{}),
	}
}

func (s *Session) Send(ctx context.Context, input runtime.Input) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	if s.started {
		s.mu.Unlock()
		return ErrSessionAlreadyStarted
	}
	s.started = true
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	command, err := s.build(input)
	if err != nil {
		slog.Error("agent cli command build failed", "session_id", s.fallbackID, "error", err)
		s.finishStartFailure(cancel)
		return err
	}
	slog.Info("agent cli command starting", "session_id", s.fallbackID, "command", command.Name, "args", safeCommandArgs(command.Args), "dir", command.Dir)
	cmd := exec.CommandContext(runCtx, command.Name, command.Args...)
	if command.Dir != "" {
		cmd.Dir = command.Dir
	}
	cmd.Env = mergeEnv(os.Environ(), command.Env)
	if command.Stdin != nil {
		cmd.Stdin = bytes.NewReader(command.Stdin)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("agent cli stdout pipe failed", "session_id", s.fallbackID, "command", command.Name, "dir", command.Dir, "error", err)
		s.finishStartFailure(cancel)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		slog.Error("agent cli stderr pipe failed", "session_id", s.fallbackID, "command", command.Name, "dir", command.Dir, "error", err)
		s.finishStartFailure(cancel)
		return err
	}
	if err := cmd.Start(); err != nil {
		slog.Error("agent cli command start failed", "session_id", s.fallbackID, "command", command.Name, "args", safeCommandArgs(command.Args), "dir", command.Dir, "error", err)
		s.finishStartFailure(cancel)
		return err
	}

	go s.run(runCtx, cancel, cmd, command, stdout, stderr)
	return nil
}

func (s *Session) Events() <-chan runtime.Event {
	return s.events
}

func (s *Session) CurrentSessionID() string {
	if id := strings.TrimSpace(s.handler.CurrentSessionID()); id != "" {
		return id
	}
	return s.fallbackID
}

func (s *Session) Alive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.alive
}

func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	started := s.started
	alreadyClosed := !s.alive
	if !alreadyClosed {
		s.alive = false
	}
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if !started {
		if !alreadyClosed {
			s.closeChannels()
		}
		return nil
	}

	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) run(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, command Command, stdout io.Reader, stderr io.Reader) {
	defer cancel()
	defer func() {
		s.mu.Lock()
		s.alive = false
		s.mu.Unlock()
		s.closeChannels()
	}()

	var stderrBuffer lockedBuffer
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		_, _ = io.Copy(&stderrBuffer, stderr)
	}()

	terminalEmitted := s.scanStdout(ctx, stdout)
	waitErr := cmd.Wait()
	stderrWG.Wait()
	stderrText := stderrBuffer.String()
	stoppedByContext := commandStoppedByContext(ctx, waitErr)

	if waitErr != nil {
		if stoppedByContext {
			slog.Info(
				"agent cli command stopped",
				"session_id", s.fallbackID,
				"provider_session_id", s.CurrentSessionID(),
				"command", command.Name,
				"args", safeCommandArgs(command.Args),
				"dir", command.Dir,
				"reason", ctx.Err(),
				"wait_error", waitErr,
				"stderr", truncateForLog(stderrText, 4000),
			)
		} else {
			slog.Error(
				"agent cli command failed",
				"session_id", s.fallbackID,
				"provider_session_id", s.CurrentSessionID(),
				"command", command.Name,
				"args", safeCommandArgs(command.Args),
				"dir", command.Dir,
				"error", waitErr,
				"stderr", truncateForLog(stderrText, 4000),
			)
		}
	} else {
		slog.Info(
			"agent cli command completed",
			"session_id", s.fallbackID,
			"provider_session_id", s.CurrentSessionID(),
			"command", command.Name,
			"dir", command.Dir,
		)
	}

	if stoppedByContext {
		return
	}

	if !terminalEmitted {
		if evt, ok := s.handler.Finish(stderrText, waitErr); ok {
			s.emit(evt)
		}
	}
}

func commandStoppedByContext(ctx context.Context, waitErr error) bool {
	return waitErr != nil && ctx.Err() != nil
}

func (s *Session) scanStdout(ctx context.Context, stdout io.Reader) bool {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	terminalEmitted := false
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		events, err := s.handler.HandleLine(line)
		if err != nil {
			slog.Error("agent cli output parse failed", "session_id", s.fallbackID, "provider_session_id", s.CurrentSessionID(), "error", err, "line", truncateForLog(string(line), 4000))
			terminalEmitted = true
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: err.Error()})
			continue
		}
		for _, evt := range events {
			if evt.Type == runtime.EventCompleted || evt.Type == runtime.EventFailed {
				terminalEmitted = true
			}
			s.emit(evt)
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		slog.Error("agent cli stdout scan failed", "session_id", s.fallbackID, "provider_session_id", s.CurrentSessionID(), "error", err)
		terminalEmitted = true
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: err.Error()})
	}
	return terminalEmitted
}

func (s *Session) emit(evt runtime.Event) {
	select {
	case <-s.done:
	case s.events <- evt:
	}
}

func (s *Session) finishStartFailure(cancel context.CancelFunc) {
	cancel()
	s.mu.Lock()
	s.alive = false
	s.started = false
	s.mu.Unlock()
	s.closeChannels()
}

func (s *Session) closeChannels() {
	s.close.Do(func() {
		close(s.done)
		close(s.events)
	})
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	env := make([]string, 0, len(base)+len(overrides))
	seen := make(map[string]struct{}, len(base)+len(overrides))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			env = append(env, item)
			continue
		}
		if _, override := overrides[key]; override {
			continue
		}
		seen[key] = struct{}{}
		env = append(env, item)
	}
	for key, value := range overrides {
		if _, ok := seen[key]; ok {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
}

func safeCommandArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	safe := append([]string(nil), args...)
	for i, arg := range safe {
		if i == len(safe)-1 {
			safe[i] = redactedArg("prompt", arg)
			continue
		}
		if strings.HasPrefix(arg, "developer_instructions=") {
			key, value, _ := strings.Cut(arg, "=")
			safe[i] = key + "=" + redactedArg("value", value)
			continue
		}
		if i > 0 && safe[i-1] == "--append-system-prompt" {
			safe[i] = redactedArg("system-prompt", arg)
		}
	}
	return safe
}

func redactedArg(label string, value string) string {
	if value == "" {
		return "<" + label + ":0 chars>"
	}
	return "<" + label + ":" + strconv.Itoa(len([]rune(value))) + " chars>"
}

func truncateForLog(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "\n[truncated]"
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.buf.String())
}
