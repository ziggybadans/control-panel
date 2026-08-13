package httpd

import (
	"context"
	"net/http"

	"github.com/ziggybadans/control-panel/internal/jobs"
	"github.com/ziggybadans/control-panel/internal/storage"
)

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	ov, err := s.Storage.Overview(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

func (s *Server) handleSnapraidInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.Storage.Snapraid(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleSnapraidOp starts a snapraid job. sync and scrub modify parity, so
// they require confirmation; status and diff are read-only.
func (s *Server) handleSnapraidOp(w http.ResponseWriter, r *http.Request) {
	op := r.PathValue("op")
	if !storage.ValidSnapraidOps[op] {
		writeErr(w, http.StatusBadRequest, "invalid snapraid operation %q", op)
		return
	}
	if op == "sync" || op == "scrub" {
		if !requireConfirm(w, r, op) {
			return
		}
	}
	argv, err := s.Storage.SnapraidCmd(op)
	if err != nil {
		s.actionResult(w, r, "snapraid."+op, "snapraid", "", err)
		return
	}
	job, err := s.Jobs.Start("snapraid."+op, "snapraid", func(ctx context.Context, out func(string)) error {
		return jobs.RunStreaming(ctx, argv, out)
	})
	_ = s.record(r, "snapraid."+op, "snapraid", "", err)
	if err != nil {
		writeErr(w, http.StatusConflict, "%s", err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

