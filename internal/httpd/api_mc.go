package httpd

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ziggybadans/control-panel/internal/mc"
)

func (s *Server) handleMCList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.MC.List())
}

func (s *Server) handleMCGet(w http.ResponseWriter, r *http.Request) {
	info, ok := s.MC.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown server %q", r.PathValue("id"))
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleMCRescan(w http.ResponseWriter, r *http.Request) {
	err := s.MC.Rescan()
	s.actionResult(w, r, "mc.rescan", "minecraft", "", err)
}

// handleMCLifecycle covers start/stop/restart/kill. Everything except start
// requires confirmation; kill is the destructive tier (typed in the UI).
func (s *Server) handleMCLifecycle(w http.ResponseWriter, r *http.Request) {
	id, verb := r.PathValue("id"), r.PathValue("verb")
	var err error
	switch verb {
	case "start":
		err = s.MC.Start(id)
	case "stop":
		if !requireConfirm(w, r, id) {
			return
		}
		err = s.MC.Stop(id)
	case "restart":
		if !requireConfirm(w, r, id) {
			return
		}
		err = s.MC.Restart(id)
	case "kill":
		if !requireConfirm(w, r, id) {
			return
		}
		err = s.MC.Kill(id)
	default:
		writeErr(w, http.StatusBadRequest, "invalid action %q", verb)
		return
	}
	s.actionResult(w, r, "mc."+verb, id, "", err)
}

func (s *Server) handleMCCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Command string `json:"command"`
	}
	if !readJSON(w, r, &body, 8192) {
		return
	}
	if body.Command == "" {
		writeErr(w, http.StatusBadRequest, "command is empty")
		return
	}
	err := s.MC.Command(id, body.Command)
	s.actionResult(w, r, "mc.command", id, body.Command, err)
}

func (s *Server) handleMCConsole(w http.ResponseWriter, r *http.Request) {
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 || tail > 2000 {
		tail = 500
	}
	lines, err := s.MC.Console(r.PathValue("id"), tail)
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

func (s *Server) handleMCConsoleStream(w http.ResponseWriter, r *http.Request) {
	ch, cancel, err := s.MC.Subscribe(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return
	}
	defer cancel()
	sse, ok := newSSE(w, r)
	if !ok {
		return
	}
	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case line, open := <-ch:
			if !open {
				return
			}
			if err := sse.send("line", line); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := sse.comment(); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleMCPropsGet(w http.ResponseWriter, r *http.Request) {
	props, err := s.MC.Properties(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"properties": props})
}

func (s *Server) handleMCPropsSet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Changes map[string]string `json:"changes"`
	}
	if !readJSON(w, r, &body, 64*1024) {
		return
	}
	if len(body.Changes) == 0 {
		writeErr(w, http.StatusBadRequest, "no changes supplied")
		return
	}
	keys := make([]string, 0, len(body.Changes))
	for k := range body.Changes {
		keys = append(keys, k)
	}
	err := s.MC.SetProperties(id, body.Changes)
	s.actionResult(w, r, "mc.properties", id, strconv.Itoa(len(keys))+" keys", err)
}

// handleMCEula writes eula=true on the user's behalf; it requires
// confirmation since it accepts Mojang's EULA for this server.
func (s *Server) handleMCEula(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !requireConfirm(w, r, id) {
		return
	}
	err := s.MC.AcceptEULA(id)
	s.actionResult(w, r, "mc.eula", id, "", err)
}

func (s *Server) handleMCPlayers(w http.ResponseWriter, r *http.Request) {
	info, err := s.MC.Players(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleMCPlayerAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	var body struct {
		Player string `json:"player"`
	}
	if !readJSON(w, r, &body, 4096) {
		return
	}
	// Bans remove access; require confirmation naming the player.
	if action == "ban" {
		if !requireConfirm(w, r, body.Player) {
			return
		}
	}
	err := s.MC.PlayerAction(id, body.Player, action)
	s.actionResult(w, r, "mc.player."+action, id, body.Player, err)
}

func (s *Server) handleMCBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := s.MC.Backups(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": backups})
}

func (s *Server) handleMCBackupCreate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.MC.CreateBackup(id)
	_ = s.record(r, "mc.backup.create", id, "", err)
	if err != nil {
		writeErr(w, http.StatusConflict, "%s", err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleMCBackupRestore(w http.ResponseWriter, r *http.Request) {
	id, name := r.PathValue("id"), r.PathValue("name")
	if !requireConfirm(w, r, name) {
		return
	}
	err := s.MC.RestoreBackup(id, name)
	s.actionResult(w, r, "mc.backup.restore", id, name, err)
}

func (s *Server) handleMCBackupDelete(w http.ResponseWriter, r *http.Request) {
	id, name := r.PathValue("id"), r.PathValue("name")
	if !requireConfirm(w, r, name) {
		return
	}
	err := s.MC.DeleteBackup(id, name)
	s.actionResult(w, r, "mc.backup.delete", id, name, err)
}

func (s *Server) handleMCConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var patch mc.ConfigPatch
	if !readJSON(w, r, &patch, 16*1024) {
		return
	}
	err := s.MC.UpdateConfig(id, patch)
	s.actionResult(w, r, "mc.config", id, "", err)
}
