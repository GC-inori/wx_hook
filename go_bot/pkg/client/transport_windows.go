//go:build windows

package client

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

type packetReader interface {
	ReadPacket() ([]byte, error)
}

type lengthPrefixedReader struct {
	conn net.Conn
}

func newPacketReader(conn net.Conn) packetReader {
	return &lengthPrefixedReader{conn: conn}
}

func (r *lengthPrefixedReader) ReadPacket() ([]byte, error) {
	var length uint32
	if err := binary.Read(r.conn, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	if length == 0 || length > maxPacketSize {
		return nil, fmt.Errorf("invalid IPC packet length: %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r.conn, body); err != nil {
		return nil, err
	}
	return body, nil
}

func writePacket(conn net.Conn, payload []byte) error {
	if len(payload) == 0 || len(payload) > maxPacketSize {
		return fmt.Errorf("invalid IPC packet length: %d", len(payload))
	}
	prefix := make([]byte, 4)
	binary.LittleEndian.PutUint32(prefix, uint32(len(payload)))
	if err := writeFull(conn, prefix); err != nil {
		return err
	}
	return writeFull(conn, payload)
}

func writeFull(conn net.Conn, data []byte) error {
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
