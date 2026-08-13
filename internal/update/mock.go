package update

import (
	"context"
	"time"
)

// mockProvider pretends an update is available and applies it as a no-op,
// so the Settings flow is exercisable in --mock development.
type mockProvider struct{ current string }

func NewMockProvider(current string) Provider {
	return &mockProvider{current: current}
}

func (m *mockProvider) Status(ctx context.Context, force bool) Status {
	return Status{
		Configured: true,
		Repo:       "ziggybadans/control-panel",
		Current:    m.current,
		Latest: &Release{
			Tag:         "v99.0.0",
			Notes:       "Mock release.\n\n- pretend bug fixes\n- pretend features",
			PublishedAt: time.Now().UTC().Format(time.RFC3339),
			AssetSize:   9 << 20,
		},
		UpdateAvailable: true,
	}
}

func (m *mockProvider) Apply(ctx context.Context, tag string) error {
	time.Sleep(1500 * time.Millisecond)
	return nil
}
