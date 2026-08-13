package files

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"

	"github.com/ziggybadans/control-panel/internal/config"
)

// SeedMock builds a small sandbox tree under dataDir/mock-files and returns
// roots pointing at it, so the file manager is fully usable in mock mode
// without touching anything real. Idempotent: an existing tree is reused.
func SeedMock(dataDir string) ([]config.FilesRoot, error) {
	base := filepath.Join(dataDir, "mock-files")
	pool := filepath.Join(base, "pool")
	srv := filepath.Join(base, "srv")

	if _, err := os.Stat(base); err != nil {
		seed := map[string]string{
			"pool/movies/Dune Part Two (2024)/Dune.Part.Two.2024.2160p.mkv": strings.Repeat("mock video data ", 4096),
			"pool/movies/Heat (1995)/Heat.1995.1080p.mkv":                   strings.Repeat("mock video data ", 2048),
			"pool/tv/Severance/S02E01.mkv":                                  strings.Repeat("mock video data ", 1024),
			"pool/tv/Severance/S02E02.mkv":                                  strings.Repeat("mock video data ", 1024),
			"pool/music/Boards of Canada/Music Has the Right/01 Wildlife Analysis.flac": strings.Repeat("mock audio ", 512),
			"pool/public/documents/server-notes.txt": "drive layout, share names, and the snapraid plan live here.\n",
			"pool/public/backups/.keep":              "",
			"srv/minecraft/fabric/server.properties": "motd=mock server\nserver-port=25565\n",
			"srv/minecraft/fabric/whitelist.json":    "[]\n",
		}
		for rel, content := range seed {
			p := filepath.Join(base, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				return nil, err
			}
		}
		if err := writeSampleZip(filepath.Join(pool, "public", "sample-archive.zip")); err != nil {
			return nil, err
		}
	}

	return []config.FilesRoot{
		{Name: "pool", Path: pool},
		{Name: "srv", Path: srv, ReadOnly: true},
	}, nil
}

func writeSampleZip(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range map[string]string{
		"readme.txt":       "extracted from sample-archive.zip\n",
		"config/app.conf":  "key = value\n",
		"config/extra.env": "MODE=demo\n",
	} {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return err
		}
	}
	return zw.Close()
}
