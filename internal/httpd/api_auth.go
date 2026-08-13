package httpd

import (
	"net/http"
	"time"

	"github.com/ziggybadans/control-panel/internal/auth"
)

const sessionCookie = "cp_session"

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.Auth.Mode == "none" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	ip := s.clientIP(r)
	if !s.Limiter.Allow(ip) {
		_ = s.record(r, "auth.login", ip, "rate limited", errRateLimited)
		writeErr(w, http.StatusTooManyRequests, "too many failed attempts — try again in a few minutes")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body, 4096) {
		return
	}
	// Bound concurrent argon2id runs (memory-hard by design); excess
	// attempts wait their turn instead of multiplying the allocation.
	select {
	case s.loginSem <- struct{}{}:
	case <-r.Context().Done():
		return
	}
	valid := s.Cfg.Auth.PasswordHash != "" && auth.VerifyPassword(s.Cfg.Auth.PasswordHash, body.Password)
	<-s.loginSem
	if !valid {
		s.Limiter.Fail(ip)
		_ = s.record(r, "auth.login", ip, "", errBadPassword)
		writeErr(w, http.StatusUnauthorized, "incorrect password")
		return
	}
	s.Limiter.Reset(ip)
	token, err := s.Sessions.Create()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "session creation failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.Cfg.TLS.Cert != "",
		MaxAge:   int(s.Cfg.SessionTTL() / time.Second),
	})
	_ = s.record(r, "auth.login", ip, "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.Sessions.Revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
		SameSite: http.SameSiteStrictMode, Secure: s.Cfg.TLS.Cert != "",
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"authRequired":  s.Cfg.Auth.Mode != "none",
		"authenticated": s.authed(r),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.Version})
}

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const (
	errRateLimited = sentinelError("rate limited")
	errBadPassword = sentinelError("incorrect password")
)
