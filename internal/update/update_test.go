package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGithub serves a minimal release API: /repos/o/r/releases/{latest,tags/x},
// plus asset downloads with the octet-stream negotiation the real API uses.
func fakeGithub(t *testing.T, binary []byte, sums string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	release := func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
			"tag_name": "v1.2.3",
			"body": "notes",
			"published_at": "2026-08-13T00:00:00Z",
			"assets": [
				{"name": %q, "url": %q, "size": %d},
				{"name": %q, "url": %q, "size": %d}
			]
		}`, assetName, srv.URL+"/assets/1", len(binary),
			sumsName, srv.URL+"/assets/2", len(sums))
	}
	mux.HandleFunc("/repos/o/r/releases/latest", release)
	mux.HandleFunc("/repos/o/r/releases/tags/v1.2.3", release)
	mux.HandleFunc("/assets/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(binary)
	})
	mux.HandleFunc("/assets/2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func elfBinary(content string) []byte {
	return append([]byte("\x7fELF"), []byte(content)...)
}

func sumsFor(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]) + "  " + assetName + "\n"
}

func newTestClient(url, current string) *Client {
	c := NewClient("o/r", "", current)
	c.apiBase = url
	return c
}

func TestStatusReportsUpdate(t *testing.T) {
	bin := elfBinary("new build")
	srv := fakeGithub(t, bin, sumsFor(bin))

	st := newTestClient(srv.URL, "v1.0.0").Status(context.Background(), true)
	if st.Error != "" {
		t.Fatalf("unexpected error: %s", st.Error)
	}
	if !st.UpdateAvailable || st.Latest == nil || st.Latest.Tag != "v1.2.3" {
		t.Fatalf("expected v1.2.3 available, got %+v", st)
	}

	st = newTestClient(srv.URL, "v1.2.3").Status(context.Background(), true)
	if st.UpdateAvailable {
		t.Fatalf("running the released tag must not report an update: %+v", st)
	}
}

func TestStatusUnconfigured(t *testing.T) {
	st := NewClient("", "", "dev").Status(context.Background(), true)
	if st.Configured || st.Error != "" {
		t.Fatalf("unconfigured status should be silent, got %+v", st)
	}
}

// installTo points the running-binary path at a scratch file by using the
// verified-download path directly (Apply's exe swap depends on os.Executable,
// which tests cannot safely redirect).
func TestDownloadVerified(t *testing.T) {
	bin := elfBinary("payload")
	srv := fakeGithub(t, bin, sumsFor(bin))
	c := newTestClient(srv.URL, "v1.0.0")

	rel, err := c.fetchRelease(context.Background(), "latest")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := c.fetchChecksum(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out")
	if err := c.downloadVerified(context.Background(), rel.assetURL, dst, sum); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(bin) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadRejectsBadChecksum(t *testing.T) {
	bin := elfBinary("payload")
	srv := fakeGithub(t, bin, sumsFor(elfBinary("different build")))
	c := newTestClient(srv.URL, "v1.0.0")

	rel, _ := c.fetchRelease(context.Background(), "latest")
	sum, err := c.fetchChecksum(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out")
	err = c.downloadVerified(context.Background(), rel.assetURL, dst, sum)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestDownloadRejectsNonELF(t *testing.T) {
	bin := []byte("#!/bin/sh\necho not a binary\n")
	srv := fakeGithub(t, bin, sumsFor(bin))
	c := newTestClient(srv.URL, "v1.0.0")

	rel, _ := c.fetchRelease(context.Background(), "latest")
	sum, _ := c.fetchChecksum(context.Background(), rel)
	err := c.downloadVerified(context.Background(), rel.assetURL,
		filepath.Join(t.TempDir(), "out"), sum)
	if err == nil || !strings.Contains(err.Error(), "not a linux executable") {
		t.Fatalf("expected ELF rejection, got %v", err)
	}
}

func TestFetchReleaseRejectsForeignAssetHost(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v1.2.3","assets":[
			{"name": %q, "url": "https://evil.example/x", "size": 1},
			{"name": %q, "url": %q, "size": 1}
		]}`, assetName, sumsName, srv.URL+"/assets/2")
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	_, err := newTestClient(srv.URL, "v1.0.0").fetchRelease(context.Background(), "latest")
	if err == nil || !strings.Contains(err.Error(), "not on") {
		t.Fatalf("expected foreign-host rejection, got %v", err)
	}
}

func TestChecksumMissingEntry(t *testing.T) {
	bin := elfBinary("payload")
	srv := fakeGithub(t, bin, "deadbeef  some-other-file\n")
	c := newTestClient(srv.URL, "v1.0.0")

	rel, _ := c.fetchRelease(context.Background(), "latest")
	_, err := c.fetchChecksum(context.Background(), rel)
	if err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Fatalf("expected missing-entry error, got %v", err)
	}
}
