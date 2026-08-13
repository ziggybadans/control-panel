package httpd

import (
	"encoding/base64"
	"net/http"
	"time"
)

// GET /api/terminal
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	if s.Term == nil || !s.Term.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":     true,
		"description": s.Term.Describe(),
		"maxSessions": s.Term.MaxSessions(),
		"sessions":    s.Term.List(),
	})
}

// POST /api/terminal {cols, rows} — opens a real shell, so it sits in the
// typed-confirmation tier and is audited.
func (s *Server) handleTerminalCreate(w http.ResponseWriter, r *http.Request) {
	if s.Term == nil || !s.Term.Enabled() {
		writeErr(w, http.StatusForbidden, "terminal is disabled (set terminal.enabled: true)")
		return
	}
	if !requireConfirm(w, r, "terminal") {
		return
	}
	var body struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}
	if !readJSON(w, r, &body, 4096) {
		return
	}
	view, err := s.Term.Create(body.Cols, body.Rows)
	_ = s.record(r, "terminal.open", view.ID, s.Term.Describe(), err)
	if err != nil {
		writeErr(w, http.StatusConflict, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// GET /api/terminal/{id}/stream — SSE: one "data" replay event, then live
// "data" chunks (base64), then "exit".
func (s *Server) handleTerminalStream(w http.ResponseWriter, r *http.Request) {
	if s.Term == nil {
		writeErr(w, http.StatusNotFound, "terminal is disabled")
		return
	}
	sess, ok := s.Term.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown session")
		return
	}
	replay, ch, cancel := sess.Subscribe()
	defer cancel()
	sse, sseOK := newSSE(w, r)
	if !sseOK {
		return
	}
	if len(replay) > 0 {
		_ = sse.send("data", map[string]string{"b64": base64.StdEncoding.EncodeToString(replay)})
	}
	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case chunk, open := <-ch:
			if !open {
				_ = sse.send("exit", map[string]bool{"exited": true})
				return
			}
			if chunk.Exit {
				_ = sse.send("exit", map[string]bool{"exited": true})
				return
			}
			if err := sse.send("data", map[string]string{"b64": base64.StdEncoding.EncodeToString(chunk.Data)}); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := sse.comment(); err != nil {
				return
			}
		}
	}
}

// POST /api/terminal/{id}/input {b64} — keystrokes. Audited at session
// level (terminal.open/close), not per keystroke.
func (s *Server) handleTerminalInput(w http.ResponseWriter, r *http.Request) {
	if s.Term == nil {
		writeErr(w, http.StatusNotFound, "terminal is disabled")
		return
	}
	sess, ok := s.Term.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown session")
		return
	}
	var body struct {
		B64 string `json:"b64"`
	}
	if !readJSON(w, r, &body, 64*1024) {
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.B64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid base64 input")
		return
	}
	if err := sess.Input(data); err != nil {
		writeErr(w, http.StatusConflict, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/terminal/{id}/resize {cols, rows}
func (s *Server) handleTerminalResize(w http.ResponseWriter, r *http.Request) {
	if s.Term == nil {
		writeErr(w, http.StatusNotFound, "terminal is disabled")
		return
	}
	sess, ok := s.Term.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown session")
		return
	}
	var body struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}
	if !readJSON(w, r, &body, 4096) {
		return
	}
	if err := sess.Resize(body.Cols, body.Rows); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/terminal/{id}
func (s *Server) handleTerminalClose(w http.ResponseWriter, r *http.Request) {
	if s.Term == nil {
		writeErr(w, http.StatusNotFound, "terminal is disabled")
		return
	}
	id := r.PathValue("id")
	if !s.Term.Close(id) {
		writeErr(w, http.StatusNotFound, "unknown session")
		return
	}
	_ = s.record(r, "terminal.close", id, "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
