//go:build linux

package main

import (
	"fmt"
	"os"
	"os/user"
	"strconv"

	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/fans"
	"github.com/ziggybadans/control-panel/internal/metrics"
	"github.com/ziggybadans/control-panel/internal/services"
	"github.com/ziggybadans/control-panel/internal/storage"
	"github.com/ziggybadans/control-panel/internal/term"
)

func newLinuxCollector(version string) metrics.Collector {
	return metrics.NewLinuxCollector(version)
}

func newLinuxStorage(cfg config.Storage) storage.Provider {
	return storage.NewLinuxProvider(cfg)
}

func newSystemdProvider(units []string) services.Provider {
	return services.NewSystemdProvider(units)
}

func newLinuxFans() fans.Provider {
	return fans.NewHwmonProvider()
}

// newTermLauncher resolves terminal config into a concrete PTY launcher,
// enforcing the run_as safety rules. Only called when terminal.enabled.
func newTermLauncher(cfg config.Config) (term.Launcher, error) {
	t := cfg.Terminal
	shell := t.Shell
	if shell == "" {
		shell = "/bin/bash"
	}
	spec := term.LaunchSpec{
		Shell: shell,
		Args:  []string{"-l"},
		Env: []string{
			"TERM=xterm-256color",
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"LANG=C.UTF-8",
			"SHELL=" + shell,
		},
	}

	if t.RunAs != "" {
		u, err := user.Lookup(t.RunAs)
		if err != nil {
			return nil, fmt.Errorf("terminal.run_as: unknown user %q: %w", t.RunAs, err)
		}
		uid, err1 := strconv.Atoi(u.Uid)
		gid, err2 := strconv.Atoi(u.Gid)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("terminal.run_as: non-numeric uid/gid for %q", t.RunAs)
		}
		if uid == 0 {
			return nil, fmt.Errorf("terminal.run_as must name an unprivileged user, not root")
		}
		if os.Geteuid() != 0 {
			return nil, fmt.Errorf("terminal.run_as requires the panel itself to run as root")
		}
		spec.UID, spec.GID = uid, gid
		spec.Username = u.Username
		spec.Dir = u.HomeDir
		spec.Env = append(spec.Env, "HOME="+u.HomeDir, "USER="+u.Username, "LOGNAME="+u.Username)
		return term.NewPTYLauncher(spec), nil
	}

	// No run_as: the shell runs as the panel's own user. Refuse to hand out
	// root shells unless that risk was explicitly accepted.
	if os.Geteuid() == 0 && !t.AllowRoot {
		return nil, fmt.Errorf("terminal.enabled with a root panel requires terminal.run_as " +
			"(recommended — e.g. useradd --system --create-home --shell /bin/bash panel-shell), " +
			"or terminal.allow_root: true to accept root shells")
	}
	u, err := user.Current()
	if err == nil {
		spec.Username = u.Username
		spec.Dir = u.HomeDir
		spec.Env = append(spec.Env, "HOME="+u.HomeDir, "USER="+u.Username, "LOGNAME="+u.Username)
	}
	return term.NewPTYLauncher(spec), nil
}
