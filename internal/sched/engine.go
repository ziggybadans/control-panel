package sched

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	tickInterval = 30 * time.Second
	retryDelay   = 5 * time.Minute
	maxRetries   = 2
	// runTimeout bounds one execution (job-based actions return quickly;
	// this guards direct ones like service restarts).
	runTimeout = 5 * time.Minute
)

// Executor performs schedule actions and validates their targets. Wired up
// in main, where all providers live.
type Executor interface {
	// Run performs the action. A nil error with a detail like
	// "skipped — …" counts as success without side effects.
	Run(ctx context.Context, s Schedule) (detail string, err error)
	// ValidateTarget checks the action's target exists right now (server
	// known, unit allowlisted, snapraid configured).
	ValidateTarget(s Schedule) error
}

// Engine owns the schedules: persistence, the timing loop, and execution
// bookkeeping (retries, last-result).
type Engine struct {
	path string
	exec Executor
	// OnRun is called after every execution attempt (audit hook).
	OnRun func(s Schedule, detail string, err error)

	poke chan struct{}

	mu    sync.Mutex
	items []*Schedule
	now   func() time.Time // injectable for tests
}

func NewEngine(dataDir string, exec Executor) *Engine {
	e := &Engine{
		path: filepath.Join(dataDir, "schedules.json"),
		exec: exec,
		poke: make(chan struct{}, 1),
		now:  time.Now,
	}
	e.load()
	return e
}

// Run drives the loop until ctx is done.
func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.poke:
		case <-t.C:
		}
		e.tick(ctx)
	}
}

// tick executes every enabled schedule that is due.
func (e *Engine) tick(ctx context.Context) {
	now := e.now()
	e.mu.Lock()
	var due []*Schedule
	for _, s := range e.items {
		if s.runNow || (s.Enabled && s.NextRun > 0 && s.NextRun <= now.UnixMilli()) {
			s.runNow = false
			due = append(due, s)
		}
	}
	e.mu.Unlock()

	for _, s := range due {
		if ctx.Err() != nil {
			return
		}
		e.runOne(ctx, s)
	}
}

// runOne executes a single schedule and updates its state. Failures retry
// after retryDelay (up to maxRetries) before falling back to the regular
// cadence.
func (e *Engine) runOne(ctx context.Context, s *Schedule) {
	e.mu.Lock()
	snapshot := *s
	e.mu.Unlock()

	rctx, cancel := context.WithTimeout(ctx, runTimeout)
	detail, err := e.exec.Run(rctx, snapshot)
	cancel()

	now := e.now()
	e.mu.Lock()
	s.LastRun = now.UnixMilli()
	if err == nil {
		s.retries = 0
		s.LastResult = detail
		if s.LastResult == "" {
			s.LastResult = "ok"
		}
		s.NextRun = s.next(now).UnixMilli()
	} else {
		s.retries++
		if s.retries <= maxRetries {
			s.LastResult = fmt.Sprintf("error: %s (retrying in %s)", err, retryDelay)
			s.NextRun = now.Add(retryDelay).UnixMilli()
		} else {
			s.retries = 0
			s.LastResult = "error: " + err.Error()
			s.NextRun = s.next(now).UnixMilli()
		}
	}
	e.saveLocked()
	e.mu.Unlock()

	if e.OnRun != nil {
		e.OnRun(snapshot, detail, err)
	}
	if err != nil {
		slog.Warn("scheduled task failed", "name", snapshot.Name, "action", snapshot.Action, "err", err)
	}
}

// --- CRUD -------------------------------------------------------------------

func (e *Engine) List() []Schedule {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Schedule, 0, len(e.items))
	for _, s := range e.items {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (e *Engine) Get(id string) (Schedule, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.items {
		if s.ID == id {
			return *s, true
		}
	}
	return Schedule{}, false
}

func (e *Engine) Create(s Schedule) (Schedule, error) {
	if err := s.Validate(); err != nil {
		return Schedule{}, err
	}
	if err := e.exec.ValidateTarget(s); err != nil {
		return Schedule{}, err
	}
	s.ID = newID()
	s.LastRun = 0
	s.LastResult = ""
	s.NextRun = s.next(e.now()).UnixMilli()

	e.mu.Lock()
	defer e.mu.Unlock()
	item := s
	e.items = append(e.items, &item)
	return s, e.saveLocked()
}

func (e *Engine) Update(id string, s Schedule) (Schedule, error) {
	if err := s.Validate(); err != nil {
		return Schedule{}, err
	}
	if err := e.exec.ValidateTarget(s); err != nil {
		return Schedule{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, cur := range e.items {
		if cur.ID != id {
			continue
		}
		s.ID = id
		s.LastRun = cur.LastRun
		s.LastResult = cur.LastResult
		s.NextRun = s.next(e.now()).UnixMilli()
		*cur = s
		return *cur, e.saveLocked()
	}
	return Schedule{}, fmt.Errorf("unknown schedule %q", id)
}

func (e *Engine) Delete(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, s := range e.items {
		if s.ID == id {
			e.items = append(e.items[:i], e.items[i+1:]...)
			return e.saveLocked()
		}
	}
	return fmt.Errorf("unknown schedule %q", id)
}

// RunNow queues one immediate execution without touching the enabled flag
// (the user asked explicitly; a disabled schedule stays disabled after).
func (e *Engine) RunNow(id string) error {
	e.mu.Lock()
	found := false
	for _, s := range e.items {
		if s.ID == id {
			s.runNow = true
			found = true
			break
		}
	}
	e.mu.Unlock()
	if !found {
		return fmt.Errorf("unknown schedule %q", id)
	}
	select {
	case e.poke <- struct{}{}:
	default:
	}
	return nil
}

// --- persistence ------------------------------------------------------------

type persisted struct {
	Schedules []*Schedule `json:"schedules"`
}

func (e *Engine) load() {
	b, err := os.ReadFile(e.path)
	if err != nil {
		return // first run
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		slog.Warn("schedules unreadable, starting fresh", "path", e.path, "err", err)
		return
	}
	now := e.now()
	for _, s := range p.Schedules {
		// Occurrences missed while the panel was down are not backfilled.
		if s.NextRun <= now.UnixMilli() {
			s.NextRun = s.next(now).UnixMilli()
		}
	}
	e.items = p.Schedules
}

// saveLocked persists atomically. Caller holds e.mu.
func (e *Engine) saveLocked() error {
	b, err := json.MarshalIndent(persisted{Schedules: e.items}, "", "  ")
	if err != nil {
		return err
	}
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, e.path)
}
