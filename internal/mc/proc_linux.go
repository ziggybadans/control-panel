//go:build linux

package mc

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const clockTicksPerSec = 100.0 // USER_HZ on all mainstream Linux builds

// procStats returns cumulative CPU ticks (utime+stime) and RSS bytes.
func procStats(pid int) (cpuTicks uint64, rssBytes uint64, ok bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, false
	}
	// comm (field 2) can contain spaces/parens; skip past the last ')'.
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, 0, false
	}
	fields := strings.Fields(s[i+1:])
	// fields[0] = state (field 3); utime = field 14 -> fields[11].
	if len(fields) < 22 {
		return 0, 0, false
	}
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	rssPages, _ := strconv.ParseUint(fields[21], 10, 64)
	return utime + stime, rssPages * uint64(os.Getpagesize()), true
}

// setProcAttrs puts the server in its own process group so terminal signals
// to the panel don't propagate to game servers.
func setProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
