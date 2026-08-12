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

// Provider lists the status of every configured app.
type Provider interface {
	List(ctx context.Context) []AppStatus
	// Configured reports whether any apps are set up (drives UI visibility).
	Configured() bool
}
