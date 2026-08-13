//go:build !linux

package main

import (
	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/fans"
	"github.com/ziggybadans/control-panel/internal/metrics"
	"github.com/ziggybadans/control-panel/internal/services"
	"github.com/ziggybadans/control-panel/internal/storage"
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
