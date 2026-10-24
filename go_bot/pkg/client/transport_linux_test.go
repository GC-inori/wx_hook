//go:build linux

package client

import (
	"bytes"
	"net"
	"testing"
)

func TestNewlineJSONReaderReadsConsecutivePackets(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	reader := newPacketReader(clientConn)
	writeDone := make(chan error, 1)
	go func() {
		_, err := serverConn.Write([]byte(`{"id":1}` + "\n" + `{"id":2}` + "\n"))
		writeDone <- err
	}()

	for _, want := range []string{`{"id":1}`, `{"id":2}`} {
		got, err := reader.ReadPacket()
		if err != nil {
			t.Fatalf("ReadPacket() error = %v", err)
		}
		if string(got) != want {
			t.Fatalf("ReadPacket() = %q, want %q", got, want)
		}
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("writing test packets: %v", err)
	}
}

func TestWritePacketAppendsNewline(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- writePacket(clientConn, []byte(`{"action":"ping"}`))
	}()

	got, err := newPacketReader(serverConn).ReadPacket()
	if err != nil {
		t.Fatalf("ReadPacket() error = %v", err)
	}
	if !bytes.Equal(got, []byte(`{"action":"ping"}`)) {
		t.Fatalf("ReadPacket() = %q", got)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("WritePacket() error = %v", err)
	}
}
