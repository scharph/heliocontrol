package service

import "sync"

// Event is pushed to UI clients (server-sent events) and the MQTT bridge.
type Event struct {
	Type string `json:"type"` // state, values, log, job, sample, recording
	Data any    `json:"data"`
}

// Bus is a small fan-out of events to subscribers. Slow subscribers miss
// events rather than blocking the publisher.
type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBus() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
