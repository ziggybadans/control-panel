package qbit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWatchability(t *testing.T) {
	// A 2-hour movie: 7200s of runtime, 6480s of budget after the cushion.
	const runtime = 2 * 60 * 60
	cases := []struct {
		name        string
		torrent     Torrent
		runtimeSec  int
		wantVerdict string
		wantWait    int
	}{
		{
			name:        "complete",
			torrent:     Torrent{Progress: 1, LeftBytes: 0},
			runtimeSec:  runtime,
			wantVerdict: "ready",
		},
		{
			name:        "stopped",
			torrent:     Torrent{Progress: 0.4, LeftBytes: 100, State: "stoppedDL"},
			runtimeSec:  runtime,
			wantVerdict: "paused",
		},
		{
			name:        "queued",
			torrent:     Torrent{Progress: 0.4, LeftBytes: 100, State: "queuedDL"},
			runtimeSec:  runtime,
			wantVerdict: "queued",
		},
		{
			name:        "no rate",
			torrent:     Torrent{Progress: 0.4, LeftBytes: 100, State: "stalledDL"},
			runtimeSec:  runtime,
			wantVerdict: "stalled",
		},
		{
			// 6 GiB left at 8 MiB/s = 768s, well inside the budget.
			name:        "outruns playback",
			torrent:     Torrent{Progress: 0.4, LeftBytes: 6 << 30, DLSpeed: 8 << 20, State: "downloading"},
			runtimeSec:  runtime,
			wantVerdict: "now",
		},
		{
			// 40 GiB left at 4 MiB/s = 10240s, 3760s past the budget.
			name:        "playback would catch up",
			torrent:     Torrent{Progress: 0.1, LeftBytes: 40 << 30, DLSpeed: 4 << 20, State: "downloading"},
			runtimeSec:  runtime,
			wantVerdict: "wait",
			wantWait:    10240 - 6480,
		},
		{
			name:        "no runtime known",
			torrent:     Torrent{Progress: 0.4, LeftBytes: 1 << 30, DLSpeed: 8 << 20, State: "downloading"},
			wantVerdict: "unknown",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Watchability(c.torrent, c.runtimeSec)
			if got.Verdict != c.wantVerdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, c.wantVerdict)
			}
			if c.wantWait != 0 && got.WaitSec != c.wantWait {
				t.Errorf("waitSec = %d, want %d", got.WaitSec, c.wantWait)
			}
		})
	}
}

// fakeQbit is a minimal stand-in for the WebUI: it demands a login, hands
// out a session cookie, and answers the three endpoints Status uses.
type fakeQbit struct {
	loginCalls int
	// paths records every request path in order.
	paths []string
	// stopUnsupported makes /torrents/stop 404, like qBittorrent 4.x.
	stopUnsupported bool
	lastForm        string
}

func (f *fakeQbit) handler() http.Handler {
	mux := http.NewServeMux()
	authed := func(r *http.Request) bool {
		c, err := r.Cookie("SID")
		return err == nil && c.Value == "session-token"
	}
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		f.loginCalls++
		_ = r.ParseForm()
		if r.Form.Get("username") != "panel" || r.Form.Get("password") != "hunter2" {
			_, _ = w.Write([]byte("Fails."))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "session-token", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.paths = append(f.paths, r.URL.Path)
		if !authed(r) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = r.ParseForm()
		f.lastForm = r.Form.Encode()
		switch r.URL.Path {
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.3"))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"dl_info_speed":1048576,
				"up_info_speed":131072,"connection_status":"connected",
				"free_space_on_disk":123456789,"use_alt_speed_limits":true,"queueing":true}}`))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"ABCDEF0123456789abcdef0123456789ABCDEF01",
				"name":"Some.Release.2024.1080p","state":"downloading","size":10737418240,
				"amount_left":5368709120,"progress":0.5,"dlspeed":5242880,"eta":1024,
				"seq_dl":true,"f_l_piece_prio":true,"num_seeds":12,"category":"radarr"}]`))
		case "/api/v2/torrents/stop":
			if f.stopUnsupported {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}
	})
	return mux
}

func newTestClient(t *testing.T, f *fakeQbit) (Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "panel", "hunter2", true), srv
}

func TestStatusParsesTorrents(t *testing.T) {
	f := &fakeQbit{}
	c, _ := newTestClient(t, f)

	st := c.Status(context.Background())
	if !st.Reachable {
		t.Fatalf("unreachable: %s", st.Error)
	}
	if st.Version != "v5.0.3" {
		t.Errorf("version = %q", st.Version)
	}
	if st.Transfer.DLSpeed != 1<<20 || !st.Transfer.AltSpeed || st.Transfer.FreeSpace != 123456789 {
		t.Errorf("transfer = %+v", st.Transfer)
	}
	if len(st.Torrents) != 1 {
		t.Fatalf("got %d torrents, want 1", len(st.Torrents))
	}
	got := st.Torrents[0]
	if got.Hash != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Errorf("hash = %q (should be lower-cased for the *arr join)", got.Hash)
	}
	if got.LeftBytes != 5<<30 || got.DLSpeed != 5<<20 || !got.Sequential {
		t.Errorf("torrent = %+v", got)
	}
	if got.Watch.Verdict != "unknown" {
		t.Errorf("verdict = %q, want unknown (no runtime yet)", got.Watch.Verdict)
	}
	if f.loginCalls != 1 {
		t.Errorf("logged in %d times, want 1", f.loginCalls)
	}

	// The second call inside the TTL must not hit the network again.
	before := len(f.paths)
	c.Status(context.Background())
	if len(f.paths) != before {
		t.Errorf("cached Status still made %d requests", len(f.paths)-before)
	}
}

func TestActionFallsBackToLegacyPath(t *testing.T) {
	f := &fakeQbit{stopUnsupported: true}
	c, _ := newTestClient(t, f)

	hash := "abcdef0123456789abcdef0123456789abcdef01"
	if err := c.Do(context.Background(), Action{Op: "pause", Hashes: []string{hash}}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	joined := strings.Join(f.paths, " ")
	if !strings.Contains(joined, "/api/v2/torrents/stop") ||
		!strings.Contains(joined, "/api/v2/torrents/pause") {
		t.Errorf("expected stop then the pause fallback, got %v", f.paths)
	}
	if !strings.Contains(f.lastForm, "hashes="+hash) {
		t.Errorf("form = %q", f.lastForm)
	}
}

func TestUnknownActionRejected(t *testing.T) {
	f := &fakeQbit{}
	c, _ := newTestClient(t, f)
	err := c.Do(context.Background(), Action{Op: "setLocation", Hashes: []string{"x"}})
	if err == nil {
		t.Fatal("expected an unknown action to be refused")
	}
	if ValidOp("setLocation") {
		t.Error("setLocation must not be in the action allowlist")
	}
}

func TestActionsDisabled(t *testing.T) {
	f := &fakeQbit{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := NewClient(srv.URL, "panel", "hunter2", false)
	if err := c.Do(context.Background(), Action{Op: "pause", Hashes: []string{"a"}}); err == nil {
		t.Fatal("read-only client performed an action")
	}
}

func TestBadCredentialsReported(t *testing.T) {
	f := &fakeQbit{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	const badPassword = "swordfish"
	c := NewClient(srv.URL, "panel", badPassword, true)
	st := c.Status(context.Background())
	if st.Reachable {
		t.Fatal("expected the status to report a failure")
	}
	if !strings.Contains(st.Error, "username or password") {
		t.Errorf("error = %q, want a credential hint", st.Error)
	}
	if strings.Contains(st.Error, badPassword) {
		t.Errorf("error leaks the password: %q", st.Error)
	}

	// Rejected credentials must not be retried on every poll: qBittorrent
	// bans an address after a handful of failed logins.
	logins := f.loginCalls
	for i := 0; i < 5; i++ {
		c.(*client).cached = nil // skip the status cache, not the backoff
		c.Status(context.Background())
	}
	if f.loginCalls != logins {
		t.Errorf("retried login %d times while backing off", f.loginCalls-logins)
	}
}
