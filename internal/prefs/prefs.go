// Package prefs persists the UI's customization blob (dashboard layout,
// theme, density) server-side so preferences follow the user across machines.
package prefs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const maxBytes = 128 * 1024

type Store struct {
	mu   sync.Mutex
	path string
}

func New(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "prefs.json")}
}

// Get returns the stored blob, or "{}" when nothing is saved yet.
func (s *Store) Get() json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil || !json.Valid(b) {
		return json.RawMessage("{}")
	}
	return b
}

// Set validates and atomically replaces the stored blob.
func (s *Store) Set(blob []byte) error {
	if len(blob) > maxBytes {
		return fmt.Errorf("preferences too large (%d bytes, max %d)", len(blob), maxBytes)
	}
	if !json.Valid(blob) {
		return fmt.Errorf("preferences must be valid JSON")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
