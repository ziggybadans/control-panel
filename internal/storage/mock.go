package storage

import (
	"context"
	"fmt"
)

// mockProvider serves a realistic NAS layout for UI development.
type mockProvider struct{}

func NewMockProvider() Provider { return &mockProvider{} }

func b(v bool) *bool { return &v }

const tib = 1 << 40

// tb converts fractional tebibytes to bytes at runtime (Go rejects
// constant-expression conversions with fractional results).
func tb(v float64) uint64 { return uint64(v * float64(tib)) }

func (m *mockProvider) Overview(ctx context.Context) (Overview, error) {
	return Overview{
		Pools: []Pool{
			{
				Name:   "pool",
				Mount:  "/mnt/pool",
				FSType: "fuse.mergerfs",
				Total:  32 * tib,
				Used:   tb(21.4),
				Branches: []Branch{
					{Path: "/mnt/disk1", Device: "sda", Total: 12 * tib, Used: tb(9.1)},
					{Path: "/mnt/disk2", Device: "sdb", Total: 12 * tib, Used: tb(8.3)},
					{Path: "/mnt/disk3", Device: "sdc", Total: 8 * tib, Used: tb(4.0)},
				},
			},
		},
		Disks: []Disk{
			{
				Name: "nvme0n1", Device: "/dev/nvme0n1", Model: "Samsung SSD 970 EVO Plus 500GB",
				Serial: "S4EVNX0N123456", SizeBytes: 500 << 30, Rotational: false, TempC: 41,
				Smart: Smart{Available: true, Healthy: b(true), PowerOnHours: 21550, PercentUsed: 4},
			},
			{
				Name: "sda", Device: "/dev/sda", Model: "WDC WD120EFBX-68B0EN0",
				Serial: "9LHV5A2J", SizeBytes: 12 * tib, Rotational: true, TempC: 34,
				Smart: Smart{Available: true, Healthy: b(true), PowerOnHours: 18493},
			},
			{
				Name: "sdb", Device: "/dev/sdb", Model: "WDC WD120EFBX-68B0EN0",
				Serial: "9LHV8K1M", SizeBytes: 12 * tib, Rotational: true, TempC: 33,
				Smart: Smart{Available: true, Healthy: b(true), PowerOnHours: 18490},
			},
			{
				Name: "sdc", Device: "/dev/sdc", Model: "ST8000VN004-2M2101",
				Serial: "WSD0R7F9", SizeBytes: 8 * tib, Rotational: true, TempC: 36,
				Smart: Smart{Available: true, Healthy: b(true), PowerOnHours: 41202, Reallocated: 12, PendingSectors: 0, CRCErrors: 2},
			},
			{
				Name: "sdd", Device: "/dev/sdd", Model: "WDC WD140EDGZ-11B1PA0",
				Serial: "2CGT9U8N", SizeBytes: 14 * tib, Rotational: true, TempC: 31,
				Smart: Smart{Available: true, Healthy: b(true), PowerOnHours: 12077},
			},
		},
		Mounts: []Mount{
			{Mount: "/", Device: "/dev/nvme0n1p2", FSType: "ext4", Total: 468 << 30, Used: 87 << 30},
			{Mount: "/boot/efi", Device: "/dev/nvme0n1p1", FSType: "vfat", Total: 511 << 20, Used: 6 << 20},
			{Mount: "/mnt/parity1", Device: "/dev/sdd1", FSType: "ext4", Total: 14 * tib, Used: tb(9.4)},
		},
	}, nil
}

func (m *mockProvider) Snapraid(ctx context.Context) (SnapraidInfo, error) {
	return SnapraidInfo{
		Configured: true,
		Installed:  true,
		ConfigPath: "/etc/snapraid.conf",
		Parity:     []string{"/mnt/parity1/snapraid.parity"},
		Content:    []string{"/var/snapraid.content", "/mnt/disk1/snapraid.content", "/mnt/disk2/snapraid.content"},
		DataDisks:  []string{"d1 /mnt/disk1/", "d2 /mnt/disk2/", "d3 /mnt/disk3/"},
	}, nil
}

func (m *mockProvider) SnapraidCmd(op string) ([]string, error) {
	if !ValidSnapraidOps[op] {
		return nil, fmt.Errorf("invalid snapraid operation %q", op)
	}
	// The mock runs a harmless local command that produces plausible output.
	return []string{"sh", "-c", mockSnapraidScript(op)}, nil
}

func mockSnapraidScript(op string) string {
	switch op {
	case "sync":
		return `echo "Self test..."; sleep 1
echo "Loading state from /var/snapraid.content..."; sleep 1
echo "Scanning disk d1..."; sleep 1
echo "Scanning disk d2..."; echo "Scanning disk d3..."; sleep 1
echo "Using 245 MiB of memory for the file-system."
echo "Initializing..."
for i in 8 21 36 52 71 89 100; do echo "$i% completed, 1204 MB accessed"; sleep 1; done
echo "Everything OK"
echo "Saving state to /var/snapraid.content..."
sleep 1; echo "Verified /var/snapraid.content in 1 seconds"`
	case "scrub":
		return `echo "Self test..."; sleep 1
echo "Loading state from /var/snapraid.content..."; sleep 1
echo "Initializing..."
for i in 12 34 58 79 100; do echo "$i% completed, 840 MB accessed"; sleep 1; done
echo "Everything OK"`
	case "diff":
		return `echo "Loading state from /var/snapraid.content..."; sleep 1
echo "Comparing..."
echo "add media/movies/Dune.Part.Two.2024.mkv"
echo "add media/tv/Severance/S02E01.mkv"
echo ""
echo "   2 equal"
echo "   2 added"
echo "   0 removed"
echo "   0 updated"
echo "   0 moved"
echo "   0 copied"
echo "   0 restored"
echo "There are differences!"`
	default: // status
		return `echo "Self test..."; sleep 1
echo "Loading state from /var/snapraid.content..."; sleep 1
echo "SnapRAID status report:"
echo ""
echo "   Files Fragmented Excess  Wasted  Used    Free  Use Name"
echo "            Files  Fragments  GB      GB      GB"
echo "   84921       12        29    0.4    9318    2963  75% d1"
echo "   79102        8        17    0.2    8499    3782  69% d2"
echo "   40233        3         5    0.1    4096    4096  50% d3"
echo " --------------------------------------------------------------------------"
echo "  204256       23        51    0.7   21913   10841  66%"
echo ""
echo "The oldest block was scrubbed 8 days ago, the median 4, the newest 0."
echo "No sync is in progress."
echo "No error detected."`
	}
}
