package mcfiles

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "server.properties"), "motd=hi\n")
	mustWrite(t, filepath.Join(root, "world", "level.dat"), "DATA")
	mustWrite(t, filepath.Join(root, "plugins", "Example.jar"), "not-really-a-jar")
	return root
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRejectsEscapes(t *testing.T) {
	root := newRoot(t)
	// A sibling file outside the root that attacks try to reach.
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	os.WriteFile(outside, []byte("secret"), 0o644)

	attacks := []string{
		"../outside.txt",
		"../../etc/passwd",
		"world/../../outside.txt",
		"/../outside.txt",
		"....//....//etc",
	}
	for _, a := range attacks {
		if abs, err := Resolve(root, a, false); err == nil {
			rootReal, _ := filepath.EvalSymlinks(root)
			if absReal, rerr := filepath.EvalSymlinks(abs); rerr == nil {
				if absReal != rootReal && !isUnder(absReal, rootReal) {
					t.Errorf("Resolve(%q) escaped to %s", a, abs)
				}
			}
		}
	}
	// Any ".." segment is rejected outright, not silently remapped.
	for _, a := range attacks {
		if strings.Contains(a, "..") && !strings.Contains(a, "....") {
			if _, err := Resolve(root, a, false); err == nil {
				t.Errorf("Resolve(%q) accepted a dot-dot path", a)
			}
		}
	}
}

func isUnder(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) &&
		!(len(rel) >= 3 && rel[:3] == "../")
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	root := newRoot(t)
	outsideDir := t.TempDir()
	mustWrite(t, filepath.Join(outsideDir, "loot.txt"), "outside")
	if err := os.Symlink(outsideDir, filepath.Join(root, "sneaky")); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "sneaky/loot.txt", true); err == nil {
		t.Error("symlink escape not rejected")
	}
	// Even resolving the symlink itself for download must fail.
	if _, _, err := OpenForDownload(root, "sneaky/loot.txt"); err == nil {
		t.Error("download through symlink escape not rejected")
	}
}

func TestRenameAndDelete(t *testing.T) {
	root := newRoot(t)
	if err := Rename(root, "world", "world_old"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "world_old", "level.dat")); err != nil {
		t.Error("rename lost contents")
	}
	// Destination collision is refused.
	if err := Rename(root, "server.properties", "world_old"); err == nil {
		t.Error("rename over existing path allowed")
	}
	if err := Delete(root, "world_old"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "world_old")); !os.IsNotExist(err) {
		t.Error("delete left directory behind")
	}
	// Deleting the root itself is refused.
	if err := Delete(root, "."); err == nil {
		t.Error("deleting server root allowed")
	}
	if err := Delete(root, "/"); err == nil {
		t.Error("deleting server root via / allowed")
	}
}

func TestZipUnzipRoundTrip(t *testing.T) {
	root := newRoot(t)
	sink := func(string) {}
	if err := Zip(context.Background(), root, "world", sink); err != nil {
		t.Fatal(err)
	}
	if err := Rename(root, "world", "world_orig"); err != nil {
		t.Fatal(err)
	}
	if err := Unzip(context.Background(), root, "world.zip", sink); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "world", "level.dat"))
	if err != nil || string(b) != "DATA" {
		t.Errorf("round trip content = %q, err %v", b, err)
	}
	// Unzip refuses to clobber an existing destination.
	if err := Unzip(context.Background(), root, "world.zip", sink); err == nil {
		t.Error("unzip over existing directory allowed")
	}
}

func TestUnzipRejectsTraversal(t *testing.T) {
	root := newRoot(t)
	evil := filepath.Join(root, "evil.zip")
	f, _ := os.Create(evil)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../pwned.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()

	if err := Unzip(context.Background(), root, "evil.zip", func(string) {}); err == nil {
		t.Fatal("traversal archive accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "pwned.txt")); err == nil {
		t.Fatal("file escaped during unzip")
	}
}

func TestAddonsToggle(t *testing.T) {
	root := newRoot(t)
	addons, err := ListAddons(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(addons) != 1 || !addons[0].Enabled || addons[0].Dir != "plugins" {
		t.Fatalf("addons = %+v", addons)
	}
	if err := ToggleAddon(root, "plugins", "Example.jar", false); err != nil {
		t.Fatal(err)
	}
	addons, _ = ListAddons(root)
	if len(addons) != 1 || addons[0].Enabled {
		t.Fatalf("after disable: %+v", addons)
	}
	if err := ToggleAddon(root, "plugins", addons[0].File, true); err != nil {
		t.Fatal(err)
	}
	addons, _ = ListAddons(root)
	if !addons[0].Enabled {
		t.Error("re-enable failed")
	}
	// Path shenanigans in the file name are rejected.
	if err := ToggleAddon(root, "plugins", "../server.properties", false); err == nil {
		t.Error("path traversal in addon name allowed")
	}
	if err := ToggleAddon(root, "config", "x.jar", false); err == nil {
		t.Error("arbitrary directory allowed")
	}
}

func TestJarMetadataFromPluginYML(t *testing.T) {
	root := newRoot(t)
	jar := filepath.Join(root, "plugins", "Real.jar")
	f, _ := os.Create(jar)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("plugin.yml")
	w.Write([]byte("name: EssentialsX\nversion: 2.20.1\nmain: com.example.Main\n"))
	zw.Close()
	f.Close()

	addons, _ := ListAddons(root)
	for _, a := range addons {
		if a.File == "Real.jar" {
			if a.Name != "EssentialsX" || a.Version != "2.20.1" {
				t.Errorf("metadata = %q %q", a.Name, a.Version)
			}
			return
		}
	}
	t.Error("Real.jar not listed")
}
