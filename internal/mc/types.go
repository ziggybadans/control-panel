// Package mc manages Minecraft server instances: discovery, process
// supervision, console streaming, player management, and backups.
package mc

import (
	"context"
	"fmt"
	"regexp"

	"github.com/ziggybadans/control-panel/internal/jobs"
)

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateCrashed  State = "crashed"
)

type ServerInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	State    State  `json:"state"`
	Version  string `json:"version,omitempty"`  // e.g. "1.21.4"
	Software string `json:"software,omitempty"` // e.g. "Paper", "Forge", "Vanilla"
	Port     int    `json:"port,omitempty"`

	OnlinePlayers []string `json:"onlinePlayers"`
	MaxPlayers    int      `json:"maxPlayers,omitempty"`

	StartedAt int64   `json:"startedAt,omitempty"` // unix ms
	PID       int     `json:"pid,omitempty"`
	CPUPct    float64 `json:"cpuPct"`
	MemBytes  uint64  `json:"memBytes"`
	MemMax    uint64  `json:"memMax,omitempty"` // from -Xmx

	Mem         string   `json:"mem,omitempty"` // e.g. "6G"
	Java        string   `json:"java,omitempty"`
	Jar         string   `json:"jar,omitempty"`
	JVMArgs     []string `json:"jvmArgs,omitempty"`
	Aikar       bool     `json:"aikar"`
	AutoStart   bool     `json:"autoStart"`
	AutoRestart bool     `json:"autoRestart"`

	EulaAccepted bool   `json:"eulaAccepted"`
	RconEnabled  bool   `json:"rconEnabled"`
	LastExit     string `json:"lastExit,omitempty"`
}

type LogLine struct {
	TS    int64  `json:"ts"` // unix ms
	Level string `json:"level"`
	Text  string `json:"text"`
}

type PropEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type BackupInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt int64  `json:"createdAt"` // unix ms
}

type NamedPlayer struct {
	Name   string `json:"name"`
	UUID   string `json:"uuid,omitempty"`
	Level  int    `json:"level,omitempty"`  // op permission level
	Reason string `json:"reason,omitempty"` // ban reason
}

type PlayerInfo struct {
	Online           []string      `json:"online"`
	MaxPlayers       int           `json:"maxPlayers"`
	WhitelistEnabled bool          `json:"whitelistEnabled"`
	Whitelist        []NamedPlayer `json:"whitelist"`
	Ops              []NamedPlayer `json:"ops"`
	Banned           []NamedPlayer `json:"banned"`
}

// ConfigPatch is the set of per-server settings adjustable from the UI.
// These are stored by the panel (data dir), layered over config-file values.
type ConfigPatch struct {
	Mem         *string `json:"mem,omitempty"`
	Aikar       *bool   `json:"aikar,omitempty"`
	AutoStart   *bool   `json:"autoStart,omitempty"`
	AutoRestart *bool   `json:"autoRestart,omitempty"`
	JVMArgs     *string `json:"jvmArgs,omitempty"` // space-separated
	Jar         *string `json:"jar,omitempty"`     // set by jar updates
}

// CreateSpec describes a new server for the setup flow.
type CreateSpec struct {
	ID         string `json:"id"`
	Flavor     string `json:"flavor"` // paper | purpur | vanilla | fabric
	Version    string `json:"version"`
	Mem        string `json:"mem"`
	Port       int    `json:"port"`
	MOTD       string `json:"motd"`
	AcceptEULA bool   `json:"acceptEula"`
}

var ValidFlavors = map[string]bool{
	"paper": true, "purpur": true, "vanilla": true, "fabric": true,
}

func (s *CreateSpec) Validate() error {
	if err := CheckID(s.ID); err != nil {
		return err
	}
	if !ValidFlavors[s.Flavor] {
		return fmt.Errorf("invalid flavor %q (paper, purpur, vanilla, fabric)", s.Flavor)
	}
	if s.Version == "" {
		return fmt.Errorf("version is required")
	}
	if s.Port < 1024 || s.Port > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535")
	}
	if s.Mem != "" && parseMem(s.Mem) == 0 {
		return fmt.Errorf("invalid memory value %q (use e.g. 4G)", s.Mem)
	}
	return nil
}

// PlayerActions maps UI actions to console commands. Player names are
// validated against playerNameRe before any command is built.
var PlayerActions = map[string]string{
	"kick":             "kick",
	"ban":              "ban",
	"pardon":           "pardon",
	"op":               "op",
	"deop":             "deop",
	"whitelist-add":    "whitelist add",
	"whitelist-remove": "whitelist remove",
}

var (
	idRe         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	playerNameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)
	backupNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+\.tar\.gz$`)
)

// CheckID validates a server id (also used as a path segment).
func CheckID(id string) error {
	if !idRe.MatchString(id) {
		return fmt.Errorf("invalid server id %q", id)
	}
	return nil
}

func CheckPlayerName(name string) error {
	if !playerNameRe.MatchString(name) {
		return fmt.Errorf("invalid player name %q", name)
	}
	return nil
}

func CheckBackupName(name string) error {
	if !backupNameRe.MatchString(name) {
		return fmt.Errorf("invalid backup name %q", name)
	}
	return nil
}

// Service is the Minecraft management API consumed by the HTTP layer.
// Implemented by Manager (real servers) and mockService (UI development).
type Service interface {
	List() []ServerInfo
	Get(id string) (ServerInfo, bool)

	Start(id string) error
	Stop(id string) error
	Restart(id string) error
	Kill(id string) error

	Command(id, cmd string) error
	Console(id string, tail int) ([]LogLine, error)
	Subscribe(id string) (<-chan LogLine, func(), error)

	Properties(id string) ([]PropEntry, error)
	SetProperties(id string, changes map[string]string) error
	AcceptEULA(id string) error

	Players(id string) (PlayerInfo, error)
	PlayerAction(id, player, action string) error

	Backups(id string) ([]BackupInfo, error)
	CreateBackup(id string) (*jobs.View, error)
	RestoreBackup(id, name string) error
	DeleteBackup(id, name string) error

	UpdateConfig(id string, patch ConfigPatch) error
	Rescan() error

	// Dir exposes the server's directory for file management (the HTTP
	// layer runs mcfiles operations against it).
	Dir(id string) (string, error)

	// CreateServer provisions a new server directory and downloads the
	// selected server jar (runs as a job).
	CreateServer(spec CreateSpec) (*jobs.View, error)
	// UpdateJar downloads a new server jar into an existing server and
	// switches the launch configuration to it (runs as a job).
	UpdateJar(id, flavor, version string) (*jobs.View, error)
	// Versions lists available game versions for a flavor (newest first).
	Versions(ctx context.Context, flavor string) ([]string, error)
}
