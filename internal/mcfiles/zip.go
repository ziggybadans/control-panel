package mcfiles

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxUnzipBytes caps total extracted size as a zip-bomb guard.
// A var so tests can lower it without writing gigabytes.
var maxUnzipBytes int64 = 20 << 30 // 20 GiB

// Zip archives a file or directory at rel into "<rel>.zip" next to it.
func Zip(ctx context.Context, root, rel string, out func(string)) error {
	src, err := Resolve(root, rel, true)
	if err != nil {
		return err
	}
	destRel := strings.TrimSuffix(rel, "/") + ".zip"
	dest, err := Resolve(root, destRel, false)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("already exists: %s", destRel)
	}

	f, err := os.Create(dest + ".partial")
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(dest + ".partial")
	}()
	zw := zip.NewWriter(f)

	// Entries are stored relative to src so that Unzip (which extracts into
	// a directory named after the archive) reproduces the original layout.
	count := 0
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
			// src is a regular file: store it under its own base name.
			if !d.IsDir() {
				name = filepath.Base(src)
			} else {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			if _, err := zw.Create(name + "/"); err != nil {
				return err
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = name
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			out("skipped " + name + ": " + err.Error())
			return nil
		}
		_, err = io.Copy(w, srcFile)
		srcFile.Close()
		if err != nil {
			return err
		}
		count++
		if count%250 == 0 {
			out(fmt.Sprintf("%d files archived…", count))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(dest+".partial", dest); err != nil {
		return err
	}
	out(fmt.Sprintf("created %s (%d files)", destRel, count))
	return nil
}

// Unzip extracts rel (a .zip inside the server dir) into a directory named
// after the archive. Entries may not escape the destination.
func Unzip(ctx context.Context, root, rel string, out func(string)) error {
	src, err := Resolve(root, rel, true)
	if err != nil {
		return err
	}
	destRel := strings.TrimSuffix(rel, ".zip")
	if destRel == rel {
		return fmt.Errorf("not a .zip archive: %s", rel)
	}
	dest, err := Resolve(root, destRel, false)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("already exists: %s — remove or rename it first", destRel)
	}

	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer zr.Close()

	cleanDest := filepath.Clean(dest)
	var total int64
	for i, zf := range zr.File {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(zf.Name))
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(filepath.Separator)) {
			return fmt.Errorf("archive entry escapes destination: %q", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		// Reject early on declared sizes, then meter what is actually
		// written: a lying header must not grant a fresh budget per entry.
		if int64(zf.UncompressedSize64) > maxUnzipBytes-total {
			return fmt.Errorf("archive too large (over %d GiB extracted)", maxUnzipBytes>>30)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, zf.Mode().Perm()|0o200)
		if err != nil {
			rc.Close()
			return err
		}
		remaining := maxUnzipBytes - total
		n, err := io.Copy(w, io.LimitReader(rc, remaining+1))
		rc.Close()
		w.Close()
		if err != nil {
			return err
		}
		total += n
		if n > remaining {
			return fmt.Errorf("archive too large (over %d GiB extracted)", maxUnzipBytes>>30)
		}
		if (i+1)%250 == 0 {
			out(fmt.Sprintf("%d entries extracted…", i+1))
		}
	}
	out(fmt.Sprintf("extracted %d entries into %s", len(zr.File), destRel))
	return nil
}
