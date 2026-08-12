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
		MC:       mc.NewMockService(bus, runner),
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

func TestPowerDisabledByDefault(t *testing.T) {
	h := testServer(t, "none").Handler()
	rec := do(h, "POST", "/api/power/reboot", map[string]string{
		"X-CP": "1", "X-Confirm": "reboot",
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("power action with power.allow=false: got %d, want 403", rec.Code)
	}
}
