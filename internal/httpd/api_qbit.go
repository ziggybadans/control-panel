package httpd

import (
	"context"
	"net/http"
	"strings"

	"github.com/ziggybadans/control-panel/internal/qbit"
)

// validHash checks an info hash: hex, 40 characters for v1 torrents and 64
// for v2. Rejecting anything else keeps the action endpoints from being
// used to smuggle arbitrary form values into the WebUI.
func validHash(h string) bool {
	if len(h) != 40 && len(h) != 64 {
		return false
	}
	for _, r := range h {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// qbitStatus fetches the torrent list and enriches it with the media each
// download will become (matched through the *arr queues on the torrent
// hash), which is what makes the watchability verdict possible.
func (s *Server) qbitStatus(ctx context.Context) qbit.Status {
	st := s.Qbit.Status(ctx)
	if !st.Reachable || len(st.Torrents) == 0 {
		return st
	}
	var media map[string]qbitMedia
	if s.Apps != nil && s.Apps.Configured() {
		media = map[string]qbitMedia{}
		for hash, d := range s.Apps.Downloads(ctx) {
			media[hash] = qbitMedia{d.Title, d.Kind, d.App, d.RuntimeSec}
		}
	}
	for i := range st.Torrents {
		t := &st.Torrents[i]
		runtime := 0
		if m, ok := media[strings.ToLower(t.Hash)]; ok {
			t.Media, t.MediaKind, t.MediaApp = m.title, m.kind, m.app
			t.RuntimeSec, runtime = m.runtimeSec, m.runtimeSec
		}
		t.Watch = qbit.Watchability(*t, runtime)
	}
	return st
}

type qbitMedia struct {
	title, kind, app string
	runtimeSec       int
}

func (s *Server) handleQbit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.qbitStatus(r.Context()))
}

func (s *Server) handleQbitFiles(w http.ResponseWriter, r *http.Request) {
	hash := strings.ToLower(r.PathValue("hash"))
	if !validHash(hash) {
		writeErr(w, http.StatusBadRequest, "invalid torrent hash")
		return
	}
	files, err := s.Qbit.Files(r.Context(), hash)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

type qbitActionReq struct {
	// Hashes selects the torrents. The literal ["all"] is accepted for
	// pause and resume only — never for delete.
	Hashes      []string `json:"hashes"`
	DeleteFiles bool     `json:"deleteFiles"`
	Value       int64    `json:"value"` // bytes/s for dllimit / uplimit
}

// handleQbitAction runs one allowlisted qBittorrent action. Deleting is
// confirm-gated (X-Confirm carries the torrent hash, or "all" for a bulk
// pause/resume), like every other destructive endpoint.
func (s *Server) handleQbitAction(w http.ResponseWriter, r *http.Request) {
	op := r.PathValue("op")
	if !qbit.ValidOp(op) {
		writeErr(w, http.StatusBadRequest, "unknown qBittorrent action %q", op)
		return
	}
	if !s.Qbit.Configured() {
		writeErr(w, http.StatusNotImplemented, "qBittorrent is not configured")
		return
	}
	if !s.Cfg.QBitActions() {
		writeErr(w, http.StatusForbidden,
			"qBittorrent actions are disabled (set qbittorrent.allow_actions: true)")
		return
	}
	var req qbitActionReq
	if !readJSON(w, r, &req, 64*1024) {
		return
	}

	act := qbit.Action{Op: op, DeleteFiles: req.DeleteFiles, Value: req.Value}
	target := op
	if !qbit.IsGlobalOp(op) {
		if len(req.Hashes) == 0 {
			writeErr(w, http.StatusBadRequest, "no torrents selected")
			return
		}
		if len(req.Hashes) == 1 && req.Hashes[0] == "all" {
			if op != "pause" && op != "resume" {
				writeErr(w, http.StatusBadRequest, "%q cannot be applied to all torrents", op)
				return
			}
			act.Hashes = []string{"all"}
			target = "all"
		} else {
			if len(req.Hashes) > 100 {
				writeErr(w, http.StatusBadRequest, "too many torrents in one request")
				return
			}
			// One torrent per delete: the confirmation names exactly what
			// is being removed.
			if op == "delete" && len(req.Hashes) != 1 {
				writeErr(w, http.StatusBadRequest, "delete one torrent at a time")
				return
			}
			for _, h := range req.Hashes {
				if !validHash(h) {
					writeErr(w, http.StatusBadRequest, "invalid torrent hash %q", h)
					return
				}
				act.Hashes = append(act.Hashes, strings.ToLower(h))
			}
			target = act.Hashes[0]
		}
	}

	// Destructive or wide-reaching operations need the confirmation header.
	if op == "delete" || target == "all" {
		if !requireConfirm(w, r, target) {
			return
		}
	}

	detail := strings.Join(act.Hashes, " ")
	if op == "delete" && req.DeleteFiles {
		detail += " (+files)"
	}
	if qbit.IsGlobalOp(op) {
		detail = ""
	}
	err := s.Qbit.Do(r.Context(), act)
	s.actionResult(w, r, "qbit."+op, target, detail, err)
}
