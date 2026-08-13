package mc

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/events"
	"github.com/ziggybadans/control-panel/internal/jobs"
)

// writeTestZip builds a zip whose entries all sit inside one top-level
// folder — the common way people archive a server directory.
func writeTestZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func waitJob(t *testing.T, runner *jobs.Runner, id string) jobs.View {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := runner.Get(id); ok && v.State != jobs.StateRunning {
			return v
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return jobs.View{}
}

func TestImportServer(t *testing.T) {
	root := t.TempDir()
	dataDir := t.TempDir()
	runner := jobs.NewRunner()
	m := NewManager(config.Minecraft{Root: root, Java: "java"}, dataDir, events.NewBus(), runner)

	zipPath := filepath.Join(t.TempDir(), "upload.zip")
	writeTestZip(t, zipPath, map[string]string{
		"myserver/server.jar":        "fake jar bytes",
		"myserver/server.properties": "server-port=25599\n",
		"myserver/world/level.dat":   "world data",
		"__MACOSX/junk":              "",
	})

	job, err := m.ImportServer("imported", "6G", zipPath)
	if err != nil {
		t.Fatalf("ImportServer: %v", err)
	}
	v := waitJob(t, runner, job.ID)
	if v.State != jobs.StateDone {
		t.Fatalf("job state = %s, err = %s, output = %v", v.State, v.Err, v.Output)
	}

	// The single top-level folder was flattened away.
	if _, err := os.Stat(filepath.Join(root, "imported", "server.jar")); err != nil {
		t.Errorf("server.jar not at server root after flatten: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "imported", "myserver")); !os.IsNotExist(err) {
		t.Errorf("inner folder should be gone after flatten")
	}
	// The upload was cleaned up, the server registered, and the jar found.
	if _, err := os.Stat(zipPath); !os.IsNotExist(err) {
		t.Errorf("uploaded zip should be deleted")
	}
	info, ok := m.Get("imported")
	if !ok {
		t.Fatal("imported server not registered after rescan")
	}
	if info.Jar != "server.jar" {
		t.Errorf("jar = %q, want auto-detected server.jar", info.Jar)
	}
	if info.Mem != "6G" {
		t.Errorf("mem = %q, want 6G override", info.Mem)
	}

	// Importing over an existing id is refused.
	if _, err := m.ImportServer("imported", "", zipPath); err == nil {
		t.Error("duplicate import should fail")
	}
}

func TestFlattenSingleDirCollisions(t *testing.T) {
	dir := t.TempDir()
	// Two real entries at top level: nothing to flatten.
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if name, err := flattenSingleDir(dir); err != nil || name != "" {
		t.Errorf("flatten = %q, %v; want no-op", name, err)
	}
}
