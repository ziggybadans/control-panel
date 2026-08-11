// Package ui embeds the built frontend and serves it as a single-page app.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the SPA: hashed assets get immutable caching, everything
// else falls back to index.html (client-side routing).
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			// SPA fallback.
			if _, ierr := fs.Stat(sub, "index.html"); ierr != nil {
				http.Error(w, "UI not built — run `make ui` and rebuild the binary", http.StatusServiceUnavailable)
				return
			}
			r.URL.Path = "/"
			path = "index.html"
		}
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
		fileServer.ServeHTTP(w, r)
	})
}
