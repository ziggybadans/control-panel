//go:build linux

package storage

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ziggybadans/control-panel/internal/config"
)

type linuxProvider struct {
	cfg config.Storage

	smartMu    sync.RWMutex
	smartCache map[string]smartResult // disk name -> last poll

	ovMu     sync.Mutex
	ovCache  Overview
	ovCached time.Time
}

type smartResult struct {
	smart Smart
	tempC float64
	model string
	serial string
}

func NewLinuxProvider(cfg config.Storage) Provider {
	p := &linuxProvider{cfg: cfg, smartCache: map[string]smartResult{}}
	if cfg.Smart {
		go p.smartLoop()
	}
	return p
}

// smartLoop refreshes SMART data every 10 minutes. smartctl wakes standby
// drives only with -n standby, which we pass to avoid spinning up idle disks.
func (p *linuxProvider) smartLoop() {
	for {
		p.refreshSmart()
		time.Sleep(10 * time.Minute)
	}
}

func (p *linuxProvider) refreshSmart() {
	if _, err := exec.LookPath("smartctl"); err != nil {
		return
	}
	for _, name := range listBlockDevices() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		res, err := querySmart(ctx, "/dev/"+name)
		cancel()
		if err != nil {
			continue
		}
		p.smartMu.Lock()
		p.smartCache[name] = res
		p.smartMu.Unlock()
	}
}

func (p *linuxProvider) Overview(ctx context.Context) (Overview, error) {
	p.ovMu.Lock()
	defer p.ovMu.Unlock()
	if time.Since(p.ovCached) < 5*time.Second {
		return p.ovCache, nil
	}
	ov := p.collect()
	p.ovCache = ov
	p.ovCached = time.Now()
	return ov, nil
}

func (p *linuxProvider) collect() Overview {
	var ov Overview
	mounts := readMounts()

	// Pools: explicit config list, else every fuse.mergerfs mount.
	poolMounts := p.cfg.Pools
	if len(poolMounts) == 0 {
		for _, m := range mounts {
			if m.fsType == "fuse.mergerfs" {
				poolMounts = append(poolMounts, m.mount)
			}
		}
	}
	poolSet := map[string]bool{}
	branchSet := map[string]bool{}
	for _, pm := range poolMounts {
		m, ok := findMount(mounts, pm)
		if !ok {
			continue
		}
		poolSet[m.mount] = true
		pool := Pool{
			Name:   filepath.Base(m.mount),
			Mount:  m.mount,
			FSType: m.fsType,
		}
		pool.Total, pool.Used = statfs(m.mount)
		// mergerfs encodes its branches in the mount source: "/a:/b:/c",
		// possibly with per-branch =RW/=RO suffixes or globs.
		for _, raw := range strings.Split(m.device, ":") {
			path := strings.SplitN(raw, "=", 2)[0]
			matches, _ := filepath.Glob(path)
			if len(matches) == 0 {
				matches = []string{path}
			}
			for _, bp := range matches {
				total, used := statfs(bp)
				if total == 0 {
					continue
				}
				branchSet[bp] = true
				ov := Branch{Path: bp, Total: total, Used: used}
				ov.Device = deviceForPath(mounts, bp)
				pool.Branches = append(pool.Branches, ov)
			}
		}
		sort.Slice(pool.Branches, func(i, j int) bool { return pool.Branches[i].Path < pool.Branches[j].Path })
		ov.Pools = append(ov.Pools, pool)
	}

	// Other real mounts (skip pool mounts, branches, and excluded paths).
	for _, m := range mounts {
		if m.fsType == "fuse.mergerfs" || poolSet[m.mount] || branchSet[m.mount] {
			continue
		}
		if excluded(m.mount, p.cfg.ExcludeMounts) {
			continue
		}
		total, used := statfs(m.mount)
		if total == 0 {
			continue
		}
		ov.Mounts = append(ov.Mounts, Mount{
			Mount: m.mount, Device: m.device, FSType: m.fsType,
			Total: total, Used: used,
		})
	}

	// Physical disks.
	p.smartMu.RLock()
	defer p.smartMu.RUnlock()
	for _, name := range listBlockDevices() {
		d := Disk{
			Name:       name,
			Device:     "/dev/" + name,
			Model:      readSys("/sys/block/" + name + "/device/model"),
			SizeBytes:  sysBlockSize(name),
			Rotational: readSys("/sys/block/"+name+"/queue/rotational") == "1",
		}
		if res, ok := p.smartCache[name]; ok {
			d.Smart = res.smart
			d.TempC = res.tempC
			if res.model != "" {
				d.Model = res.model
			}
			d.Serial = res.serial
		}
		ov.Disks = append(ov.Disks, d)
	}
	return ov
}

// --- snapraid ---------------------------------------------------------------

func (p *linuxProvider) Snapraid(ctx context.Context) (SnapraidInfo, error) {
	info := SnapraidInfo{ConfigPath: p.cfg.Snapraid.Config}
	if p.cfg.Snapraid.Config == "" {
		return info, nil
	}
	if _, err := exec.LookPath(p.snapraidBinary()); err == nil {
		info.Installed = true
	}
	f, err := os.Open(p.cfg.Snapraid.Config)
	if err != nil {
		return info, nil
	}
	defer f.Close()
	info.Configured = true
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(strings.TrimSpace(sc.Text()))
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		switch fields[0] {
		case "parity", "2-parity", "3-parity":
			info.Parity = append(info.Parity, fields[1])
		case "content":
			info.Content = append(info.Content, fields[1])
		case "data", "disk":
			if len(fields) >= 3 {
				info.DataDisks = append(info.DataDisks, fields[1]+" "+fields[2])
			}
		}
	}
	return info, nil
}

func (p *linuxProvider) snapraidBinary() string {
	if p.cfg.Snapraid.Binary != "" {
		return p.cfg.Snapraid.Binary
	}
	return "snapraid"
}

func (p *linuxProvider) SnapraidCmd(op string) ([]string, error) {
	if !ValidSnapraidOps[op] {
		return nil, fmt.Errorf("invalid snapraid operation %q", op)
	}
	if p.cfg.Snapraid.Config == "" {
		return nil, fmt.Errorf("snapraid is not configured (set storage.snapraid.config)")
	}
	bin, err := exec.LookPath(p.snapraidBinary())
	if err != nil {
		return nil, fmt.Errorf("snapraid binary not found: %w", err)
	}
	return []string{bin, "-c", p.cfg.Snapraid.Config, op}, nil
}

// --- helpers ----------------------------------------------------------------

type procMount struct {
	device, mount, fsType string
}

var pseudoFS = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true,
	"tmpfs": true, "cgroup": true, "cgroup2": true, "pstore": true,
	"securityfs": true, "debugfs": true, "tracefs": true, "configfs": true,
	"fusectl": true, "mqueue": true, "hugetlbfs": true, "bpf": true,
	"binfmt_misc": true, "autofs": true, "rpc_pipefs": true, "nsfs": true,
	"overlay": true, "squashfs": true, "ramfs": true, "efivarfs": true,
	"fuse.gvfsd-fuse": true, "fuse.portal": true,
}

func readMounts() []procMount {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []procMount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || pseudoFS[fields[2]] {
			continue
		}
		mnt := strings.ReplaceAll(fields[1], `\040`, " ")
		out = append(out, procMount{device: fields[0], mount: mnt, fsType: fields[2]})
	}
	return out
}

func findMount(mounts []procMount, path string) (procMount, bool) {
	for _, m := range mounts {
		if m.mount == path {
			return m, true
		}
	}
	return procMount{}, false
}

// deviceForPath returns the disk name (e.g. "sda") whose mount is the longest
// prefix of path.
func deviceForPath(mounts []procMount, path string) string {
	best := ""
	dev := ""
	for _, m := range mounts {
		if strings.HasPrefix(path, m.mount) && len(m.mount) > len(best) {
			best = m.mount
			dev = m.device
		}
	}
	dev = strings.TrimPrefix(dev, "/dev/")
	// Strip partition suffix: sda1 -> sda, nvme0n1p2 -> nvme0n1.
	if i := strings.Index(dev, "p"); i > 0 && strings.HasPrefix(dev, "nvme") {
		if _, err := strconv.Atoi(dev[i+1:]); err == nil {
			dev = dev[:i]
		}
	} else {
		dev = strings.TrimRightFunc(dev, func(r rune) bool { return r >= '0' && r <= '9' })
	}
	return dev
}

func excluded(mount string, patterns []string) bool {
	for _, p := range patterns {
		if mount == p || strings.HasPrefix(mount, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

func statfs(path string) (total, used uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0
	}
	bs := uint64(st.Bsize)
	total = st.Blocks * bs
	free := st.Bavail * bs
	if total >= free {
		used = total - free
	}
	return total, used
}

func listBlockDevices() []string {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
			strings.HasPrefix(name, "dm-") || strings.HasPrefix(name, "zram") ||
			strings.HasPrefix(name, "sr") || strings.HasPrefix(name, "md") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func readSys(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func sysBlockSize(name string) uint64 {
	v, err := strconv.ParseUint(readSys("/sys/block/"+name+"/size"), 10, 64)
	if err != nil {
		return 0
	}
	return v * 512
}
