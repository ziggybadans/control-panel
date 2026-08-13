package mc

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ziggybadans/control-panel/internal/jobs"
)

// Dir returns (creating on first use) a real seeded directory for the mock
// server, so the file manager, addons, and map detection are fully
// functional in mock mode using the exact same code paths as production.
func (m *mockService) Dir(id string) (string, error) {
	s, err := m.sv(id)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(m.dataDir, "mock-servers", id)
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		if err := seedMockDir(dir, s); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func seedMockDir(dir string, s *mockServer) error {
	write := func(rel, content string) error {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(content), 0o644)
	}

	var props strings.Builder
	props.WriteString("#Minecraft server properties\n")
	for _, p := range s.props {
		fmt.Fprintf(&props, "%s=%s\n", p.Key, p.Value)
	}
	if err := write("server.properties", props.String()); err != nil {
		return err
	}
	_ = write("eula.txt", "eula=true\n")
	_ = write("logs/latest.log", "[12:00:00] [Server thread/INFO]: Done (3.914s)!\n")
	_ = write("world/level.dat", strings.Repeat("\x00NBT", 512))
	_ = write("world/region/r.0.0.mca", strings.Repeat("\x00", 64*1024))
	_ = write("world/region/r.0.1.mca", strings.Repeat("\x00", 64*1024))
	_ = write(s.info.Jar, "placeholder jar bytes")
	_ = write("banned-players.json", "[]\n")

	switch s.info.ID {
	case "survival":
		if err := writeAddonJar(filepath.Join(dir, "plugins", "Dynmap-3.7-beta-6-spigot.jar"),
			"plugin.yml", "name: dynmap\nversion: 3.7-beta-6\nmain: org.dynmap.bukkit.DynmapPlugin\n"); err != nil {
			return err
		}
		_ = writeAddonJar(filepath.Join(dir, "plugins", "EssentialsX-2.20.1.jar"),
			"plugin.yml", "name: Essentials\nversion: 2.20.1\nmain: com.earth2me.essentials.Essentials\n")
		_ = writeAddonJar(filepath.Join(dir, "plugins", "WorldEdit-7.3.0.jar.disabled"),
			"plugin.yml", "name: WorldEdit\nversion: 7.3.0\nmain: com.sk89q.worldedit.bukkit.WorldEditPlugin\n")
		_ = write("plugins/dynmap/configuration.txt", "# Dynmap configuration\nwebserver-port: 8123\n")
	case "atm10":
		_ = writeAddonJar(filepath.Join(dir, "mods", "appliedenergistics2-19.0.10.jar"),
			"META-INF/neoforge.mods.toml", "[[mods]]\nmodId=\"ae2\"\nversion=\"19.0.10\"\ndisplayName=\"Applied Energistics 2\"\n")
		_ = writeAddonJar(filepath.Join(dir, "mods", "jei-1.21.1-19.5.0.jar"),
			"META-INF/neoforge.mods.toml", "[[mods]]\nmodId=\"jei\"\nversion=\"19.5.0\"\ndisplayName=\"Just Enough Items\"\n")
	}
	return nil
}

// writeAddonJar creates a tiny but valid jar (zip) with one metadata file
// so the addon scanner's real parsing runs in mock mode.
func writeAddonJar(path, metaName, metaContent string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(metaName)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(metaContent)); err != nil {
		return err
	}
	return zw.Close()
}

// --- setup / jar update / versions -----------------------------------------

var mockVersions = []string{"1.21.4", "1.21.3", "1.21.1", "1.20.6", "1.20.4"}

func (m *mockService) Versions(ctx context.Context, flavor string) ([]string, error) {
	if !ValidFlavors[flavor] {
		return nil, fmt.Errorf("invalid flavor %q", flavor)
	}
	return mockVersions, nil
}

// ImportServer pretends to extract the uploaded zip and registers a new
// mock server with what it "found" inside.
func (m *mockService) ImportServer(id, mem, zipPath string) (*jobs.View, error) {
	if err := CheckID(id); err != nil {
		return nil, err
	}
	m.mu.Lock()
	_, exists := m.servers[id]
	port := 25564
	for _, s := range m.servers {
		if s.info.Port > port {
			port = s.info.Port
		}
	}
	port++
	m.mu.Unlock()
	if exists {
		return nil, fmt.Errorf("server %q already exists", id)
	}

	return m.runner.Start("mc.import", id, func(ctx context.Context, out func(string)) error {
		defer os.Remove(zipPath)
		out("extracting archive…")
		for _, n := range []int{250, 500, 750, 1000} {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(600 * time.Millisecond):
			}
			out(fmt.Sprintf("%d entries extracted…", n))
		}
		out("moved contents of \"" + id + "\" up to the server directory")
		if mem == "" {
			mem = "4G"
		}
		sv := &mockServer{
			info: ServerInfo{
				ID: id, Name: id,
				Dir:   "/srv/minecraft/" + id,
				State: StateStopped, Version: "1.21.4",
				Software: "Imported", Port: port,
				MaxPlayers: 20, Mem: mem, Java: "java", Jar: "server.jar",
				EulaAccepted:  true,
				OnlinePlayers: []string{},
			},
			props:   defaultProps(id, port, "Imported server", false),
			backups: []BackupInfo{},
			ring:    newLogRing(2000),
		}
		m.mu.Lock()
		m.servers[id] = sv
		m.mu.Unlock()
		m.publish()
		out("detected server jar: server.jar")
		out(fmt.Sprintf("imported %q — review its settings before starting", id))
		return nil
	})
}

func (m *mockService) CreateServer(spec CreateSpec) (*jobs.View, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if _, exists := m.servers[spec.ID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("server %q already exists", spec.ID)
	}
	for id, s := range m.servers {
		if s.info.Port == spec.Port {
			m.mu.Unlock()
			return nil, fmt.Errorf("port %d is already used by %q", spec.Port, id)
		}
	}
	m.mu.Unlock()

	return m.runner.Start("mc.create", spec.ID, func(ctx context.Context, out func(string)) error {
		jar := fmt.Sprintf("%s-%s.jar", spec.Flavor, spec.Version)
		out(fmt.Sprintf("downloading %s from api.%smc.io", jar, spec.Flavor))
		for _, pct := range []int{10, 34, 61, 88, 100} {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(700 * time.Millisecond):
			}
			out(fmt.Sprintf("%d%% (%d / 52 MiB)", pct, pct*52/100))
		}
		out("writing initial configuration")

		mem := spec.Mem
		if mem == "" {
			mem = "4G"
		}
		sv := &mockServer{
			info: ServerInfo{
				ID: spec.ID, Name: spec.ID,
				Dir:   "/srv/minecraft/" + spec.ID,
				State: StateStopped, Version: spec.Version,
				Software:     strings.ToUpper(spec.Flavor[:1]) + spec.Flavor[1:],
				Port:         spec.Port,
				MaxPlayers:   20, Mem: mem, Java: "java", Jar: jar,
				EulaAccepted: spec.AcceptEULA,
				OnlinePlayers: []string{},
			},
			props:   defaultProps(spec.ID, spec.Port, spec.MOTD, false),
			backups: []BackupInfo{},
			ring:    newLogRing(2000),
		}
		m.mu.Lock()
		m.servers[spec.ID] = sv
		m.mu.Unlock()
		m.publish()
		out(fmt.Sprintf("server %q is ready — start it from the panel", spec.ID))
		return nil
	})
}

func (m *mockService) UpdateJar(id, flavor, version string) (*jobs.View, error) {
	s, err := m.sv(id)
	if err != nil {
		return nil, err
	}
	if !ValidFlavors[flavor] {
		return nil, fmt.Errorf("invalid flavor %q", flavor)
	}
	if version == "" {
		return nil, fmt.Errorf("version is required")
	}
	return m.runner.Start("mc.jar", id, func(ctx context.Context, out func(string)) error {
		jar := fmt.Sprintf("%s-%s.jar", flavor, version)
		out(fmt.Sprintf("downloading %s", jar))
		for _, pct := range []int{22, 58, 100} {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(800 * time.Millisecond):
			}
			out(fmt.Sprintf("%d%% (%d / 49 MiB)", pct, pct*49/100))
		}
		m.mu.Lock()
		s.info.Jar = jar
		s.info.Version = version
		s.info.Software = strings.ToUpper(flavor[:1]) + flavor[1:]
		m.mu.Unlock()
		m.publish()
		out(fmt.Sprintf("now using %s — restart the server to apply", jar))
		out("the previous jar was kept in the server directory for rollback")
		return nil
	})
}
