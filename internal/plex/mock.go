package plex

import (
	"context"
	"time"
)

type mockProvider struct {
	start time.Time
}

func NewMockProvider() Provider { return &mockProvider{start: time.Now()} }

func (m *mockProvider) Status(ctx context.Context) Status {
	elapsed := time.Since(m.start).Milliseconds()
	return Status{
		Configured: true,
		Reachable:  true,
		Version:    "1.41.3.9314",
		Sessions: []Session{
			{
				User: "zig", Title: "Dune: Part Two", Type: "movie",
				Player: "Living Room TV", Product: "Plex for Apple TV", State: "playing",
				ProgressMS: 47*60*1000 + elapsed, DurationMS: 166 * 60 * 1000,
				Decision: "directplay", BitrateKbps: 24800,
			},
			{
				User: "mika", Title: "The We We Are", Grandparent: "Severance", Type: "episode",
				Player: "Pixel 8", Product: "Plex for Android", State: "playing",
				ProgressMS: 12*60*1000 + elapsed, DurationMS: 41 * 60 * 1000,
				Decision: "transcode", BitrateKbps: 4200,
			},
		},
		Libraries: []Library{
			{Title: "Movies", Type: "movie", Count: 4211},
			{Title: "TV Shows", Type: "show", Count: 8542},
			{Title: "Music", Type: "artist", Count: 1289},
			{Title: "Photos", Type: "photo", Count: 20419},
		},
	}
}
