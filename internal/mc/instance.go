package mc

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// resolvedCfg is the effective per-server configuration after layering
// panel-managed overrides on top of the config file.
type resolvedCfg struct {
	Java        string
	Jar         string // empty = use run.sh
	RunScript   string
	Mem         string
	JVMArgs     []string
	Aikar       bool
	AutoStart   bool
	AutoRestart bool

	// RunAsUser (with resolved uid/gid) de-privileges the server process.
	RunAsUser string
	RunAsUID  int
	RunAsGID  int
}

// aikarFlags are the widely used GC tuning flags for Paper-family servers.
var aikarFlags = []string{
	"-XX:+UseG1GC", "-XX:+ParallelRefProcEnabled", "-XX:MaxGCPauseMillis=200",
	"-XX:+UnlockExperimentalVMOptions", "-XX:+DisableExplicitGC",
	"-XX:+AlwaysPreTouch", "-XX:G1NewSizePercent=30", "-XX:G1MaxNewSizePercent=40",
	"-XX:G1HeapRegionSize=8M", "-XX:G1ReservePercent=20", "-XX:G1HeapWastePercent=5",
	"-XX:G1MixedGCCountTarget=4", "-XX:InitiatingHeapOccupancyPercent=15",
	"-XX:G1MixedGCLiveThresholdPercent=90", "-XX:G1RSetUpdatingPauseTimePercent=5",
	"-XX:SurvivorRatio=32", "-XX:+PerfDisableSharedMem", "-XX:MaxTenuringThreshold=1",
}

type instance struct {
	id  string
	dir string

	mu            sync.Mutex
	cfg           resolvedCfg
	state         State
	proc          *exec.Cmd
	stdin         *os.File
	pid           int
	startedAt     time.Time
	stopRequested bool
	lastExit      string
	restartTimes  []time.Time
	version       string
	software      string
	port          int
	maxPlayers    int
	rconEnabled   bool
	players       map[string]struct{}
	cpuPct        float64
	memBytes      uint64
	prevCPUTicks  uint64
	prevCPUAt     time.Time

	ring     *logRing
	onChange func() // set by manager; called (unlocked) after state changes
	exited   chan struct{}
}

func newInstance(id, dir string, cfg resolvedCfg) *instance {
	return &instance{
		id:      id,
		dir:     dir,
		cfg:     cfg,
		state:   StateStopped,
		players: map[string]struct{}{},
		ring:    newLogRing(2000),
	}
}

func (in *instance) Info() ServerInfo {
	in.mu.Lock()
	defer in.mu.Unlock()
	info := ServerInfo{
		ID:           in.id,
		Name:         in.id,
		Dir:          in.dir,
		State:        in.state,
		Version:      in.version,
		Software:     in.software,
		Port:         in.port,
		MaxPlayers:   in.maxPlayers,
		PID:          in.pid,
		CPUPct:       in.cpuPct,
		MemBytes:     in.memBytes,
		MemMax:       parseMem(in.cfg.Mem),
		Mem:          in.cfg.Mem,
		Java:         in.cfg.Java,
		Jar:          in.cfg.Jar,
		JVMArgs:      in.cfg.JVMArgs,
		Aikar:        in.cfg.Aikar,
		AutoStart:    in.cfg.AutoStart,
		AutoRestart:  in.cfg.AutoRestart,
		RconEnabled:  in.rconEnabled,
		LastExit:     in.lastExit,
		EulaAccepted: in.eulaAcceptedLocked(),
	}
	if !in.startedAt.IsZero() && (in.state == StateRunning || in.state == StateStarting || in.state == StateStopping) {
		info.StartedAt = in.startedAt.UnixMilli()
	}
	info.OnlinePlayers = make([]string, 0, len(in.players))
	for p := range in.players {
		info.OnlinePlayers = append(info.OnlinePlayers, p)
	}
	sortStrings(info.OnlinePlayers)
	return info
}

func (in *instance) eulaAcceptedLocked() bool {
	b, err := os.ReadFile(filepath.Join(in.dir, "eula.txt"))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(b)), "eula=true")
}

// refreshProps re-reads server.properties-derived fields.
func (in *instance) refreshProps() {
	b, err := os.ReadFile(filepath.Join(in.dir, "server.properties"))
	if err != nil {
		return
	}
	content := string(b)
	in.mu.Lock()
	defer in.mu.Unlock()
	if p, err := strconv.Atoi(PropValue(content, "server-port")); err == nil {
		in.port = p
	}
	if mp, err := strconv.Atoi(PropValue(content, "max-players")); err == nil {
		in.maxPlayers = mp
	}
	in.rconEnabled = PropValue(content, "enable-rcon") == "true"
}

// Start launches the server process. Errors are returned synchronously for
// preconditions; runtime failures surface through state + console.
func (in *instance) Start() error {
	in.refreshProps()
	in.mu.Lock()
	if in.state != StateStopped && in.state != StateCrashed {
		in.mu.Unlock()
		return fmt.Errorf("server %s is %s", in.id, in.state)
	}
	if !in.eulaAcceptedLocked() {
		in.mu.Unlock()
		return fmt.Errorf("EULA not accepted for %s — accept it in the server settings first", in.id)
	}
	argv, err := in.buildArgvLocked()
	if err != nil {
		in.mu.Unlock()
		return err
	}
	if in.port != 0 {
		if ln, err := net.Listen("tcp", fmt.Sprintf(":%d", in.port)); err != nil {
			in.mu.Unlock()
			return fmt.Errorf("port %d is already in use", in.port)
		} else {
			ln.Close()
		}
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = in.dir
	setProcAttrs(cmd)
	if in.cfg.RunAsUser != "" {
		// Third-party server code (mods, plugins) must not run with the
		// panel's privileges. Hand the directory over, then drop.
		if err := chownTree(in.dir, in.cfg.RunAsUID, in.cfg.RunAsGID); err != nil {
			in.mu.Unlock()
			return fmt.Errorf("chown %s to %s: %w", in.dir, in.cfg.RunAsUser, err)
		}
		if err := applyRunAs(cmd, in.cfg.RunAsUID, in.cfg.RunAsGID); err != nil {
			in.mu.Unlock()
			return fmt.Errorf("run_as %s: %w", in.cfg.RunAsUser, err)
		}
		// Some mods resolve user.home; point it inside the server dir
		// rather than at root's home.
		cmd.Env = append(os.Environ(), "HOME="+in.dir, "USER="+in.cfg.RunAsUser)
	}

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		in.mu.Unlock()
		return err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		stdinR.Close()
		stdinW.Close()
		in.mu.Unlock()
		return err
	}
	cmd.Stdin = stdinR
	cmd.Stdout = outW
	cmd.Stderr = outW

	if err := cmd.Start(); err != nil {
		stdinR.Close()
		stdinW.Close()
		outR.Close()
		outW.Close()
		in.mu.Unlock()
		return fmt.Errorf("start %s: %w", in.id, err)
	}
	stdinR.Close()
	outW.Close()

	in.proc = cmd
	in.stdin = stdinW
	in.pid = cmd.Process.Pid
	in.state = StateStarting
	in.startedAt = time.Now()
	in.stopRequested = false
	in.lastExit = ""
	in.players = map[string]struct{}{}
	in.exited = make(chan struct{})
	exited := in.exited
	in.mu.Unlock()

	in.appendPanelLine("Starting: " + strings.Join(argv, " "))
	in.notify()

	go in.pumpOutput(outR)
	go in.waitForExit(cmd, exited)
	return nil
}

func (in *instance) buildArgvLocked() ([]string, error) {
	if in.cfg.Jar != "" {
		argv := []string{in.cfg.Java}
		if in.cfg.Mem != "" {
			argv = append(argv, "-Xms"+in.cfg.Mem, "-Xmx"+in.cfg.Mem)
		}
		if in.cfg.Aikar {
			argv = append(argv, aikarFlags...)
		}
		argv = append(argv, in.cfg.JVMArgs...)
		argv = append(argv, "-jar", in.cfg.Jar, "nogui")
		return argv, nil
	}
	if in.cfg.RunScript != "" {
		return []string{"/bin/sh", in.cfg.RunScript, "nogui"}, nil
	}
	return nil, fmt.Errorf("no server jar or run.sh found in %s — set minecraft.servers.%s.jar", in.dir, in.id)
}

var (
	levelRe   = regexp.MustCompile(`\[[^\]]*/(INFO|WARN|ERROR|FATAL|DEBUG|TRACE)\]`)
	ansiRe    = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	versionRe = regexp.MustCompile(`Starting minecraft server version (\S+)`)
	paperRe   = regexp.MustCompile(`This server is running (\w+) version`)
	joinRe    = regexp.MustCompile(`\]: ([A-Za-z0-9_]{1,16}) joined the game`)
	leaveRe   = regexp.MustCompile(`\]: ([A-Za-z0-9_]{1,16}) left the game`)
)

func (in *instance) pumpOutput(r *os.File) {
	defer r.Close()
	buf := make([]byte, 0, 64*1024)
	chunk := make([]byte, 8*1024)
	for {
		n, err := r.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			for {
				nl := indexByte(buf, '\n')
				if nl < 0 {
					break
				}
				line := strings.TrimRight(string(buf[:nl]), "\r")
				buf = buf[nl+1:]
				in.handleLine(line)
			}
		}
		if err != nil {
			if len(buf) > 0 {
				in.handleLine(string(buf))
			}
			return
		}
	}
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

func (in *instance) handleLine(raw string) {
	text := ansiRe.ReplaceAllString(raw, "")
	if strings.TrimSpace(text) == "" {
		return
	}
	level := "INFO"
	if m := levelRe.FindStringSubmatch(text); m != nil {
		level = m[1]
	}
	in.ring.Append(LogLine{TS: time.Now().UnixMilli(), Level: level, Text: text})

	changed := false
	in.mu.Lock()
	if m := versionRe.FindStringSubmatch(text); m != nil {
		in.version = m[1]
		in.software = "Vanilla"
		changed = true
	}
	if m := paperRe.FindStringSubmatch(text); m != nil {
		in.software = m[1]
		changed = true
	}
	if in.state == StateStarting && strings.Contains(text, "Done (") {
		in.state = StateRunning
		changed = true
	}
	if in.state == StateRunning && strings.Contains(text, "Stopping server") {
		in.state = StateStopping
		changed = true
	}
	if m := joinRe.FindStringSubmatch(text); m != nil {
		in.players[m[1]] = struct{}{}
		changed = true
	}
	if m := leaveRe.FindStringSubmatch(text); m != nil {
		delete(in.players, m[1])
		changed = true
	}
	in.mu.Unlock()
	if changed {
		in.notify()
	}
}

func (in *instance) waitForExit(cmd *exec.Cmd, exited chan struct{}) {
	err := cmd.Wait()
	close(exited)

	in.mu.Lock()
	wasRequested := in.stopRequested
	prevState := in.state
	in.proc = nil
	if in.stdin != nil {
		in.stdin.Close()
		in.stdin = nil
	}
	in.pid = 0
	in.cpuPct = 0
	in.memBytes = 0
	in.players = map[string]struct{}{}

	crashed := !wasRequested && (err != nil || prevState == StateStarting || prevState == StateRunning)
	when := time.Now().Format("15:04:05")
	if crashed {
		in.state = StateCrashed
		detail := "unexpected exit"
		if err != nil {
			detail = err.Error()
		}
		in.lastExit = fmt.Sprintf("crashed at %s (%s)", when, detail)
	} else {
		in.state = StateStopped
		in.lastExit = "stopped cleanly at " + when
	}
	autoRestart := crashed && in.cfg.AutoRestart && in.recentRestartsLocked() < 3
	if autoRestart {
		in.restartTimes = append(in.restartTimes, time.Now())
	}
	in.mu.Unlock()

	in.appendPanelLine("Server process exited: " + in.lastExit)
	in.notify()

	if autoRestart {
		in.appendPanelLine("Auto-restart in 10s (crash recovery)")
		time.Sleep(10 * time.Second)
		if err := in.Start(); err != nil {
			in.appendPanelLine("Auto-restart failed: " + err.Error())
		}
	}
}

func (in *instance) recentRestartsLocked() int {
	cutoff := time.Now().Add(-10 * time.Minute)
	n := 0
	for _, t := range in.restartTimes {
		if t.After(cutoff) {
			n++
		}
	}
	return n
}

// Stop asks the server to shut down gracefully, escalating to SIGTERM and
// SIGKILL if it does not comply. Blocks until the process is gone.
func (in *instance) Stop() error {
	in.mu.Lock()
	if in.state != StateRunning && in.state != StateStarting {
		in.mu.Unlock()
		return fmt.Errorf("server %s is %s", in.id, in.state)
	}
	in.stopRequested = true
	in.state = StateStopping
	proc := in.proc
	exited := in.exited
	in.writeCommandLocked("stop")
	in.mu.Unlock()
	in.notify()

	select {
	case <-exited:
		return nil
	case <-time.After(90 * time.Second):
	}
	in.appendPanelLine("Graceful stop timed out, sending SIGTERM")
	if proc != nil && proc.Process != nil {
		_ = proc.Process.Signal(os.Interrupt)
	}
	select {
	case <-exited:
		return nil
	case <-time.After(15 * time.Second):
	}
	in.appendPanelLine("SIGTERM ignored, killing process")
	if proc != nil && proc.Process != nil {
		_ = proc.Process.Kill()
	}
	<-exited
	return nil
}

// Kill terminates the process immediately (data loss possible; the API layer
// requires typed confirmation for this).
func (in *instance) Kill() error {
	in.mu.Lock()
	proc := in.proc
	exited := in.exited
	if proc == nil {
		in.mu.Unlock()
		return fmt.Errorf("server %s is not running", in.id)
	}
	in.stopRequested = true
	in.mu.Unlock()
	_ = proc.Process.Kill()
	<-exited
	return nil
}

// Command writes one line to the server's stdin.
func (in *instance) Command(cmd string) error {
	if strings.ContainsAny(cmd, "\r\n") {
		return fmt.Errorf("command must be a single line")
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.state != StateRunning && in.state != StateStarting && in.state != StateStopping {
		return fmt.Errorf("server %s is not running", in.id)
	}
	return in.writeCommandLocked(cmd)
}

func (in *instance) writeCommandLocked(cmd string) error {
	if in.stdin == nil {
		return fmt.Errorf("stdin unavailable")
	}
	_, err := in.stdin.Write([]byte(cmd + "\n"))
	return err
}

// waitForLine blocks until a console line contains substr (or timeout).
func (in *instance) waitForLine(substr string, timeout time.Duration) bool {
	ch, cancel := in.ring.Subscribe()
	defer cancel()
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return false
			}
			if strings.Contains(line.Text, substr) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// sampleResources updates CPU% and RSS from /proc (no-op off Linux).
func (in *instance) sampleResources() {
	in.mu.Lock()
	pid := in.pid
	in.mu.Unlock()
	if pid == 0 {
		return
	}
	ticks, rss, ok := procStats(pid)
	if !ok {
		return
	}
	now := time.Now()
	in.mu.Lock()
	if !in.prevCPUAt.IsZero() && ticks >= in.prevCPUTicks {
		elapsed := now.Sub(in.prevCPUAt).Seconds()
		if elapsed > 0 {
			in.cpuPct = float64(ticks-in.prevCPUTicks) / clockTicksPerSec / elapsed * 100
		}
	}
	in.prevCPUTicks = ticks
	in.prevCPUAt = now
	in.memBytes = rss
	in.mu.Unlock()
}

// appendPanelLine adds a panel-originated line to the console (marked so the
// UI can distinguish it from real server output).
func (in *instance) appendPanelLine(text string) {
	in.ring.Append(LogLine{TS: time.Now().UnixMilli(), Level: "PANEL", Text: text})
}

func (in *instance) notify() {
	if in.onChange != nil {
		in.onChange()
	}
}

func parseMem(s string) uint64 {
	if s == "" {
		return 0
	}
	mult := uint64(1)
	switch s[len(s)-1] {
	case 'G', 'g':
		mult = 1 << 30
	case 'M', 'm':
		mult = 1 << 20
	case 'K', 'k':
		mult = 1 << 10
	default:
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	v, err := strconv.ParseUint(s[:len(s)-1], 10, 64)
	if err != nil {
		return 0
	}
	return v * mult
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
