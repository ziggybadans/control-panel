package services

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

type mockProvider struct {
	mu    sync.Mutex
	units []string
	state map[string]*Service
}

func NewMockProvider() Provider {
	now := time.Now()
	mk := func(unit, desc, active, sub string, sinceAgo time.Duration, pid int, mem uint64, enabled string) *Service {
		return &Service{
			Unit: unit, Description: desc, LoadState: "loaded",
			ActiveState: active, SubState: sub,
			Since: now.Add(-sinceAgo).UnixMilli(),
			PID:   pid, MemBytes: mem, Enabled: enabled,
		}
	}
	state := map[string]*Service{
		"plexmediaserver.service": mk("plexmediaserver.service", "Plex Media Server", "active", "running", 37*24*time.Hour, 1204, 780<<20, "enabled"),
		"smbd.service":            mk("smbd.service", "Samba SMB Daemon", "active", "running", 37*24*time.Hour, 998, 42<<20, "enabled"),
		"sshd.service":            mk("sshd.service", "OpenBSD Secure Shell server", "active", "running", 37*24*time.Hour, 812, 9<<20, "enabled"),
		"docker.service":          mk("docker.service", "Docker Application Container Engine", "inactive", "dead", 12*24*time.Hour, 0, 0, "disabled"),
		"tailscaled.service":      mk("tailscaled.service", "Tailscale node agent", "active", "running", 37*24*time.Hour, 875, 31<<20, "enabled"),
		"snapraid-sync.timer":     mk("snapraid-sync.timer", "Nightly SnapRAID sync", "active", "waiting", 37*24*time.Hour, 0, 0, "enabled"),
	}
	var units []string
	for u := range state {
		units = append(units, u)
	}
	return &mockProvider{units: units, state: state}
}

func (m *mockProvider) List(ctx context.Context) ([]Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	order := []string{
		"plexmediaserver.service", "smbd.service", "sshd.service",
		"tailscaled.service", "docker.service", "snapraid-sync.timer",
	}
	out := make([]Service, 0, len(order))
	for _, u := range order {
		if s, ok := m.state[u]; ok {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (m *mockProvider) Action(ctx context.Context, unit, verb string) error {
	if err := CheckAction(m.units, unit, verb); err != nil {
		return err
	}
	time.Sleep(600 * time.Millisecond) // simulate systemctl latency
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state[unit]
	switch verb {
	case "start", "restart":
		s.ActiveState, s.SubState = "active", "running"
		s.PID = 2000 + rand.IntN(6000)
		if s.MemBytes == 0 {
			s.MemBytes = 30 << 20
		}
	case "stop":
		s.ActiveState, s.SubState = "inactive", "dead"
		s.PID = 0
		s.MemBytes = 0
	}
	s.Since = time.Now().UnixMilli()
	return nil
}

func (m *mockProvider) Logs(ctx context.Context, unit string, lines int) ([]string, error) {
	if err := CheckUnit(m.units, unit); err != nil {
		return nil, err
	}
	base := time.Now().Add(-90 * time.Minute)
	var out []string
	for i := 0; i < 14; i++ {
		ts := base.Add(time.Duration(i) * 6 * time.Minute).Format("2006-01-02T15:04:05-0700")
		out = append(out, fmt.Sprintf("%s bastion %s[1204]: %s", ts, unit[:len(unit)-len(".service")], mockLogLine(unit, i)))
	}
	return out, nil
}

func mockLogLine(unit string, i int) string {
	plex := []string{
		"Scanned library section 1 (Movies): 4211 items",
		"Transcode session started for user zig (Dune: Part Two, 1080p -> 720p)",
		"Butler task CleanOldBundles completed in 2.1s",
		"Streaming session ended, reason: completed",
		"Detected intro markers for 12 episodes in section 2",
	}
	generic := []string{
		"Reloading configuration",
		"Connection from 10.0.0.31 established",
		"Periodic health check passed",
		"Cache flushed (128 entries)",
		"Worker pool resized to 4",
	}
	if unit == "plexmediaserver.service" {
		return plex[i%len(plex)]
	}
	return generic[i%len(generic)]
}
