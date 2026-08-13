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

func loadYAML(t *testing.T, yaml string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestRemoteNoneRequiresOptIn(t *testing.T) {
	// mode none on a non-loopback listen: refused without the explicit flag.
	if _, err := loadYAML(t, "listen: \"0.0.0.0:9090\"\nauth:\n  mode: none\n"); err == nil {
		t.Error("auth.mode none on 0.0.0.0 should be rejected without allow_remote_none")
	}
	// Loopback listen: fine.
	if _, err := loadYAML(t, "listen: \"127.0.0.1:9090\"\nauth:\n  mode: none\n"); err != nil {
		t.Errorf("auth.mode none on loopback rejected: %v", err)
	}
	if _, err := loadYAML(t, "listen: \"localhost:9090\"\nauth:\n  mode: none\n"); err != nil {
		t.Errorf("auth.mode none on localhost rejected: %v", err)
	}
	// Explicit opt-in: allowed.
	if _, err := loadYAML(t, "listen: \"0.0.0.0:9090\"\nauth:\n  mode: none\n  allow_remote_none: true\n"); err != nil {
		t.Errorf("explicit allow_remote_none rejected: %v", err)
	}
}

func TestTrustedProxiesValidated(t *testing.T) {
	if _, err := loadYAML(t, "trusted_proxies: [\"not-an-ip\"]\n"); err == nil {
		t.Error("invalid trusted_proxies entry should be rejected")
	}
	cfg, err := loadYAML(t, "trusted_proxies: [\"127.0.0.1\", \"10.0.0.0/8\"]\n")
	if err != nil {
		t.Fatalf("valid trusted_proxies rejected: %v", err)
	}
	if got := len(cfg.TrustedProxyNets()); got != 2 {
		t.Errorf("TrustedProxyNets: got %d nets, want 2", got)
	}
}
