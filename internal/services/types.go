// Package services exposes a strict allowlist of systemd units for viewing
// and controlling.
package services

import (
	"context"
	"fmt"
	"regexp"
)

type Service struct {
	Unit        string `json:"unit"`
	Description string `json:"description"`
	LoadState   string `json:"loadState"`   // loaded / not-found
	ActiveState string `json:"activeState"` // active / inactive / failed / activating
	SubState    string `json:"subState"`    // running / dead / waiting…
	Since       int64  `json:"since,omitempty"` // unix ms of last state change
	PID         int    `json:"pid,omitempty"`
	MemBytes    uint64 `json:"memBytes,omitempty"`
	Enabled     string `json:"enabled,omitempty"` // enabled / disabled / static
}

type Provider interface {
	List(ctx context.Context) ([]Service, error)
	// Action runs verb ("start" | "stop" | "restart") on an allowlisted unit.
	Action(ctx context.Context, unit, verb string) error
	Logs(ctx context.Context, unit string, lines int) ([]string, error)
}

// DefaultUnits is used when the config lists none. Units that don't exist on
// the host are shown as not-found and can be hidden in the UI.
var DefaultUnits = []string{
	"plexmediaserver.service",
	"smbd.service",
	"nmbd.service",
	"sshd.service",
	"nfs-server.service",
	"docker.service",
	"tailscaled.service",
	"snapraid-sync.timer",
}

var unitRe = regexp.MustCompile(`^[A-Za-z0-9:_.@\\-]+\.(service|timer|socket|mount|target|path)$`)

var validVerbs = map[string]bool{"start": true, "stop": true, "restart": true}

// CheckAction validates unit against the allowlist and verb against the
// fixed verb set. Every implementation must call this.
func CheckAction(allow []string, unit, verb string) error {
	if !validVerbs[verb] {
		return fmt.Errorf("invalid action %q", verb)
	}
	return CheckUnit(allow, unit)
}

func CheckUnit(allow []string, unit string) error {
	if !unitRe.MatchString(unit) {
		return fmt.Errorf("invalid unit name %q", unit)
	}
	for _, u := range allow {
		if u == unit {
			return nil
		}
	}
	return fmt.Errorf("unit %q is not in the configured allowlist", unit)
}
