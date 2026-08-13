//go:build !linux

package main

import (
	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/fans"
	"github.com/ziggybadans/control-panel/internal/metrics"
	"github.com/ziggybadans/control-panel/internal/services"
	"github.com/ziggybadans/control-panel/internal/storage"
	"github.com/ziggybadans/control-panel/internal/term"
)

// Real providers are Linux-only; main forces --mock on other platforms, so
// these stubs are never reached but must exist to compile.

func newLinuxCollector(version string) metrics.Collector {
	return metrics.NewMockCollector(version)
}

func newLinuxStorage(cfg config.Storage) storage.Provider {
	return storage.NewMockProvider()
}

func newSystemdProvider(units []string) services.Provider {
	return services.NewMockProvider()
}

func newLinuxFans() fans.Provider {
	return fans.NewMockProvider()
}

func newTermLauncher(cfg config.Config) (term.Launcher, error) {
	// Real PTYs are Linux-only; main forces --mock elsewhere, so this is
	// never reached.
	return nil, nil
}
