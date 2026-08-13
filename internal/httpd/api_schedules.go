package httpd

import (
	"net/http"

	"github.com/ziggybadans/control-panel/internal/sched"
)

// GET /api/schedules
func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	if s.Sched == nil {
		writeJSON(w, http.StatusOK, map[string]any{"schedules": []sched.Schedule{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": s.Sched.List()})
}

// POST /api/schedules — creating a standing rule is audited; the actions it
// runs later are themselves confined to the panel's allowlists.
func (s *Server) handleScheduleCreate(w http.ResponseWriter, r *http.Request) {
	if s.Sched == nil {
		writeErr(w, http.StatusNotImplemented, "scheduler unavailable")
		return
	}
	var sc sched.Schedule
	if !readJSON(w, r, &sc, 16*1024) {
		return
	}
	created, err := s.Sched.Create(sc)
	_ = s.record(r, "sched.create", sc.Name, sc.Action+" "+sc.Describe(), err)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// PUT /api/schedules/{id}
func (s *Server) handleScheduleUpdate(w http.ResponseWriter, r *http.Request) {
	if s.Sched == nil {
		writeErr(w, http.StatusNotImplemented, "scheduler unavailable")
		return
	}
	var sc sched.Schedule
	if !readJSON(w, r, &sc, 16*1024) {
		return
	}
	updated, err := s.Sched.Update(r.PathValue("id"), sc)
	_ = s.record(r, "sched.update", sc.Name, sc.Action+" "+sc.Describe(), err)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/schedules/{id}
func (s *Server) handleScheduleDelete(w http.ResponseWriter, r *http.Request) {
	if s.Sched == nil {
		writeErr(w, http.StatusNotImplemented, "scheduler unavailable")
		return
	}
	id := r.PathValue("id")
	name := id
	if sc, ok := s.Sched.Get(id); ok {
		name = sc.Name
	}
	err := s.Sched.Delete(id)
	_ = s.record(r, "sched.delete", name, "", err)
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/schedules/{id}/run — one immediate execution.
func (s *Server) handleScheduleRun(w http.ResponseWriter, r *http.Request) {
	if s.Sched == nil {
		writeErr(w, http.StatusNotImplemented, "scheduler unavailable")
		return
	}
	id := r.PathValue("id")
	name := id
	if sc, ok := s.Sched.Get(id); ok {
		name = sc.Name
	}
	err := s.Sched.RunNow(id)
	_ = s.record(r, "sched.run-now", name, "", err)
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
