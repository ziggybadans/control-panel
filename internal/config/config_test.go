package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAreSafe(t *testing.T) {
	cfg := Default()
	if cfg.Auth.Mode != "password" {
		t.Error("default auth mode must be password")
	}
	if cfg.Power.Allow {
		t.Error("power actions must be disabled by default")
	}
}

func TestLoadValidatesAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
listen: "0.0.0.0:1234"
minecraft:
  root: /srv/mc
metrics:
  interval_ms: 50
`), 0o644)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "0.0.0.0:1234" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	if cfg.Metrics.IntervalMS < 250 {
		t.Error("interval floor not applied")
	}
	if cfg.Minecraft.BackupDir != "/srv/mc/.backups" {
		t.Errorf("backup dir default = %q", cfg.Minecraft.BackupDir)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("listne: \"typo\"\n"), 0o644)
	if _, err := Load(path); err == nil {
		t.Error("typo'd config key should be rejected, not silently ignored")
	}
}

func TestLoadRejectsHalfTLS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("tls:\n  cert: /x.pem\n"), 0o644)
	if _, err := Load(path); err == nil {
		t.Error("cert without key should be rejected")
	}
}
