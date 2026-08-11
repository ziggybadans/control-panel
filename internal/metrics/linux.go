//go:build linux

package metrics

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// linuxCollector reads /proc and /sys directly — no cgo, no dependencies.
type linuxCollector struct {
	version string

	prevTS    time.Time
	prevCPU   []cpuTicks // index 0 = aggregate
	prevNet   map[string]netCounter
	prevDisk  map[string]diskCounter
	havePrev  bool
	coreCount int
}

func NewLinuxCollector(version string) Collector {
	return &linuxCollector{
		version:  version,
		prevNet:  map[string]netCounter{},
		prevDisk: map[string]diskCounter{},
	}
}

type cpuTicks struct{ busy, total uint64 }

type netCounter struct{ rx, tx uint64 }

type diskCounter struct {
	readSectors, writeSectors uint64
	ioTicksMS                 uint64
}

func (l *linuxCollector) Info(ctx context.Context) (SystemInfo, error) {
	info := SystemInfo{Arch: goArch(), Version: l.version}
	info.Hostname, _ = os.Hostname()
	info.OS = osRelease()
	info.Kernel = readTrim("/proc/sys/kernel/osrelease")
	info.CPUModel, info.CPUCores = cpuInfo()
	if mi := readMeminfo(); mi != nil {
		info.MemTotal = mi["MemTotal"] * 1024
	}
	info.BootTime = bootTimeMS()
	return info, nil
}

func (l *linuxCollector) Sample(ctx context.Context) (Snapshot, error) {
	now := time.Now()
	snap := Snapshot{TS: now.UnixMilli()}

	// Load average.
	if fields := strings.Fields(readTrim("/proc/loadavg")); len(fields) >= 3 {
		for i := 0; i < 3; i++ {
			snap.Load[i], _ = strconv.ParseFloat(fields[i], 64)
		}
	}

	// Memory.
	if mi := readMeminfo(); mi != nil {
		total := mi["MemTotal"] * 1024
		avail := mi["MemAvailable"] * 1024
		snap.MemTotal = total
		if avail < total {
			snap.MemUsed = total - avail
		}
		snap.MemCached = (mi["Cached"] + mi["Buffers"]) * 1024
		snap.SwapTotal = mi["SwapTotal"] * 1024
		if mi["SwapFree"] < mi["SwapTotal"] {
			snap.SwapUsed = (mi["SwapTotal"] - mi["SwapFree"]) * 1024
		}
	}

	// CPU.
	cpus := readCPUTicks()
	elapsed := now.Sub(l.prevTS).Seconds()
	if l.havePrev && len(cpus) == len(l.prevCPU) && len(cpus) > 0 {
		snap.CPU = cpuPercent(l.prevCPU[0], cpus[0])
		for i := 1; i < len(cpus); i++ {
			snap.PerCore = append(snap.PerCore, cpuPercent(l.prevCPU[i], cpus[i]))
		}
	} else if len(cpus) > 1 {
		snap.PerCore = make([]float64, len(cpus)-1)
	}

	// Network.
	nets := readNetCounters()
	for _, name := range sortedKeys(nets) {
		c := nets[name]
		rate := NetRate{Name: name, RxTotal: c.rx, TxTotal: c.tx}
		if prev, ok := l.prevNet[name]; ok && l.havePrev && elapsed > 0 {
			rate.RxBps = deltaRate(c.rx, prev.rx, elapsed)
			rate.TxBps = deltaRate(c.tx, prev.tx, elapsed)
		}
		snap.Net = append(snap.Net, rate)
	}

	// Disk I/O.
	disks := readDiskCounters()
	for _, name := range sortedKeys(disks) {
		c := disks[name]
		rate := DiskRate{Name: name}
		if prev, ok := l.prevDisk[name]; ok && l.havePrev && elapsed > 0 {
			rate.ReadBps = deltaRate(c.readSectors*512, prev.readSectors*512, elapsed)
			rate.WriteBps = deltaRate(c.writeSectors*512, prev.writeSectors*512, elapsed)
			util := deltaRate(c.ioTicksMS, prev.ioTicksMS, elapsed) / 10 // ms/s -> %
			if util > 100 {
				util = 100
			}
			rate.UtilPct = util
		}
		snap.Disk = append(snap.Disk, rate)
	}

	snap.Temps = readTemps()

	l.prevTS = now
	l.prevCPU = cpus
	l.prevNet = nets
	l.prevDisk = disks
	l.havePrev = true
	return snap, nil
}

// --- /proc parsing helpers --------------------------------------------------

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readMeminfo() map[string]uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]uint64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err == nil {
			out[key] = v // kB
		}
	}
	return out
}

func readCPUTicks() []cpuTicks {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []cpuTicks
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu") {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		var vals []uint64
		for _, f := range fields[1:] {
			v, _ := strconv.ParseUint(f, 10, 64)
			vals = append(vals, v)
		}
		var total uint64
		for _, v := range vals {
			total += v
		}
		idle := vals[3]
		if len(vals) > 4 {
			idle += vals[4] // iowait counts as not-busy
		}
		out = append(out, cpuTicks{busy: total - idle, total: total})
	}
	return out
}

func cpuPercent(prev, cur cpuTicks) float64 {
	dTotal := float64(cur.total - prev.total)
	if dTotal <= 0 {
		return 0
	}
	pct := float64(cur.busy-prev.busy) / dTotal * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func readNetCounters() map[string]netCounter {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]netCounter{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 10 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		out[name] = netCounter{rx: rx, tx: tx}
	}
	return out
}

var physicalDisk = regexp.MustCompile(`^(sd[a-z]+|nvme\d+n\d+|vd[a-z]+|mmcblk\d+)$`)

func readDiskCounters() map[string]diskCounter {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]diskCounter{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if !physicalDisk.MatchString(name) {
			continue
		}
		rs, _ := strconv.ParseUint(fields[5], 10, 64)
		ws, _ := strconv.ParseUint(fields[9], 10, 64)
		it, _ := strconv.ParseUint(fields[12], 10, 64)
		out[name] = diskCounter{readSectors: rs, writeSectors: ws, ioTicksMS: it}
	}
	return out
}

// readTemps walks /sys/class/hwmon for temperature sensors.
func readTemps() []Temp {
	var out []Temp
	hwmons, err := filepath.Glob("/sys/class/hwmon/hwmon*")
	if err != nil {
		return nil
	}
	for _, dir := range hwmons {
		chip := readTrim(filepath.Join(dir, "name"))
		inputs, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
		for _, in := range inputs {
			raw := readTrim(in)
			milli, err := strconv.ParseFloat(raw, 64)
			if err != nil || milli <= 0 {
				continue
			}
			label := readTrim(strings.TrimSuffix(in, "_input") + "_label")
			name := chip
			switch {
			case label != "" && chip != "":
				name = chip + " " + label
			case label != "":
				name = label
			}
			out = append(out, Temp{Label: name, C: milli / 1000})
		}
	}
	return out
}

func osRelease() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return "Linux"
}

func cpuInfo() (model string, cores int) {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "", 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "model name") {
			if model == "" {
				if _, v, ok := strings.Cut(line, ":"); ok {
					model = strings.TrimSpace(v)
				}
			}
		}
		if strings.HasPrefix(line, "processor") {
			cores++
		}
	}
	return model, cores
}

func bootTimeMS() int64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "btime "); ok {
			sec, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			return sec * 1000
		}
	}
	return 0
}
