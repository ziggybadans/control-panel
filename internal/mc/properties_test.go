package mc

import (
	"strings"
	"testing"
)

const sampleProps = `#Minecraft server properties
#Mon Aug 10 03:00:00 UTC 2026
enable-jmx-monitoring=false
rcon.port=25575
gamemode=survival
enable-command-block=false
motd=A Minecraft Server
pvp=true

difficulty=easy
`

func TestParseProperties(t *testing.T) {
	entries := ParseProperties(sampleProps)
	got := map[string]string{}
	for _, e := range entries {
		got[e.Key] = e.Value
	}
	want := map[string]string{
		"enable-jmx-monitoring": "false",
		"rcon.port":             "25575",
		"gamemode":              "survival",
		"enable-command-block":  "false",
		"motd":                  "A Minecraft Server",
		"pvp":                   "true",
		"difficulty":            "easy",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestUpdatePropertiesPreservesLayout(t *testing.T) {
	out := UpdateProperties(sampleProps, map[string]string{
		"gamemode": "creative",
		"motd":     "Hello · World",
	})
	if !strings.Contains(out, "#Minecraft server properties") {
		t.Error("comment line lost")
	}
	if !strings.Contains(out, "gamemode=creative") {
		t.Error("gamemode not updated")
	}
	if !strings.Contains(out, "motd=Hello · World") {
		t.Error("motd not updated")
	}
	// Untouched keys must survive byte-for-byte.
	if !strings.Contains(out, "rcon.port=25575") {
		t.Error("untouched key altered")
	}
	// Order preserved: jmx line still before gamemode.
	if strings.Index(out, "enable-jmx-monitoring") > strings.Index(out, "gamemode=") {
		t.Error("key order changed")
	}
	// Blank line before difficulty preserved.
	if !strings.Contains(out, "\n\ndifficulty=easy") {
		t.Error("blank line lost")
	}
}

func TestUpdatePropertiesAppendsNewKeys(t *testing.T) {
	out := UpdateProperties("a=1\n", map[string]string{"b": "2", "c": "3"})
	if !strings.Contains(out, "b=2") || !strings.Contains(out, "c=3") {
		t.Fatalf("new keys not appended: %q", out)
	}
}

func TestUpdatePropertiesIdempotentWithNoChanges(t *testing.T) {
	if out := UpdateProperties(sampleProps, map[string]string{}); out != sampleProps {
		t.Error("no-op update altered content")
	}
}

func TestPropValue(t *testing.T) {
	if v := PropValue(sampleProps, "rcon.port"); v != "25575" {
		t.Errorf("PropValue = %q", v)
	}
	if v := PropValue(sampleProps, "missing"); v != "" {
		t.Errorf("missing key = %q, want empty", v)
	}
}
