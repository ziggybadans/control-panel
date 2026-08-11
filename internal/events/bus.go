// Package events is a small in-process pub/sub bus feeding the SSE stream.
package events

import "sync"

type Event struct {
	Name string
	Data any
}

type Bus struct {
	mu   sync.Mutex
	subs map[int]chan Event
	next int
}

func NewBus() *Bus {
	return &Bus{subs: make(map[int]chan Event)}
}

// Subscribe registers a buffered subscriber channel. The caller must call
// the returned cancel func when done.
func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan Event, buffer)
	b.mu.Lock()
	id := b.next
	b.next++
	b.subs[id] = ch
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if c, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(c)
		}
		b.mu.Unlock()
	}
}

// Publish delivers to all subscribers without blocking; slow subscribers
// drop events (each event is a full snapshot, so drops are harmless).
func (b *Bus) Publish(name string, data any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- Event{Name: name, Data: data}:
		default:
		}
	}
}

// Subscribers returns the current subscriber count (used to idle samplers
// when nobody is watching).
func (b *Bus) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
