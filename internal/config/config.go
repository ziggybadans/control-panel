// Package config loads and validates the panel configuration file.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen  string `yaml:"listen"`
	DataDir string `yaml:"data_dir"`

	Auth      Auth      `yaml:"auth"`
	TLS       TLS       `yaml:"tls"`
	Metrics   Metrics   `yaml:"metrics"`
	Storage   Storage   `yaml:"storage"`
	Services  Services  `yaml:"services"`
	Plex      Plex      `yaml:"plex"`
	Apps      []App     `yaml:"apps"`
	Minecraft Minecraft `yaml:"minecraft"`
	Power     Power     `yaml:"power"`
	Update    Update    `yaml:"update"`
}

// App is one media-stack integration (Radarr, Sonarr, Overseerr, …).
type App struct {
	// Name shown in the UI (defaults to the capitalized type).
	Name string `yaml:"name"`
	// Type selects the API dialect: radarr | sonarr | lidarr | readarr |
	// whisparr | prowlarr | overseerr | jellyseerr.
	Type   string `yaml:"type"`
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

type Auth struct {
	// Mode is "password" (default) or "none". "none" disables authentication
	// entirely and must only be used on trusted networks or behind another
	// authenticating proxy.
	Mode         string `yaml:"mode"`
	PasswordHash string `yaml:"password_hash"`
	SessionHours int    `yaml:"session_hours"`
}

type TLS struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type Metrics struct {
	IntervalMS int `yaml:"interval_ms"` // sample period while clients are connected
	History    int `yaml:"history"`     // number of samples kept for charts
}

type Storage struct {
	// Pools lists mergerfs mount points. Empty means auto-detect from
	// /proc/mounts (any fuse.mergerfs mount).
	Pools []string `yaml:"pools"`
	// Smart enables SMART polling via smartctl (if installed).
	Smart bool `yaml:"smart"`
	// ExcludeMounts hides mounts from the panel (in addition to pseudo
	// filesystems, which are always hidden).
	ExcludeMounts []string `yaml:"exclude_mounts"`
	Snapraid      Snapraid `yaml:"snapraid"`
}

type Snapraid struct {
	// Config is the path to snapraid.conf. Empty disables the integration.
	Config string `yaml:"config"`
	// Binary overrides the snapraid executable path (default: from $PATH).
	Binary string `yaml:"binary"`
}

type Services struct {
	// Units is the allowlist of systemd units shown in — and controllable
	// from — the panel. Anything not listed here cannot be touched.
	Units []string `yaml:"units"`
	// AllowActions globally enables start/stop/restart. When false the
	// services page is read-only.
	AllowActions bool `yaml:"allow_actions"`
}

type Plex struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type Minecraft struct {
	// Root directory containing one subdirectory per server.
	Root string `yaml:"root"`
	// Java is the default java executable (overridable per server).
	Java string `yaml:"java"`
	// BackupDir stores server backups (default: <root>/.backups).
	BackupDir string `yaml:"backup_dir"`
	// Resume restarts servers that were running when the panel stopped.
	Resume  *bool               `yaml:"resume"`
	Servers map[string]MCServer `yaml:"servers"`
}

type MCServer struct {
	// Mem is the JVM heap, e.g. "4G" (sets -Xms and -Xmx).
	Mem  string `yaml:"mem"`
	Java string `yaml:"java"`
	// Jar overrides jar auto-detection.
	Jar string `yaml:"jar"`
	// JVMArgs are appended verbatim to the java command line.
	JVMArgs []string `yaml:"jvm_args"`
	// Aikar enables the standard Aikar GC flags.
	Aikar bool `yaml:"aikar"`
	// AutoStart launches the server when the panel starts.
	AutoStart bool `yaml:"autostart"`
	// AutoRestart relaunches the server after a crash (max 3 in 10 minutes).
	AutoRestart bool `yaml:"auto_restart"`
}

type Power struct {
	// Allow enables the reboot/shutdown endpoints. Off by default.
	Allow bool `yaml:"allow"`
}

type Update struct {
	// Repo pins the GitHub "owner/name" repository the panel updates itself
	// from. Only this repo's releases are ever fetched. Empty disables the
	// in-panel updater.
	Repo string `yaml:"repo"`
	// Token authenticates release requests when the repo is private
	// (fine-grained PAT with contents: read-only). Public repos need none.
	Token string `yaml:"token"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Listen:  "0.0.0.0:9090",
		DataDir: "/var/lib/control-panel",
		Auth: Auth{
			Mode:         "password",
			SessionHours: 24 * 7,
		},
		Metrics: Metrics{
			IntervalMS: 1000,
			History:    600,
		},
		Storage: Storage{Smart: true},
		Services: Services{
			AllowActions: true,
		},
		Plex: Plex{URL: "http://127.0.0.1:32400"},
		Minecraft: Minecraft{
			Root: "/srv/minecraft",
			Java: "java",
		},
	}
}

// Load reads path (if non-empty) over the defaults and validates the result.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	switch c.Auth.Mode {
	case "", "password":
		c.Auth.Mode = "password"
	case "none":
	default:
		return fmt.Errorf("auth.mode must be \"password\" or \"none\", got %q", c.Auth.Mode)
	}
	if c.Auth.SessionHours <= 0 {
		c.Auth.SessionHours = 24 * 7
	}
	if c.Metrics.IntervalMS < 250 {
		c.Metrics.IntervalMS = 250
	}
	if c.Metrics.History < 60 {
		c.Metrics.History = 60
	}
	if c.Metrics.History > 3600 {
		c.Metrics.History = 3600
	}
	if (c.TLS.Cert == "") != (c.TLS.Key == "") {
		return fmt.Errorf("tls.cert and tls.key must be set together")
	}
	if c.Minecraft.BackupDir == "" {
		c.Minecraft.BackupDir = c.Minecraft.Root + "/.backups"
	}
	if c.Minecraft.Java == "" {
		c.Minecraft.Java = "java"
	}
	if r := c.Update.Repo; r != "" {
		parts := strings.Split(r, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(r, ":") {
			return fmt.Errorf("update.repo must be \"owner/name\", got %q", r)
		}
	}
	for i := range c.Apps {
		app := &c.Apps[i]
		if !ValidAppTypes[app.Type] {
			return fmt.Errorf("apps[%d]: unknown type %q (valid: radarr, sonarr, lidarr, readarr, whisparr, prowlarr, overseerr, jellyseerr)", i, app.Type)
		}
		if app.URL == "" {
			return fmt.Errorf("apps[%d] (%s): url is required", i, app.Type)
		}
		if app.Name == "" {
			app.Name = strings.ToUpper(app.Type[:1]) + app.Type[1:]
		}
	}
	return nil
}

// ValidAppTypes is the set of supported media-stack integrations.
var ValidAppTypes = map[string]bool{
	"radarr": true, "sonarr": true, "lidarr": true, "readarr": true,
	"whisparr": true, "prowlarr": true, "overseerr": true, "jellyseerr": true,
}

// SessionTTL returns the configured session lifetime.
func (c *Config) SessionTTL() time.Duration {
	return time.Duration(c.Auth.SessionHours) * time.Hour
}

// ResumeMC reports whether previously-running Minecraft servers should be
// restarted on panel startup (default true).
func (c *Config) ResumeMC() bool {
	return c.Minecraft.Resume == nil || *c.Minecraft.Resume
}
