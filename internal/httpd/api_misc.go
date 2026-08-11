package httpd

import (
	"io"
	"net/http"
	"strconv"
)

func (s *Server) handlePrefsGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(s.Prefs.Get())
}

func (s *Server) handlePrefsSet(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256*1024))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body: %s", err)
		return
	}
	if err := s.Prefs.Set(body); err != nil {
		writeErr(w, http.StatusBadRequest, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	entries, total := s.Audit.List(offset, limit)
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": total})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Jobs.List())
}

func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request) {
	job, ok := s.Jobs.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.Jobs.Cancel(id) {
		writeErr(w, http.StatusNotFound, "job is not running")
		return
	}
	_ = s.record(r, "job.cancel", id, "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handlePower reboots or shuts down the host. Disabled unless power.allow is
// set; always requires typed confirmation.
func (s *Server) handlePower(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if action != "reboot" && action != "shutdown" {
		writeErr(w, http.StatusBadRequest, "invalid power action %q", action)
		return
	}
	if !s.Cfg.Power.Allow {
		writeErr(w, http.StatusForbidden, "power actions are disabled (set power.allow: true)")
		return
	}
	if s.Power == nil {
		writeErr(w, http.StatusNotImplemented, "power actions unsupported on this platform")
		return
	}
	if !requireConfirm(w, r, action) {
		return
	}
	err := s.Power(action)
	s.actionResult(w, r, "power."+action, s.SysInfo.Hostname, "", err)
}
