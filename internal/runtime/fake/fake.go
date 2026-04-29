package fake

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/runtime"
)

var errSessionClosed = errors.New("fake runtime session closed")

type Runtime struct{}

func New() runtime.Runtime {
	return Runtime{}
}

func (Runtime) StartSession(ctx context.Context, req runtime.StartSessionRequest) (runtime.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sessionID := "fake:" + req.SessionKey
	if req.SessionKey == "" {
		sessionID = "fake:" + req.AgentID
	}
	return &session{
		id:     sessionID,
		events: make(chan runtime.Event, 8),
		done:   make(chan struct{}),
		alive:  true,
	}, nil
}

type session struct {
	id        string
	events    chan runtime.Event
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup

	mu    sync.RWMutex
	alive bool
}

func (s *session) Send(ctx context.Context, input runtime.Input) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	text := strings.TrimSpace(input.Prompt)
	response := "Echo: " + text
	if text == "" {
		response = "Echo: empty message"
	}

	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return errSessionClosed
	}
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		if !s.emit(runtime.Event{Type: runtime.EventDelta, Text: response}) {
			return
		}
		s.emit(runtime.Event{Type: runtime.EventCompleted, Text: response})
	}()

	return nil
}

func (s *session) Events() <-chan runtime.Event {
	return s.events
}

func (s *session) CurrentSessionID() string {
	return s.id
}

func (s *session) Alive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.alive
}

func (s *session) Close(ctx context.Context) error {
	var done chan struct{}
	s.closeOnce.Do(func() {
		done = make(chan struct{})
		go func() {
			s.mu.Lock()
			s.alive = false
			close(s.done)
			s.mu.Unlock()

			s.wg.Wait()
			close(s.events)
			close(done)
		}()
	})
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *session) RespondToInputRequest(questionID string, answer string) error {
	return errors.New("fake runtime does not support input requests")
}

func (s *session) emit(evt runtime.Event) bool {
	select {
	case <-s.done:
		return false
	case s.events <- evt:
		return true
	}
}
