package mc

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// readPlayerFile parses whitelist.json / ops.json / banned-players.json.
// These files are owned by the server; the panel only reads them and issues
// console commands for changes so the server stays the source of truth.
func readPlayerFile(dir, name string) []NamedPlayer {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil
	}
	var raw []struct {
		Name   string `json:"name"`
		UUID   string `json:"uuid"`
		Level  int    `json:"level"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return nil
	}
	out := make([]NamedPlayer, 0, len(raw))
	for _, p := range raw {
		out = append(out, NamedPlayer{Name: p.Name, UUID: p.UUID, Level: p.Level, Reason: p.Reason})
	}
	return out
}
