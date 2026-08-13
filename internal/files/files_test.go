package files

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ziggybadans/control-panel/internal/config"
)

func TestServiceLookup(t *testing.T) {
	s := New([]config.FilesRoot{
		{Name: "pool", Path: "/mnt/pool"},
		{Name: "srv", Path: "/srv", ReadOnly: true},
	})
	if !s.Configured() {
		t.Fatal("expected configured")
	}
	r, err := s.Get("srv")
	if err != nil || !r.ReadOnly {
		t.Fatalf("Get(srv) = %+v, %v", r, err)
	}
	if _, err := s.Get("etc"); err == nil {
		t.Error("unknown root should error")
	}
	if New(nil).Configured() {
		t.Error("empty service should not be configured")
	}
}

func TestStreamZip(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("docs/a.txt", "alpha")
	mustWrite("docs/sub/b.txt", "beta")
	mustWrite("top.txt", "top")

	var buf bytes.Buffer
	if err := StreamZip(context.Background(), root, "docs", &buf); err != nil {
		t.Fatalf("StreamZip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
		if !f.FileInfo().IsDir() && f.Method != zip.Store {
			t.Errorf("%s: method = %d, want Store (speed over ratio)", f.Name, f.Method)
		}
	}
	for _, want := range []string{"a.txt", "sub/", "sub/b.txt"} {
		if !got[want] {
			t.Errorf("missing zip entry %q (have %v)", want, got)
		}
	}

	// A single file zips under its own base name.
	buf.Reset()
	if err := StreamZip(context.Background(), root, "top.txt", &buf); err != nil {
		t.Fatalf("StreamZip(file): %v", err)
	}
	zr, _ = zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if len(zr.File) != 1 || zr.File[0].Name != "top.txt" {
		t.Errorf("single-file zip entries = %v", zr.File)
	}

	// Escapes are refused by Resolve.
	if err := StreamZip(context.Background(), root, "../outside", &buf); err == nil {
		t.Error("path escape should fail")
	}
}

func TestSeedMockIdempotent(t *testing.T) {
	dir := t.TempDir()
	r1, err := SeedMock(dir)
	if err != nil {
		t.Fatalf("SeedMock: %v", err)
	}
	if len(r1) != 2 {
		t.Fatalf("roots = %v", r1)
	}
	marker := filepath.Join(r1[0].Path, "marker.txt")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedMock(dir); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("re-seeding must not clobber existing sandbox contents")
	}
}
