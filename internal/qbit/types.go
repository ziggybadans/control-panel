// Package qbit integrates the qBittorrent WebUI API (v2): transfer stats,
// the torrent list, and the small allowlisted set of actions the panel
// exposes. Credentials stay server-side; the panel never proxies arbitrary
// WebUI calls.
package qbit

import (
	"context"
	"math"
)

// Torrent is one entry of the qBittorrent transfer list, plus the panel's
// own derived fields (media match, watchability).
type Torrent struct {
	Hash       string  `json:"hash"`
	Name       string  `json:"name"`
	State      string  `json:"state"` // downloading | stalledDL | pausedUP | …
	Category   string  `json:"category,omitempty"`
	Tags       string  `json:"tags,omitempty"`
	SizeBytes  int64   `json:"sizeBytes"`
	Downloaded int64   `json:"downloaded"`
	LeftBytes  int64   `json:"leftBytes"`
	Progress   float64 `json:"progress"` // 0-1
	DLSpeed    int64   `json:"dlSpeed"`  // bytes/s
	UPSpeed    int64   `json:"upSpeed"`  // bytes/s
	// ETASec is qBittorrent's own estimate. It is capped at 8640000
	// (100 days) to mean "unknown"; Watch.ETASec is the panel's estimate.
	ETASec       int64   `json:"etaSec"`
	Ratio        float64 `json:"ratio"`
	Seeds        int     `json:"seeds"`
	SeedsTotal   int     `json:"seedsTotal"`
	Peers        int     `json:"peers"`
	PeersTotal   int     `json:"peersTotal"`
	Priority     int     `json:"priority"` // queue position (0 = not queued)
	Sequential   bool    `json:"sequential"`
	FirstLast    bool    `json:"firstLast"` // first/last piece priority
	ForceStart   bool    `json:"forceStart"`
	AddedOn      int64   `json:"addedOn"` // unix seconds
	CompletedOn  int64   `json:"completedOn,omitempty"`
	SavePath     string  `json:"savePath,omitempty"`
	ContentPath  string  `json:"contentPath,omitempty"`
	Availability float64 `json:"availability,omitempty"`
	Tracker      string  `json:"tracker,omitempty"`

	// Media fields are filled by the panel when the torrent matches a
	// Radarr/Sonarr queue item (joined on the torrent hash).
	Media      string `json:"media,omitempty"`     // "Dune: Part Two"
	MediaKind  string `json:"mediaKind,omitempty"` // movie | episode
	MediaApp   string `json:"mediaApp,omitempty"`  // Radarr | Sonarr
	RuntimeSec int    `json:"runtimeSec,omitempty"`

	Watch Watch `json:"watch"`
}

// File is one file inside a torrent.
type File struct {
	Name      string  `json:"name"`
	SizeBytes int64   `json:"sizeBytes"`
	Progress  float64 `json:"progress"` // 0-1
	Priority  int     `json:"priority"` // 0 = do not download
}

// Transfer is the global session state.
type Transfer struct {
	DLSpeed    int64  `json:"dlSpeed"`
	UPSpeed    int64  `json:"upSpeed"`
	DLData     int64  `json:"dlData"` // this session
	UPData     int64  `json:"upData"`
	DLLimit    int64  `json:"dlLimit"` // bytes/s, 0 = unlimited
	UPLimit    int64  `json:"upLimit"`
	AltSpeed   bool   `json:"altSpeed"` // alternative limits engaged
	Connection string `json:"connection"`
	DHTNodes   int64  `json:"dhtNodes"`
	FreeSpace  int64  `json:"freeSpace,omitempty"`
	Queueing   bool   `json:"queueing"` // queue priority actions need it
}

// Status is the whole picture the panel serves for one qBittorrent
// instance.
type Status struct {
	Configured   bool      `json:"configured"`
	Reachable    bool      `json:"reachable"`
	Error        string    `json:"error,omitempty"`
	Version      string    `json:"version,omitempty"`
	URL          string    `json:"url,omitempty"` // for "open WebUI" links
	AllowActions bool      `json:"allowActions"`
	Transfer     Transfer  `json:"transfer"`
	Torrents     []Torrent `json:"torrents"`
	Total        int       `json:"total"` // before MaxTorrents truncation
}

// Watch answers "can I start watching this now without playback catching up
// to the download?".
//
// The math: playing from the start takes RuntimeSec; the remaining bytes
// take ETASec at the current rate. Starting now is safe when the download
// finishes before playback reaches the end, with a 10% cushion for
// bitrate peaks. Otherwise WaitSec is how long to wait before starting.
//
// It assumes pieces arrive in order (sequential download) — without that a
// partial file has holes anywhere, so Sequential is reported alongside.
type Watch struct {
	// Verdict: ready (complete) | now | wait | stalled | paused | queued |
	// unknown ("unknown" = no runtime, i.e. the torrent isn't matched to a
	// Radarr/Sonarr item).
	Verdict    string `json:"verdict"`
	ETASec     int    `json:"etaSec,omitempty"`  // panel estimate at current rate
	WaitSec    int    `json:"waitSec,omitempty"` // for verdict "wait"
	RuntimeSec int    `json:"runtimeSec,omitempty"`
	Sequential bool   `json:"sequential"`
}

// watchHeadroom reserves a slice of the runtime as cushion: a download that
// only just keeps up will stutter on high-bitrate scenes.
const watchHeadroom = 0.9

// Watchability derives the Watch verdict for a torrent. runtimeSec is the
// media's playing time (0 when unknown).
func Watchability(t Torrent, runtimeSec int) Watch {
	w := Watch{Sequential: t.Sequential, RuntimeSec: runtimeSec}
	switch {
	case t.Progress >= 1 || t.LeftBytes <= 0:
		w.Verdict = "ready"
		return w
	case isStopped(t.State):
		w.Verdict = "paused"
		return w
	case isQueued(t.State):
		w.Verdict = "queued"
		return w
	case t.DLSpeed <= 0:
		w.Verdict = "stalled"
		return w
	}
	eta := int(math.Ceil(float64(t.LeftBytes) / float64(t.DLSpeed)))
	w.ETASec = eta
	if runtimeSec <= 0 {
		w.Verdict = "unknown"
		return w
	}
	budget := int(float64(runtimeSec) * watchHeadroom)
	if eta <= budget {
		w.Verdict = "now"
		return w
	}
	w.Verdict = "wait"
	w.WaitSec = eta - budget
	return w
}

// isStopped reports whether a torrent is deliberately not running.
// qBittorrent 5.x renamed the paused* states to stopped*.
func isStopped(state string) bool {
	switch state {
	case "pausedDL", "pausedUP", "stoppedDL", "stoppedUP":
		return true
	}
	return false
}

func isQueued(state string) bool {
	return state == "queuedDL" || state == "queuedUP"
}

// Action is one allowlisted operation. Ops that take no hashes (global
// limits) ignore Hashes, and only Delete uses DeleteFiles.
type Action struct {
	Op          string
	Hashes      []string
	DeleteFiles bool
	Value       int64 // bytes/s for dllimit / uplimit
}

// Provider is the panel's view of a qBittorrent instance.
type Provider interface {
	// Configured reports whether a URL is set (drives UI visibility).
	Configured() bool
	// Status returns the session state and torrent list (briefly cached).
	Status(ctx context.Context) Status
	// Files lists the files inside one torrent.
	Files(ctx context.Context, hash string) ([]File, error)
	// Do performs an allowlisted action.
	Do(ctx context.Context, a Action) error
}
