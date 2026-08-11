// Package jobs runs long operations (snapraid sync/scrub, backups) one at a
// time, capturing streamed output for the UI to poll.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type State string

const (
	StateRunning  State = "running"
	StateDone     State = "done"
	StateFailed   State = "failed"
	StateCanceled State = "canceled"
)

const (
	maxOutputLines = 4000
	maxRecent      = 20
)

type Job struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`   // e.g. "snapraid.sync", "mc.backup"
	Target    string `json:"target"` // e.g. pool name, server id
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt,omitempty"`
	State     State  `json:"state"`
	Err       string `json:"err,omitempty"`

	mu      sync.Mutex
	output  []string
	dropped int
	cancel  context.CancelFunc
}

// View is a Job snapshot safe for JSON serialization.
type View struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Target    string   `json:"target"`
	StartedAt int64    `json:"startedAt"`
	EndedAt   int64    `json:"endedAt,omitempty"`
	State     State    `json:"state"`
	Err       string   `json:"err,omitempty"`
	Output    []string `json:"output,omitempty"`
	Dropped   int      `json:"dropped,omitempty"` // lines trimmed from the front
}

type Runner struct {
	mu      sync.Mutex
	current *Job
	recent  []*Job
	// Notify is called (if set) whenever a job starts or finishes.
	Notify func()
}

func NewRunner() *Runner { return &Runner{} }

// Start begins fn in a goroutine. Only one job may run at a time; a second
// Start returns an error naming the running job.
func (r *Runner) Start(kind, target string, fn func(ctx context.Context, out func(line string)) error) (*View, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		return nil, fmt.Errorf("job %s (%s) is already running", r.current.Kind, r.current.Target)
	}
	ctx, cancel := context.WithCancel(context.Background())
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	j := &Job{
		ID:        hex.EncodeToString(raw),
		Kind:      kind,
		Target:    target,
		StartedAt: time.Now().UnixMilli(),
		State:     StateRunning,
		cancel:    cancel,
	}
	r.current = j
	go func() {
		err := fn(ctx, j.appendLine)
		r.mu.Lock()
		defer r.mu.Unlock()
		j.EndedAt = time.Now().UnixMilli()
		switch {
		case ctx.Err() != nil:
			j.State = StateCanceled
		case err != nil:
			j.State = StateFailed
			j.Err = err.Error()
		default:
			j.State = StateDone
		}
		r.current = nil
		r.recent = append(r.recent, j)
		if len(r.recent) > maxRecent {
			r.recent = r.recent[len(r.recent)-maxRecent:]
		}
		if r.Notify != nil {
			go r.Notify()
		}
	}()
	if r.Notify != nil {
		go r.Notify()
	}
	v := j.view(false)
	return &v, nil
}

func (j *Job) appendLine(line string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.output = append(j.output, line)
	if len(j.output) > maxOutputLines {
		trim := len(j.output) - maxOutputLines
		j.output = j.output[trim:]
		j.dropped += trim
	}
}

func (j *Job) view(withOutput bool) View {
	j.mu.Lock()
	defer j.mu.Unlock()
	v := View{
		ID: j.ID, Kind: j.Kind, Target: j.Target,
		StartedAt: j.StartedAt, EndedAt: j.EndedAt,
		State: j.State, Err: j.Err, Dropped: j.dropped,
	}
	if withOutput {
		v.Output = append([]string(nil), j.output...)
	}
	return v
}

// Current returns the running job (without output), if any.
func (r *Runner) Current() *View {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return nil
	}
	v := r.current.view(false)
	return &v
}

// List returns the running job plus recent history, newest first.
func (r *Runner) List() []View {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []View
	if r.current != nil {
		out = append(out, r.current.view(false))
	}
	for i := len(r.recent) - 1; i >= 0; i-- {
		out = append(out, r.recent[i].view(false))
	}
	return out
}

// Get returns a job with full output.
func (r *Runner) Get(id string) (View, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil && r.current.ID == id {
		return r.current.view(true), true
	}
	for _, j := range r.recent {
		if j.ID == id {
			return j.view(true), true
		}
	}
	return View{}, false
}

// Cancel stops the running job if its ID matches.
func (r *Runner) Cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil && r.current.ID == id {
		r.current.cancel()
		return true
	}
	return false
}
