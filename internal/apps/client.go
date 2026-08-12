package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ziggybadans/control-panel/internal/config"
)

const (
	cacheTTL     = 15 * time.Second
	maxQueueRows = 5
)

type client struct {
	apps []config.App
	http *http.Client

	mu     sync.Mutex
	cached []AppStatus
	at     time.Time
}

func NewClient(apps []config.App) Provider {
	return &client{
		apps: apps,
		http: &http.Client{Timeout: 6 * time.Second},
	}
}

func (c *client) Configured() bool { return len(c.apps) > 0 }

// List fetches all apps in parallel, memoized briefly so dashboard widget,
// apps page, and menu bar polling share one round of upstream calls.
func (c *client) List(ctx context.Context) []AppStatus {
	c.mu.Lock()
	if time.Since(c.at) < cacheTTL && c.cached != nil {
		out := c.cached
		c.mu.Unlock()
		return out
	}
	c.mu.Unlock()

	out := make([]AppStatus, len(c.apps))
	var wg sync.WaitGroup
	for i, app := range c.apps {
		wg.Add(1)
		go func(i int, app config.App) {
			defer wg.Done()
			out[i] = c.fetch(ctx, app)
		}(i, app)
	}
	wg.Wait()

	c.mu.Lock()
	c.cached = out
	c.at = time.Now()
	c.mu.Unlock()
	return out
}

// apiBase returns the API path prefix for a given app type.
func apiBase(appType string) string {
	switch appType {
	case "radarr", "sonarr", "whisparr":
		return "/api/v3"
	case "lidarr", "readarr", "prowlarr":
		return "/api/v1"
	default: // overseerr, jellyseerr
		return "/api/v1"
	}
}

func isSeerr(appType string) bool {
	return appType == "overseerr" || appType == "jellyseerr"
}

// hasLibrary reports whether the app manages a media library (queue,
// missing, calendar). Prowlarr only manages indexers.
func hasLibrary(appType string) bool {
	switch appType {
	case "radarr", "sonarr", "lidarr", "readarr", "whisparr":
		return true
	}
	return false
}

func (c *client) fetch(ctx context.Context, app config.App) AppStatus {
	st := AppStatus{
		Name:         app.Name,
		Type:         app.Type,
		URL:          strings.TrimRight(app.URL, "/"),
		HealthIssues: []string{},
		Queue:        []QueueItem{},
	}
	if isSeerr(app.Type) {
		c.fetchSeerr(ctx, app, &st)
	} else {
		c.fetchArr(ctx, app, &st)
	}
	return st
}

// get performs one API request and decodes JSON into out.
func (c *client) get(ctx context.Context, app config.App, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET",
		strings.TrimRight(app.URL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", app.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("unreachable: %s", sanitizeErr(err, app.APIKey))
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("API key rejected (%d)", resp.StatusCode)
	default:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func sanitizeErr(err error, key string) string {
	msg := err.Error()
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "***")
	}
	return msg
}

// --- *arr family ------------------------------------------------------------

func (c *client) fetchArr(ctx context.Context, app config.App, st *AppStatus) {
	base := apiBase(app.Type)

	var status struct {
		Version string `json:"version"`
	}
	if err := c.get(ctx, app, base+"/system/status", &status); err != nil {
		st.Error = err.Error()
		return
	}
	st.Reachable = true
	st.Version = status.Version

	var health []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := c.get(ctx, app, base+"/health", &health); err == nil {
		for _, h := range health {
			st.HealthIssues = append(st.HealthIssues, h.Message)
		}
	}

	if !hasLibrary(app.Type) {
		return
	}

	var queue struct {
		TotalRecords int `json:"totalRecords"`
		Records      []struct {
			Title    string  `json:"title"`
			Status   string  `json:"status"`
			TimeLeft string  `json:"timeleft"`
			Size     float64 `json:"size"`
			SizeLeft float64 `json:"sizeleft"`
			Series   *struct {
				Title string `json:"title"`
			} `json:"series"`
			Movie *struct {
				Title string `json:"title"`
			} `json:"movie"`
		} `json:"records"`
	}
	if err := c.get(ctx, app, base+"/queue?page=1&pageSize=10", &queue); err == nil {
		st.QueueCount = queue.TotalRecords
		for _, r := range queue.Records {
			if len(st.Queue) >= maxQueueRows {
				break
			}
			item := QueueItem{Title: r.Title, Status: r.Status, TimeLeft: r.TimeLeft}
			// Prefer the media title over the raw release name when present.
			if r.Series != nil && r.Series.Title != "" {
				item.Title = r.Series.Title
			} else if r.Movie != nil && r.Movie.Title != "" {
				item.Title = r.Movie.Title
			}
			if r.Size > 0 {
				item.Progress = (r.Size - r.SizeLeft) / r.Size * 100
			}
			st.Queue = append(st.Queue, item)
		}
	}

	var missing struct {
		TotalRecords int `json:"totalRecords"`
	}
	if err := c.get(ctx, app, base+"/wanted/missing?page=1&pageSize=1", &missing); err == nil {
		st.Missing = missing.TotalRecords
	}

	// Upcoming releases in the next 7 days.
	start := time.Now().UTC()
	end := start.Add(7 * 24 * time.Hour)
	q := url.Values{
		"start": {start.Format(time.RFC3339)},
		"end":   {end.Format(time.RFC3339)},
	}
	var calendar []json.RawMessage
	if err := c.get(ctx, app, base+"/calendar?"+q.Encode(), &calendar); err == nil {
		st.UpcomingWeek = len(calendar)
	}
}

// --- Overseerr / Jellyseerr -------------------------------------------------

func (c *client) fetchSeerr(ctx context.Context, app config.App, st *AppStatus) {
	var status struct {
		Version string `json:"version"`
	}
	if err := c.get(ctx, app, "/api/v1/status", &status); err != nil {
		st.Error = err.Error()
		return
	}
	st.Reachable = true
	st.Version = status.Version

	var counts RequestCounts
	if err := c.get(ctx, app, "/api/v1/request/count", &counts); err == nil {
		st.Requests = &counts
	}
}
