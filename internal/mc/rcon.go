package mc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// Minimal RCON client (Source RCON protocol, as implemented by Minecraft).
// Used for structured queries; lifecycle commands go through stdin, which is
// always available for panel-managed processes.

const (
	rconAuth        int32 = 3
	rconCommand     int32 = 2
	rconResponse    int32 = 0
	rconMaxBodySize       = 4096
)

func encodePacket(id, typ int32, body string) []byte {
	// length(int32) id(int32) type(int32) body \x00 \x00
	length := int32(4 + 4 + len(body) + 2)
	buf := bytes.NewBuffer(make([]byte, 0, 4+length))
	binary.Write(buf, binary.LittleEndian, length)
	binary.Write(buf, binary.LittleEndian, id)
	binary.Write(buf, binary.LittleEndian, typ)
	buf.WriteString(body)
	buf.Write([]byte{0, 0})
	return buf.Bytes()
}

func decodePacket(r io.Reader) (id, typ int32, body string, err error) {
	var length int32
	if err = binary.Read(r, binary.LittleEndian, &length); err != nil {
		return
	}
	if length < 10 || length > rconMaxBodySize+10 {
		err = fmt.Errorf("rcon: invalid packet length %d", length)
		return
	}
	payload := make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return
	}
	id = int32(binary.LittleEndian.Uint32(payload[0:4]))
	typ = int32(binary.LittleEndian.Uint32(payload[4:8]))
	body = string(bytes.TrimRight(payload[8:], "\x00"))
	return
}

type rconClient struct {
	conn net.Conn
	next int32
}

func rconDial(addr, password string, timeout time.Duration) (*rconClient, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	c := &rconClient{conn: conn, next: 1}
	conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(encodePacket(c.next, rconAuth, password)); err != nil {
		conn.Close()
		return nil, err
	}
	// Some servers send an empty response packet before the auth reply.
	for {
		id, typ, _, err := decodePacket(conn)
		if err != nil {
			conn.Close()
			return nil, err
		}
		if typ != rconCommand && typ != rconResponse {
			continue
		}
		if typ == rconCommand || id == c.next {
			if id == -1 {
				conn.Close()
				return nil, fmt.Errorf("rcon: authentication refused")
			}
			return c, nil
		}
	}
}

func (c *rconClient) Command(cmd string, timeout time.Duration) (string, error) {
	c.next++
	c.conn.SetDeadline(time.Now().Add(timeout))
	if _, err := c.conn.Write(encodePacket(c.next, rconCommand, cmd)); err != nil {
		return "", err
	}
	id, _, body, err := decodePacket(c.conn)
	if err != nil {
		return "", err
	}
	if id != c.next {
		return "", fmt.Errorf("rcon: response id mismatch")
	}
	return body, nil
}

func (c *rconClient) Close() { c.conn.Close() }
