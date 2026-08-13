package fans

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// mockProvider simulates a typical tower: CPU fan, two intakes, one exhaust,
// with temperatures that drift and RPM that tracks duty.
type mockProvider struct {
	mu    sync.Mutex
	start time.Time
	fans  map[string]*mockFan
	order []string
}

type mockFan struct {
	id, label string
	maxRPM    int
	readOnly  bool
	taken     bool
	manualPct float64
}

func NewMockProvider() Provider {
	p := &mockProvider{start: time.Now(), fans: map[string]*mockFan{}}
	for _, f := range []*mockFan{
		{id: "mock:pwm1", label: "CPU fan", maxRPM: 2100},
		{id: "mock:pwm2", label: "Front intake 1", maxRPM: 1500},
		{id: "mock:pwm3", label: "Front intake 2", maxRPM: 1500},
		{id: "mock:pwm4", label: "Rear exhaust", maxRPM: 1350},
		// Demonstrates a driver that gates PWM writes (nct6683-style).
		{id: "mock:pwm5", label: "Chipset fan", maxRPM: 3000, readOnly: true},
	} {
		p.fans[f.id] = f
		p.order = append(p.order, f.id)
	}
	return p
}

func (p *mockProvider) t() float64 { return time.Since(p.start).Seconds() }

func mockWave(t, period, phase float64) float64 {
	return 0.5 + 0.5*math.Sin(2*math.Pi*t/period+phase)
}

func (p *mockProvider) cpuTemp() float64 {
	t := p.t()
	return 42 + 21*mockWave(t, 150, 0)*mockWave(t, 37, 1.1) + (rand.Float64()-0.5)*1.2
}

func (p *mockProvider) Sensors() []Sensor {
	t := p.t()
	return []Sensor{
		{ID: "mock:cpu", Label: "CPU package", C: round1(p.cpuTemp())},
		{ID: "mock:system", Label: "System", C: round1(37 + 5*mockWave(t, 300, 2))},
		{ID: "mock:nvme", Label: "NVMe composite", C: round1(39 + 6*mockWave(t, 240, 0.6))},
		{ID: "mock:drive", Label: "Drive bay", C: round1(33 + 3*mockWave(t, 600, 3))},
	}
}

func (p *mockProvider) Fans() []Fan {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Fan, 0, len(p.order))
	for _, id := range p.order {
		f := p.fans[id]
		out = append(out, Fan{ID: f.id, Label: f.label, HasRPM: true, Writable: !f.readOnly})
	}
	return out
}

// autoDuty models the firmware curve fans follow before the panel takes over.
func (p *mockProvider) autoDuty() float64 {
	d := 22 + (p.cpuTemp()-40)*2.1
	return math.Min(100, math.Max(20, d))
}

func (p *mockProvider) Read(id string) (int, float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.fans[id]
	if !ok {
		return -1, 0, fmt.Errorf("unknown fan %q", id)
	}
	duty := p.autoDuty()
	if f.taken {
		duty = f.manualPct
	}
	rpm := 0
	if duty > 0 {
		rpm = int(float64(f.maxRPM)*(0.12+0.88*duty/100) + (rand.Float64()-0.5)*40)
	}
	return rpm, round1(duty), nil
}

func (p *mockProvider) SetDuty(id string, pct float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.fans[id]
	if !ok {
		return fmt.Errorf("unknown fan %q", id)
	}
	f.taken = true
	f.manualPct = math.Min(100, math.Max(0, pct))
	return nil
}

func (p *mockProvider) Release(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.fans[id]
	if !ok {
		return fmt.Errorf("unknown fan %q", id)
	}
	f.taken = false
	return nil
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
