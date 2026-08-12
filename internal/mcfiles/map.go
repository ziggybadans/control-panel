package mcfiles

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type MapInfo struct {
	Detected bool   `json:"detected"`
	Type     string `json:"type,omitempty"` // dynmap | bluemap | squaremap | pl3xmap
	Port     int    `json:"port,omitempty"`
}

// mapAddons: jar-name fragment -> (type, default port, config probe).
var mapAddons = []struct {
	fragment string
	typ      string
	port     int
}{
	{"dynmap", "dynmap", 8123},
	{"bluemap", "bluemap", 8100},
	{"squaremap", "squaremap", 8080},
	{"pl3xmap", "pl3xmap", 8080},
}

var portRe = regexp.MustCompile(`(?mi)^\s*(?:webserver-)?port[:=]\s*(\d{2,5})`)

// DetectMap looks for a known web-map plugin/mod and its configured port.
func DetectMap(root string) MapInfo {
	addons, err := ListAddons(root)
	if err != nil {
		return MapInfo{}
	}
	for _, a := range addons {
		if !a.Enabled {
			continue
		}
		lower := strings.ToLower(a.File)
		for _, m := range mapAddons {
			if !strings.Contains(lower, m.fragment) {
				continue
			}
			info := MapInfo{Detected: true, Type: m.typ, Port: m.port}
			if p := probeConfiguredPort(root, m.typ); p > 0 {
				info.Port = p
			}
			return info
		}
	}
	return MapInfo{}
}

// probeConfiguredPort greps well-known config files for a port setting.
func probeConfiguredPort(root, typ string) int {
	var candidates []string
	switch typ {
	case "dynmap":
		candidates = []string{"plugins/dynmap/configuration.txt"}
	case "bluemap":
		candidates = []string{"plugins/BlueMap/webserver.conf", "config/bluemap/webserver.conf"}
	case "squaremap":
		candidates = []string{"plugins/squaremap/config.yml"}
	case "pl3xmap":
		candidates = []string{"plugins/Pl3xMap/config.yml"}
	}
	for _, rel := range candidates {
		abs, err := Resolve(root, filepath.ToSlash(rel), true)
		if err != nil {
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil || len(b) > 1<<20 {
			continue
		}
		if m := portRe.FindSubmatch(b); m != nil {
			if p, err := strconv.Atoi(string(m[1])); err == nil && p > 0 && p < 65536 {
				return p
			}
		}
	}
	return 0
}
