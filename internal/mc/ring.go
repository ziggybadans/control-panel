package mc

import "sync"

// logRing keeps the last capacity console lines and fans new lines out to
// subscribers. Slow subscribers drop lines rather than blocking the server's
// stdout pump.
type logRing struct {
	mu    sync.Mutex
	buf   []LogLine
	start int // index of oldest line
	count int
	subs  map[int]chan LogLine
	next  int
}

func newLogRing(capacity int) *logRing {
	return &logRing{
		buf:  make([]LogLine, capacity),
		subs: map[int]chan LogLine{},
	}
}

func (r *logRing) Append(line LogLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := (r.start + r.count) % len(r.buf)
	r.buf[idx] = line
	if r.count < len(r.buf) {
		r.count++
	} else {
		r.start = (r.start + 1) % len(r.buf)
	}
	for _, ch := range r.subs {
		select {
		case ch <- line:
		default:
		}
	}
}

// Tail returns up to n most recent lines, oldest first.
func (r *logRing) Tail(n int) []LogLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > r.count {
		n = r.count
	}
	out := make([]LogLine, n)
	for i := 0; i < n; i++ {
		out[i] = r.buf[(r.start+r.count-n+i)%len(r.buf)]
	}
	return out
}

func (r *logRing) Subscribe() (<-chan LogLine, func()) {
	ch := make(chan LogLine, 1024)
	r.mu.Lock()
	id := r.next
	r.next++
	r.subs[id] = ch
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		if c, ok := r.subs[id]; ok {
			delete(r.subs, id)
			close(c)
		}
		r.mu.Unlock()
	}
}
