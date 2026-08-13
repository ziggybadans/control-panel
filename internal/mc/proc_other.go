//go:build !linux

package mc

import (
	"fmt"
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

// Privilege dropping is Linux-only; other platforms only run the mock service.
func applyRunAs(cmd *exec.Cmd, uid, gid int) error {
	return fmt.Errorf("minecraft.run_as is only supported on Linux")
}

func chownTree(root string, uid, gid int) error { return nil }
