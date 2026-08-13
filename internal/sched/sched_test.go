package sched

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubExec struct {
	mu      sync.Mutex
	runs    []Schedule
	fail    int // fail the first N runs
	badTgt  bool
	details string
}

func (s *stubExec) Run(ctx context.Context, sc Schedule) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, sc)
	if s.fail > 0 {
		s.fail--
		return "", fmt.Errorf("boom")
	}
	return s.details, nil
}

func (s *stubExec) ValidateTarget(sc Schedule) error {
	if s.badTgt {
		return fmt.Errorf("no such target")
	}
	return nil
}

func (s *stubExec) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.runs)
}

func valid() Schedule {
	return Schedule{
		Name: "nightly backup", Enabled: true,
		Daily: "04:00", Action: "mc.backup", Server: "survival", Keep: 7,
	}
}

func TestValidate(t *testing.T) {
	bad := []func(*Schedule){
		func(s *Schedule) { s.Name = "" },
		func(s *Schedule) { s.Action = "shell.exec" }, // never
		func(s *Schedule) { s.Daily = "" },            // no recurrence
		func(s *Schedule) { s.Every = "6h" },          // two recurrences
		func(s *Schedule) { s.Daily = "24:00" },
		func(s *Schedule) { s.Daily = ""; s.Every = "1m" },        // too frequent
		func(s *Schedule) { s.Daily = ""; s.Weekly = "mon 4:00" }, // HH required
		func(s *Schedule) { s.Server = "" },
		func(s *Schedule) { s.Keep = 101 },
		func(s *Schedule) { s.Action = "mc.command" },                     // no command
		func(s *Schedule) { s.Action = "mc.command"; s.Command = "a\nb" }, // multiline
		func(s *Schedule) { s.Action = "service.restart"; s.Server = "" }, // no unit
	}
	for i, mutate := range bad {
		s := valid()
		mutate(&s)
		if err := s.Validate(); err == nil {
			t.Errorf("case %d: expected validation error for %+v", i, s)
		}
	}
	good := []Schedule{
		valid(),
		{Name: "x", Every: "6h", Action: "snapraid.sync"},
		{Name: "x", Weekly: "sun 03:30", Action: "snapraid.scrub"},
		{Name: "x", Every: "30m", Action: "mc.command", Server: "s", Command: "save-all"},
		{Name: "x", Daily: "23:59", Action: "service.restart", Unit: "smbd.service"},
	}
	for i, s := range good {
		if err := s.Validate(); err != nil {
			t.Errorf("good case %d: %v", i, err)
		}
	}
}

func TestNextRun(t *testing.T) {
	loc := time.Local
	base := time.Date(2026, 8, 13, 10, 0, 0, 0, loc) // a Thursday

	s := Schedule{Every: "6h"}
	if got := s.next(base); !got.Equal(base.Add(6 * time.Hour)) {
		t.Errorf("every: got %v", got)
	}

	s = Schedule{Daily: "04:00"}
	want := time.Date(2026, 8, 14, 4, 0, 0, 0, loc) // tomorrow (04:00 passed)
	if got := s.next(base); !got.Equal(want) {
		t.Errorf("daily past: got %v, want %v", got, want)
	}
	s = Schedule{Daily: "22:30"}
	want = time.Date(2026, 8, 13, 22, 30, 0, 0, loc) // later today
	if got := s.next(base); !got.Equal(want) {
		t.Errorf("daily future: got %v, want %v", got, want)
	}

	s = Schedule{Weekly: "thu 09:00"} // 09:00 already passed this Thursday
	want = time.Date(2026, 8, 20, 9, 0, 0, 0, loc)
	if got := s.next(base); !got.Equal(want) {
		t.Errorf("weekly past: got %v, want %v", got, want)
	}
	s = Schedule{Weekly: "sun 05:00"}
	want = time.Date(2026, 8, 16, 5, 0, 0, 0, loc)
	if got := s.next(base); !got.Equal(want) {
		t.Errorf("weekly future: got %v, want %v", got, want)
	}
}

func TestEngineCRUDAndPersistence(t *testing.T) {
	dir := t.TempDir()
	ex := &stubExec{}
	e := NewEngine(dir, ex)

	created, err := e.Create(valid())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" || created.NextRun == 0 {
		t.Fatalf("created = %+v", created)
	}
	if _, err := e.Create(Schedule{Name: "bad", Daily: "04:00", Action: "mc.backup"}); err == nil {
		t.Error("create without server should fail")
	}
	ex.badTgt = true
	if _, err := e.Create(valid()); err == nil {
		t.Error("create with unknown target should fail")
	}
	ex.badTgt = false

	upd := created
	upd.Name = "renamed"
	upd.Daily = ""
	upd.Every = "12h"
	if _, err := e.Update(created.ID, upd); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Reload from disk.
	e2 := NewEngine(dir, ex)
	list := e2.List()
	if len(list) != 1 || list[0].Name != "renamed" || list[0].Every != "12h" {
		t.Fatalf("reloaded = %+v", list)
	}

	if err := e2.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(e2.List()) != 0 {
		t.Error("delete did not remove")
	}
}

func TestEngineExecutionAndRetry(t *testing.T) {
	dir := t.TempDir()
	ex := &stubExec{details: "backup started"}
	e := NewEngine(dir, ex)
	now := time.Date(2026, 8, 13, 3, 59, 0, 0, time.Local)
	e.now = func() time.Time { return now }

	s, err := e.Create(valid()) // daily 04:00 → due in 1 minute
	if err != nil {
		t.Fatal(err)
	}

	e.tick(context.Background())
	if ex.count() != 0 {
		t.Fatal("ran before due time")
	}

	now = now.Add(2 * time.Minute) // 04:01
	e.tick(context.Background())
	if ex.count() != 1 {
		t.Fatalf("runs = %d, want 1", ex.count())
	}
	got, _ := e.Get(s.ID)
	if got.LastResult != "backup started" {
		t.Errorf("lastResult = %q", got.LastResult)
	}
	next := time.UnixMilli(got.NextRun)
	if next.Hour() != 4 || next.Day() != 14 {
		t.Errorf("next run = %v, want tomorrow 04:00", next)
	}

	// Failure path: retries after 5m, then falls back to cadence.
	ex.fail = 3
	now = time.Date(2026, 8, 14, 4, 1, 0, 0, time.Local)
	e.tick(context.Background())
	got, _ = e.Get(s.ID)
	if !strings.Contains(got.LastResult, "retrying") {
		t.Errorf("lastResult = %q, want retrying", got.LastResult)
	}
	now = now.Add(6 * time.Minute)
	e.tick(context.Background()) // retry 1 (fails)
	now = now.Add(6 * time.Minute)
	e.tick(context.Background()) // retry 2 (fails) → give up until next occurrence
	got, _ = e.Get(s.ID)
	if strings.Contains(got.LastResult, "retrying") {
		t.Errorf("lastResult = %q, want final error", got.LastResult)
	}
	if d := time.UnixMilli(got.NextRun).Day(); d != 15 {
		t.Errorf("next run day = %d, want 15 (next occurrence)", d)
	}

	// RunNow works on disabled schedules without enabling them.
	dis := got
	dis.Enabled = false
	if _, err := e.Update(s.ID, dis); err != nil {
		t.Fatal(err)
	}
	before := ex.count()
	if err := e.RunNow(s.ID); err != nil {
		t.Fatal(err)
	}
	e.tick(context.Background())
	if ex.count() != before+1 {
		t.Error("RunNow did not execute")
	}
	got, _ = e.Get(s.ID)
	if got.Enabled {
		t.Error("RunNow must not enable a disabled schedule")
	}
}
