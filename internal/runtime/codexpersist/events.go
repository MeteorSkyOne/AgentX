package codexpersist

import "github.com/meteorsky/agentx/internal/runtime"

func (s *persistentSession) emit(evt runtime.Event) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.events == nil {
		return
	}
	if s.done != nil {
		select {
		case <-s.done:
			return
		default:
		}
	}
	select {
	case s.events <- evt:
	default:
	}
}

func (s *persistentSession) emitFailed(errText string) {
	s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText})
	s.mu.Lock()
	s.alive = false
	s.mu.Unlock()
	s.closeEventStream()
}

func (s *persistentSession) closeEventStream() {
	s.closeOnce.Do(func() {
		s.eventMu.Lock()
		defer s.eventMu.Unlock()
		if s.done != nil {
			close(s.done)
		}
		if s.events != nil {
			close(s.events)
		}
	})
}

func (s *persistentSession) releaseTurn() {
	s.mu.Lock()
	held := s.turnHeld
	s.turnHeld = false
	s.mu.Unlock()
	if held {
		s.process.ReleaseTurn()
	}
}
