package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Sessions stores opaque bearer tokens. Only SHA-256 digests of tokens are
// kept (and persisted), so a leaked session file cannot be replayed.
type Sessions struct {
	mu   sync.Mutex
	ttl  time.Duration
	path string           // persistence file, "" = memory only
	byID map[string]int64 // sha256(token) hex -> expiry unix ms
}

func NewSessions(ttl time.Duration, dataDir string) *Sessions {
	s := &Sessions{
		ttl:  ttl,
		byID: make(map[string]int64),
	}
	if dataDir != "" {
		s.path = filepath.Join(dataDir, "sessions.json")
		s.load()
	}
	return s
}

// Create mints a new session token.
func (s *Sessions) Create() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	s.byID[digest(token)] = time.Now().Add(s.ttl).UnixMilli()
	s.saveLocked()
	return token, nil
}

// Valid reports whether token is a live session.
func (s *Sessions) Valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.byID[digest(token)]
	return ok && time.Now().UnixMilli() < exp
}

// Revoke deletes a session token.
func (s *Sessions) Revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, digest(token))
	s.saveLocked()
}

func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Sessions) gcLocked() {
	now := time.Now().UnixMilli()
	for id, exp := range s.byID {
		if exp <= now {
			delete(s.byID, id)
		}
	}
}

func (s *Sessions) load() {
	if s.path == "" {
		return
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &s.byID)
	s.gcLocked()
}

func (s *Sessions) saveLocked() {
	if s.path == "" {
		return
	}
	b, err := json.Marshal(s.byID)
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}
