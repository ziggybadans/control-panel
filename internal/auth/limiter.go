package auth

import (
	"sync"
	"time"
)

// LoginLimiter throttles failed login attempts per remote IP.
type LoginLimiter struct {
	mu       sync.Mutex
	window   time.Duration
	max      int
	failures map[string][]time.Time
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{
		window:   window,
		max:      max,
		failures: make(map[string][]time.Time),
	}
}

// Allow reports whether ip may attempt a login right now.
func (l *LoginLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recentLocked(ip)) < l.max
}

// Fail records a failed attempt for ip.
func (l *LoginLimiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.recentLocked(ip), time.Now())
}

// Reset clears failures for ip (after a successful login).
func (l *LoginLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}

func (l *LoginLimiter) recentLocked(ip string) []time.Time {
	cutoff := time.Now().Add(-l.window)
	kept := l.failures[ip][:0:0]
	for _, t := range l.failures[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, ip)
	} else {
		l.failures[ip] = kept
	}
	return kept
}
