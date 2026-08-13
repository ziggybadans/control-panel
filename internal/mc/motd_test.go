package mc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestParseLegacyMOTD(t *testing.T) {
	got := ParseLegacyMOTD("§6All the Mods §l10§r §7[§e1.21§7]")
	want := []MOTDSegment{
		{Text: "All the Mods ", Color: "gold"},
		{Text: "10", Color: "gold", Bold: true},
		{Text: " "},
		{Text: "[", Color: "gray"},
		{Text: "1.21", Color: "yellow"},
		{Text: "]", Color: "gray"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}

	// A color code resets formatting (vanilla semantics).
	got = ParseLegacyMOTD("§l§cbold red§anot bold green")
	want = []MOTDSegment{
		{Text: "bold red", Color: "red"},
		{Text: "not bold green", Color: "green"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("color reset: got %+v", got)
	}

	// Plain text passes through untouched.
	if got := ParseLegacyMOTD("A Minecraft Server"); len(got) != 1 || got[0].Text != "A Minecraft Server" {
		t.Errorf("plain: got %+v", got)
	}
}

func TestMOTDFromComponents(t *testing.T) {
	// The shape MiniMOTD and friends send: nested components with colors.
	raw := `{"text":"","extra":[{"text":"Welcome ","color":"#00fb9a","bold":true},{"text":"to the server","color":"gray","extra":[{"text":"!","color":"yellow"}]}]}`
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	got := motdFromDescription(v)
	want := []MOTDSegment{
		{Text: "Welcome ", Color: "#00fb9a", Bold: true},
		{Text: "to the server", Color: "gray"},
		{Text: "!", Color: "yellow"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}

	// Plain-string descriptions still work.
	got = motdFromDescription("§bhello")
	if len(got) != 1 || got[0].Color != "aqua" {
		t.Errorf("string description: got %+v", got)
	}
}

// TestStatusPing runs a canned server-list-ping responder and checks the
// full wire round-trip.
func TestStatusPing(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	statusJSON := `{"version":{"name":"Paper 1.21.4","protocol":769},` +
		`"players":{"max":20,"online":3},` +
		`"description":{"text":"A cozy ","extra":[{"text":"survival","color":"green"},{"text":" world"}]},` +
		`"favicon":"data:image/png;base64,iVBORw0KGgo="}`

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Consume handshake + status request frames (length-prefixed).
		buf := make([]byte, 512)
		_, _ = conn.Read(buf)
		// Response: len, id 0, string len, json.
		body := appendVarInt(nil, 0x00)
		body = appendVarInt(body, int32(len(statusJSON)))
		body = append(body, statusJSON...)
		framed := appendVarInt(nil, int32(len(body)))
		framed = append(framed, body...)
		_, _ = conn.Write(framed)
	}()

	st, err := StatusPing(context.Background(), "127.0.0.1", port, 2*time.Second)
	if err != nil {
		t.Fatalf("StatusPing: %v", err)
	}
	want := []MOTDSegment{
		{Text: "A cozy "},
		{Text: "survival", Color: "green"},
		{Text: " world"},
	}
	if !reflect.DeepEqual(st.MOTD, want) {
		t.Errorf("motd = %+v", st.MOTD)
	}
	if st.Favicon != "data:image/png;base64,iVBORw0KGgo=" {
		t.Errorf("favicon = %q", st.Favicon)
	}
}

// TestVarIntRoundtrip pins the encoding used on the wire.
func TestVarIntRoundtrip(t *testing.T) {
	_ = binary.BigEndian // silence unused import when cases change
	for _, v := range []int32{0, 1, 127, 128, 255, 25565, 2097151, -1} {
		b := appendVarInt(nil, v)
		got, err := readVarInt(newByteReader(b))
		if err != nil || got != v {
			t.Errorf("roundtrip %d: got %d, err %v", v, got, err)
		}
	}
}

type byteReader struct {
	b []byte
	i int
}

func newByteReader(b []byte) *byteReader { return &byteReader{b: b} }

func (r *byteReader) ReadByte() (byte, error) {
	if r.i >= len(r.b) {
		return 0, context.DeadlineExceeded
	}
	c := r.b[r.i]
	r.i++
	return c, nil
}
