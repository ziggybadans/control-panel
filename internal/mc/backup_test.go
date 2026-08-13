package mc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTarGzRoundTripWithExclusions(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "server.properties"), "motd=hi\n")
	mustWrite(t, filepath.Join(src, "world", "level.dat"), "DATA")
	mustWrite(t, filepath.Join(src, "logs", "latest.log"), "SPAM")
	mustWrite(t, filepath.Join(src, "cache", "x.bin"), "CACHE")
	mustWrite(t, filepath.Join(src, "session.lock"), "LOCK")

	archive := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := tarGzDir(context.Background(), src, archive, func(string) {}); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := extractTarGz(archive, dest); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"server.properties", "world/level.dat"} {
		if _, err := os.Stat(filepath.Join(dest, want)); err != nil {
			t.Errorf("missing %s after round trip", want)
		}
	}
	for _, excluded := range []string{"logs/latest.log", "cache/x.bin", "session.lock"} {
		if _, err := os.Stat(filepath.Join(dest, excluded)); err == nil {
			t.Errorf("%s should have been excluded", excluded)
		}
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	// Hand-craft an archive containing "../escape.txt".
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.tar.gz")
	writeEvilArchive(t, archive)

	dest := filepath.Join(dir, "dest")
	_ = os.MkdirAll(dest, 0o755)
	if err := extractTarGz(archive, dest); err == nil {
		t.Fatal("expected traversal error")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err == nil {
		t.Fatal("file escaped the destination directory")
	}
}

func TestExtractSkipsSymlinkToPrefixSibling(t *testing.T) {
	// A symlink whose target is a SIBLING of the destination that shares its
	// name as a string prefix ("dest-evil" vs "dest") must not be created:
	// the containment check needs the path separator, not a bare prefix.
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil-link.tar.gz")
	writeSymlinkArchive(t, archive, "link", "../dest-evil/x")

	dest := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(archive, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link")); err == nil {
		t.Fatal("symlink pointing outside the destination was created")
	}
}

func TestExtractAllowsInternalSymlink(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "ok-link.tar.gz")
	writeSymlinkArchive(t, archive, "link", "world")

	dest := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(archive, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link")); err != nil {
		t.Fatal("relative symlink inside the destination should be preserved")
	}
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
