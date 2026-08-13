package httpd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ziggybadans/control-panel/internal/audit"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSON(w, status, map[string]string{"error": fmt.Sprintf(format, args...)})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) bool {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %s", err)
		return false
	}
	return true
}

// requireConfirm enforces server-side confirmation for dangerous actions:
// the client must echo the exact target in the X-Confirm header, which the
// UI populates only after the user confirms (typed confirmation for the
// most destructive tier).
func requireConfirm(w http.ResponseWriter, r *http.Request, target string) bool {
	if r.Header.Get("X-Confirm") != target {
		writeErr(w, http.StatusPreconditionRequired,
			"confirmation required: set X-Confirm header to %q", target)
		return false
	}
	return true
}

// record writes an audit entry for a mutating action and returns err
// unchanged for inline error handling.
func (s *Server) record(r *http.Request, action, target, detail string, err error) error {
	e := audit.Entry{
		IP:     s.clientIP(r),
		Action: action,
		Target: target,
		Detail: detail,
		OK:     err == nil,
	}
	if err != nil {
		e.Err = err.Error()
	}
	s.Audit.Record(e)
	return err
}

// actionResult finishes a mutating endpoint: audit + JSON response.
func (s *Server) actionResult(w http.ResponseWriter, r *http.Request, action, target, detail string, err error) {
	_ = s.record(r, action, target, detail, err)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
