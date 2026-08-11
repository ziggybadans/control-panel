// Package metrics samples system vitals (CPU, memory, network, disk I/O,
// temperatures) and keeps a ring of recent snapshots for charts.
package metrics

import "context"

// SystemInfo is static host information, fetched once at startup.
type SystemInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`     // e.g. "Debian GNU/Linux 12 (bookworm)"
	Kernel   string `json:"kernel"` // e.g. "6.1.0-28-amd64"
	Arch     string `json:"arch"`
	CPUModel string `json:"cpuModel"`
	CPUCores int    `json:"cpuCores"`
	MemTotal uint64 `json:"memTotal"` // bytes
	BootTime int64  `json:"bootTime"` // unix ms
	Version  string `json:"version"`  // panel version
	Mock     bool   `json:"mock"`
}

// Snapshot is one computed sample; rates are derived from counter deltas.
type Snapshot struct {
	TS        int64      `json:"ts"` // unix ms
	CPU       float64    `json:"cpu"`
	PerCore   []float64  `json:"perCore"`
	Load      [3]float64 `json:"load"`
	MemUsed   uint64     `json:"memUsed"`
	MemTotal  uint64     `json:"memTotal"`
	MemCached uint64     `json:"memCached"`
	SwapUsed  uint64     `json:"swapUsed"`
	SwapTotal uint64     `json:"swapTotal"`
	Net       []NetRate  `json:"net"`
	Disk      []DiskRate `json:"disk"`
	Temps     []Temp     `json:"temps"`
}

type NetRate struct {
	Name    string  `json:"name"`
	RxBps   float64 `json:"rxBps"`
	TxBps   float64 `json:"txBps"`
	RxTotal uint64  `json:"rxTotal"`
	TxTotal uint64  `json:"txTotal"`
}

type DiskRate struct {
	Name     string  `json:"name"`
	ReadBps  float64 `json:"readBps"`
	WriteBps float64 `json:"writeBps"`
	UtilPct  float64 `json:"utilPct"`
}

type Temp struct {
	Label string  `json:"label"`
	C     float64 `json:"c"`
}

// Collector produces snapshots for one platform. Implementations keep any
// state needed to turn raw counters into rates between calls.
type Collector interface {
	Info(ctx context.Context) (SystemInfo, error)
	// Sample returns the current snapshot. The first call after startup may
	// report zero rates (no previous counters to diff against).
	Sample(ctx context.Context) (Snapshot, error)
}
