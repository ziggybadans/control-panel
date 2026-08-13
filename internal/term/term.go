// Package term provides the panel's opt-in web terminal: real PTY sessions
// on Linux (a login shell, optionally de-privileged to terminal.run_as) and
// a harmless simulated shell in mock mode.
//
// This is the one deliberate exception to the panel's allowlist-exec rule —
// a terminal is a shell by definition. It is therefore off by default,
// requires typed confirmation to open, audits session open/close, enforces
// a session cap and idle timeout, and (when the panel runs as root) refuses
// to start unless a run_as user is configured or root is explicitly allowed.
package term

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"
)

// Proc is one running terminal bound to a PTY (or a mock equivalent).
type Proc interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// Launcher starts terminal processes.
type Launcher interface {
	Start(cols, rows int) (Proc, error)
	// Describe names what a session runs, e.g. "/bin/bash as ziggy".
	Describe() string
}

const (
	// replayMax bounds the scrollback kept for reconnect replay.
	replayMax = 256 * 1024
	// subBuffer is each SSE subscriber's chunk queue; a subscriber that
	// falls this far behind is dropped (it can reconnect and replay).
	subBuffer = 1024
)

// Chunk is one unit of session output.
type Chunk struct {
	Data []byte
	Exit bool // no more output after this
}

// Session is one live terminal.
type Session struct {
	ID        string `json:"id"`
	StartedAt int64  `json:"startedAt"` // unix ms

	mgr  *Manager
	proc Proc

	mu         sync.Mutex
	replay     []byte
	subs       map[int]chan Chunk
	nextSub    int
	lastActive time.Time
	exited     bool
}

// View is a Session snapshot for JSON.
type View struct {
	ID         string `json:"id"`
	StartedAt  int64  `json:"startedAt"`
	LastActive int64  `json:"lastActive"`
}

// Manager owns terminal sessions.
type Manager struct {
	launcher    Launcher // nil = feature unavailable
	maxSessions int
	idleTimeout time.Duration

	mu       sync.Mutex
	sessions map[string]*Session
	stopped  bool
}

func NewManager(l Launcher, maxSessions int, idleTimeout time.Duration) *Manager {
	if maxSessions <= 0 {
		maxSessions = 2
	}
	if idleTimeout <= 0 {
		idleTimeout = 15 * time.Minute
	}
	m := &Manager{
		launcher:    l,
		maxSessions: maxSessions,
		idleTimeout: idleTimeout,
		sessions:    map[string]*Session{},
	}
	if l != nil {
		go m.reap()
	}
	return m
}

func (m *Manager) Enabled() bool { return m.launcher != nil }

func (m *Manager) Describe() string {
	if m.launcher == nil {
		return ""
	}
	return m.launcher.Describe()
}

func (m *Manager) MaxSessions() int { return m.maxSessions }

func (m *Manager) List() []View {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []View{}
	for _, s := range m.sessions {
		s.mu.Lock()
		out = append(out, View{ID: s.ID, StartedAt: s.StartedAt, LastActive: s.lastActive.UnixMilli()})
		s.mu.Unlock()
	}
	return out
}

// Create starts a new session.
func (m *Manager) Create(cols, rows int) (View, error) {
	if m.launcher == nil {
		return View{}, fmt.Errorf("terminal is disabled")
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return View{}, fmt.Errorf("panel is shutting down")
	}
	if len(m.sessions) >= m.maxSessions {
		m.mu.Unlock()
		return View{}, fmt.Errorf("session limit reached (%d)", m.maxSessions)
	}
	m.mu.Unlock()

	cols, rows = clampSize(cols, rows)
	proc, err := m.launcher.Start(cols, rows)
	if err != nil {
		return View{}, err
	}
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	s := &Session{
		ID:         hex.EncodeToString(raw),
		StartedAt:  time.Now().UnixMilli(),
		mgr:        m,
		proc:       proc,
		subs:       map[int]chan Chunk{},
		lastActive: time.Now(),
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	go s.pump()
	return View{ID: s.ID, StartedAt: s.StartedAt, LastActive: s.lastActive.UnixMilli()}, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Close ends one session. Reports whether it existed.
func (m *Manager) Close(id string) bool {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if ok {
		s.close()
	}
	return ok
}

// CloseAll ends every session (panel shutdown).
func (m *Manager) CloseAll() {
	m.mu.Lock()
	m.stopped = true
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.Unlock()
	for _, s := range list {
		s.close()
	}
}

// reap closes idle sessions.
func (m *Manager) reap() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		m.mu.Lock()
		if m.stopped {
			m.mu.Unlock()
			return
		}
		var idle []*Session
		for _, s := range m.sessions {
			s.mu.Lock()
			if time.Since(s.lastActive) > m.idleTimeout {
				idle = append(idle, s)
			}
			s.mu.Unlock()
		}
		m.mu.Unlock()
		for _, s := range idle {
			s.close()
		}
	}
}

// --- session ----------------------------------------------------------------

// pump moves PTY output to the replay buffer and subscribers.
func (s *Session) pump() {
	buf := make([]byte, 8192)
	for {
		n, err := s.proc.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.replay = append(s.replay, chunk...)
			if len(s.replay) > replayMax {
				s.replay = s.replay[len(s.replay)-replayMax:]
			}
			s.broadcastLocked(Chunk{Data: chunk})
			s.mu.Unlock()
		}
		if err != nil {
			s.close()
			return
		}
	}
}

// broadcastLocked fans a chunk out; slow subscribers are dropped (they can
// reconnect and replay). Caller holds s.mu.
func (s *Session) broadcastLocked(c Chunk) {
	for id, ch := range s.subs {
		select {
		case ch <- c:
		default:
			delete(s.subs, id)
			close(ch)
		}
	}
}

// Subscribe returns the replay snapshot plus a live chunk channel.
func (s *Session) Subscribe() (replay []byte, ch <-chan Chunk, cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	replay = append([]byte(nil), s.replay...)
	c := make(chan Chunk, subBuffer)
	if s.exited {
		c <- Chunk{Exit: true}
		close(c)
		return replay, c, func() {}
	}
	id := s.nextSub
	s.nextSub++
	s.subs[id] = c
	return replay, c, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if ch, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(ch)
		}
	}
}

// Input writes keyboard data to the terminal.
func (s *Session) Input(data []byte) error {
	s.mu.Lock()
	if s.exited {
		s.mu.Unlock()
		return fmt.Errorf("session has ended")
	}
	s.lastActive = time.Now()
	s.mu.Unlock()
	_, err := s.proc.Write(data)
	return err
}

func (s *Session) Resize(cols, rows int) error {
	cols, rows = clampSize(cols, rows)
	s.mu.Lock()
	s.lastActive = time.Now()
	s.mu.Unlock()
	return s.proc.Resize(cols, rows)
}

// close tears the session down exactly once and removes it from the manager.
func (s *Session) close() {
	s.mu.Lock()
	if s.exited {
		s.mu.Unlock()
		return
	}
	s.exited = true
	s.broadcastLocked(Chunk{Exit: true})
	for id, ch := range s.subs {
		delete(s.subs, id)
		close(ch)
	}
	s.mu.Unlock()

	_ = s.proc.Close()
	s.mgr.mu.Lock()
	delete(s.mgr.sessions, s.ID)
	s.mgr.mu.Unlock()
}

func clampSize(cols, rows int) (int, int) {
	if cols < 2 {
		cols = 80
	}
	if cols > 500 {
		cols = 500
	}
	if rows < 2 {
		rows = 24
	}
	if rows > 300 {
		rows = 300
	}
	return cols, rows
}
