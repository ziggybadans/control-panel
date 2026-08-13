//go:build linux

package main

import (
	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/fans"
	"github.com/ziggybadans/control-panel/internal/metrics"
	"github.com/ziggybadans/control-panel/internal/services"
	"github.com/ziggybadans/control-panel/internal/storage"
)

func newLinuxCollector(version string) metrics.Collector {
	return metrics.NewLinuxCollector(version)
}

func newLinuxStorage(cfg config.Storage) storage.Provider {
	return storage.NewLinuxProvider(cfg)
}

func newSystemdProvider(units []string) services.Provider {
	return services.NewSystemdProvider(units)
}

func newLinuxFans() fans.Provider {
	return fans.NewHwmonProvider()
}
