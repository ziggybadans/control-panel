package mc

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ziggybadans/control-panel/internal/events"
	"github.com/ziggybadans/control-panel/internal/jobs"
)

// mockService simulates several Minecraft servers with live consoles so the
// full UI can be exercised without Java or a Linux host.
type mockService struct {
	bus     *events.Bus
	runner  *jobs.Runner
	dataDir string

	mu      sync.Mutex
	servers map[string]*mockServer
}

type mockServer struct {
	info    ServerInfo
	ring    *logRing
	props   []PropEntry
	backups []BackupInfo
	wl      []NamedPlayer
	ops     []NamedPlayer
	banned  []NamedPlayer
	stopReq chan struct{}
}

var mockPlayers = []string{"Draxx", "moonpetal", "TerraFirma_", "quills", "BlockbyBlock", "Astris"}

func NewMockService(bus *events.Bus, runner *jobs.Runner, dataDir string) Service {
	m := &mockService{bus: bus, runner: runner, dataDir: dataDir, servers: map[string]*mockServer{}}

	survival := &mockServer{
		info: ServerInfo{
			ID: "survival", Name: "survival", Dir: "/srv/minecraft/survival",
			State: StateStopped, Version: "1.21.4", Software: "Paper", Port: 25565,
			MaxPlayers: 20, Mem: "6G", Java: "java", Jar: "paper-1.21.4-115.jar",
			Aikar: true, AutoStart: true, AutoRestart: true,
			EulaAccepted: true, RconEnabled: true,
			OnlinePlayers: []string{},
		},
		props: defaultProps("survival", 25565, "A cozy survival world", true),
		backups: []BackupInfo{
			{Name: "2026-08-10_030000.tar.gz", SizeBytes: 4_812_331_520, CreatedAt: time.Now().Add(-38 * time.Hour).UnixMilli()},
			{Name: "2026-08-08_030000.tar.gz", SizeBytes: 4_711_224_115, CreatedAt: time.Now().Add(-86 * time.Hour).UnixMilli()},
			{Name: "2026-08-05_030000.tar.gz", SizeBytes: 4_598_872_064, CreatedAt: time.Now().Add(-158 * time.Hour).UnixMilli()},
		},
		wl: []NamedPlayer{
			{Name: "Draxx", UUID: "5f8a1c3e-1234-4a5b-8c7d-9e0f1a2b3c4d"},
			{Name: "moonpetal", UUID: "7c2b9d4f-5678-4c6d-9e8f-0a1b2c3d4e5f"},
			{Name: "quills", UUID: "1a2b3c4d-9abc-4d5e-8f70-2b3c4d5e6f70"},
			{Name: "TerraFirma_", UUID: "3c4d5e6f-def0-4f70-9081-4d5e6f708192"},
		},
		ops: []NamedPlayer{{Name: "Draxx", UUID: "5f8a1c3e-1234-4a5b-8c7d-9e0f1a2b3c4d", Level: 4}},
		banned: []NamedPlayer{
			{Name: "griefer99", UUID: "9e0f1a2b-3456-4b5c-8d7e-1f2a3b4c5d6e", Reason: "Destroyed spawn builds"},
		},
	}

	creative := &mockServer{
		info: ServerInfo{
			ID: "creative", Name: "creative", Dir: "/srv/minecraft/creative",
			State: StateStopped, Version: "1.21.4", Software: "Vanilla", Port: 25566,
			MaxPlayers: 10, Mem: "3G", Java: "java", Jar: "server.jar",
			EulaAccepted: true, OnlinePlayers: []string{},
		},
		props:   defaultProps("creative", 25566, "Build without limits", false),
		backups: []BackupInfo{{Name: "2026-08-01_120000.tar.gz", SizeBytes: 1_204_882_432, CreatedAt: time.Now().Add(-10 * 24 * time.Hour).UnixMilli()}},
		wl:      []NamedPlayer{},
		ops:     []NamedPlayer{{Name: "Draxx", UUID: "5f8a1c3e-1234-4a5b-8c7d-9e0f1a2b3c4d", Level: 4}},
	}

	atm := &mockServer{
		info: ServerInfo{
			ID: "atm10", Name: "atm10", Dir: "/srv/minecraft/atm10",
			State: StateStopped, Version: "1.21.1", Software: "NeoForge", Port: 25567,
			MaxPlayers: 8, Mem: "10G", Java: "java",
			JVMArgs: []string{"-Dfml.readTimeout=180"},
			EulaAccepted: false, OnlinePlayers: []string{},
		},
		props:   defaultProps("atm10", 25567, "All the Mods 10", true),
		backups: []BackupInfo{},
		wl:      []NamedPlayer{{Name: "Draxx", UUID: "5f8a1c3e-1234-4a5b-8c7d-9e0f1a2b3c4d"}},
	}

	for _, s := range []*mockServer{survival, creative, atm} {
		s.ring = newLogRing(2000)
		m.servers[s.info.ID] = s
	}

	// Boot the survival server so the first screen is alive.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = m.Start("survival")
	}()
	go m.activityLoop()
	return m
}

func defaultProps(level string, port int, motd string, whitelist bool) []PropEntry {
	wl := "false"
	if whitelist {
		wl = "true"
	}
	return []PropEntry{
		{"allow-flight", "false"}, {"allow-nether", "true"}, {"difficulty", "normal"},
		{"enable-command-block", "false"}, {"enable-rcon", "true"}, {"enforce-whitelist", wl},
		{"gamemode", "survival"}, {"hardcore", "false"}, {"level-name", level},
		{"level-seed", ""}, {"max-players", "20"}, {"motd", motd},
		{"online-mode", "true"}, {"pvp", "true"}, {"rcon.password", "••••••••"},
		{"rcon.port", fmt.Sprint(port + 10)}, {"server-port", fmt.Sprint(port)},
		{"simulation-distance", "10"}, {"spawn-protection", "16"}, {"view-distance", "12"},
		{"white-list", wl},
	}
}

func (m *mockService) sv(id string) (*mockServer, error) {
	if err := CheckID(id); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[id]
	if !ok {
		return nil, fmt.Errorf("unknown server %q", id)
	}
	return s, nil
}

func (m *mockService) log(s *mockServer, level, text string) {
	s.ring.Append(LogLine{TS: time.Now().UnixMilli(), Level: level, Text: text})
}

func (m *mockService) mcLog(s *mockServer, level, msg string) {
	ts := time.Now().Format("15:04:05")
	m.log(s, level, fmt.Sprintf("[%s] [Server thread/%s]: %s", ts, level, msg))
}

func (m *mockService) publish() {
	m.bus.Publish("mc", m.List())
}

// --- lifecycle --------------------------------------------------------------

func (m *mockService) Start(id string) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if s.info.State != StateStopped && s.info.State != StateCrashed {
		m.mu.Unlock()
		return fmt.Errorf("server %s is %s", id, s.info.State)
	}
	if !s.info.EulaAccepted {
		m.mu.Unlock()
		return fmt.Errorf("EULA not accepted for %s — accept it in the server settings first", id)
	}
	s.info.State = StateStarting
	s.info.StartedAt = time.Now().UnixMilli()
	s.info.PID = 3000 + rand.IntN(5000)
	s.info.LastExit = ""
	s.stopReq = make(chan struct{})
	m.mu.Unlock()
	m.publish()

	go func() {
		lines := []struct {
			delay time.Duration
			level, msg string
		}{
			{200 * time.Millisecond, "INFO", "Starting minecraft server version " + s.info.Version},
			{300 * time.Millisecond, "INFO", "Loading properties"},
			{200 * time.Millisecond, "INFO", "This server is running " + s.info.Software + " version " + s.info.Version},
			{600 * time.Millisecond, "INFO", "Preparing level \"" + s.info.Name + "\""},
			{900 * time.Millisecond, "INFO", "Preparing start region for dimension minecraft:overworld"},
			{700 * time.Millisecond, "INFO", "Preparing spawn area: 42%"},
			{600 * time.Millisecond, "INFO", "Preparing spawn area: 87%"},
			{500 * time.Millisecond, "INFO", "Time elapsed: 3521 ms"},
		}
		for _, l := range lines {
			select {
			case <-s.stopReq:
				return
			case <-time.After(l.delay):
			}
			m.mcLog(s, l.level, l.msg)
		}
		m.mu.Lock()
		if s.info.State != StateStarting {
			m.mu.Unlock()
			return
		}
		s.info.State = StateRunning
		m.mu.Unlock()
		m.mcLog(s, "INFO", fmt.Sprintf("Done (3.914s)! For help, type \"help\""))
		if s.info.RconEnabled {
			m.mcLog(s, "INFO", "RCON running on 0.0.0.0:"+fmt.Sprint(s.info.Port+10))
		}
		m.publish()
	}()
	return nil
}

func (m *mockService) Stop(id string) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if s.info.State != StateRunning && s.info.State != StateStarting {
		m.mu.Unlock()
		return fmt.Errorf("server %s is %s", id, s.info.State)
	}
	s.info.State = StateStopping
	close(s.stopReq)
	players := append([]string(nil), s.info.OnlinePlayers...)
	m.mu.Unlock()
	m.publish()

	m.mcLog(s, "INFO", "Stopping server")
	for _, p := range players {
		m.mcLog(s, "INFO", p+" lost connection: Server closed")
		m.mcLog(s, "INFO", p+" left the game")
	}
	time.Sleep(700 * time.Millisecond)
	m.mcLog(s, "INFO", "Saving chunks for level 'ServerLevel["+s.info.Name+"]'/minecraft:overworld")
	time.Sleep(900 * time.Millisecond)
	m.mcLog(s, "INFO", "ThreadedAnvilChunkStorage: All dimensions are saved")

	m.mu.Lock()
	s.info.State = StateStopped
	s.info.OnlinePlayers = []string{}
	s.info.PID = 0
	s.info.CPUPct = 0
	s.info.MemBytes = 0
	s.info.LastExit = "stopped cleanly at " + time.Now().Format("15:04:05")
	m.mu.Unlock()
	m.publish()
	return nil
}

func (m *mockService) Restart(id string) error {
	if err := m.Stop(id); err != nil {
		return err
	}
	return m.Start(id)
}

func (m *mockService) Kill(id string) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if s.info.State == StateStopped || s.info.State == StateCrashed {
		m.mu.Unlock()
		return fmt.Errorf("server %s is not running", id)
	}
	if s.stopReq != nil {
		select {
		case <-s.stopReq:
		default:
			close(s.stopReq)
		}
	}
	s.info.State = StateStopped
	s.info.OnlinePlayers = []string{}
	s.info.PID = 0
	s.info.CPUPct = 0
	s.info.MemBytes = 0
	s.info.LastExit = "killed (SIGKILL) at " + time.Now().Format("15:04:05")
	m.mu.Unlock()
	m.log(s, "PANEL", "Server process killed")
	m.publish()
	return nil
}

// --- console ----------------------------------------------------------------

func (m *mockService) Command(id, cmd string) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	if strings.ContainsAny(cmd, "\r\n") {
		return fmt.Errorf("command must be a single line")
	}
	m.mu.Lock()
	state := s.info.State
	online := append([]string(nil), s.info.OnlinePlayers...)
	max := s.info.MaxPlayers
	m.mu.Unlock()
	if state != StateRunning && state != StateStarting {
		return fmt.Errorf("server %s is not running", id)
	}

	m.log(s, "CMD", "> "+cmd)
	verb, rest, _ := strings.Cut(cmd, " ")
	switch verb {
	case "stop":
		go func() { _ = m.Stop(id) }()
	case "list":
		m.mcLog(s, "INFO", fmt.Sprintf("There are %d of a max of %d players online: %s",
			len(online), max, strings.Join(online, ", ")))
	case "say":
		m.mcLog(s, "INFO", "[Server] "+rest)
	case "kick":
		name, _, _ := strings.Cut(rest, " ")
		m.removePlayer(s, name, "Kicked by an operator")
	case "ban":
		name, _, _ := strings.Cut(rest, " ")
		m.removePlayer(s, name, "Banned by an operator")
		m.mu.Lock()
		s.banned = append(s.banned, NamedPlayer{Name: name, Reason: "Banned by an operator"})
		m.mu.Unlock()
		m.mcLog(s, "INFO", "Banned "+name+": Banned by an operator")
	case "pardon":
		name, _, _ := strings.Cut(rest, " ")
		m.mu.Lock()
		s.banned = filterPlayers(s.banned, name)
		m.mu.Unlock()
		m.mcLog(s, "INFO", "Unbanned "+name)
	case "op":
		name, _, _ := strings.Cut(rest, " ")
		m.mu.Lock()
		s.ops = append(filterPlayers(s.ops, name), NamedPlayer{Name: name, Level: 4})
		m.mu.Unlock()
		m.mcLog(s, "INFO", "Made "+name+" a server operator")
	case "deop":
		name, _, _ := strings.Cut(rest, " ")
		m.mu.Lock()
		s.ops = filterPlayers(s.ops, name)
		m.mu.Unlock()
		m.mcLog(s, "INFO", "Made "+name+" no longer a server operator")
	case "whitelist":
		sub, name, _ := strings.Cut(rest, " ")
		m.mu.Lock()
		switch sub {
		case "add":
			s.wl = append(filterPlayers(s.wl, name), NamedPlayer{Name: name})
			m.mu.Unlock()
			m.mcLog(s, "INFO", "Added "+name+" to the whitelist")
		case "remove":
			s.wl = filterPlayers(s.wl, name)
			m.mu.Unlock()
			m.mcLog(s, "INFO", "Removed "+name+" from the whitelist")
		default:
			m.mu.Unlock()
			m.mcLog(s, "INFO", "Whitelist has "+fmt.Sprint(len(s.wl))+" entries")
		}
	case "save-all":
		m.mcLog(s, "INFO", "Saving the game (this may take a moment!)")
		m.mcLog(s, "INFO", "Saved the game")
	case "save-off":
		m.mcLog(s, "INFO", "Automatic saving is now disabled")
	case "save-on":
		m.mcLog(s, "INFO", "Automatic saving is now enabled")
	case "tps":
		m.mcLog(s, "INFO", "TPS from last 1m, 5m, 15m: 20.0, 20.0, 19.98")
	default:
		m.mcLog(s, "INFO", "Executed command: "+cmd)
	}
	return nil
}

func (m *mockService) removePlayer(s *mockServer, name, reason string) {
	m.mu.Lock()
	kept := s.info.OnlinePlayers[:0:0]
	found := false
	for _, p := range s.info.OnlinePlayers {
		if p == name {
			found = true
		} else {
			kept = append(kept, p)
		}
	}
	s.info.OnlinePlayers = kept
	m.mu.Unlock()
	if found {
		m.mcLog(s, "INFO", name+" lost connection: "+reason)
		m.mcLog(s, "INFO", name+" left the game")
		m.publish()
	} else {
		m.mcLog(s, "WARN", "No player was found")
	}
}

func filterPlayers(list []NamedPlayer, name string) []NamedPlayer {
	out := list[:0:0]
	for _, p := range list {
		if !strings.EqualFold(p.Name, name) {
			out = append(out, p)
		}
	}
	return out
}

func (m *mockService) Console(id string, tail int) ([]LogLine, error) {
	s, err := m.sv(id)
	if err != nil {
		return nil, err
	}
	return s.ring.Tail(tail), nil
}

func (m *mockService) Subscribe(id string) (<-chan LogLine, func(), error) {
	s, err := m.sv(id)
	if err != nil {
		return nil, nil, err
	}
	ch, cancel := s.ring.Subscribe()
	return ch, cancel, nil
}

// activityLoop simulates resource usage, chat, and player churn.
func (m *mockService) activityLoop() {
	tick := 0
	for range time.Tick(2 * time.Second) {
		tick++
		m.mu.Lock()
		servers := make([]*mockServer, 0, len(m.servers))
		for _, s := range m.servers {
			servers = append(servers, s)
		}
		m.mu.Unlock()

		changed := false
		for _, s := range servers {
			m.mu.Lock()
			running := s.info.State == StateRunning
			if running {
				base := 12 + 3*float64(len(s.info.OnlinePlayers))
				s.info.CPUPct = clampF(base+(rand.Float64()-0.5)*8, 2, 90)
				target := uint64(float64(parseMem(s.info.Mem)) * (0.45 + 0.1*rand.Float64()))
				s.info.MemBytes = target
			}
			m.mu.Unlock()
			if !running {
				continue
			}
			changed = true

			// Occasional world events.
			if tick%5 == 0 && rand.IntN(3) == 0 {
				m.mcLog(s, "INFO", randomActivity())
			}
			// Player churn on the survival server.
			if s.info.ID == "survival" && tick%4 == 0 {
				m.mu.Lock()
				online := len(s.info.OnlinePlayers)
				m.mu.Unlock()
				switch {
				case online < 3 && rand.IntN(2) == 0:
					cand := mockPlayers[rand.IntN(len(mockPlayers))]
					if !containsStr(s.info.OnlinePlayers, cand) {
						m.mu.Lock()
						s.info.OnlinePlayers = append(s.info.OnlinePlayers, cand)
						sort.Strings(s.info.OnlinePlayers)
						m.mu.Unlock()
						m.mcLog(s, "INFO", cand+" joined the game")
						m.publish()
					}
				case online > 0 && rand.IntN(6) == 0:
					m.mu.Lock()
					p := s.info.OnlinePlayers[rand.IntN(len(s.info.OnlinePlayers))]
					m.mu.Unlock()
					m.removePlayer(s, p, "Disconnected")
				}
			}
		}
		if changed && m.bus.Subscribers() > 0 {
			m.publish()
		}
	}
}

func randomActivity() string {
	msgs := []string{
		"[Draxx] anyone have spare elytra?",
		"[moonpetal] omw to the end portal",
		"Draxx has made the advancement [Serious Dedication]",
		"Villager trades refreshed at spawn trading hall",
		"[quills] the new farm is insane",
		"moonpetal fell from a high place",
		"Saved the game",
	}
	return msgs[rand.IntN(len(msgs))]
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- info / listing ---------------------------------------------------------

func (m *mockService) List() []ServerInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ServerInfo, 0, len(m.servers))
	for _, s := range m.servers {
		info := s.info
		info.OnlinePlayers = append([]string{}, s.info.OnlinePlayers...)
		info.MemMax = parseMem(s.info.Mem)
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *mockService) Get(id string) (ServerInfo, bool) {
	s, err := m.sv(id)
	if err != nil {
		return ServerInfo{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	info := s.info
	info.OnlinePlayers = append([]string{}, s.info.OnlinePlayers...)
	info.MemMax = parseMem(s.info.Mem)
	return info, true
}

// --- properties / eula ------------------------------------------------------

func (m *mockService) Properties(id string) ([]PropEntry, error) {
	s, err := m.sv(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]PropEntry(nil), s.props...), nil
}

func (m *mockService) SetProperties(id string, changes map[string]string) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range s.props {
		if v, ok := changes[e.Key]; ok {
			s.props[i].Value = v
			delete(changes, e.Key)
		}
	}
	for k, v := range changes {
		s.props = append(s.props, PropEntry{Key: k, Value: v})
	}
	return nil
}

func (m *mockService) AcceptEULA(id string) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s.info.EulaAccepted = true
	m.mu.Unlock()
	m.publish()
	return nil
}

// --- players ----------------------------------------------------------------

func (m *mockService) Players(id string) (PlayerInfo, error) {
	s, err := m.sv(id)
	if err != nil {
		return PlayerInfo{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	wlEnabled := false
	for _, p := range s.props {
		if p.Key == "white-list" && p.Value == "true" {
			wlEnabled = true
		}
	}
	return PlayerInfo{
		Online:           append([]string(nil), s.info.OnlinePlayers...),
		MaxPlayers:       s.info.MaxPlayers,
		WhitelistEnabled: wlEnabled,
		Whitelist:        append([]NamedPlayer(nil), s.wl...),
		Ops:              append([]NamedPlayer(nil), s.ops...),
		Banned:           append([]NamedPlayer(nil), s.banned...),
	}, nil
}

func (m *mockService) PlayerAction(id, player, action string) error {
	if err := CheckPlayerName(player); err != nil {
		return err
	}
	cmd, ok := PlayerActions[action]
	if !ok {
		return fmt.Errorf("invalid player action %q", action)
	}
	return m.Command(id, cmd+" "+player)
}

// --- backups ----------------------------------------------------------------

func (m *mockService) Backups(id string) ([]BackupInfo, error) {
	s, err := m.sv(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]BackupInfo(nil), s.backups...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

func (m *mockService) CreateBackup(id string) (*jobs.View, error) {
	s, err := m.sv(id)
	if err != nil {
		return nil, err
	}
	return m.runner.Start("mc.backup", id, func(ctx context.Context, out func(string)) error {
		m.mu.Lock()
		running := s.info.State == StateRunning
		m.mu.Unlock()
		if running {
			out("pausing world saves (save-off, save-all flush)")
			m.mcLog(s, "INFO", "Automatic saving is now disabled")
			m.mcLog(s, "INFO", "Saved the game")
		}
		steps := []string{
			"archiving /srv/minecraft/" + id + " -> /srv/minecraft/.backups/" + id,
			"500 files, 312.4 MiB archived…",
			"1000 files, 1204.1 MiB archived…",
			"1500 files, 2811.0 MiB archived…",
		}
		for _, step := range steps {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1200 * time.Millisecond):
			}
			out(step)
		}
		if running {
			m.mcLog(s, "INFO", "Automatic saving is now enabled")
			out("world saves resumed (save-on)")
		}
		name := time.Now().Format("2006-01-02_150405") + ".tar.gz"
		m.mu.Lock()
		s.backups = append(s.backups, BackupInfo{
			Name: name, SizeBytes: 4_902_120_000, CreatedAt: time.Now().UnixMilli(),
		})
		m.mu.Unlock()
		out("archive complete: 1893 files, 4675.4 MiB (compressed 4675.4 MiB)")
		return nil
	})
}

func (m *mockService) RestoreBackup(id, name string) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	if err := CheckBackupName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.info.State != StateStopped && s.info.State != StateCrashed {
		return fmt.Errorf("server %s must be stopped before restoring a backup", id)
	}
	for _, b := range s.backups {
		if b.Name == name {
			m.log(s, "PANEL", "Restored backup "+name+" (previous data kept at /srv/minecraft/.backups/"+id+"/replaced-…)")
			return nil
		}
	}
	return fmt.Errorf("backup not found: %s", name)
}

func (m *mockService) DeleteBackup(id, name string) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	if err := CheckBackupName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := s.backups[:0:0]
	found := false
	for _, b := range s.backups {
		if b.Name == name {
			found = true
		} else {
			kept = append(kept, b)
		}
	}
	if !found {
		return fmt.Errorf("backup not found: %s", name)
	}
	s.backups = kept
	return nil
}

// --- config -----------------------------------------------------------------

func (m *mockService) UpdateConfig(id string, patch ConfigPatch) error {
	s, err := m.sv(id)
	if err != nil {
		return err
	}
	if patch.Mem != nil && *patch.Mem != "" && parseMem(*patch.Mem) == 0 {
		return fmt.Errorf("invalid memory value %q (use e.g. 4G, 2048M)", *patch.Mem)
	}
	m.mu.Lock()
	if patch.Mem != nil {
		s.info.Mem = *patch.Mem
	}
	if patch.Aikar != nil {
		s.info.Aikar = *patch.Aikar
	}
	if patch.AutoStart != nil {
		s.info.AutoStart = *patch.AutoStart
	}
	if patch.AutoRestart != nil {
		s.info.AutoRestart = *patch.AutoRestart
	}
	if patch.JVMArgs != nil {
		s.info.JVMArgs = strings.Fields(*patch.JVMArgs)
	}
	m.mu.Unlock()
	m.publish()
	return nil
}

func (m *mockService) Rescan() error { return nil }
