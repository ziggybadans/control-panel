package mc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/events"
	"github.com/ziggybadans/control-panel/internal/jobs"
)

// Manager supervises real Minecraft server processes discovered under the
// configured root directory.
type Manager struct {
	cfg     config.Minecraft
	dataDir string
	bus     *events.Bus
	runner  *jobs.Runner
	fetch   *fetcher

	mu        sync.Mutex
	instances map[string]*instance
	overrides map[string]ConfigPatch
}

func NewManager(cfg config.Minecraft, dataDir string, bus *events.Bus, runner *jobs.Runner) *Manager {
	m := &Manager{
		cfg:       cfg,
		dataDir:   dataDir,
		bus:       bus,
		runner:    runner,
		fetch:     newFetcher(),
		instances: map[string]*instance{},
		overrides: map[string]ConfigPatch{},
	}
	m.loadOverrides()
	if err := m.Rescan(); err != nil {
		slog.Warn("minecraft discovery failed", "err", err)
	}
	go m.resourceLoop()
	go m.pingLoop()
	return m
}

// Rescan discovers server directories, adding new ones and dropping stopped
// ones whose directory disappeared. Running instances are never dropped.
func (m *Manager) Rescan() error {
	entries, err := os.ReadDir(m.cfg.Root)
	if err != nil {
		return fmt.Errorf("read minecraft root %s: %w", m.cfg.Root, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	seen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		id := e.Name()
		if CheckID(id) != nil {
			slog.Warn("skipping minecraft dir with unusable name", "dir", id)
			continue
		}
		dir := filepath.Join(m.cfg.Root, id)
		if !looksLikeServer(dir) {
			continue
		}
		seen[id] = true
		if inst, ok := m.instances[id]; ok {
			inst.mu.Lock()
			inst.cfg = m.resolveCfgLocked(id, dir)
			inst.mu.Unlock()
			continue
		}
		inst := newInstance(id, dir, m.resolveCfgLocked(id, dir))
		inst.onChange = m.publish
		inst.refreshProps()
		m.instances[id] = inst
	}
	for id, inst := range m.instances {
		if !seen[id] {
			inst.mu.Lock()
			gone := inst.state == StateStopped || inst.state == StateCrashed
			inst.mu.Unlock()
			if gone {
				delete(m.instances, id)
			}
		}
	}
	go m.publish()
	return nil
}

func looksLikeServer(dir string) bool {
	for _, name := range []string{"server.properties", "server.jar", "run.sh", "eula.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	jars, _ := filepath.Glob(filepath.Join(dir, "*.jar"))
	return len(jars) > 0
}

// resolveCfgLocked layers panel overrides over the config file entry.
func (m *Manager) resolveCfgLocked(id, dir string) resolvedCfg {
	fileCfg := m.cfg.Servers[id]
	rc := resolvedCfg{
		Java:        firstNonEmpty(fileCfg.Java, m.cfg.Java, "java"),
		Mem:         firstNonEmpty(fileCfg.Mem, "4G"),
		JVMArgs:     fileCfg.JVMArgs,
		Aikar:       fileCfg.Aikar,
		AutoStart:   fileCfg.AutoStart,
		AutoRestart: fileCfg.AutoRestart,
		RunAsUser:   m.cfg.RunAs,
		RunAsUID:    m.cfg.RunAsUID,
		RunAsGID:    m.cfg.RunAsGID,
	}
	// Jar resolution: explicit config > server.jar > single *.jar > run.sh.
	if fileCfg.Jar != "" {
		rc.Jar = fileCfg.Jar
	} else if _, err := os.Stat(filepath.Join(dir, "server.jar")); err == nil {
		rc.Jar = "server.jar"
	} else if jars, _ := filepath.Glob(filepath.Join(dir, "*.jar")); len(jars) == 1 {
		rc.Jar = filepath.Base(jars[0])
	}
	if rc.Jar == "" {
		if _, err := os.Stat(filepath.Join(dir, "run.sh")); err == nil {
			rc.RunScript = "run.sh"
		}
	}
	if ov, ok := m.overrides[id]; ok {
		if ov.Mem != nil {
			rc.Mem = *ov.Mem
		}
		if ov.Aikar != nil {
			rc.Aikar = *ov.Aikar
		}
		if ov.AutoStart != nil {
			rc.AutoStart = *ov.AutoStart
		}
		if ov.AutoRestart != nil {
			rc.AutoRestart = *ov.AutoRestart
		}
		if ov.JVMArgs != nil {
			rc.JVMArgs = strings.Fields(*ov.JVMArgs)
		}
		// A panel-managed jar choice (setup/update) wins if the file exists.
		if ov.Jar != nil && *ov.Jar != "" {
			if _, err := os.Stat(filepath.Join(dir, *ov.Jar)); err == nil {
				rc.Jar = *ov.Jar
				rc.RunScript = ""
			}
		}
	}
	return rc
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// StartupResume starts autostart servers plus (optionally) those that were
// running when the panel last shut down.
func (m *Manager) StartupResume(resume bool) {
	var toStart []string
	m.mu.Lock()
	prev := map[string]bool{}
	if resume {
		for _, id := range m.loadRunningState() {
			prev[id] = true
		}
	}
	for id, inst := range m.instances {
		if inst.cfg.AutoStart || prev[id] {
			toStart = append(toStart, id)
		}
	}
	m.mu.Unlock()
	sort.Strings(toStart)
	for _, id := range toStart {
		if err := m.Start(id); err != nil {
			slog.Warn("autostart failed", "server", id, "err", err)
		}
	}
}

// GracefulShutdown stops all running servers, remembering them for resume.
func (m *Manager) GracefulShutdown(timeout time.Duration) {
	m.saveRunningState()
	var wg sync.WaitGroup
	m.mu.Lock()
	for _, inst := range m.instances {
		inst.mu.Lock()
		running := inst.state == StateRunning || inst.state == StateStarting
		inst.mu.Unlock()
		if running {
			wg.Add(1)
			go func(in *instance) {
				defer wg.Done()
				_ = in.Stop()
			}(inst)
		}
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		slog.Warn("timed out waiting for minecraft servers to stop")
	}
}

func (m *Manager) resourceLoop() {
	for range time.Tick(2 * time.Second) {
		m.mu.Lock()
		insts := make([]*instance, 0, len(m.instances))
		for _, in := range m.instances {
			insts = append(insts, in)
		}
		m.mu.Unlock()
		active := false
		for _, in := range insts {
			in.sampleResources()
			in.mu.Lock()
			if in.state != StateStopped && in.state != StateCrashed {
				active = true
			}
			in.mu.Unlock()
		}
		if active && m.bus.Subscribers() > 0 {
			m.publish()
		}
	}
}

func (m *Manager) publish() {
	m.bus.Publish("mc", m.List())
	m.saveRunningState()
}

// --- Service interface ------------------------------------------------------

func (m *Manager) List() []ServerInfo {
	m.mu.Lock()
	insts := make([]*instance, 0, len(m.instances))
	for _, in := range m.instances {
		insts = append(insts, in)
	}
	m.mu.Unlock()
	out := make([]ServerInfo, 0, len(insts))
	for _, in := range insts {
		out = append(out, in.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *Manager) Get(id string) (ServerInfo, bool) {
	in, ok := m.inst(id)
	if !ok {
		return ServerInfo{}, false
	}
	return in.Info(), true
}

func (m *Manager) inst(id string) (*instance, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	in, ok := m.instances[id]
	return in, ok
}

func (m *Manager) instErr(id string) (*instance, error) {
	if err := CheckID(id); err != nil {
		return nil, err
	}
	in, ok := m.inst(id)
	if !ok {
		return nil, fmt.Errorf("unknown server %q", id)
	}
	return in, nil
}

func (m *Manager) Start(id string) error {
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	return in.Start()
}

func (m *Manager) Stop(id string) error {
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	return in.Stop()
}

func (m *Manager) Restart(id string) error {
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	if err := in.Stop(); err != nil {
		return err
	}
	return in.Start()
}

func (m *Manager) Kill(id string) error {
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	return in.Kill()
}

func (m *Manager) Command(id, cmd string) error {
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	return in.Command(cmd)
}

func (m *Manager) Console(id string, tail int) ([]LogLine, error) {
	in, err := m.instErr(id)
	if err != nil {
		return nil, err
	}
	return in.ring.Tail(tail), nil
}

func (m *Manager) Subscribe(id string) (<-chan LogLine, func(), error) {
	in, err := m.instErr(id)
	if err != nil {
		return nil, nil, err
	}
	ch, cancel := in.ring.Subscribe()
	return ch, cancel, nil
}

func (m *Manager) Properties(id string) ([]PropEntry, error) {
	in, err := m.instErr(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(in.dir, "server.properties"))
	if err != nil {
		return nil, fmt.Errorf("read server.properties: %w", err)
	}
	return ParseProperties(string(b)), nil
}

func (m *Manager) SetProperties(id string, changes map[string]string) error {
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	for k, v := range changes {
		if strings.ContainsAny(k, "\r\n=") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("invalid characters in property %q", k)
		}
	}
	path := filepath.Join(in.dir, "server.properties")
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read server.properties: %w", err)
	}
	updated := UpdateProperties(string(b), changes)
	if err := os.WriteFile(path+".tmp", []byte(updated), 0o644); err != nil {
		return err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return err
	}
	in.refreshProps()
	go m.publish()
	return nil
}

func (m *Manager) AcceptEULA(id string) error {
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	content := "# Accepted via control panel on " + time.Now().Format(time.RFC3339) + "\n" +
		"# https://aka.ms/MinecraftEULA\n" +
		"eula=true\n"
	if err := os.WriteFile(filepath.Join(in.dir, "eula.txt"), []byte(content), 0o644); err != nil {
		return err
	}
	go m.publish()
	return nil
}

func (m *Manager) Players(id string) (PlayerInfo, error) {
	in, err := m.instErr(id)
	if err != nil {
		return PlayerInfo{}, err
	}
	info := in.Info()
	pi := PlayerInfo{
		Online:     info.OnlinePlayers,
		MaxPlayers: info.MaxPlayers,
		Whitelist:  readPlayerFile(in.dir, "whitelist.json"),
		Ops:        readPlayerFile(in.dir, "ops.json"),
		Banned:     readPlayerFile(in.dir, "banned-players.json"),
	}
	if b, err := os.ReadFile(filepath.Join(in.dir, "server.properties")); err == nil {
		pi.WhitelistEnabled = PropValue(string(b), "white-list") == "true" ||
			PropValue(string(b), "enforce-whitelist") == "true"
	}
	return pi, nil
}

func (m *Manager) PlayerAction(id, player, action string) error {
	if err := CheckPlayerName(player); err != nil {
		return err
	}
	cmd, ok := PlayerActions[action]
	if !ok {
		return fmt.Errorf("invalid player action %q", action)
	}
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	// Player management needs the live server: it owns the JSON files and
	// resolves names to UUIDs.
	return in.Command(cmd + " " + player)
}

// --- backups ----------------------------------------------------------------

func (m *Manager) backupDir(id string) string {
	return filepath.Join(m.cfg.BackupDir, id)
}

func (m *Manager) Backups(id string) ([]BackupInfo, error) {
	if _, err := m.instErr(id); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(m.backupDir(id))
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, err
	}
	out := []BackupInfo{}
	for _, e := range entries {
		if e.IsDir() || CheckBackupName(e.Name()) != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Name:      e.Name(),
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime().UnixMilli(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

func (m *Manager) CreateBackup(id string) (*jobs.View, error) {
	in, err := m.instErr(id)
	if err != nil {
		return nil, err
	}
	return m.runner.Start("mc.backup", id, func(ctx context.Context, out func(string)) error {
		in.mu.Lock()
		running := in.state == StateRunning
		in.mu.Unlock()

		if running {
			out("pausing world saves (save-off, save-all flush)")
			_ = in.Command("save-off")
			_ = in.Command("save-all flush")
			if !in.waitForLine("Saved the game", 60*time.Second) {
				out("warning: no save confirmation within 60s, archiving anyway")
			}
			defer func() {
				_ = in.Command("save-on")
				out("world saves resumed (save-on)")
			}()
		}

		dir := m.backupDir(id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		name := time.Now().Format("2006-01-02_150405") + ".tar.gz"
		out("archiving " + in.dir + " -> " + filepath.Join(dir, name))
		return tarGzDir(ctx, in.dir, filepath.Join(dir, name), out)
	})
}

func (m *Manager) RestoreBackup(id, name string) error {
	if err := CheckBackupName(name); err != nil {
		return err
	}
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	in.mu.Lock()
	stopped := in.state == StateStopped || in.state == StateCrashed
	in.mu.Unlock()
	if !stopped {
		return fmt.Errorf("server %s must be stopped before restoring a backup", id)
	}
	archive := filepath.Join(m.backupDir(id), name)
	if _, err := os.Stat(archive); err != nil {
		return fmt.Errorf("backup not found: %s", name)
	}

	// Extract next to the live dir, keep the replaced dir, then swap.
	tmp := in.dir + ".restore-tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	if err := extractTarGz(archive, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("extract failed (server untouched): %w", err)
	}
	replaced := filepath.Join(m.backupDir(id), "replaced-"+time.Now().Format("2006-01-02_150405"))
	if err := os.Rename(in.dir, replaced); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, in.dir); err != nil {
		// Try to roll back the swap.
		_ = os.Rename(replaced, in.dir)
		_ = os.RemoveAll(tmp)
		return err
	}
	in.appendPanelLine("Restored backup " + name + " (previous data kept at " + replaced + ")")
	in.refreshProps()
	go m.publish()
	return nil
}

func (m *Manager) DeleteBackup(id, name string) error {
	if err := CheckBackupName(name); err != nil {
		return err
	}
	if _, err := m.instErr(id); err != nil {
		return err
	}
	return os.Remove(filepath.Join(m.backupDir(id), name))
}

// --- panel-managed overrides ------------------------------------------------

func (m *Manager) UpdateConfig(id string, patch ConfigPatch) error {
	in, err := m.instErr(id)
	if err != nil {
		return err
	}
	if patch.Mem != nil && *patch.Mem != "" && parseMem(*patch.Mem) == 0 {
		return fmt.Errorf("invalid memory value %q (use e.g. 4G, 2048M)", *patch.Mem)
	}
	m.mu.Lock()
	cur := m.overrides[id]
	if patch.Mem != nil {
		cur.Mem = patch.Mem
	}
	if patch.Aikar != nil {
		cur.Aikar = patch.Aikar
	}
	if patch.AutoStart != nil {
		cur.AutoStart = patch.AutoStart
	}
	if patch.AutoRestart != nil {
		cur.AutoRestart = patch.AutoRestart
	}
	if patch.JVMArgs != nil {
		cur.JVMArgs = patch.JVMArgs
	}
	if patch.Jar != nil {
		cur.Jar = patch.Jar
	}
	m.overrides[id] = cur
	m.saveOverridesLocked()
	newCfg := m.resolveCfgLocked(id, in.dir)
	m.mu.Unlock()

	in.mu.Lock()
	in.cfg = newCfg
	in.mu.Unlock()
	go m.publish()
	return nil
}

func (m *Manager) overridesPath() string {
	return filepath.Join(m.dataDir, "mc-overrides.json")
}

func (m *Manager) loadOverrides() {
	b, err := os.ReadFile(m.overridesPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &m.overrides)
}

func (m *Manager) saveOverridesLocked() {
	b, err := json.MarshalIndent(m.overrides, "", "  ")
	if err != nil {
		return
	}
	tmp := m.overridesPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, m.overridesPath())
	}
}

// --- running-state persistence (for resume across panel restarts) -----------

func (m *Manager) statePath() string {
	return filepath.Join(m.dataDir, "mc-running.json")
}

func (m *Manager) saveRunningState() {
	var running []string
	m.mu.Lock()
	for id, in := range m.instances {
		in.mu.Lock()
		if in.state == StateRunning || in.state == StateStarting {
			running = append(running, id)
		}
		in.mu.Unlock()
	}
	m.mu.Unlock()
	sort.Strings(running)
	b, _ := json.Marshal(running)
	tmp := m.statePath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, m.statePath())
	}
}

func (m *Manager) loadRunningState() []string {
	b, err := os.ReadFile(m.statePath())
	if err != nil {
		return nil
	}
	var ids []string
	_ = json.Unmarshal(b, &ids)
	return ids
}
