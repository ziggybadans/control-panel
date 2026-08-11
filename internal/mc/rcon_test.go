package mc

import (
	"bytes"
	"testing"
)

func TestRconPacketRoundTrip(t *testing.T) {
	pkt := encodePacket(7, rconCommand, "list")
	id, typ, body, err := decodePacket(bytes.NewReader(pkt))
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 || typ != rconCommand || body != "list" {
		t.Errorf("got id=%d typ=%d body=%q", id, typ, body)
	}
}

func TestRconPacketEmptyBody(t *testing.T) {
	pkt := encodePacket(1, rconAuth, "")
	id, typ, body, err := decodePacket(bytes.NewReader(pkt))
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 || typ != rconAuth || body != "" {
		t.Errorf("got id=%d typ=%d body=%q", id, typ, body)
	}
}

func TestRconPacketLayout(t *testing.T) {
	pkt := encodePacket(1, rconCommand, "ab")
	// length = 4(id) + 4(type) + 2(body) + 2(nulls) = 12
	want := []byte{
		12, 0, 0, 0, // length LE
		1, 0, 0, 0, // id
		2, 0, 0, 0, // type SERVERDATA_EXECCOMMAND
		'a', 'b', 0, 0,
	}
	if !bytes.Equal(pkt, want) {
		t.Errorf("packet = %v, want %v", pkt, want)
	}
}

func TestDecodeRejectsBadLength(t *testing.T) {
	// length field of 4 is below the 10-byte protocol minimum
	bad := []byte{4, 0, 0, 0, 1, 0, 0, 0}
	if _, _, _, err := decodePacket(bytes.NewReader(bad)); err == nil {
		t.Error("expected error for undersized packet")
	}
}

func TestIDValidation(t *testing.T) {
	for _, ok := range []string{"survival", "atm10", "My-Server_2"} {
		if err := CheckID(ok); err != nil {
			t.Errorf("CheckID(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "../etc", "a b", "x/y", ".hidden", strings64()} {
		if err := CheckID(bad); err == nil {
			t.Errorf("CheckID(%q) should fail", bad)
		}
	}
}

func strings64() string {
	s := ""
	for i := 0; i < 70; i++ {
		s += "a"
	}
	return s
}

func TestPlayerNameValidation(t *testing.T) {
	if err := CheckPlayerName("Notch_123"); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{"", "has space", "way_too_long_for_minecraft", "semi;colon"} {
		if err := CheckPlayerName(bad); err == nil {
			t.Errorf("CheckPlayerName(%q) should fail", bad)
		}
	}
}

func TestBackupNameValidation(t *testing.T) {
	if err := CheckBackupName("2026-08-10_030000.tar.gz"); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{"../../etc/passwd", "x.zip", "a/b.tar.gz", ".tar.gz"} {
		if err := CheckBackupName(bad); err == nil {
			t.Errorf("CheckBackupName(%q) should fail", bad)
		}
	}
}

func TestParseMem(t *testing.T) {
	cases := map[string]uint64{
		"4G": 4 << 30, "512M": 512 << 20, "1024k": 1024 << 10,
		"6g": 6 << 30, "": 0, "abc": 0, "12X": 0,
	}
	for in, want := range cases {
		if got := parseMem(in); got != want {
			t.Errorf("parseMem(%q) = %d, want %d", in, got, want)
		}
	}
}
