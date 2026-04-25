package eventbus

import (
	"context"
	"sync"

	"github.com/meteorsky/agentx/internal/domain"
)

type Filter struct {
	OrganizationID string
	ConversationID string
}

type Bus struct {
	mu   sync.RWMutex
	next int
	subs map[int]subscription
}

type subscription struct {
	filter Filter
	ch     chan domain.Event
}

func New() *Bus {
	return &Bus{subs: make(map[int]subscription)}
}

func (b *Bus) Subscribe(ctx context.Context, filter Filter) (<-chan domain.Event, func()) {
	if ctx == nil {
		panic("eventbus: nil context")
	}

	b.mu.Lock()
	id := b.next
	b.next++
	ch := make(chan domain.Event, 32)
	b.subs[id] = subscription{filter: filter, ch: ch}
	b.mu.Unlock()

	done := make(chan struct{})
	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			if sub, ok := b.subs[id]; ok {
				delete(b.subs, id)
				close(sub.ch)
			}
			b.mu.Unlock()
			close(done)
		})
	}

	go func() {
		select {
		case <-ctx.Done():
			unsubscribe()
		case <-done:
		}
	}()

	return ch, unsubscribe
}

func (b *Bus) Publish(evt domain.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subs {
		if !matches(sub.filter, evt) {
			continue
		}
		select {
		case sub.ch <- evt:
		default:
		}
	}
}

func matches(filter Filter, evt domain.Event) bool {
	if filter.OrganizationID != "" && filter.OrganizationID != evt.OrganizationID {
		return false
	}
	if filter.ConversationID != "" && filter.ConversationID != evt.ConversationID {
		return false
	}
	return true
}
