package mc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ziggybadans/control-panel/internal/jobs"
	"github.com/ziggybadans/control-panel/internal/mcfiles"
)

// ImportServer creates <root>/<id> from an uploaded zip archive (already
// saved to zipPath) and rescans so it becomes a managed server. Runs as a
// job; the zip is deleted afterwards either way. Extraction carries the
// mcfiles guards (no path escapes, bounded total size), and an archive
// whose content sits inside a single top-level folder is flattened so the
// server files land directly in the server directory.
func (m *Manager) ImportServer(id, mem, zipPath string) (*jobs.View, error) {
	if err := CheckID(id); err != nil {
		return nil, err
	}
	if mem != "" && parseMem(mem) == 0 {
		return nil, fmt.Errorf("invalid memory value %q (use e.g. 4G)", mem)
	}
	m.mu.Lock()
	_, exists := m.instances[id]
	m.mu.Unlock()
	if exists {
		return nil, fmt.Errorf("server %q already exists", id)
	}
	dir := filepath.Join(m.cfg.Root, id)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("directory %s already exists", dir)
	}

	return m.runner.Start("mc.import", id, func(ctx context.Context, out func(string)) error {
		defer os.Remove(zipPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		out("extracting archive…")
		if err := mcfiles.UnzipInto(ctx, zipPath, dir, out); err != nil {
			// Leave nothing half-imported.
			_ = os.RemoveAll(dir)
			return err
		}
		if flattened, err := flattenSingleDir(dir); err != nil {
			out("note: could not flatten top-level folder: " + err.Error())
		} else if flattened != "" {
			out(fmt.Sprintf("moved contents of %q up to the server directory", flattened))
		}

		if mem != "" {
			m.mu.Lock()
			patch := m.overrides[id]
			memv := mem
			patch.Mem = &memv
			m.overrides[id] = patch
			m.saveOverridesLocked()
			m.mu.Unlock()
		}

		if err := m.Rescan(); err != nil {
			return err
		}
		if info, ok := m.Get(id); ok && info.Jar == "" {
			out("no server jar auto-detected — pick one under Settings → Server jar")
		} else if ok {
			out(fmt.Sprintf("detected server jar: %s", info.Jar))
		}
		out(fmt.Sprintf("imported %q — review its settings before starting", id))
		return nil
	})
}

// flattenSingleDir handles the common "zipped the folder, not its contents"
// case: when dir contains exactly one directory (ignoring archive junk),
// its children move up one level. Returns the flattened folder's name, or
// "" when nothing needed doing. Aborts without changes on any collision.
func flattenSingleDir(dir string) (string, error) {
	junk := map[string]bool{"__MACOSX": true, ".DS_Store": true}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var real []os.DirEntry
	for _, e := range entries {
		if !junk[e.Name()] {
			real = append(real, e)
		}
	}
	if len(real) != 1 || !real[0].IsDir() {
		return "", nil
	}
	inner := filepath.Join(dir, real[0].Name())
	children, err := os.ReadDir(inner)
	if err != nil {
		return "", err
	}
	for _, c := range children {
		if _, err := os.Lstat(filepath.Join(dir, c.Name())); err == nil {
			return "", fmt.Errorf("name collision on %q", c.Name())
		}
	}
	for _, c := range children {
		if err := os.Rename(filepath.Join(inner, c.Name()), filepath.Join(dir, c.Name())); err != nil {
			return "", err
		}
	}
	if err := os.Remove(inner); err != nil {
		return "", err
	}
	return real[0].Name(), nil
}
