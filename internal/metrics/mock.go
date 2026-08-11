package metrics

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// mockCollector produces plausible synthetic data so the full UI can be
// developed and demonstrated on any platform.
type mockCollector struct {
	version string
	start   time.Time
	rxTotal uint64
	txTotal uint64
}

func NewMockCollector(version string) Collector {
	return &mockCollector{version: version, start: time.Now()}
}

func (m *mockCollector) Info(ctx context.Context) (SystemInfo, error) {
	return SystemInfo{
		Hostname: "bastion",
		OS:       "Debian GNU/Linux 12 (bookworm)",
		Kernel:   "6.1.0-28-amd64",
		Arch:     "amd64",
		CPUModel: "Intel(R) Core(TM) i5-8500T CPU @ 2.10GHz",
		CPUCores: 6,
		MemTotal: 32 * 1 << 30,
		BootTime: time.Now().Add(-37*24*time.Hour - 5*time.Hour).UnixMilli(),
		Version:  m.version,
		Mock:     true,
	}, nil
}

// wave returns a smooth pseudo-random signal in [0,1].
func wave(t, period, phase float64) float64 {
	return 0.5 + 0.5*math.Sin(2*math.Pi*t/period+phase)
}

func (m *mockCollector) Sample(ctx context.Context) (Snapshot, error) {
	t := time.Since(m.start).Seconds()
	jitter := func(scale float64) float64 { return (rand.Float64() - 0.5) * scale }

	cpu := 8 + 14*wave(t, 210, 0) + 24*wave(t, 47, 1.3)*wave(t, 13, 0.4) + jitter(6)
	cpu = clamp(cpu, 1, 98)

	perCore := make([]float64, 6)
	for i := range perCore {
		perCore[i] = clamp(cpu+jitter(22)+8*wave(t, 31, float64(i)), 0, 100)
	}

	memUsed := (14.2 + 2.2*wave(t, 300, 0.7) + jitter(0.2)) * float64(1<<30)
	rxRate := (2.4 + 26*wave(t, 90, 2.1)*wave(t, 17, 0)) * 1e6 // Plex streaming bursts
	txRate := (0.9 + 9*wave(t, 75, 4.0)) * 1e6
	m.rxTotal += uint64(rxRate)
	m.txTotal += uint64(txRate)

	read := (0.5 + 42*wave(t, 130, 1.1)*wave(t, 23, 2)) * 1e6
	write := (0.2 + 11*wave(t, 160, 0.2)) * 1e6

	return Snapshot{
		TS:        time.Now().UnixMilli(),
		CPU:       cpu,
		PerCore:   perCore,
		Load:      [3]float64{round2(cpu / 100 * 6 * 0.8), round2(cpu / 100 * 6 * 0.7), round2(cpu / 100 * 6 * 0.6)},
		MemUsed:   uint64(memUsed),
		MemTotal:  32 * 1 << 30,
		MemCached: uint64((9.8 + 1.1*wave(t, 400, 3)) * float64(1<<30)),
		SwapUsed:  256 << 20,
		SwapTotal: 4 << 30,
		Net: []NetRate{
			{Name: "enp2s0", RxBps: rxRate, TxBps: txRate, RxTotal: m.rxTotal, TxTotal: m.txTotal},
		},
		Disk: []DiskRate{
			{Name: "nvme0n1", ReadBps: read * 0.3, WriteBps: write * 0.6, UtilPct: clamp(read/3e6, 0, 60)},
			{Name: "sda", ReadBps: read, WriteBps: write * 0.2, UtilPct: clamp(read/1.2e6, 0, 95)},
			{Name: "sdb", ReadBps: read * 0.4, WriteBps: write * 0.3, UtilPct: clamp(read/2.4e6, 0, 80)},
			{Name: "sdc", ReadBps: read * 0.1, WriteBps: write, UtilPct: clamp(write/1.5e6, 0, 70)},
			{Name: "sdd", ReadBps: 0, WriteBps: 0, UtilPct: 0},
		},
		Temps: []Temp{
			{Label: "coretemp Package id 0", C: round1(42 + cpu/4 + jitter(1.5))},
			{Label: "nvme Composite", C: round1(38 + cpu/8 + jitter(1))},
			{Label: "drivetemp sda", C: round1(34 + 2*wave(t, 600, 0))},
			{Label: "drivetemp sdb", C: round1(33 + 2*wave(t, 640, 1))},
			{Label: "drivetemp sdc", C: round1(36 + 2*wave(t, 580, 2))},
			{Label: "drivetemp sdd", C: round1(31 + wave(t, 700, 3))},
		},
	}, nil
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
