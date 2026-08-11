package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Publisher receives each new snapshot (implemented by the event bus).
type Publisher interface {
	Publish(name string, data any)
	Subscribers() int
}

// Sampler drives a Collector on a ticker and keeps a history ring.
// While no SSE client is connected it slows to idleEvery to stay near-zero
// cost, but keeps sampling so history has no long gaps.
type Sampler struct {
	c        Collector
	pub      Publisher
	interval time.Duration
	idle     time.Duration

	mu   sync.RWMutex
	ring []Snapshot
	max  int
}

func NewSampler(c Collector, pub Publisher, interval time.Duration, history int) *Sampler {
	idle := 10 * time.Second
	if interval > idle {
		idle = interval
	}
	return &Sampler{
		c:        c,
		pub:      pub,
		interval: interval,
		idle:     idle,
		max:      history,
	}
}

// Run blocks until ctx is done.
func (s *Sampler) Run(ctx context.Context) {
	// Prime the collector so the first published sample has real rates.
	_, _ = s.c.Sample(ctx)
	t := time.NewTimer(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		snap, err := s.c.Sample(ctx)
		if err != nil {
			slog.Warn("metrics sample failed", "err", err)
		} else {
			s.mu.Lock()
			s.ring = append(s.ring, snap)
			if len(s.ring) > s.max {
				s.ring = s.ring[len(s.ring)-s.max:]
			}
			s.mu.Unlock()
			s.pub.Publish("metrics", snap)
		}
		next := s.interval
		if s.pub.Subscribers() == 0 {
			next = s.idle
		}
		t.Reset(next)
	}
}

// History returns the most recent snapshots, oldest first.
func (s *Sampler) History() []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Snapshot, len(s.ring))
	copy(out, s.ring)
	return out
}
