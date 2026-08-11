package httpd

import (
	"net/http"
	"strconv"
)

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	list, err := s.Services.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"services":     list,
		"allowActions": s.Cfg.Services.AllowActions,
	})
}

func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	unit, verb := r.PathValue("unit"), r.PathValue("verb")
	if !s.Cfg.Services.AllowActions {
		writeErr(w, http.StatusForbidden, "service actions are disabled in the panel config")
		return
	}
	// Stopping or restarting a service can interrupt users; require the
	// client to confirm. Start is safe.
	if verb == "stop" || verb == "restart" {
		if !requireConfirm(w, r, unit) {
			return
		}
	}
	err := s.Services.Action(r.Context(), unit, verb)
	s.actionResult(w, r, "service."+verb, unit, "", err)
	if err == nil {
		// Push the refreshed list so all clients update immediately.
		if list, lerr := s.Services.List(r.Context()); lerr == nil {
			s.Bus.Publish("services", list)
		}
	}
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 {
		lines = 200
	}
	logs, err := s.Services.Logs(r.Context(), r.PathValue("unit"), lines)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": logs})
}
