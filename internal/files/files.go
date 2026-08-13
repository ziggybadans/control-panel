// Package files exposes configured directory trees ("roots") to the panel's
// general file manager. All path handling is delegated to mcfiles.Resolve,
// which confines every operation to its root — lexically (no "..") and
// physically (symlinks may not lead outside).
package files

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/mcfiles"
)

// Root is one browsable tree.
type Root struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	ReadOnly bool   `json:"readOnly"`
}

type Service struct {
	roots []Root
}

func New(cfgRoots []config.FilesRoot) *Service {
	s := &Service{}
	for _, r := range cfgRoots {
		s.roots = append(s.roots, Root{Name: r.Name, Path: r.Path, ReadOnly: r.ReadOnly})
	}
	return s
}

func (s *Service) Configured() bool { return len(s.roots) > 0 }

func (s *Service) Roots() []Root {
	out := make([]Root, len(s.roots))
	copy(out, s.roots)
	return out
}

func (s *Service) Get(name string) (Root, error) {
	for _, r := range s.roots {
		if r.Name == name {
			return r, nil
		}
	}
	return Root{}, fmt.Errorf("unknown root %q", name)
}

// StreamZip writes a zip of the file or directory at rel directly to w
// (nothing is staged on disk). Entries are stored uncompressed: downloads
// are LAN-bound and mostly media, so raw throughput beats deflate.
func StreamZip(ctx context.Context, root, rel string, w io.Writer) error {
	src, err := mcfiles.Resolve(root, rel, true)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		relInSrc, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relInSrc)
		if name == "." {
			if d.IsDir() {
				return nil
			}
			name = filepath.Base(src) // zipping a single regular file
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			_, err := zw.Create(name + "/")
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = name
		hdr.Method = zip.Store
		entry, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil // unreadable file: skip rather than abort mid-stream
		}
		_, err = io.Copy(entry, f)
		f.Close()
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}
