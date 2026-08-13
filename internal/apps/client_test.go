package apps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ziggybadans/control-panel/internal/config"
)

// fakeRadarr serves a minimal Radarr v3 API surface.
func fakeRadarr(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Api-Key") != apiKey {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			h(w, r)
		}
	}
	mux.HandleFunc("/api/v3/system/status", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"version":"5.14.0.9383","appName":"Radarr"}`))
	}))
	mux.HandleFunc("/api/v3/health", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"type":"warning","message":"Indexer X is unavailable"}]`))
	}))
	mux.HandleFunc("/api/v3/queue", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"totalRecords":2,"records":[
			{"title":"Some.Release.2160p.WEB-DL","status":"downloading","timeleft":"00:10:00",
			 "size":1000,"sizeleft":250,"downloadId":"ABCDEF0123456789ABCDEF0123456789ABCDEF01",
			 "movie":{"title":"Dune: Part Two","runtime":166}},
			{"title":"Other.Release","status":"queued","size":0,"sizeleft":0}
		]}`))
	}))
	mux.HandleFunc("/api/v3/wanted/missing", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"totalRecords":7}`))
	}))
	mux.HandleFunc("/api/v3/calendar", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{},{},{}]`))
	}))
	return httptest.NewServer(mux)
}

func TestClientFetchesRadarr(t *testing.T) {
	srv := fakeRadarr(t, "secret")
	defer srv.Close()

	c := NewClient([]config.App{{Name: "Radarr", Type: "radarr", URL: srv.URL, APIKey: "secret"}})
	list := c.List(context.Background())
	if len(list) != 1 {
		t.Fatalf("got %d apps", len(list))
	}
	app := list[0]
	if !app.Reachable {
		t.Fatalf("unreachable: %s", app.Error)
	}
	if app.Version != "5.14.0.9383" {
		t.Errorf("version = %q", app.Version)
	}
	if len(app.HealthIssues) != 1 || app.HealthIssues[0] != "Indexer X is unavailable" {
		t.Errorf("health = %v", app.HealthIssues)
	}
	if app.QueueCount != 2 || len(app.Queue) != 2 {
		t.Fatalf("queue = %d/%d", app.QueueCount, len(app.Queue))
	}
	// Media title preferred over release name; progress from size deltas.
	if app.Queue[0].Title != "Dune: Part Two" {
		t.Errorf("queue title = %q", app.Queue[0].Title)
	}
	if app.Queue[0].Progress != 75 {
		t.Errorf("progress = %v, want 75", app.Queue[0].Progress)
	}
	// Zero-size items must not divide by zero.
	if app.Queue[1].Progress != 0 {
		t.Errorf("zero-size progress = %v", app.Queue[1].Progress)
	}
	if app.Missing != 7 || app.UpcomingWeek != 3 {
		t.Errorf("missing=%d upcoming=%d", app.Missing, app.UpcomingWeek)
	}
}

// The download index is what lets the panel tell a torrent which media it
// will become — and how long that media plays for.
func TestDownloadsIndexedByHash(t *testing.T) {
	srv := fakeRadarr(t, "secret")
	defer srv.Close()

	c := NewClient([]config.App{{Name: "Radarr", Type: "radarr", URL: srv.URL, APIKey: "secret"}})
	dl := c.Downloads(context.Background())
	// Keys are lower-cased: qBittorrent reports hashes in lower case,
	// the *arr apps in upper.
	got, ok := dl["abcdef0123456789abcdef0123456789abcdef01"]
	if !ok {
		t.Fatalf("download not indexed: %v", dl)
	}
	if got.Title != "Dune: Part Two" || got.Kind != "movie" || got.App != "Radarr" {
		t.Errorf("download = %+v", got)
	}
	if got.RuntimeSec != 166*60 {
		t.Errorf("runtime = %ds, want %ds", got.RuntimeSec, 166*60)
	}
	// Records without a download id cannot be matched and must be skipped.
	if len(dl) != 1 {
		t.Errorf("indexed %d downloads, want 1", len(dl))
	}
}

func TestClientRejectedKey(t *testing.T) {
	srv := fakeRadarr(t, "secret")
	defer srv.Close()

	c := NewClient([]config.App{{Name: "Radarr", Type: "radarr", URL: srv.URL, APIKey: "wrong"}})
	list := c.List(context.Background())
	if list[0].Reachable {
		t.Error("should be unreachable with bad key")
	}
	if list[0].Error == "" {
		t.Error("expected an error message")
	}
}

func TestListIsCached(t *testing.T) {
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"1.0"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient([]config.App{{Name: "Prowlarr", Type: "prowlarr", URL: srv.URL, APIKey: "k"}})
	c.List(context.Background())
	first := calls
	c.List(context.Background())
	if calls != first {
		t.Errorf("second List within TTL hit upstream (%d -> %d calls)", first, calls)
	}
}
