package term

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// mockLauncher provides a simulated shell for mock mode: it echoes input,
// supports line editing, and answers a handful of canned commands. Nothing
// is ever executed.
type mockLauncher struct{}

func NewMockLauncher() Launcher { return &mockLauncher{} }

func (mockLauncher) Describe() string { return "simulated shell (mock mode)" }

func (mockLauncher) Start(cols, rows int) (Proc, error) {
	pr, pw := io.Pipe()
	p := &mockProc{pr: pr, pw: pw, start: time.Now()}
	go func() {
		p.print("control-panel mock shell — nothing here is executed.\r\n")
		p.print("Type \"help\" for the available commands.\r\n\r\n")
		p.prompt()
	}()
	return p, nil
}

type mockProc struct {
	pr    *io.PipeReader
	pw    *io.PipeWriter
	start time.Time

	mu     sync.Mutex
	line   []byte
	closed bool
}

func (p *mockProc) Read(b []byte) (int, error) { return p.pr.Read(b) }

func (p *mockProc) print(s string) {
	_, _ = p.pw.Write([]byte(s))
}

func (p *mockProc) prompt() {
	p.print("\x1b[38;5;110mmock@bastion\x1b[0m:\x1b[38;5;150m~\x1b[0m$ ")
}

func (p *mockProc) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	for _, c := range b {
		switch {
		case c == '\r' || c == '\n':
			cmd := strings.TrimSpace(string(p.line))
			p.line = nil
			p.print("\r\n")
			if p.run(cmd) {
				return len(b), nil // exit
			}
			p.prompt()
		case c == 0x7f || c == '\b':
			if len(p.line) > 0 {
				p.line = p.line[:len(p.line)-1]
				p.print("\b \b")
			}
		case c == 0x03: // ^C
			p.line = nil
			p.print("^C\r\n")
			p.prompt()
		case c == 0x0c: // ^L
			p.print("\x1b[2J\x1b[H")
			p.prompt()
			p.print(string(p.line))
		case c >= 0x20:
			p.line = append(p.line, c)
			p.print(string([]byte{c}))
		}
	}
	return len(b), nil
}

// run answers one command; returns true when the session should end.
func (p *mockProc) run(cmd string) bool {
	switch {
	case cmd == "":
	case cmd == "help":
		p.print("mock shell commands: help, ls, pwd, whoami, hostname, uname, uptime, clear, exit\r\n")
	case cmd == "ls":
		p.print("docker/  media/  minecraft/  notes.txt\r\n")
	case cmd == "pwd":
		p.print("/home/mock\r\n")
	case cmd == "whoami":
		p.print("mock\r\n")
	case cmd == "hostname":
		p.print("bastion\r\n")
	case strings.HasPrefix(cmd, "uname"):
		p.print("Linux bastion 6.1.0-28-amd64 x86_64 GNU/Linux\r\n")
	case cmd == "uptime":
		p.print(fmt.Sprintf(" up %s,  1 user,  load average: 0.42, 0.35, 0.31\r\n",
			time.Since(p.start).Round(time.Second)))
	case cmd == "clear":
		p.print("\x1b[2J\x1b[H")
	case cmd == "exit" || cmd == "logout":
		p.print("logout\r\n")
		p.closeLocked()
		return true
	default:
		p.print(fmt.Sprintf("%s: command not found (mock shell — try \"help\")\r\n",
			strings.Fields(cmd)[0]))
	}
	return false
}

func (p *mockProc) Resize(cols, rows int) error { return nil }

func (p *mockProc) closeLocked() {
	if !p.closed {
		p.closed = true
		p.pw.Close()
	}
}

func (p *mockProc) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
	p.pr.Close()
	return nil
}
