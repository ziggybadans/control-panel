package httpd

import "net/http"

func (s *Server) handlePlex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Plex.Status(r.Context()))
}
