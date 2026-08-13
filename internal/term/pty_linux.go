//go:build linux

package term

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// LaunchSpec is a fully resolved terminal command (main resolves run_as,
// shell, and home before the manager ever starts).
type LaunchSpec struct {
	Shell string
	Args  []string
	Dir   string
	Env   []string
	// UID/GID de-privilege the shell (0/0 with Username "" = run as the
	// panel's own user).
	UID, GID int
	Username string // for Describe only
}

type ptyLauncher struct {
	spec LaunchSpec
}

// NewPTYLauncher returns a Launcher that runs spec on a fresh PTY.
func NewPTYLauncher(spec LaunchSpec) Launcher {
	return &ptyLauncher{spec: spec}
}

func (l *ptyLauncher) Describe() string {
	who := l.spec.Username
	if who == "" {
		who = "the panel user"
	}
	return fmt.Sprintf("%s as %s", l.spec.Shell, who)
}

func (l *ptyLauncher) Start(cols, rows int) (Proc, error) {
	// Open the PTY master and unlock its slave (pure Go; no cgo, no deps
	// beyond x/sys).
	ptm, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open ptmx: %w", err)
	}
	slaveNum, err := unix.IoctlGetInt(int(ptm.Fd()), unix.TIOCGPTN)
	if err != nil {
		ptm.Close()
		return nil, fmt.Errorf("TIOCGPTN: %w", err)
	}
	if err := unix.IoctlSetPointerInt(int(ptm.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		ptm.Close()
		return nil, fmt.Errorf("unlock pty: %w", err)
	}
	pts, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", slaveNum), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		ptm.Close()
		return nil, fmt.Errorf("open pts: %w", err)
	}
	_ = setWinsize(ptm, cols, rows)

	cmd := exec.Command(l.spec.Shell, l.spec.Args...)
	cmd.Dir = l.spec.Dir
	cmd.Env = l.spec.Env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pts, pts, pts
	// New session with the PTY slave (child fd 0) as controlling terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if l.spec.UID != 0 || l.spec.GID != 0 {
		if os.Geteuid() != 0 {
			pts.Close()
			ptm.Close()
			return nil, fmt.Errorf("terminal.run_as requires the panel to run as root")
		}
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid:    uint32(l.spec.UID),
			Gid:    uint32(l.spec.GID),
			Groups: []uint32{},
		}
	}
	if err := cmd.Start(); err != nil {
		pts.Close()
		ptm.Close()
		return nil, fmt.Errorf("start %s: %w", l.spec.Shell, err)
	}
	pts.Close() // child holds its own copy now
	return &ptyProc{ptm: ptm, cmd: cmd}, nil
}

type ptyProc struct {
	ptm  *os.File
	cmd  *exec.Cmd
	once sync.Once
}

func (p *ptyProc) Read(b []byte) (int, error)  { return p.ptm.Read(b) }
func (p *ptyProc) Write(b []byte) (int, error) { return p.ptm.Write(b) }

func (p *ptyProc) Resize(cols, rows int) error {
	return setWinsize(p.ptm, cols, rows)
}

// Close hangs the shell up (SIGHUP, escalating to SIGKILL) and reaps it.
func (p *ptyProc) Close() error {
	p.once.Do(func() {
		p.ptm.Close()
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(syscall.SIGHUP)
			done := make(chan struct{})
			go func() {
				_ = p.cmd.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = p.cmd.Process.Kill()
				<-done
			}
		}
	})
	return nil
}

func setWinsize(f *os.File, cols, rows int) error {
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &unix.Winsize{
		Col: uint16(cols),
		Row: uint16(rows),
	})
}
