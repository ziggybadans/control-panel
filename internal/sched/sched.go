// Package sched runs user-defined scheduled tasks: automatic Minecraft
// backups, service restarts, snapraid runs, console commands — each an
// allowlisted panel action on a simple recurrence (interval, daily, or
// weekly). There is deliberately no way to schedule arbitrary commands: a
// schedule can only do what the panel itself can do, and every execution
// lands in the audit log.
package sched

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Schedule is one recurring task.
type Schedule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`

	// Exactly one recurrence field is set.
	Every  string `json:"every,omitempty"`  // Go duration, e.g. "6h" (min 5m)
	Daily  string `json:"daily,omitempty"`  // "HH:MM", server-local time
	Weekly string `json:"weekly,omitempty"` // "mon 04:30"

	// Action + its parameters (validated against ValidActions and the
	// executor's live targets).
	Action  string `json:"action"`
	Server  string `json:"server,omitempty"`  // mc.* target
	Unit    string `json:"unit,omitempty"`    // service.restart target
	Command string `json:"command,omitempty"` // mc.command text
	// Keep prunes mc.backup archives beyond this count (0 = keep all).
	Keep int `json:"keep,omitempty"`
	// OnlyIfRunning skips mc.backup / mc.command when the server is down.
	// mc.restart always skips stopped servers (a schedule must never
	// surprise-start one).
	OnlyIfRunning bool `json:"onlyIfRunning,omitempty"`

	NextRun    int64  `json:"nextRun"` // unix ms
	LastRun    int64  `json:"lastRun,omitempty"`
	LastResult string `json:"lastResult,omitempty"`

	retries int
	runNow  bool
}

// ValidActions is the complete set of schedulable operations.
var ValidActions = map[string]bool{
	"mc.backup":       true,
	"mc.restart":      true,
	"mc.command":      true,
	"service.restart": true,
	"snapraid.sync":   true,
	"snapraid.scrub":  true,
}

var (
	dailyRe  = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)
	weeklyRe = regexp.MustCompile(`^(sun|mon|tue|wed|thu|fri|sat) ([01]\d|2[0-3]):([0-5]\d)$`)
	weekdays = map[string]time.Weekday{
		"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
		"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday,
		"sat": time.Saturday,
	}
)

// Validate checks everything that doesn't need live targets.
func (s *Schedule) Validate() error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" || len(s.Name) > 60 {
		return fmt.Errorf("name is required (max 60 characters)")
	}
	if !ValidActions[s.Action] {
		return fmt.Errorf("invalid action %q", s.Action)
	}
	set := 0
	for _, v := range []string{s.Every, s.Daily, s.Weekly} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		return fmt.Errorf("exactly one of every / daily / weekly must be set")
	}
	switch {
	case s.Every != "":
		d, err := time.ParseDuration(s.Every)
		if err != nil {
			return fmt.Errorf("invalid interval %q (use e.g. 30m, 6h)", s.Every)
		}
		if d < 5*time.Minute || d > 30*24*time.Hour {
			return fmt.Errorf("interval must be between 5m and 720h")
		}
	case s.Daily != "":
		if !dailyRe.MatchString(s.Daily) {
			return fmt.Errorf("daily time must be HH:MM (24h), got %q", s.Daily)
		}
	case s.Weekly != "":
		if !weeklyRe.MatchString(s.Weekly) {
			return fmt.Errorf("weekly must be \"<sun..sat> HH:MM\", got %q", s.Weekly)
		}
	}
	switch s.Action {
	case "mc.backup", "mc.restart", "mc.command":
		if s.Server == "" {
			return fmt.Errorf("a server is required for %s", s.Action)
		}
	case "service.restart":
		if s.Unit == "" {
			return fmt.Errorf("a unit is required for service.restart")
		}
	}
	if s.Action == "mc.command" {
		if strings.TrimSpace(s.Command) == "" {
			return fmt.Errorf("a command is required for mc.command")
		}
		if strings.ContainsAny(s.Command, "\r\n") {
			return fmt.Errorf("command must be a single line")
		}
	}
	if s.Keep < 0 || s.Keep > 100 {
		return fmt.Errorf("keep must be 0-100 (0 = keep all)")
	}
	return nil
}

// next returns the first occurrence strictly after now.
func (s *Schedule) next(now time.Time) time.Time {
	switch {
	case s.Every != "":
		d, _ := time.ParseDuration(s.Every)
		return now.Add(d)
	case s.Daily != "":
		m := dailyRe.FindStringSubmatch(s.Daily)
		return nextAtTime(now, atoi(m[1]), atoi(m[2]), nil)
	case s.Weekly != "":
		m := weeklyRe.FindStringSubmatch(s.Weekly)
		day := weekdays[m[1]]
		return nextAtTime(now, atoi(m[2]), atoi(m[3]), &day)
	}
	return now.Add(time.Hour) // unreachable after Validate
}

// nextAtTime finds the next hh:mm (optionally on a specific weekday)
// strictly after now, respecting DST via time.Date.
func nextAtTime(now time.Time, hh, mm int, day *time.Weekday) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
	for i := 0; i < 8; i++ {
		if t.After(now) && (day == nil || t.Weekday() == *day) {
			return t
		}
		t = time.Date(t.Year(), t.Month(), t.Day()+1, hh, mm, 0, 0, now.Location())
	}
	return t
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// Describe renders the recurrence for audit details.
func (s *Schedule) Describe() string {
	switch {
	case s.Every != "":
		return "every " + s.Every
	case s.Daily != "":
		return "daily at " + s.Daily
	case s.Weekly != "":
		return "weekly " + s.Weekly
	}
	return ""
}

func newID() string {
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}
