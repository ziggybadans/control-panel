package fans

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// stubProvider is a deterministic Provider for controller tests.
type stubProvider struct {
	mu       sync.Mutex
	sensors  []Sensor
	duty     map[string]float64
	taken    map[string]bool
	released []string
}

func newStub() *stubProvider {
	return &stubProvider{
		sensors: []Sensor{{ID: "cpu", Label: "CPU", C: 50}},
		duty:    map[string]float64{},
		taken:   map[string]bool{},
	}
}

func (s *stubProvider) Fans() []Fan {
	return []Fan{{ID: "f1", Label: "Fan 1", HasRPM: true}}
}

func (s *stubProvider) Sensors() []Sensor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Sensor(nil), s.sensors...)
}

func (s *stubProvider) Read(id string) (int, float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return 1000, s.duty[id], nil
}

func (s *stubProvider) SetDuty(id string, pct float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.duty[id] = pct
	s.taken[id] = true
	return nil
}

func (s *stubProvider) Release(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taken[id] = false
	s.released = append(s.released, id)
	return nil
}

type nopPub struct{}

func (nopPub) Publish(string, any) {}
func (nopPub) Subscribers() int    { return 0 }

func TestCurveDuty(t *testing.T) {
	pts := []CurvePoint{{TempC: 30, DutyPct: 20}, {TempC: 50, DutyPct: 40}, {TempC: 70, DutyPct: 100}}
	cases := []struct{ temp, want float64 }{
		{10, 20},  // below first point: flat
		{30, 20},  // exactly first
		{40, 30},  // halfway 30→50 = halfway 20→40
		{60, 70},  // halfway 50→70 = halfway 40→100
		{70, 100}, // exactly last
		{90, 100}, // beyond last: flat
	}
	for _, c := range cases {
		if got := CurveDuty(pts, c.temp); got != c.want {
			t.Errorf("CurveDuty(%v) = %v, want %v", c.temp, got, c.want)
		}
	}
	if got := CurveDuty(nil, 40); got != 100 {
		t.Errorf("empty curve should failsafe to 100, got %v", got)
	}
}

func TestSettingsValidate(t *testing.T) {
	exists := func(id string) bool { return id == "cpu" }
	bad := []Settings{
		{Mode: "chaos"},
		{Mode: ModeManual, ManualPct: 101},
		{Mode: ModeManual, ManualPct: -1},
		{Mode: ModeCurve, Sensor: "cpu", Points: []CurvePoint{{TempC: 30, DutyPct: 20}}},            // 1 point
		{Mode: ModeCurve, Sensor: "gone", Points: []CurvePoint{{30, 20}, {50, 40}}},                 // unknown sensor
		{Mode: ModeCurve, Sensor: "cpu", Points: []CurvePoint{{50, 20}, {30, 40}}},                  // unsorted
		{Mode: ModeCurve, Sensor: "cpu", Points: []CurvePoint{{30, 20}, {30, 40}}},                  // duplicate temp
		{Mode: ModeCurve, Sensor: "cpu", Points: []CurvePoint{{30, 20}, {200, 40}}},                 // temp range
		{Mode: ModeCurve, Sensor: "cpu", Points: []CurvePoint{{30, 120}, {50, 40}}},                 // duty range
		{Mode: ModeCurve, Sensor: "cpu"},                                                            // no points
		{Mode: ModeCurve, Points: []CurvePoint{{30, 20}, {50, 40}}},                                 // no sensor
	}
	for i, s := range bad {
		if err := s.Validate(exists); err == nil {
			t.Errorf("case %d: expected validation error for %+v", i, s)
		}
	}
	good := []Settings{
		{Mode: ModeAuto},
		{Mode: ModeManual, ManualPct: 55},
		{Mode: ModeCurve, Sensor: "cpu", Points: []CurvePoint{{30, 20}, {50, 40}, {70, 100}}},
	}
	for i, s := range good {
		if err := s.Validate(exists); err != nil {
			t.Errorf("case %d: unexpected error: %v", i, err)
		}
	}
}

func newTestController(t *testing.T, p Provider, control bool) *Controller {
	t.Helper()
	return NewController(p, nopPub{}, t.TempDir(), time.Second, control)
}

func TestControllerCurveAndFailsafe(t *testing.T) {
	p := newStub()
	c := newTestController(t, p, true)

	err := c.Set("f1", Settings{
		Mode: ModeCurve, Sensor: "cpu",
		Points: []CurvePoint{{TempC: 40, DutyPct: 20}, {TempC: 60, DutyPct: 80}},
	})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	c.tick()
	if got := p.duty["f1"]; got != 50 { // cpu=50 → halfway 20..80
		t.Errorf("curve duty = %v, want 50", got)
	}

	// Sensor disappears → failsafe 100%.
	p.mu.Lock()
	p.sensors = nil
	p.mu.Unlock()
	c.tick()
	if got := p.duty["f1"]; got != 100 {
		t.Errorf("failsafe duty = %v, want 100", got)
	}
	state := c.LiveState()
	if len(state.Fans) != 1 || !state.Fans[0].Failsafe {
		t.Errorf("expected failsafe flag in state, got %+v", state.Fans)
	}
}

func TestControllerAutoReleases(t *testing.T) {
	p := newStub()
	c := newTestController(t, p, true)
	if err := c.Set("f1", Settings{Mode: ModeManual, ManualPct: 30}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	c.tick()
	if !p.taken["f1"] {
		t.Fatal("manual mode should take the fan")
	}
	if err := c.Set("f1", Settings{Mode: ModeAuto}); err != nil {
		t.Fatalf("Set auto: %v", err)
	}
	c.tick()
	if p.taken["f1"] {
		t.Error("auto mode should release the fan")
	}
}

func TestControllerReleaseAll(t *testing.T) {
	p := newStub()
	c := newTestController(t, p, true)
	_ = c.Set("f1", Settings{Mode: ModeManual, ManualPct: 30})
	c.tick()
	c.ReleaseAll()
	if p.taken["f1"] {
		t.Error("ReleaseAll should hand the fan back to firmware")
	}
	c.ReleaseAll() // idempotent
}

func TestControlDisabled(t *testing.T) {
	p := newStub()
	c := newTestController(t, p, false)
	if err := c.Set("f1", Settings{Mode: ModeManual, ManualPct: 30}); err == nil {
		t.Error("Set should fail when fans.control is false")
	}
	c.tick()
	if p.taken["f1"] {
		t.Error("disabled controller must never write PWM")
	}
}

func TestSettingsPersistence(t *testing.T) {
	p := newStub()
	dir := t.TempDir()
	c := NewController(p, nopPub{}, dir, time.Second, true)
	want := Settings{Mode: ModeManual, ManualPct: 42}
	if err := c.Set("f1", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	c2 := NewController(newStub(), nopPub{}, dir, time.Second, true)
	got, ok := c2.Snap().Settings["f1"]
	if !ok || got.Mode != want.Mode || got.ManualPct != want.ManualPct {
		t.Errorf("reloaded settings = %+v (ok=%v), want %+v", got, ok, want)
	}
}

func TestUnknownFan(t *testing.T) {
	c := newTestController(t, newStub(), true)
	if err := c.Set("nope", Settings{Mode: ModeManual, ManualPct: 10}); err == nil {
		t.Error("expected error for unknown fan")
	} else if want := fmt.Sprintf("unknown fan %q", "nope"); err.Error() != want {
		t.Logf("error text: %v", err) // informational; exact text not critical
	}
}
