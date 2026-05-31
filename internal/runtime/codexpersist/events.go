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
		s.processItemsDelivered += eventProcessItemCount(evt)
	default:
	}
}

// eventProcessItemCount mirrors the app layer's runtimeProcessItems: an event
// contributes len(Process) items, or a single synthesized "thinking" item when
// only Thinking is set. The goal loop uses the running total to place
// process-break markers at the right offsets.
func eventProcessItemCount(evt runtime.Event) int {
	if len(evt.Process) > 0 {
		return len(evt.Process)
	}
	if evt.Thinking != "" {
		return 1
	}
	return 0
}

// deliveredProcessCount returns how many process items have been delivered to the
// event consumer so far.
func (s *persistentSession) deliveredProcessCount() int {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	return s.processItemsDelivered
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
