package httpd

import "net/http"

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") == "1"
	writeJSON(w, http.StatusOK, s.Update.Status(r.Context(), force))
}

// handleUpdateApply installs a released binary and restarts the panel.
// The client must name the exact tag it is installing and echo it in the
// confirmation header; the updater re-fetches that tag so what was shown
// to the user is what gets installed.
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tag string `json:"tag"`
	}
	if !readJSON(w, r, &body, 4096) {
		return
	}
	if body.Tag == "" {
		writeErr(w, http.StatusBadRequest, "tag is required")
		return
	}
	if !requireConfirm(w, r, body.Tag) {
		return
	}
	err := s.Update.Apply(r.Context(), body.Tag)
	s.actionResult(w, r, "panel.update", body.Tag, "", err)
}
