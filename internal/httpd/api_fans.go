package httpd

import (
	"net/http"

	"github.com/ziggybadans/control-panel/internal/fans"
)

// GET /api/fans — discovery, live state, sensors, and stored settings.
func (s *Server) handleFans(w http.ResponseWriter, r *http.Request) {
	if s.Fans == nil {
		writeJSON(w, http.StatusOK, fans.Snapshot{})
		return
	}
	writeJSON(w, http.StatusOK, s.Fans.Snap())
}

// PUT /api/fans/{id} — apply auto/manual/curve settings to one fan. Wrong
// settings can under-cool hardware, so the exact fan id must be confirmed.
func (s *Server) handleFanSet(w http.ResponseWriter, r *http.Request) {
	if s.Fans == nil {
		writeErr(w, http.StatusNotImplemented, "fan control unavailable")
		return
	}
	id := r.PathValue("id")
	if !requireConfirm(w, r, id) {
		return
	}
	var set fans.Settings
	if !readJSON(w, r, &set, 16*1024) {
		return
	}
	err := s.Fans.Set(id, set)
	s.actionResult(w, r, "fans.set", id, set.Mode, err)
}
