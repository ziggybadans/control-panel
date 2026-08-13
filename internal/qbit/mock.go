package qbit

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// MockHashes are the torrent hashes the mock provider serves. The apps mock
// keys its Radarr/Sonarr queue on the same values so the media join (and
// therefore the watchability verdict) is exercised in mock mode.
var MockHashes = struct{ Dune, Severance, WildRobot, ISO string }{
	Dune:      "5a1f0e3c9b7d2648a0c4e17f83b6d5920ae4c318",
	Severance: "b27c4d81f6ea3095c7d2148be503f9a6d81c4720",
	WildRobot: "c93e7a25d0b84f1697c3d5e208a1b46f7d90e3c5",
	ISO:       "d40b8f61ca29e7350d81b4f92c6a70e5183db29f",
}

// mockProvider simulates a busy qBittorrent: progress advances with the
// clock and the actions actually take effect, so the UI can be driven
// end-to-end without a real instance.
type mockProvider struct {
	mu       sync.Mutex
	start    time.Time
	stopped  map[string]bool
	seq      map[string]bool
	flPrio   map[string]bool
	gone     map[string]bool
	altSpeed bool
	dlLimit  int64
	upLimit  int64
}

func NewMockProvider() Provider {
	return &mockProvider{
		start:   time.Now(),
		stopped: map[string]bool{},
		seq:     map[string]bool{MockHashes.Dune: true},
		flPrio:  map[string]bool{MockHashes.Dune: true},
		gone:    map[string]bool{},
	}
}

func (m *mockProvider) Configured() bool { return true }

func (m *mockProvider) Status(ctx context.Context) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	elapsed := time.Since(m.start).Seconds()

	// Each entry loops so a long-running demo never goes stale.
	specs := []struct {
		hash, name, category string
		size                 int64
		pct                  float64
		speed                int64
		seeds, peers         int
		done                 bool
	}{
		{
			hash: MockHashes.Dune, category: "radarr",
			name: "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-GROUP",
			size: 48 << 30, pct: math.Mod(34+elapsed/3, 100), speed: 41 << 20,
			seeds: 27, peers: 4,
		},
		{
			hash: MockHashes.Severance, category: "sonarr",
			name: "Severance.S02E03.1080p.WEB-DL.DDP5.1.H.264-GROUP",
			size: 4 << 30, pct: math.Mod(12+elapsed/5, 100), speed: 2 << 20,
			seeds: 9, peers: 11,
		},
		{
			hash: MockHashes.WildRobot, category: "radarr",
			name: "The.Wild.Robot.2024.1080p.WEB-DL.H.264-GROUP",
			size: 9 << 30, pct: math.Mod(67+elapsed/9, 100), speed: 0,
			seeds: 0, peers: 2,
		},
		{
			hash: MockHashes.ISO, category: "",
			name: "debian-12.8.0-amd64-DVD-1.iso",
			size: 4 << 30, pct: 100, speed: 0, seeds: 0, peers: 6, done: true,
		},
	}

	st := Status{
		Configured: true, Reachable: true, AllowActions: true,
		Version: "v5.0.3", URL: "http://127.0.0.1:8085",
		Torrents: []Torrent{},
	}
	var totalDL, totalUP int64
	for _, s := range specs {
		if m.gone[s.hash] {
			continue
		}
		t := Torrent{
			Hash: s.hash, Name: s.name, Category: s.category,
			SizeBytes: s.size, Progress: s.pct / 100,
			Seeds: s.seeds, SeedsTotal: s.seeds + 3,
			Peers: s.peers, PeersTotal: s.peers + 5,
			Sequential: m.seq[s.hash], FirstLast: m.flPrio[s.hash],
			AddedOn:  time.Now().Add(-90 * time.Minute).Unix(),
			SavePath: "/media/pool/downloads/incomplete",
			Tracker:  "https://tracker.example.invalid/announce",
			Ratio:    1.42,
		}
		t.Downloaded = int64(float64(s.size) * t.Progress)
		t.LeftBytes = s.size - t.Downloaded
		switch {
		case s.done || t.Progress >= 1:
			t.State, t.Progress, t.LeftBytes = "uploading", 1, 0
			t.UPSpeed = 512 << 10
			t.CompletedOn = time.Now().Add(-20 * time.Minute).Unix()
		case m.stopped[s.hash]:
			t.State = "stoppedDL"
		case s.speed == 0:
			t.State = "stalledDL"
		default:
			t.State, t.DLSpeed = "downloading", s.speed
			t.UPSpeed = s.speed / 20
			t.ETASec = t.LeftBytes / s.speed
		}
		totalDL += t.DLSpeed
		totalUP += t.UPSpeed
		t.Watch = Watchability(t, 0)
		st.Torrents = append(st.Torrents, t)
	}
	st.Total = len(st.Torrents)
	st.Transfer = Transfer{
		DLSpeed: totalDL, UPSpeed: totalUP,
		DLData: 214 << 30, UPData: 63 << 30,
		DLLimit: m.dlLimit, UPLimit: m.upLimit,
		AltSpeed: m.altSpeed, Connection: "connected",
		DHTNodes: 342, FreeSpace: 287 << 30, Queueing: true,
	}
	return st
}

func (m *mockProvider) Files(ctx context.Context, hash string) ([]File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gone[hash] {
		return nil, fmt.Errorf("unknown torrent")
	}
	pct := math.Mod(34+time.Since(m.start).Seconds()/3, 100) / 100
	return []File{
		{Name: "Dune.Part.Two.2024.2160p.mkv", SizeBytes: 47 << 30, Progress: pct, Priority: 1},
		{Name: "Subs/English.srt", SizeBytes: 84 << 10, Progress: 1, Priority: 1},
		{Name: "sample.mkv", SizeBytes: 61 << 20, Progress: 0, Priority: 0},
	}, nil
}

func (m *mockProvider) Do(ctx context.Context, a Action) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch a.Op {
	case "altspeed":
		m.altSpeed = !m.altSpeed
		return nil
	case "dllimit":
		m.dlLimit = a.Value
		return nil
	case "uplimit":
		m.upLimit = a.Value
		return nil
	}
	if len(a.Hashes) == 0 {
		return fmt.Errorf("no torrents selected")
	}
	hashes := a.Hashes
	// "all" is qBittorrent's own wildcard (pause all / resume all).
	if len(hashes) == 1 && hashes[0] == "all" {
		hashes = []string{
			MockHashes.Dune, MockHashes.Severance, MockHashes.WildRobot, MockHashes.ISO,
		}
	}
	for _, h := range hashes {
		h = strings.ToLower(h)
		switch a.Op {
		case "pause":
			m.stopped[h] = true
		case "resume":
			delete(m.stopped, h)
		case "sequential":
			m.seq[h] = !m.seq[h]
		case "firstlast":
			m.flPrio[h] = !m.flPrio[h]
		case "delete":
			m.gone[h] = true
		case "recheck", "top", "bottom":
			// No visible effect in the mock.
		default:
			return fmt.Errorf("unknown action %q", a.Op)
		}
	}
	return nil
}
