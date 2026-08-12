package apps

import (
	"context"
	"math"
	"sync"
	"time"
)

// mockProvider simulates a typical media stack, with queue progress that
// advances over time so the UI feels live.
type mockProvider struct {
	mu    sync.Mutex
	start time.Time
}

func NewMockProvider() Provider {
	return &mockProvider{start: time.Now()}
}

func (m *mockProvider) Configured() bool { return true }

func (m *mockProvider) List(ctx context.Context) []AppStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	elapsed := time.Since(m.start).Seconds()
	// Simulated download progress: loops so the demo never goes stale.
	p1 := math.Mod(34+elapsed/3, 100)
	p2 := math.Mod(67+elapsed/9, 100)
	p3 := math.Mod(12+elapsed/5, 100)

	return []AppStatus{
		{
			Name: "Radarr", Type: "radarr", URL: "http://127.0.0.1:7878",
			Reachable: true, Version: "5.14.0.9383",
			HealthIssues: []string{},
			QueueCount:   2,
			Queue: []QueueItem{
				{Title: "Dune: Part Two", Status: "downloading", Progress: p1, TimeLeft: "00:18:22"},
				{Title: "The Wild Robot", Status: "downloading", Progress: p2, TimeLeft: "01:02:45"},
			},
			Missing:      7,
			UpcomingWeek: 3,
		},
		{
			Name: "Sonarr", Type: "sonarr", URL: "http://127.0.0.1:8989",
			Reachable: true, Version: "4.0.11.2680",
			HealthIssues: []string{
				"Indexer prowlarr-books is unavailable due to failures",
			},
			QueueCount: 1,
			Queue: []QueueItem{
				{Title: "Severance", Status: "downloading", Progress: p3, TimeLeft: "00:41:10"},
			},
			Missing:      23,
			UpcomingWeek: 11,
		},
		{
			Name: "Prowlarr", Type: "prowlarr", URL: "http://127.0.0.1:9696",
			Reachable: true, Version: "1.27.0.4852",
			HealthIssues: []string{},
			Queue:        []QueueItem{},
		},
		{
			Name: "Jellyseerr", Type: "jellyseerr", URL: "http://127.0.0.1:5055",
			Reachable: true, Version: "2.3.0",
			HealthIssues: []string{},
			Queue:        []QueueItem{},
			Requests: &RequestCounts{
				Pending: 3, Approved: 12, Processing: 2, Available: 148, Total: 165,
			},
		},
	}
}
