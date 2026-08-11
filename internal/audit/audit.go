// Package audit records every mutating action taken through the panel to an
// append-only JSONL file.
package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	TS     int64  `json:"ts"` // unix ms
	IP     string `json:"ip"`
	Action string `json:"action"` // e.g. "service.restart", "mc.start"
	Target string `json:"target"` // e.g. "plexmediaserver.service"
	Detail string `json:"detail,omitempty"`
	OK     bool   `json:"ok"`
	Err    string `json:"err,omitempty"`
}

const (
	maxFileBytes = 4 << 20 // rotate past 4 MiB; one .1 generation kept
	maxListScan  = 5000    // hard cap on entries read for a listing
)

type Log struct {
	mu   sync.Mutex
	path string
}

func New(dataDir string) *Log {
	return &Log{path: filepath.Join(dataDir, "audit.log")}
}

// Record appends an entry. Failures are silent by design: auditing must
// never break the action being audited (they do surface in the panel log).
func (l *Log) Record(e Entry) {
	if e.TS == 0 {
		e.TS = time.Now().UnixMilli()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rotateLocked()
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// List returns entries newest-first: total count seen and one page.
func (l *Log) List(offset, limit int) ([]Entry, int) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	var all []Entry
	for _, p := range []string{l.path + ".1", l.path} {
		all = append(all, readEntries(p)...)
	}
	if len(all) > maxListScan {
		all = all[len(all)-maxListScan:]
	}
	// Reverse to newest-first.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	total := len(all)
	if offset >= total {
		return []Entry{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total
}

func readEntries(path string) []Entry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func (l *Log) rotateLocked() {
	st, err := os.Stat(l.path)
	if err != nil || st.Size() < maxFileBytes {
		return
	}
	_ = os.Rename(l.path, l.path+".1")
}
