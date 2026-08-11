//go:build !linux

package mc

import (
	"os/exec"
	"syscall"
)

const clockTicksPerSec = 100.0

// procStats is only implemented on Linux; the mock service is used for UI
// development on other platforms.
func procStats(pid int) (cpuTicks uint64, rssBytes uint64, ok bool) {
	return 0, 0, false
}

func setProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
