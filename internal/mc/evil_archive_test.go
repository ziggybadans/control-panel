package mc

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"testing"
)

// writeEvilArchive builds a tar.gz whose entry path escapes the extraction
// directory (used to prove the extractor refuses it).
func writeEvilArchive(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("pwned")
	if err := tw.WriteHeader(&tar.Header{
		Name: "../escape.txt",
		Mode: 0o644,
		Size: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

// writeSymlinkArchive builds a tar.gz containing a single symlink entry.
func writeSymlinkArchive(t *testing.T, path, name, linkname string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeSymlink,
		Name:     name,
		Linkname: linkname,
		Mode:     0o777,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}
