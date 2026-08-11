//go:build linux

package services

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type systemdProvider struct {
	units    []string
	bootTime int64 // unix ms, for converting monotonic timestamps
}

func NewSystemdProvider(units []string) Provider {
	if len(units) == 0 {
		units = DefaultUnits
	}
	return &systemdProvider{units: units, bootTime: readBootTimeMS()}
}

var showProps = []string{
	"Id", "Description", "LoadState", "ActiveState", "SubState",
	"ActiveEnterTimestampMonotonic", "InactiveEnterTimestampMonotonic",
	"MainPID", "MemoryCurrent", "UnitFileState",
}

func (s *systemdProvider) List(ctx context.Context) ([]Service, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := []string{"show", "--property=" + strings.Join(showProps, ",")}
	args = append(args, "--")
	args = append(args, s.units...)
	out, err := exec.CommandContext(ctx, "systemctl", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl show: %w", err)
	}

	var services []Service
	// Blocks are separated by blank lines, in the order units were passed.
	blocks := strings.Split(strings.TrimSpace(string(out)), "\n\n")
	for i, block := range blocks {
		props := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			if k, v, ok := strings.Cut(line, "="); ok {
				props[k] = v
			}
		}
		svc := Service{
			Unit:        props["Id"],
			Description: props["Description"],
			LoadState:   props["LoadState"],
			ActiveState: props["ActiveState"],
			SubState:    props["SubState"],
			Enabled:     props["UnitFileState"],
		}
		// systemctl show omits Id for unloadable units on some versions.
		if svc.Unit == "" && i < len(s.units) {
			svc.Unit = s.units[i]
		}
		if pid, err := strconv.Atoi(props["MainPID"]); err == nil && pid > 0 {
			svc.PID = pid
		}
		if mem, err := strconv.ParseUint(props["MemoryCurrent"], 10, 64); err == nil && mem < 1<<62 {
			svc.MemBytes = mem
		}
		// Prefer the enter-timestamp of the current state.
		key := "ActiveEnterTimestampMonotonic"
		if svc.ActiveState != "active" && svc.ActiveState != "activating" {
			key = "InactiveEnterTimestampMonotonic"
		}
		if usec, err := strconv.ParseInt(props[key], 10, 64); err == nil && usec > 0 && s.bootTime > 0 {
			svc.Since = s.bootTime + usec/1000
		}
		services = append(services, svc)
	}
	return services, nil
}

func (s *systemdProvider) Action(ctx context.Context, unit, verb string) error {
	if err := CheckAction(s.units, unit, verb); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "--no-ask-password", verb, "--", unit).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("systemctl %s %s: %s", verb, unit, msg)
	}
	return nil
}

func (s *systemdProvider) Logs(ctx context.Context, unit string, lines int) ([]string, error) {
	if err := CheckUnit(s.units, unit); err != nil {
		return nil, err
	}
	if lines < 10 {
		lines = 10
	}
	if lines > 1000 {
		lines = 1000
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "journalctl",
		"-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "--output=short-iso").Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl: %w", err)
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), nil
}

func readBootTimeMS() int64 {
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
