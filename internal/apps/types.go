// Package apps integrates the media-management stack (Radarr, Sonarr and
// friends, plus Overseerr/Jellyseerr) read-only: queues, health, requests.
// API keys stay server-side; the panel exposes derived status only.
package apps

import "context"

type QueueItem struct {
	Title    string  `json:"title"`
	Status   string  `json:"status"` // downloading | queued | paused | …
	Progress float64 `json:"progress"` // 0-100
	TimeLeft string  `json:"timeLeft,omitempty"`
}

type RequestCounts struct {
	Pending    int `json:"pending"`
	Approved   int `json:"approved"`
	Processing int `json:"processing"`
	Available  int `json:"available"`
	Total      int `json:"total"`
}

type AppStatus struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	URL       string `json:"url"` // for "open app" links (no key included)
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
	Version   string `json:"version,omitempty"`

	// Health issues reported by the app itself (message strings).
	HealthIssues []string `json:"healthIssues"`

	// *arr apps.
	QueueCount   int         `json:"queueCount"`
	Queue        []QueueItem `json:"queue"` // first few items
	Missing      int         `json:"missing,omitempty"`
	UpcomingWeek int         `json:"upcomingWeek,omitempty"`

	// Overseerr / Jellyseerr.
	Requests *RequestCounts `json:"requests,omitempty"`
}

// Download is one queued grab, keyed by the download client's own id (for
// torrents, the info hash). It carries what the panel cannot learn from the
// download client itself: which media this is, and how long it plays for.
type Download struct {
	Title      string `json:"title"`
	Kind       string `json:"kind"` // movie | episode
	App        string `json:"app"`  // Radarr | Sonarr | …
	RuntimeSec int    `json:"runtimeSec"`
}

// Provider lists the status of every configured app.
type Provider interface {
	List(ctx context.Context) []AppStatus
	// Configured reports whether any apps are set up (drives UI visibility).
	Configured() bool
	// Downloads indexes every queued item by lower-cased download id, so
	// the panel can match a torrent to the media it will become.
	Downloads(ctx context.Context) map[string]Download
}
