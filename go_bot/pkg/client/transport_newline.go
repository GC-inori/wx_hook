package client

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
)

// The Linux newline framing codec is platform-independent so it is exercised
// by local Windows tests as well as Linux tests.
type newlineJSONReader struct{ reader *bufio.Reader }

func newNewlineReader(conn net.Conn) *newlineJSONReader {
	return &newlineJSONReader{reader: bufio.NewReader(conn)}
}

func (r *newlineJSONReader) ReadPacket() ([]byte, error) {
	var packet []byte
	for {
		fragment, err := r.reader.ReadSlice('\n')
		if len(packet)+len(fragment) > maxPacketSize {
			return nil, fmt.Errorf("IPC JSON line exceeds %d bytes", maxPacketSize)
		}
		packet = append(packet, fragment...)
		if err == nil {
			packet = bytes.TrimSuffix(packet, []byte{'\n'})
			packet = bytes.TrimSuffix(packet, []byte{'\r'})
			if len(packet) == 0 {
				return nil, errors.New("received empty IPC JSON line")
			}
			return packet, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(packet) > 0 {
			return nil, errors.New("incomplete IPC JSON line without newline")
		}
		return nil, err
	}
}

func writeNewlinePacket(conn net.Conn, payload []byte) error {
	// The native bound includes the terminating newline.
	if len(payload) == 0 || len(payload) >= maxPacketSize {
		return fmt.Errorf("invalid IPC JSON line length: %d", len(payload))
	}
	line := make([]byte, len(payload)+1)
	copy(line, payload)
	line[len(payload)] = '\n'
	for len(line) > 0 {
		n, err := conn.Write(line)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		line = line[n:]
	}
	return nil
}
