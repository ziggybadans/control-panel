// Package httpd is the panel's HTTP layer: REST API, SSE streams, and the
// embedded web UI.
package httpd

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ziggybadans/control-panel/internal/apps"
	"github.com/ziggybadans/control-panel/internal/audit"
	"github.com/ziggybadans/control-panel/internal/auth"
	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/events"
	"github.com/ziggybadans/control-panel/internal/httpd/ui"
	"github.com/ziggybadans/control-panel/internal/jobs"
	"github.com/ziggybadans/control-panel/internal/mc"
	"github.com/ziggybadans/control-panel/internal/metrics"
	"github.com/ziggybadans/control-panel/internal/plex"
	"github.com/ziggybadans/control-panel/internal/prefs"
	"github.com/ziggybadans/control-panel/internal/services"
	"github.com/ziggybadans/control-panel/internal/storage"
)

type Deps struct {
	Cfg      config.Config
	Version  string
	Bus      *events.Bus
	Sessions *auth.Sessions
	Limiter  *auth.LoginLimiter
	Audit    *audit.Log
	Prefs    *prefs.Store
	Jobs     *jobs.Runner
	Sampler  *metrics.Sampler
	SysInfo  metrics.SystemInfo
	Storage  storage.Provider
	Services services.Provider
	Plex     plex.Provider
	Apps     apps.Provider
	MC       mc.Service
	// Power executes "reboot" or "shutdown" (nil = unsupported platform).
	Power func(action string) error
}

type Server struct {
	Deps
	mux *http.ServeMux
}

func New(d Deps) *Server {
	s := &Server{Deps: d, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.mux

	// Auth (login is public; the rest is behind requireAuth).
	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)
	m.HandleFunc("GET /api/auth/session", s.handleSession)
	m.HandleFunc("GET /api/health", s.handleHealth)

	// System.
	m.HandleFunc("GET /api/system", s.handleSystem)
	m.HandleFunc("GET /api/summary", s.handleSummary)
	m.HandleFunc("GET /api/metrics/history", s.handleMetricsHistory)
	m.HandleFunc("GET /api/events", s.handleEvents)

	// Storage.
	m.HandleFunc("GET /api/storage", s.handleStorage)
	m.HandleFunc("GET /api/storage/snapraid", s.handleSnapraidInfo)
	m.HandleFunc("POST /api/storage/snapraid/{op}", s.handleSnapraidOp)

	// Services.
	m.HandleFunc("GET /api/services", s.handleServices)
	m.HandleFunc("POST /api/services/{unit}/{verb}", s.handleServiceAction)
	m.HandleFunc("GET /api/services/{unit}/logs", s.handleServiceLogs)

	// Plex.
	m.HandleFunc("GET /api/plex", s.handlePlex)

	// Media apps (*arr suite, Overseerr/Jellyseerr).
	m.HandleFunc("GET /api/apps", s.handleApps)

	// Minecraft.
	m.HandleFunc("GET /api/minecraft", s.handleMCList)
	m.HandleFunc("POST /api/minecraft/rescan", s.handleMCRescan)
	m.HandleFunc("POST /api/minecraft/create", s.handleMCCreate)
	m.HandleFunc("GET /api/minecraft/meta/versions", s.handleMCVersions)
	m.HandleFunc("GET /api/minecraft/{id}/files", s.handleMCFilesList)
	m.HandleFunc("POST /api/minecraft/{id}/files/op", s.handleMCFilesOp)
	m.HandleFunc("GET /api/minecraft/{id}/files/download", s.handleMCFilesDownload)
	m.HandleFunc("POST /api/minecraft/{id}/files/upload", s.handleMCFilesUpload)
	m.HandleFunc("GET /api/minecraft/{id}/addons", s.handleMCAddons)
	m.HandleFunc("POST /api/minecraft/{id}/addons/toggle", s.handleMCAddonToggle)
	m.HandleFunc("GET /api/minecraft/{id}/map", s.handleMCMap)
	m.HandleFunc("POST /api/minecraft/{id}/jar", s.handleMCJar)
	m.HandleFunc("GET /api/minecraft/{id}", s.handleMCGet)
	m.HandleFunc("POST /api/minecraft/{id}/{verb}", s.handleMCLifecycle)
	m.HandleFunc("POST /api/minecraft/{id}/command", s.handleMCCommand)
	m.HandleFunc("GET /api/minecraft/{id}/console", s.handleMCConsole)
	m.HandleFunc("GET /api/minecraft/{id}/console/stream", s.handleMCConsoleStream)
	m.HandleFunc("GET /api/minecraft/{id}/properties", s.handleMCPropsGet)
	m.HandleFunc("PUT /api/minecraft/{id}/properties", s.handleMCPropsSet)
	m.HandleFunc("POST /api/minecraft/{id}/eula", s.handleMCEula)
	m.HandleFunc("GET /api/minecraft/{id}/players", s.handleMCPlayers)
	m.HandleFunc("POST /api/minecraft/{id}/players/{action}", s.handleMCPlayerAction)
	m.HandleFunc("GET /api/minecraft/{id}/backups", s.handleMCBackups)
	m.HandleFunc("POST /api/minecraft/{id}/backups", s.handleMCBackupCreate)
	m.HandleFunc("POST /api/minecraft/{id}/backups/{name}/restore", s.handleMCBackupRestore)
	m.HandleFunc("DELETE /api/minecraft/{id}/backups/{name}", s.handleMCBackupDelete)
	m.HandleFunc("PUT /api/minecraft/{id}/config", s.handleMCConfig)

	// Misc.
	m.HandleFunc("GET /api/prefs", s.handlePrefsGet)
	m.HandleFunc("PUT /api/prefs", s.handlePrefsSet)
	m.HandleFunc("GET /api/audit", s.handleAudit)
	m.HandleFunc("GET /api/jobs", s.handleJobs)
	m.HandleFunc("GET /api/jobs/{id}", s.handleJobGet)
	m.HandleFunc("POST /api/jobs/{id}/cancel", s.handleJobCancel)
	m.HandleFunc("POST /api/power/{action}", s.handlePower)

	// Embedded UI (catch-all).
	m.Handle("/", ui.Handler())
}

// Handler applies the middleware chain.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = s.requireAuth(h)
	h = s.csrfGuard(h)
	h = securityHeaders(h)
	h = recoverPanics(h)
	return h
}

// ListenAndServe blocks. TLS is used when configured.
func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:    s.Cfg.Listen,
		Handler: s.Handler(),
		// WriteTimeout stays 0 for SSE; slow-loris protection comes from
		// ReadHeaderTimeout and the kernel.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
	slog.Info("listening", "addr", s.Cfg.Listen, "tls", s.Cfg.TLS.Cert != "")
	if s.Cfg.TLS.Cert != "" {
		return srv.ListenAndServeTLS(s.Cfg.TLS.Cert, s.Cfg.TLS.Key)
	}
	return srv.ListenAndServe()
}

// --- middleware -------------------------------------------------------------

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic in handler", "err", err, "path", r.URL.Path)
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		// frame-src allows embedding server map UIs (Dynmap/BlueMap/…)
		// hosted on the same machine; frame-ancestors still forbids anyone
		// from embedding the panel itself.
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; connect-src 'self'; frame-src http: https:; "+
				"frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// csrfGuard blocks cross-origin mutations: same-origin is enforced through
// the Origin header (when present) plus a custom header requirement, which
// cross-origin HTML forms cannot set.
func (s *Server) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") &&
			r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				if !originMatchesHost(origin, r.Host) {
					writeErr(w, http.StatusForbidden, "cross-origin request rejected")
					return
				}
			}
			if r.Header.Get("X-CP") == "" {
				writeErr(w, http.StatusForbidden, "missing X-CP header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func originMatchesHost(origin, host string) bool {
	rest, ok := strings.CutPrefix(origin, "http://")
	if !ok {
		rest, ok = strings.CutPrefix(origin, "https://")
	}
	if !ok {
		return false
	}
	return strings.EqualFold(rest, host)
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	public := map[string]bool{
		"/api/auth/login":   true,
		"/api/auth/session": true,
		"/api/health":       true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || public[r.URL.Path] || s.Cfg.Auth.Mode == "none" {
			next.ServeHTTP(w, r)
			return
		}
		if !s.authed(r) {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authed(r *http.Request) bool {
	if s.Cfg.Auth.Mode == "none" {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	return err == nil && s.Sessions.Valid(c.Value)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
