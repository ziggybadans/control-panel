//go:build linux

package fans

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// hwmonProvider drives fans through /sys/class/hwmon. Chips are scanned once
// at startup; readings go straight to sysfs on every call.
type hwmonProvider struct {
	mu    sync.Mutex
	fans  map[string]*hwmonFan
	order []string
	temps []hwmonTemp
}

type hwmonFan struct {
	id, label  string
	pwmPath    string
	enablePath string // may be "" (chip without pwmN_enable)
	tachPath   string // may be ""

	// Pre-takeover state, restored on Release.
	taken      bool
	origPWM    string
	origEnable string
}

type hwmonTemp struct {
	id, label, path string
}

var pwmRe = regexp.MustCompile(`^pwm([0-9]+)$`)
var tempRe = regexp.MustCompile(`^temp([0-9]+)_input$`)

func NewHwmonProvider() Provider {
	p := &hwmonProvider{fans: map[string]*hwmonFan{}}
	p.scan()
	return p
}

func (p *hwmonProvider) scan() {
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	// Sort numerically so chip disambiguation is as stable as hwmon
	// enumeration allows.
	sort.Slice(dirs, func(i, j int) bool { return hwmonNum(dirs[i]) < hwmonNum(dirs[j]) })

	seen := map[string]int{}
	for _, dir := range dirs {
		chip := readTrimmed(filepath.Join(dir, "name"))
		if chip == "" {
			chip = filepath.Base(dir)
		}
		// Two chips with the same name get #1, #2… suffixes.
		if n := seen[chip]; n > 0 {
			seen[chip] = n + 1
			chip = fmt.Sprintf("%s#%d", chip, n)
		} else {
			seen[chip] = 1
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if m := pwmRe.FindStringSubmatch(e.Name()); m != nil {
				idx := m[1]
				f := &hwmonFan{
					id:      chip + ":pwm" + idx,
					label:   chip + " pwm" + idx,
					pwmPath: filepath.Join(dir, e.Name()),
				}
				if lbl := readTrimmed(filepath.Join(dir, "fan"+idx+"_label")); lbl != "" {
					f.label = lbl
				}
				if pathExists(filepath.Join(dir, "pwm"+idx+"_enable")) {
					f.enablePath = filepath.Join(dir, "pwm"+idx+"_enable")
				}
				if pathExists(filepath.Join(dir, "fan"+idx+"_input")) {
					f.tachPath = filepath.Join(dir, "fan"+idx+"_input")
				}
				p.fans[f.id] = f
				p.order = append(p.order, f.id)
			}
			if m := tempRe.FindStringSubmatch(e.Name()); m != nil {
				idx := m[1]
				tempChip, chipLabel := chip, chip
				// drivetemp chips are one-per-disk with identical names;
				// key and label them by the disk instead ("sda · ST4000…").
				// Note: an ID built on the block name changes if the kernel
				// re-enumerates disks — a curve bound to it then fails safe
				// (fan to 100%) rather than following the wrong drive.
				if strings.HasPrefix(chip, "drivetemp") {
					if block := blockDevName(dir); block != "" {
						tempChip = "drivetemp-" + block
						chipLabel = block
						if model := readTrimmed(filepath.Join(dir, "device", "model")); model != "" {
							chipLabel = block + " · " + model
						}
					}
				}
				label := readTrimmed(filepath.Join(dir, "temp"+idx+"_label"))
				if label == "" {
					if tempChip != chip {
						label = chipLabel // disk sensors: the disk is the label
					} else {
						label = chipLabel + " temp" + idx
					}
				} else {
					label = chipLabel + " " + label
				}
				p.temps = append(p.temps, hwmonTemp{
					id:    tempChip + ":temp" + idx,
					label: label,
					path:  filepath.Join(dir, e.Name()),
				})
			}
		}
	}
}

func (p *hwmonProvider) Fans() []Fan {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Fan, 0, len(p.order))
	for _, id := range p.order {
		f := p.fans[id]
		out = append(out, Fan{
			ID:       f.id,
			Label:    f.label,
			HasRPM:   f.tachPath != "",
			Writable: isWritable(f.pwmPath),
		})
	}
	return out
}

// isWritable reports whether the kernel created the PWM attribute with a
// write bit. Drivers that gate control by vendor (nct6683) expose 0444
// files; even root cannot usefully write those.
func isWritable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().Perm()&0o200 != 0
}

func (p *hwmonProvider) Sensors() []Sensor {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Sensor, 0, len(p.temps))
	for _, t := range p.temps {
		raw := readTrimmed(t.path)
		if raw == "" {
			continue // unreadable this tick; curve failsafe covers users of it
		}
		milli, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		out = append(out, Sensor{ID: t.id, Label: t.label, C: milli / 1000})
	}
	return out
}

func (p *hwmonProvider) Read(id string) (int, float64, error) {
	p.mu.Lock()
	f, ok := p.fans[id]
	p.mu.Unlock()
	if !ok {
		return -1, 0, fmt.Errorf("unknown fan %q", id)
	}
	rpm := -1
	if f.tachPath != "" {
		if v, err := strconv.Atoi(readTrimmed(f.tachPath)); err == nil {
			rpm = v
		}
	}
	raw, err := strconv.ParseFloat(readTrimmed(f.pwmPath), 64)
	if err != nil {
		return rpm, 0, fmt.Errorf("read %s: %w", f.pwmPath, err)
	}
	return rpm, raw / 255 * 100, nil
}

func (p *hwmonProvider) SetDuty(id string, pct float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.fans[id]
	if !ok {
		return fmt.Errorf("unknown fan %q", id)
	}
	if !f.taken {
		// Remember what firmware had configured so Release can restore it.
		f.origPWM = readTrimmed(f.pwmPath)
		if f.enablePath != "" {
			f.origEnable = readTrimmed(f.enablePath)
			if err := writeSysfs(f.enablePath, "1"); err != nil {
				return err
			}
		}
		f.taken = true
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	v := int(pct/100*255 + 0.5)
	return writeSysfs(f.pwmPath, strconv.Itoa(v))
}

func (p *hwmonProvider) Release(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.fans[id]
	if !ok {
		return fmt.Errorf("unknown fan %q", id)
	}
	if !f.taken {
		return nil
	}
	if f.origPWM != "" {
		if err := writeSysfs(f.pwmPath, f.origPWM); err != nil {
			return err
		}
	}
	if f.enablePath != "" && f.origEnable != "" {
		if err := writeSysfs(f.enablePath, f.origEnable); err != nil {
			return err
		}
	}
	f.taken = false
	return nil
}

func writeSysfs(path, value string) error {
	err := os.WriteFile(path, []byte(value), 0o644)
	if errors.Is(err, syscall.EROFS) {
		return fmt.Errorf("write %s: %w — sysfs is mounted read-only; the systemd unit "+
			"must not set ProtectKernelTunables=true (see deploy/control-panel.service)", path, err)
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("write %s: %w — the kernel driver refused the write "+
			"(some drivers gate PWM control by board vendor)", path, err)
	}
	return err
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hwmonNum(dir string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(dir), "hwmon"))
	return n
}

// blockDevName returns the block device (e.g. "sda") behind a hwmon chip,
// or "" when there is none.
func blockDevName(hwmonDir string) string {
	entries, err := os.ReadDir(filepath.Join(hwmonDir, "device", "block"))
	if err != nil || len(entries) == 0 {
		return ""
	}
	return entries[0].Name()
}
