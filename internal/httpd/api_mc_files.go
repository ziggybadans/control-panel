package httpd

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/ziggybadans/control-panel/internal/mc"
	"github.com/ziggybadans/control-panel/internal/mcfiles"
)

const maxUploadBytes = 8 << 30 // 8 GiB per request (modpacks are large)

// serverDir resolves the server id from the request path, writing the error
// response itself when the server is unknown.
func (s *Server) serverDir(w http.ResponseWriter, r *http.Request) (id, dir string, ok bool) {
	id = r.PathValue("id")
	dir, err := s.MC.Dir(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return "", "", false
	}
	return id, dir, true
}

// GET /api/minecraft/{id}/files?path=
func (s *Server) handleMCFilesList(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := s.serverDir(w, r)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")
	entries, err := mcfiles.List(dir, rel)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": rel, "entries": entries})
}

// POST /api/minecraft/{id}/files/op {action, path, to}
func (s *Server) handleMCFilesOp(w http.ResponseWriter, r *http.Request) {
	id, dir, ok := s.serverDir(w, r)
	if !ok {
		return
	}
	var body struct {
		Action string `json:"action"` // rename | mkdir | delete | zip | unzip
		Path   string `json:"path"`
		To     string `json:"to,omitempty"`
	}
	if !readJSON(w, r, &body, 16*1024) {
		return
	}
	if body.Path == "" {
		writeErr(w, http.StatusBadRequest, "path is required")
		return
	}
	target := id + ":" + body.Path

	switch body.Action {
	case "rename":
		if body.To == "" {
			writeErr(w, http.StatusBadRequest, "to is required for rename")
			return
		}
		err := mcfiles.Rename(dir, body.Path, body.To)
		s.actionResult(w, r, "mc.file.rename", target, "-> "+body.To, err)
	case "mkdir":
		err := mcfiles.Mkdir(dir, body.Path)
		s.actionResult(w, r, "mc.file.mkdir", target, "", err)
	case "delete":
		// Destructive: the UI collects a typed confirmation of the base name.
		if !requireConfirm(w, r, path.Base(strings.TrimSuffix(body.Path, "/"))) {
			return
		}
		err := mcfiles.Delete(dir, body.Path)
		s.actionResult(w, r, "mc.file.delete", target, "", err)
	case "zip", "unzip":
		op := body.Action
		relPath := body.Path
		job, err := s.Jobs.Start("mc.file."+op, id, func(ctx context.Context, out func(string)) error {
			if op == "zip" {
				return mcfiles.Zip(ctx, dir, relPath, out)
			}
			return mcfiles.Unzip(ctx, dir, relPath, out)
		})
		_ = s.record(r, "mc.file."+op, target, "", err)
		if err != nil {
			writeErr(w, http.StatusConflict, "%s", err)
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		writeErr(w, http.StatusBadRequest, "invalid action %q", body.Action)
	}
}

// GET /api/minecraft/{id}/files/download?path=
func (s *Server) handleMCFilesDownload(w http.ResponseWriter, r *http.Request) {
	id, dir, ok := s.serverDir(w, r)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")
	f, info, err := mcfiles.OpenForDownload(dir, rel)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	defer f.Close()
	name := path.Base(rel)
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", fmt.Sprint(info.Size()))
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(name)))
	_, _ = io.Copy(w, f)
	_ = s.record(r, "mc.file.download", id+":"+rel, "", nil)
}

// POST /api/minecraft/{id}/files/upload?path=&overwrite= (multipart)
func (s *Server) handleMCFilesUpload(w http.ResponseWriter, r *http.Request) {
	id, dir, ok := s.serverDir(w, r)
	if !ok {
		return
	}
	destRel := r.URL.Query().Get("path")
	overwrite := r.URL.Query().Get("overwrite") == "1"

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "multipart upload expected: %s", err)
		return
	}
	var saved []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "read upload: %s", err)
			return
		}
		name := path.Base(part.FileName())
		if name == "" || name == "." || name == "/" {
			continue // not a file field
		}
		rel := path.Join(destRel, name)
		dst, err := mcfiles.CreateForUpload(dir, rel, overwrite)
		if err != nil {
			_ = s.record(r, "mc.file.upload", id+":"+rel, "", err)
			writeErr(w, http.StatusUnprocessableEntity, "%s", err)
			return
		}
		_, err = io.Copy(dst, part)
		dst.Close()
		if err != nil {
			_ = s.record(r, "mc.file.upload", id+":"+rel, "", err)
			writeErr(w, http.StatusInternalServerError, "write %s: %s", name, err)
			return
		}
		saved = append(saved, name)
		_ = s.record(r, "mc.file.upload", id+":"+rel, "", nil)
	}
	if len(saved) == 0 {
		writeErr(w, http.StatusBadRequest, "no files in upload")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "saved": saved})
}

// GET /api/minecraft/{id}/addons
func (s *Server) handleMCAddons(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := s.serverDir(w, r)
	if !ok {
		return
	}
	addons, err := mcfiles.ListAddons(dir)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%s", err)
		return
	}
	if addons == nil {
		addons = []mcfiles.Addon{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"addons": addons})
}

// POST /api/minecraft/{id}/addons/toggle {dir, file, enable}
func (s *Server) handleMCAddonToggle(w http.ResponseWriter, r *http.Request) {
	id, dir, ok := s.serverDir(w, r)
	if !ok {
		return
	}
	var body struct {
		Dir    string `json:"dir"`
		File   string `json:"file"`
		Enable bool   `json:"enable"`
	}
	if !readJSON(w, r, &body, 4096) {
		return
	}
	verb := "disable"
	if body.Enable {
		verb = "enable"
	}
	err := mcfiles.ToggleAddon(dir, body.Dir, body.File, body.Enable)
	s.actionResult(w, r, "mc.addon."+verb, id+":"+body.File, "", err)
}

// GET /api/minecraft/{id}/map
func (s *Server) handleMCMap(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := s.serverDir(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, mcfiles.DetectMap(dir))
}

// GET /api/minecraft/meta/versions?flavor=
func (s *Server) handleMCVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := s.MC.Versions(r.Context(), r.URL.Query().Get("flavor"))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

// POST /api/minecraft/create
func (s *Server) handleMCCreate(w http.ResponseWriter, r *http.Request) {
	var spec mc.CreateSpec
	if !readJSON(w, r, &spec, 16*1024) {
		return
	}
	job, err := s.MC.CreateServer(spec)
	_ = s.record(r, "mc.create", spec.ID, spec.Flavor+" "+spec.Version, err)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

// POST /api/minecraft/{id}/jar {flavor, version} — changes what the server
// runs, so it requires confirmation.
func (s *Server) handleMCJar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !requireConfirm(w, r, id) {
		return
	}
	var body struct {
		Flavor  string `json:"flavor"`
		Version string `json:"version"`
	}
	if !readJSON(w, r, &body, 4096) {
		return
	}
	job, err := s.MC.UpdateJar(id, body.Flavor, body.Version)
	_ = s.record(r, "mc.jar", id, body.Flavor+" "+body.Version, err)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}
