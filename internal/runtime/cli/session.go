package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/runtime"
)

var ErrSessionClosed = errors.New("cli runtime session closed")
var ErrSessionAlreadyStarted = errors.New("cli runtime session already started")

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  map[string]string
}

type CommandBuilder func(input runtime.Input) Command

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

	command := s.build(input)
	cmd := exec.CommandContext(runCtx, command.Name, command.Args...)
	if command.Dir != "" {
		cmd.Dir = command.Dir
	}
	cmd.Env = mergeEnv(os.Environ(), command.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.finishStartFailure(cancel)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.finishStartFailure(cancel)
		return err
	}
	if err := cmd.Start(); err != nil {
		s.finishStartFailure(cancel)
		return err
	}

	go s.run(runCtx, cancel, cmd, stdout, stderr)
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

func (s *Session) run(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, stdout io.Reader, stderr io.Reader) {
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

	if !terminalEmitted {
		if evt, ok := s.handler.Finish(stderrBuffer.String(), waitErr); ok {
			s.emit(evt)
		}
	}
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
