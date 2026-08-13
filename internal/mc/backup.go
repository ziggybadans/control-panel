package mc

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// backupExcludes are directory/file names skipped when archiving a server.
var backupExcludes = map[string]bool{
	"logs": true, "crash-reports": true, "cache": true,
	".DS_Store": true, "session.lock": true, "debug": true,
}

// tarGzDir archives srcDir into destFile. out receives progress lines.
func tarGzDir(ctx context.Context, srcDir, destFile string, out func(string)) error {
	tmp := destFile + ".partial"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(tmp)
	}()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	var files, bytes int64
	err = filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Exclude by top-level name or any path segment match.
		for _, seg := range strings.Split(rel, string(filepath.Separator)) {
			if backupExcludes[seg] {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// Skip sockets, device files, etc.
		if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			hdr.Linkname = link
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			src, err := os.Open(path)
			if err != nil {
				// File vanished mid-backup (server churn); note and move on.
				out("skipped " + rel + ": " + err.Error())
				return nil
			}
			n, err := io.Copy(tw, src)
			src.Close()
			if err != nil {
				return fmt.Errorf("archiving %s: %w", rel, err)
			}
			files++
			bytes += n
			if files%500 == 0 {
				out(fmt.Sprintf("%d files, %.1f MiB archived…", files, float64(bytes)/(1<<20)))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, destFile); err != nil {
		return err
	}
	st, _ := os.Stat(destFile)
	out(fmt.Sprintf("archive complete: %d files, %.1f MiB (compressed %.1f MiB)",
		files, float64(bytes)/(1<<20), float64(st.Size())/(1<<20)))
	return nil
}

// maxRestoreBytes bounds total extraction as a decompression-bomb guard.
// Generous on purpose: legitimate modded worlds run large, and a hostile
// archive is stopped long before it can exhaust the pool.
const maxRestoreBytes = 512 << 30 // 512 GiB

// extractTarGz extracts archive into destDir, refusing entries that would
// escape it.
func extractTarGz(archive, destDir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	cleanDest := filepath.Clean(destDir)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(target, cleanDest+string(filepath.Separator)) && target != cleanDest {
			return fmt.Errorf("archive entry escapes destination: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, hdr.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Only allow relative links that stay inside the destination.
			// The separator matters: without it a link to a sibling of the
			// destination that shares its name as a prefix would slip through.
			linkTarget := filepath.Clean(filepath.Join(filepath.Dir(target), hdr.Linkname))
			inside := linkTarget == cleanDest ||
				strings.HasPrefix(linkTarget, cleanDest+string(filepath.Separator))
			if filepath.IsAbs(hdr.Linkname) || !inside {
				continue
			}
			_ = os.Symlink(hdr.Linkname, target)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, hdr.FileInfo().Mode().Perm())
			if err != nil {
				return err
			}
			n, err := io.Copy(dst, tr)
			dst.Close()
			if err != nil {
				return err
			}
			total += n
			if total > maxRestoreBytes {
				return fmt.Errorf("archive too large (over %d GiB extracted)", maxRestoreBytes>>30)
			}
		}
	}
}
