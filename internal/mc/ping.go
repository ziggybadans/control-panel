package mc

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"time"
)

// PingStatus is what the server advertises to the multiplayer list: the
// live MOTD (as sent — including plugin rewrites like MiniMOTD) and the
// server icon.
type PingStatus struct {
	MOTD    []MOTDSegment
	Favicon string // "data:image/png;base64,…" or ""
}

const maxStatusJSON = 1 << 20 // an icon is ~10KB; 1MB is generous

// StatusPing performs a Minecraft server-list ping (handshake with next
// state 1, then a status request) and parses the JSON response.
func StatusPing(ctx context.Context, host string, port int, timeout time.Duration) (*PingStatus, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	// Handshake: protocol version -1 (status works regardless), host, port,
	// next state 1 (status).
	var hs []byte
	hs = appendVarInt(hs, 0x00) // packet id
	hs = appendVarInt(hs, -1)
	hs = appendVarInt(hs, int32(len(host)))
	hs = append(hs, host...)
	hs = binary.BigEndian.AppendUint16(hs, uint16(port))
	hs = appendVarInt(hs, 1)
	if err := writePacket(conn, hs); err != nil {
		return nil, err
	}
	// Status request.
	if err := writePacket(conn, appendVarInt(nil, 0x00)); err != nil {
		return nil, err
	}

	r := bufio.NewReader(conn)
	if _, err := readVarInt(r); err != nil { // packet length
		return nil, err
	}
	if id, err := readVarInt(r); err != nil || id != 0 {
		return nil, fmt.Errorf("unexpected status packet (id %d, err %v)", id, err)
	}
	strLen, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if strLen < 0 || strLen > maxStatusJSON {
		return nil, fmt.Errorf("status response too large (%d bytes)", strLen)
	}
	raw := make([]byte, strLen)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, err
	}

	var payload struct {
		Description json.RawMessage `json:"description"`
		Favicon     string          `json:"favicon"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}
	st := &PingStatus{}
	if len(payload.Description) > 0 {
		var desc any
		if err := json.Unmarshal(payload.Description, &desc); err == nil {
			st.MOTD = motdFromDescription(desc)
		}
	}
	// Only accept the format the protocol specifies; anything else is
	// dropped rather than forwarded to the browser.
	if strings.HasPrefix(payload.Favicon, "data:image/png;base64,") {
		st.Favicon = payload.Favicon
	}
	return st, nil
}

// pingLoop keeps each running server's advertised MOTD and icon fresh via
// local server-list pings. It idles while no client is connected; the last
// known values stick around when a server stops so cards stay informative.
func (m *Manager) pingLoop() {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for range t.C {
		if m.bus.Subscribers() == 0 {
			continue
		}
		m.mu.Lock()
		insts := make([]*instance, 0, len(m.instances))
		for _, in := range m.instances {
			insts = append(insts, in)
		}
		m.mu.Unlock()

		changed := false
		for _, in := range insts {
			in.mu.Lock()
			state, port := in.state, in.port
			in.mu.Unlock()
			if state != StateRunning || port == 0 {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			st, err := StatusPing(ctx, "127.0.0.1", port, 3*time.Second)
			cancel()
			if err != nil {
				continue // not up yet / query refused; try again next tick
			}
			in.mu.Lock()
			if !slices.Equal(in.motd, st.MOTD) || in.icon != st.Favicon {
				in.motd = st.MOTD
				in.icon = st.Favicon
				changed = true
			}
			in.mu.Unlock()
		}
		if changed {
			m.publish()
		}
	}
}

// --- wire format ------------------------------------------------------------

func writePacket(w io.Writer, body []byte) error {
	framed := appendVarInt(nil, int32(len(body)))
	framed = append(framed, body...)
	_, err := w.Write(framed)
	return err
}

func appendVarInt(b []byte, v int32) []byte {
	u := uint32(v)
	for {
		if u&^0x7F == 0 {
			return append(b, byte(u))
		}
		b = append(b, byte(u&0x7F|0x80))
		u >>= 7
	}
}

func readVarInt(r io.ByteReader) (int32, error) {
	var result uint32
	for i := 0; i < 5; i++ {
		bb, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint32(bb&0x7F) << (7 * i)
		if bb&0x80 == 0 {
			return int32(result), nil
		}
	}
	return 0, fmt.Errorf("varint too long")
}
