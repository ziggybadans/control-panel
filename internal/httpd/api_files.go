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

	"github.com/ziggybadans/control-panel/internal/files"
	"github.com/ziggybadans/control-panel/internal/mcfiles"
)

// fileRoot resolves the ?root= (or body) name, writing the error response
// itself on failure. mutating additionally rejects read-only roots.
func (s *Server) fileRoot(w http.ResponseWriter, name string, mutating bool) (files.Root, bool) {
	if s.Files == nil || !s.Files.Configured() {
		writeErr(w, http.StatusNotFound, "no file roots configured (files.roots in config.yaml)")
		return files.Root{}, false
	}
	root, err := s.Files.Get(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "%s", err)
		return files.Root{}, false
	}
	if mutating && root.ReadOnly {
		writeErr(w, http.StatusForbidden, "root %q is read-only", root.Name)
		return files.Root{}, false
	}
	return root, true
}

// GET /api/files
func (s *Server) handleFilesRoots(w http.ResponseWriter, r *http.Request) {
	if s.Files == nil || !s.Files.Configured() {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false, "roots": []files.Root{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": true, "roots": s.Files.Roots()})
}

// GET /api/files/list?root=&path=
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	root, ok := s.fileRoot(w, r.URL.Query().Get("root"), false)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")
	entries, err := mcfiles.List(root.Path, rel)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "%s", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"root": root.Name, "path": rel, "entries": entries})
}

// POST /api/files/op {root, action, path, to}
func (s *Server) handleFilesOp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Root   string `json:"root"`
		Action string `json:"action"` // rename | mkdir | delete | unzip
		Path   string `json:"path"`
		To     string `json:"to,omitempty"`
	}
	if !readJSON(w, r, &body, 16*1024) {
		return
	}
	root, ok := s.fileRoot(w, body.Root, true)
	if !ok {
		return
	}
	if body.Path == "" {
		writeErr(w, http.StatusBadRequest, "path is required")
		return
	}
	target := root.Name + ":" + body.Path

	switch body.Action {
	case "rename":
		if body.To == "" {
			writeErr(w, http.StatusBadRequest, "to is required for rename")
			return
		}
		err := mcfiles.Rename(root.Path, body.Path, body.To)
		s.actionResult(w, r, "files.rename", target, "-> "+body.To, err)
	case "mkdir":
		err := mcfiles.Mkdir(root.Path, body.Path)
		s.actionResult(w, r, "files.mkdir", target, "", err)
	case "delete":
		// Destructive: the UI collects a (typed, for directories)
		// confirmation of the base name.
		if !requireConfirm(w, r, path.Base(strings.TrimSuffix(body.Path, "/"))) {
			return
		}
		err := mcfiles.Delete(root.Path, body.Path)
		s.actionResult(w, r, "files.delete", target, "", err)
	case "unzip":
		relPath := body.Path
		rootPath := root.Path
		job, err := s.Jobs.Start("files.unzip", root.Name, func(ctx context.Context, out func(string)) error {
			return mcfiles.Unzip(ctx, rootPath, relPath, out)
		})
		_ = s.record(r, "files.unzip", target, "", err)
		if err != nil {
			writeErr(w, http.StatusConflict, "%s", err)
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		writeErr(w, http.StatusBadRequest, "invalid action %q", body.Action)
	}
}

// GET /api/files/download?root=&path= — regular files stream raw; a
// directory streams as an on-the-fly zip (nothing staged on disk).
func (s *Server) handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	root, ok := s.fileRoot(w, r.URL.Query().Get("root"), false)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")

	if f, info, err := mcfiles.OpenForDownload(root.Path, rel); err == nil {
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
		_ = s.record(r, "files.download", root.Name+":"+rel, "", nil)
		return
	}

	// Not a regular file: try a directory zip stream.
	name := path.Base(strings.TrimSuffix(rel, "/"))
	if rel == "" || name == "." || name == "/" {
		name = root.Name
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename*=UTF-8''%s.zip", url.PathEscape(name)))
	if err := files.StreamZip(r.Context(), root.Path, rel, w); err != nil {
		// Headers may already be gone; log through audit and cut the stream.
		_ = s.record(r, "files.download", root.Name+":"+rel, "zip", err)
		return
	}
	_ = s.record(r, "files.download", root.Name+":"+rel, "zip", nil)
}

// POST /api/files/upload?root=&path=&overwrite=1 (multipart)
func (s *Server) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	root, ok := s.fileRoot(w, r.URL.Query().Get("root"), true)
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
		dst, err := mcfiles.CreateForUpload(root.Path, rel, overwrite)
		if err != nil {
			_ = s.record(r, "files.upload", root.Name+":"+rel, "", err)
			writeErr(w, http.StatusUnprocessableEntity, "%s", err)
			return
		}
		_, err = io.Copy(dst, part)
		dst.Close()
		if err != nil {
			_ = s.record(r, "files.upload", root.Name+":"+rel, "", err)
			writeErr(w, http.StatusInternalServerError, "write %s: %s", name, err)
			return
		}
		saved = append(saved, name)
		_ = s.record(r, "files.upload", root.Name+":"+rel, "", nil)
	}
	if len(saved) == 0 {
		writeErr(w, http.StatusBadRequest, "no files in upload")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "saved": saved})
}
