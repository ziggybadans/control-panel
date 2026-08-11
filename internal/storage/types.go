// Package storage reports pools (mergerfs), physical disks with SMART data,
// and snapraid integration.
package storage

import "context"

type Overview struct {
	Pools  []Pool  `json:"pools"`
	Disks  []Disk  `json:"disks"`
	Mounts []Mount `json:"mounts"` // real mounts outside pools (root, boot…)
}

type Pool struct {
	Name     string   `json:"name"`
	Mount    string   `json:"mount"`
	FSType   string   `json:"fsType"`
	Total    uint64   `json:"total"`
	Used     uint64   `json:"used"`
	Branches []Branch `json:"branches"`
}

type Branch struct {
	Path   string `json:"path"`
	Device string `json:"device"` // underlying disk name, e.g. "sda"
	Total  uint64 `json:"total"`
	Used   uint64 `json:"used"`
}

type Mount struct {
	Mount  string `json:"mount"`
	Device string `json:"device"`
	FSType string `json:"fsType"`
	Total  uint64 `json:"total"`
	Used   uint64 `json:"used"`
}

type Disk struct {
	Name       string  `json:"name"`   // e.g. "sda"
	Device     string  `json:"device"` // e.g. "/dev/sda"
	Model      string  `json:"model"`
	Serial     string  `json:"serial,omitempty"`
	SizeBytes  uint64  `json:"sizeBytes"`
	Rotational bool    `json:"rotational"`
	TempC      float64 `json:"tempC,omitempty"`
	Smart      Smart   `json:"smart"`
}

type Smart struct {
	Available      bool  `json:"available"`
	Healthy        *bool `json:"healthy,omitempty"` // nil = unknown
	PowerOnHours   int   `json:"powerOnHours,omitempty"`
	Reallocated    int   `json:"reallocated,omitempty"`    // ATA attr 5
	PendingSectors int   `json:"pendingSectors,omitempty"` // ATA attr 197
	CRCErrors      int   `json:"crcErrors,omitempty"`      // ATA attr 199
	PercentUsed    int   `json:"percentUsed,omitempty"`    // NVMe wear
	MediaErrors    int   `json:"mediaErrors,omitempty"`    // NVMe
}

type SnapraidInfo struct {
	Configured bool     `json:"configured"`
	Installed  bool     `json:"installed"`
	ConfigPath string   `json:"configPath,omitempty"`
	Parity     []string `json:"parity,omitempty"`  // parity file paths
	Content    []string `json:"content,omitempty"` // content file paths
	DataDisks  []string `json:"dataDisks,omitempty"`
}

// Provider supplies storage data. Long snapraid operations are not run here —
// SnapraidCmd builds a validated argv that the jobs runner executes.
type Provider interface {
	Overview(ctx context.Context) (Overview, error)
	Snapraid(ctx context.Context) (SnapraidInfo, error)
	// SnapraidCmd returns the argv for op ("status", "sync", "scrub", "diff").
	SnapraidCmd(op string) ([]string, error)
}

// ValidSnapraidOps is the complete set of snapraid operations the panel can
// run. Anything else is rejected.
var ValidSnapraidOps = map[string]bool{
	"status": true, "sync": true, "scrub": true, "diff": true,
}
