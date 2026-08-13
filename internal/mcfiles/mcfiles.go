// Package mcfiles implements file management inside a Minecraft server
// directory. Every operation resolves paths through Resolve, which confines
// access to the server root — lexically (no ".." escape) and physically
// (symlinks may not lead outside).
package mcfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Entry struct {
	Name    string `json:"name"`
	Dir     bool   `json:"dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // unix ms
}

// ErrEscape is returned when a path would leave the server directory.
var ErrEscape = fmt.Errorf("path escapes the server directory")

// Resolve maps a client-supplied relative path to an absolute path inside
// root. mustExist controls whether the final component is required to exist
// (creation targets resolve their parent instead).
func Resolve(root, rel string, mustExist bool) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("invalid path")
	}
	// Reject ".." outright: silently remapping a traversal attempt would be
	// confusing even though Clean below makes it harmless.
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == ".." {
			return "", ErrEscape
		}
	}
	// Force the path to be interpreted relative to root: Clean("/"+rel)
	// collapses anything that slipped through lexically.
	clean := filepath.Clean("/" + filepath.FromSlash(rel))
	abs := filepath.Join(root, clean)

	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("server directory unavailable: %w", err)
	}

	// Physically verify the deepest existing ancestor (or the path itself)
	// stays inside root even through symlinks.
	check := abs
	for {
		real, err := filepath.EvalSymlinks(check)
		if err == nil {
			if real != rootReal && !strings.HasPrefix(real, rootReal+string(filepath.Separator)) {
				return "", ErrEscape
			}
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(check)
		if parent == check {
			return "", ErrEscape
		}
		check = parent
	}
	if mustExist {
		if _, err := os.Lstat(abs); err != nil {
			return "", fmt.Errorf("not found: %s", rel)
		}
	}
	return abs, nil
}

// List returns the entries of a directory, directories first.
func List(root, rel string) ([]Entry, error) {
	dir, err := Resolve(root, rel, true)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		entry := Entry{Name: e.Name(), Dir: e.IsDir()}
		// A failed stat (racing deletion, exotic fs) must not hide the
		// entry — show it with zero metadata instead.
		if info, err := e.Info(); err == nil {
			entry.Size = info.Size()
			entry.ModTime = info.ModTime().UnixMilli()
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// Rename moves/renames within the server directory. The destination's
// parent must exist; the destination itself must not.
func Rename(root, fromRel, toRel string) error {
	from, err := Resolve(root, fromRel, true)
	if err != nil {
		return err
	}
	to, err := Resolve(root, toRel, false)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("destination already exists: %s", toRel)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Rename(from, to)
}

func Mkdir(root, rel string) error {
	abs, err := Resolve(root, rel, false)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

// Delete removes a file or directory tree. The server root itself is
// protected.
func Delete(root, rel string) error {
	abs, err := Resolve(root, rel, true)
	if err != nil {
		return err
	}
	rootReal, _ := filepath.EvalSymlinks(root)
	if absReal, err := filepath.EvalSymlinks(abs); err == nil && absReal == rootReal {
		return fmt.Errorf("refusing to delete the server directory itself")
	}
	return os.RemoveAll(abs)
}

// OpenForDownload returns a regular file for streaming to the client.
func OpenForDownload(root, rel string) (*os.File, os.FileInfo, error) {
	abs, err := Resolve(root, rel, true)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("not a regular file: %s", rel)
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, nil, err
	}
	return f, info, nil
}

// CreateForUpload opens a destination file for an upload, creating parent
// directories as needed. Refuses to overwrite unless overwrite is set.
func CreateForUpload(root, rel string, overwrite bool) (*os.File, error) {
	abs, err := Resolve(root, rel, false)
	if err != nil {
		return nil, err
	}
	if !overwrite {
		if _, err := os.Lstat(abs); err == nil {
			return nil, fmt.Errorf("already exists: %s", rel)
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(abs, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
}
