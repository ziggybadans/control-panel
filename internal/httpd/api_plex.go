package httpd

import "net/http"

func (s *Server) handlePlex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Plex.Status(r.Context()))
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": s.Apps.Configured(),
		"apps":       s.Apps.List(r.Context()),
	})
}
