package fans

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Publisher receives fan state each tick (implemented by the event bus).
type Publisher interface {
	Publish(name string, data any)
	Subscribers() int
}

// Controller runs the fan control loop: it evaluates each fan's settings
// (auto/manual/curve) on a ticker, writes PWM through the Provider, and
// publishes live state for the UI.
type Controller struct {
	p        Provider
	pub      Publisher
	path     string // settings persistence
	interval time.Duration
	control  bool // config gate: when false, never write PWM

	poke chan struct{} // wakes the loop after a settings change

	mu       sync.Mutex
	settings map[string]Settings
	names    map[string]string // user labels, overriding hardware ones
	engaged  map[string]bool   // fans the panel currently drives
	state    []State
	sensors  []Sensor
}

// Live is the payload of the "fans" SSE event and part of GET /api/fans.
type Live struct {
	Fans    []State  `json:"fans"`
	Sensors []Sensor `json:"sensors"`
}

// Snapshot is the GET /api/fans response.
type Snapshot struct {
	Supported bool                `json:"supported"`
	Control   bool                `json:"control"`
	Fans      []State             `json:"fans"`
	Sensors   []Sensor            `json:"sensors"`
	Settings  map[string]Settings `json:"settings"`
	Names     map[string]string   `json:"names"`
}

func NewController(p Provider, pub Publisher, dataDir string, interval time.Duration, control bool) *Controller {
	if interval < 500*time.Millisecond {
		interval = 500 * time.Millisecond
	}
	c := &Controller{
		p:        p,
		pub:      pub,
		path:     filepath.Join(dataDir, "fans.json"),
		interval: interval,
		control:  control,
		poke:     make(chan struct{}, 1),
		settings: map[string]Settings{},
		names:    map[string]string{},
		engaged:  map[string]bool{},
	}
	c.load()
	// Prime state so the first GET/SSE payload is populated.
	c.tick()
	return c
}

// Supported reports whether the platform exposes any controllable fans.
func (c *Controller) Supported() bool { return len(c.p.Fans()) > 0 }

// Control reports whether PWM writes are enabled by config.
func (c *Controller) Control() bool { return c.control }

// Run drives the loop until ctx is done, then releases every engaged fan.
func (c *Controller) Run(ctx context.Context) {
	t := time.NewTimer(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			c.ReleaseAll()
			return
		case <-c.poke:
		case <-t.C:
		}
		c.tick()
		// Idle down when nobody is watching and nothing is under control.
		next := c.interval
		if c.pub.Subscribers() == 0 && !c.anyEngaged() {
			next = 10 * time.Second
		}
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		t.Reset(next)
	}
}

func (c *Controller) anyEngaged() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, on := range c.engaged {
		if on {
			return true
		}
	}
	return false
}

// tick evaluates every fan once and publishes the result.
func (c *Controller) tick() {
	sensors := c.p.Sensors()
	byID := make(map[string]Sensor, len(sensors))
	for _, s := range sensors {
		byID[s.ID] = s
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.sensors = sensors

	fansList := c.p.Fans()
	states := make([]State, 0, len(fansList))
	for _, f := range fansList {
		label := f.Label
		if custom := c.names[f.ID]; custom != "" {
			label = custom
		}
		st := State{ID: f.ID, Label: label, HWLabel: f.Label, RPM: -1, Mode: ModeAuto}
		set, ok := c.settings[f.ID]
		if ok {
			st.Mode = set.Mode
		}

		if c.control {
			switch st.Mode {
			case ModeManual:
				st.TargetPct = set.ManualPct
				if err := c.p.SetDuty(f.ID, st.TargetPct); err != nil {
					st.Err = err.Error()
				} else {
					c.engaged[f.ID] = true
				}
			case ModeCurve:
				sensor, found := byID[set.Sensor]
				if !found {
					// Failsafe: an unreadable/vanished sensor must never
					// leave the fan slow.
					st.TargetPct = 100
					st.Failsafe = true
					st.Err = fmt.Sprintf("sensor %q unavailable — driving 100%%", set.Sensor)
				} else {
					temp := sensor.C
					st.SourceTempC = &temp
					st.TargetPct = CurveDuty(set.Points, temp)
				}
				if err := c.p.SetDuty(f.ID, st.TargetPct); err != nil {
					st.Err = err.Error()
				} else {
					c.engaged[f.ID] = true
				}
			default: // auto
				if c.engaged[f.ID] {
					if err := c.p.Release(f.ID); err != nil {
						st.Err = err.Error()
					} else {
						c.engaged[f.ID] = false
					}
				}
			}
		}

		rpm, duty, err := c.p.Read(f.ID)
		if err != nil {
			if st.Err == "" {
				st.Err = err.Error()
			}
		} else {
			st.RPM = rpm
			st.DutyPct = duty
		}
		states = append(states, st)
	}
	c.state = states

	c.pub.Publish("fans", Live{Fans: states, Sensors: sensors})
}

// ReleaseAll returns every engaged fan to firmware control (panel shutdown,
// or the loop exiting). Idempotent.
func (c *Controller) ReleaseAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, on := range c.engaged {
		if !on {
			continue
		}
		if err := c.p.Release(id); err != nil {
			slog.Warn("fan release failed", "fan", id, "err", err)
		} else {
			c.engaged[id] = false
		}
	}
}

// Set validates and stores settings for one fan, persists them, and applies
// them immediately.
func (c *Controller) Set(id string, s Settings) error {
	fanKnown := false
	for _, f := range c.p.Fans() {
		if f.ID == id {
			fanKnown = true
			break
		}
	}
	if !fanKnown {
		return fmt.Errorf("unknown fan %q", id)
	}
	if !c.control && s.Mode != ModeAuto {
		return fmt.Errorf("fan control is disabled (set fans.control: true)")
	}
	sensorExists := func(sid string) bool {
		for _, sn := range c.p.Sensors() {
			if sn.ID == sid {
				return true
			}
		}
		return false
	}
	if err := s.Validate(sensorExists); err != nil {
		return err
	}

	c.mu.Lock()
	if s.Mode == ModeAuto {
		delete(c.settings, id)
	} else {
		c.settings[id] = s
	}
	err := c.save()
	c.mu.Unlock()
	if err != nil {
		return err
	}

	// Apply without waiting for the next tick.
	select {
	case c.poke <- struct{}{}:
	default:
	}
	return nil
}

// SetName stores a custom label for one fan (empty reverts to the hardware
// label), persists it, and refreshes state.
func (c *Controller) SetName(id, name string) error {
	fanKnown := false
	for _, f := range c.p.Fans() {
		if f.ID == id {
			fanKnown = true
			break
		}
	}
	if !fanKnown {
		return fmt.Errorf("unknown fan %q", id)
	}
	name = strings.TrimSpace(name)
	if len(name) > 40 {
		return fmt.Errorf("name too long (max 40 characters)")
	}
	c.mu.Lock()
	if name == "" {
		delete(c.names, id)
	} else {
		c.names[id] = name
	}
	err := c.save()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case c.poke <- struct{}{}:
	default:
	}
	return nil
}

// LiveState returns the last published live payload (SSE initial state).
func (c *Controller) LiveState() Live {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Live{Fans: c.state, Sensors: c.sensors}
}

// Snap returns the full API snapshot including per-fan settings.
func (c *Controller) Snap() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	settings := make(map[string]Settings, len(c.settings))
	for k, v := range c.settings {
		settings[k] = v
	}
	names := make(map[string]string, len(c.names))
	for k, v := range c.names {
		names[k] = v
	}
	return Snapshot{
		Supported: len(c.p.Fans()) > 0,
		Control:   c.control,
		Fans:      c.state,
		Sensors:   c.sensors,
		Settings:  settings,
		Names:     names,
	}
}

// --- persistence ------------------------------------------------------------

type persisted struct {
	Fans  map[string]Settings `json:"fans"`
	Names map[string]string   `json:"names,omitempty"`
}

func (c *Controller) load() {
	b, err := os.ReadFile(c.path)
	if err != nil {
		return // first run
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		slog.Warn("fan settings unreadable, starting fresh", "path", c.path, "err", err)
		return
	}
	if p.Fans != nil {
		c.settings = p.Fans
	}
	if p.Names != nil {
		c.names = p.Names
	}
}

// save writes settings atomically. Caller holds c.mu.
func (c *Controller) save() error {
	b, err := json.MarshalIndent(persisted{Fans: c.settings, Names: c.names}, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
