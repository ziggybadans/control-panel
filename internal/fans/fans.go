// Package fans monitors chassis/CPU fans and optionally drives them with
// user-defined curves. Discovery and monitoring are read-only; taking
// control of a fan (manual duty or a temp→duty curve) writes hwmon PWM
// values and is gated behind config + server-side confirmation.
//
// Safety properties:
//   - fans stay under firmware ("auto") control until explicitly switched
//   - a curve whose sensor disappears or fails to read drives the fan to
//     100% (failsafe), never to silence
//   - releasing a fan (or shutting the panel down) restores the exact
//     pwm/pwm_enable values it had before the panel took over
package fans

import (
	"fmt"
	"sort"
)

// Fan is one controllable PWM output.
type Fan struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	HasRPM bool   `json:"hasRpm"`
}

// State is the live view of one fan, published on the SSE stream.
type State struct {
	ID    string `json:"id"`
	Label string `json:"label"` // custom name when set, else hardware label
	// HWLabel is the hardware label (rename placeholder in the UI).
	HWLabel string  `json:"hwLabel,omitempty"`
	RPM     int     `json:"rpm"` // -1 when the fan has no tach
	DutyPct float64 `json:"dutyPct"`
	Mode    string  `json:"mode"` // auto | manual | curve
	// TargetPct is the duty the controller is asking for (manual/curve).
	TargetPct float64 `json:"targetPct,omitempty"`
	// SourceTempC is the curve's sensor reading this tick.
	SourceTempC *float64 `json:"sourceTempC,omitempty"`
	// Failsafe is set when the curve sensor was unreadable and the fan was
	// driven to 100%.
	Failsafe bool   `json:"failsafe,omitempty"`
	Err      string `json:"err,omitempty"`
}

// Sensor is a temperature input usable as a curve source.
type Sensor struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	C     float64 `json:"c"`
}

// CurvePoint maps a temperature to a fan duty.
type CurvePoint struct {
	TempC   float64 `json:"tempC"`
	DutyPct float64 `json:"dutyPct"`
}

// Settings is the persisted per-fan configuration.
type Settings struct {
	Mode      string       `json:"mode"` // auto | manual | curve
	ManualPct float64      `json:"manualPct,omitempty"`
	Sensor    string       `json:"sensor,omitempty"`
	Points    []CurvePoint `json:"points,omitempty"`
}

// Provider abstracts the platform (hwmon on Linux, synthetic in mock mode).
type Provider interface {
	// Fans lists PWM outputs. IDs are stable across restarts.
	Fans() []Fan
	// Sensors lists temperature inputs with current readings.
	Sensors() []Sensor
	// Read returns current RPM (-1 without a tach) and duty (0-100).
	Read(id string) (rpm int, dutyPct float64, err error)
	// SetDuty takes manual control of the fan and applies duty (0-100).
	SetDuty(id string, pct float64) error
	// Release returns the fan to the control state it had before the first
	// SetDuty (firmware/auto). Releasing a never-taken fan is a no-op.
	Release(id string) error
}

const (
	ModeAuto   = "auto"
	ModeManual = "manual"
	ModeCurve  = "curve"

	MaxCurvePoints = 12
)

// Validate checks a settings payload against the available sensors.
func (s Settings) Validate(sensorExists func(id string) bool) error {
	switch s.Mode {
	case ModeAuto:
		return nil
	case ModeManual:
		if s.ManualPct < 0 || s.ManualPct > 100 {
			return fmt.Errorf("manual duty must be 0-100%%, got %.1f", s.ManualPct)
		}
		return nil
	case ModeCurve:
		if s.Sensor == "" {
			return fmt.Errorf("curve mode needs a temperature sensor")
		}
		if !sensorExists(s.Sensor) {
			return fmt.Errorf("unknown sensor %q", s.Sensor)
		}
		if len(s.Points) < 2 {
			return fmt.Errorf("a curve needs at least 2 points")
		}
		if len(s.Points) > MaxCurvePoints {
			return fmt.Errorf("a curve may have at most %d points", MaxCurvePoints)
		}
		prev := -1.0
		for _, p := range s.Points {
			if p.TempC < 0 || p.TempC > 120 {
				return fmt.Errorf("curve temperature %.1f°C out of range (0-120)", p.TempC)
			}
			if p.DutyPct < 0 || p.DutyPct > 100 {
				return fmt.Errorf("curve duty %.1f%% out of range (0-100)", p.DutyPct)
			}
			if p.TempC <= prev {
				return fmt.Errorf("curve temperatures must be strictly increasing")
			}
			prev = p.TempC
		}
		return nil
	default:
		return fmt.Errorf("mode must be auto, manual, or curve; got %q", s.Mode)
	}
}

// CurveDuty interpolates the duty for temp along points (assumed sorted by
// temperature ascending): flat before the first and after the last point,
// linear in between.
func CurveDuty(points []CurvePoint, temp float64) float64 {
	if len(points) == 0 {
		return 100 // no curve = failsafe
	}
	if temp <= points[0].TempC {
		return points[0].DutyPct
	}
	last := points[len(points)-1]
	if temp >= last.TempC {
		return last.DutyPct
	}
	i := sort.Search(len(points), func(i int) bool { return points[i].TempC >= temp })
	a, b := points[i-1], points[i]
	frac := (temp - a.TempC) / (b.TempC - a.TempC)
	return a.DutyPct + frac*(b.DutyPct-a.DutyPct)
}
