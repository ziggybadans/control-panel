package httpd

import (
	"bufio"
	"context"
	"net/http"
	"os/exec"

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
		return runStreaming(ctx, argv, out)
	})
	_ = s.record(r, "snapraid."+op, "snapraid", "", err)
	if err != nil {
		writeErr(w, http.StatusConflict, "%s", err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

// runStreaming executes argv, feeding each output line to out.
func runStreaming(ctx context.Context, argv []string, out func(string)) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(pipe)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		out(sc.Text())
	}
	return cmd.Wait()
}
