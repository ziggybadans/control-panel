package mcfiles

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const disabledSuffix = ".disabled"

type Addon struct {
	File    string `json:"file"` // filename incl. .disabled suffix when off
	Dir     string `json:"dir"`  // "plugins" or "mods"
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Enabled bool   `json:"enabled"`
	Size    int64  `json:"sizeBytes"`
}

// ListAddons scans plugins/ and mods/ for jars (including disabled ones)
// and extracts display metadata from each jar when possible.
func ListAddons(root string) ([]Addon, error) {
	var out []Addon
	for _, dir := range []string{"plugins", "mods"} {
		abs, err := Resolve(root, dir, false)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			continue // directory absent — fine
		}
		for _, e := range entries {
			name := e.Name()
			enabled := strings.HasSuffix(name, ".jar")
			if !enabled && !strings.HasSuffix(name, ".jar"+disabledSuffix) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			a := Addon{
				File:    name,
				Dir:     dir,
				Enabled: enabled,
				Size:    info.Size(),
			}
			a.Name, a.Version = jarMetadata(filepath.Join(abs, name))
			if a.Name == "" {
				a.Name = strings.TrimSuffix(strings.TrimSuffix(name, disabledSuffix), ".jar")
			}
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir < out[j].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ToggleAddon enables/disables a jar via the conventional .disabled rename.
func ToggleAddon(root, dir, file string, enable bool) error {
	if dir != "plugins" && dir != "mods" {
		return fmt.Errorf("invalid addon directory %q", dir)
	}
	if strings.ContainsAny(file, "/\\") {
		return fmt.Errorf("invalid addon file name")
	}
	cur := filepath.ToSlash(filepath.Join(dir, file))
	var next string
	switch {
	case enable && strings.HasSuffix(file, disabledSuffix):
		next = filepath.ToSlash(filepath.Join(dir, strings.TrimSuffix(file, disabledSuffix)))
	case !enable && strings.HasSuffix(file, ".jar"):
		next = cur + disabledSuffix
	default:
		return nil // already in the requested state
	}
	return Rename(root, cur, next)
}

// jarMetadata pulls a human name/version out of a plugin or mod jar:
// plugin.yml (Bukkit/Paper), fabric.mod.json (Fabric), mods.toml (Forge/
// NeoForge). Best-effort — errors just mean "use the filename".
func jarMetadata(path string) (name, version string) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", ""
	}
	defer zr.Close()
	for _, f := range zr.File {
		switch f.Name {
		case "plugin.yml", "paper-plugin.yml":
			return parsePluginYML(f)
		case "fabric.mod.json":
			return parseFabricJSON(f)
		case "META-INF/mods.toml", "META-INF/neoforge.mods.toml":
			return parseModsTOML(f)
		}
	}
	return "", ""
}

func readZipFile(f *zip.File, limit int64) []byte {
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	b := make([]byte, 0, 4096)
	buf := bufio.NewReader(rc)
	chunk := make([]byte, 4096)
	for int64(len(b)) < limit {
		n, err := buf.Read(chunk)
		b = append(b, chunk[:n]...)
		if err != nil {
			break
		}
	}
	return b
}

var (
	ymlNameRe    = regexp.MustCompile(`(?m)^name:\s*['"]?([^'"\n#]+)`)
	ymlVersionRe = regexp.MustCompile(`(?m)^version:\s*['"]?([^'"\n#]+)`)
	tomlNameRe   = regexp.MustCompile(`displayName\s*=\s*"([^"]+)"`)
	tomlVerRe    = regexp.MustCompile(`(?m)^\s*version\s*=\s*"([^"]+)"`)
)

func parsePluginYML(f *zip.File) (string, string) {
	b := readZipFile(f, 64*1024)
	var name, version string
	if m := ymlNameRe.FindSubmatch(b); m != nil {
		name = strings.TrimSpace(string(m[1]))
	}
	if m := ymlVersionRe.FindSubmatch(b); m != nil {
		version = strings.TrimSpace(string(m[1]))
	}
	return name, version
}

func parseFabricJSON(f *zip.File) (string, string) {
	var doc struct {
		Name    string `json:"name"`
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	if json.Unmarshal(readZipFile(f, 256*1024), &doc) != nil {
		return "", ""
	}
	if doc.Name != "" {
		return doc.Name, doc.Version
	}
	return doc.ID, doc.Version
}

func parseModsTOML(f *zip.File) (string, string) {
	b := readZipFile(f, 64*1024)
	var name, version string
	if m := tomlNameRe.FindSubmatch(b); m != nil {
		name = string(m[1])
	}
	if m := tomlVerRe.FindSubmatch(b); m != nil {
		version = string(m[1])
	}
	return name, version
}
