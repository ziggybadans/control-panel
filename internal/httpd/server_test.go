package httpd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ziggybadans/control-panel/internal/apps"
	"github.com/ziggybadans/control-panel/internal/audit"
	"github.com/ziggybadans/control-panel/internal/auth"
	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/events"
	"github.com/ziggybadans/control-panel/internal/fans"
	"github.com/ziggybadans/control-panel/internal/files"
	"github.com/ziggybadans/control-panel/internal/jobs"
	"github.com/ziggybadans/control-panel/internal/mc"
	"github.com/ziggybadans/control-panel/internal/metrics"
	"github.com/ziggybadans/control-panel/internal/plex"
	"github.com/ziggybadans/control-panel/internal/prefs"
	"github.com/ziggybadans/control-panel/internal/services"
	"github.com/ziggybadans/control-panel/internal/storage"
)

func testServer(t *testing.T, authMode string) *Server {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Auth.Mode = authMode
	cfg.DataDir = dir
	bus := events.NewBus()
	runner := jobs.NewRunner()
	return New(Deps{
		Cfg:      cfg,
		Version:  "test",
		Bus:      bus,
		Sessions: auth.NewSessions(time.Hour, dir),
		Limiter:  auth.NewLoginLimiter(5, time.Minute),
		Audit:    audit.New(dir),
		Prefs:    prefs.New(dir),
		Jobs:     runner,
		Sampler:  metrics.NewSampler(metrics.NewMockCollector("test"), bus, time.Second, 60),
		Storage:  storage.NewMockProvider(),
		Services: services.NewMockProvider(),
		Plex:     plex.NewMockProvider(),
		Apps:     apps.NewMockProvider(),
		MC:       mc.NewMockService(bus, runner, dir),
		Fans:     fans.NewController(fans.NewMockProvider(), bus, dir, time.Second, true),
		Files: files.New([]config.FilesRoot{
			{Name: "data", Path: dir},
			{Name: "ro", Path: dir, ReadOnly: true},
		}),
	})
}

func do(h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDangerousEndpointsRequireConfirmHeader(t *testing.T) {
	h := testServer(t, "none").Handler()
	cases := []struct {
		method, path, target string
	}{
		{"POST", "/api/minecraft/survival/stop", "survival"},
		{"POST", "/api/minecraft/survival/kill", "survival"},
		{"POST", "/api/services/smbd.service/stop", "smbd.service"},
		{"POST", "/api/storage/snapraid/sync", "sync"},
		{"DELETE", "/api/minecraft/survival/backups/x.tar.gz", "x.tar.gz"},
		{"PUT", "/api/fans/mock:pwm1", "mock:pwm1"},
	}
	for _, c := range cases {
		// Without X-Confirm: rejected with 428.
		rec := do(h, c.method, c.path, map[string]string{"X-CP": "1"})
		if rec.Code != http.StatusPreconditionRequired {
			t.Errorf("%s %s without confirm: got %d, want 428", c.method, c.path, rec.Code)
		}
		// With the WRONG confirm value: still rejected.
		rec = do(h, c.method, c.path, map[string]string{"X-CP": "1", "X-Confirm": "wrong"})
		if rec.Code != http.StatusPreconditionRequired {
			t.Errorf("%s %s with wrong confirm: got %d, want 428", c.method, c.path, rec.Code)
		}
	}
}

func TestMutationsRequireCSRFHeader(t *testing.T) {
	h := testServer(t, "none").Handler()
	rec := do(h, "POST", "/api/minecraft/survival/start", nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("mutation without X-CP: got %d, want 403", rec.Code)
	}
	rec = do(h, "POST", "/api/minecraft/creative/start", map[string]string{
		"X-CP": "1", "Origin": "http://evil.example",
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin mutation: got %d, want 403", rec.Code)
	}
}

func TestAuthRequiredWhenPasswordMode(t *testing.T) {
	h := testServer(t, "password").Handler()
	rec := do(h, "GET", "/api/minecraft", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated request: got %d, want 401", rec.Code)
	}
	// Session probe is public so the login screen can render.
	rec = do(h, "GET", "/api/auth/session", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("session probe: got %d, want 200", rec.Code)
	}
}

func TestClientIPTrustedProxies(t *testing.T) {
	s := testServer(t, "none")

	req := httptest.NewRequest("GET", "/api/health", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	// No trusted proxies configured: XFF is attacker-controlled, ignore it.
	if got := s.clientIP(req); got != "203.0.113.7" {
		t.Errorf("untrusted XFF honored: got %q", got)
	}

	s.Cfg.TrustedProxies = []string{"203.0.113.7"}
	s.trustedProxies = s.Cfg.TrustedProxyNets()
	// Trusted proxy in front: use the rightmost non-trusted hop.
	if got := s.clientIP(req); got != "198.51.100.9" {
		t.Errorf("XFF via trusted proxy: got %q, want 198.51.100.9", got)
	}
	// Spoofed extra hops from the client stay behind the real client hop.
	req.Header.Set("X-Forwarded-For", "10.9.9.9, 198.51.100.9")
	if got := s.clientIP(req); got != "198.51.100.9" {
		t.Errorf("rightmost non-trusted hop: got %q, want 198.51.100.9", got)
	}
}

func doBody(h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestFilesSafety(t *testing.T) {
	h := testServer(t, "none").Handler()
	hdr := map[string]string{"X-CP": "1"}

	// Delete without the confirm header: 428.
	rec := doBody(h, "POST", "/api/files/op",
		`{"root":"data","action":"delete","path":"x.txt"}`, hdr)
	if rec.Code != http.StatusPreconditionRequired {
		t.Errorf("delete without confirm: got %d, want 428", rec.Code)
	}

	// Any write on a read-only root: 403, even with confirm.
	rec = doBody(h, "POST", "/api/files/op",
		`{"root":"ro","action":"delete","path":"x.txt"}`,
		map[string]string{"X-CP": "1", "X-Confirm": "x.txt"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("delete on read-only root: got %d, want 403", rec.Code)
	}
	rec = doBody(h, "POST", "/api/files/op",
		`{"root":"ro","action":"mkdir","path":"new"}`, hdr)
	if rec.Code != http.StatusForbidden {
		t.Errorf("mkdir on read-only root: got %d, want 403", rec.Code)
	}
	rec = doBody(h, "POST", "/api/files/upload?root=ro", "", hdr)
	if rec.Code != http.StatusForbidden {
		t.Errorf("upload to read-only root: got %d, want 403", rec.Code)
	}

	// Path traversal is refused.
	rec = doBody(h, "GET", "/api/files/list?root=data&path=..%2F..%2Fetc", "", hdr)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("traversal list: got %d, want 422", rec.Code)
	}

	// Unknown root.
	rec = doBody(h, "GET", "/api/files/list?root=nope", "", hdr)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown root: got %d, want 404", rec.Code)
	}
}

func TestPowerDisabledByDefault(t *testing.T) {
	h := testServer(t, "none").Handler()
	rec := do(h, "POST", "/api/power/reboot", map[string]string{
		"X-CP": "1", "X-Confirm": "reboot",
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("power action with power.allow=false: got %d, want 403", rec.Code)
	}
}
