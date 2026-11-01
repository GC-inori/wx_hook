package client

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

func TestLinuxNewlineCodecOnLocalPlatform(t *testing.T) {
	local, peer := net.Pipe()
	defer local.Close()
	defer peer.Close()
	text := `{"text":"` + strings.Repeat("x", 8192) + `"}`
	done := make(chan error, 1)
	go func() {
		if err := writeNewlinePacket(peer, []byte(text)); err != nil {
			done <- err
			return
		}
		done <- writeNewlinePacket(peer, []byte(`{"second":true}`))
	}()
	reader := newNewlineReader(local)
	for _, want := range []string{text, `{"second":true}`} {
		got, err := reader.ReadPacket()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatal("fragmented or consecutive NDJSON frame corrupted")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestLinuxNewlineCodecRejectsMalformedFrames(t *testing.T) {
	for _, text := range []string{"\n", "\r\n", `{"incomplete":true}`, strings.Repeat("x", maxPacketSize) + "\n"} {
		reader := &newlineJSONReader{reader: bufio.NewReader(strings.NewReader(text))}
		if _, err := reader.ReadPacket(); err == nil {
			t.Fatalf("malformed frame accepted (length=%d)", len(text))
		}
	}
	reader := &newlineJSONReader{reader: bufio.NewReader(strings.NewReader("{}\r\n"))}
	if body, err := reader.ReadPacket(); err != nil || string(body) != "{}" {
		t.Fatalf("CRLF not supported: %q %v", body, err)
	}
}

func TestLinuxNewlineWriterBounds(t *testing.T) {
	for _, payload := range [][]byte{nil, make([]byte, maxPacketSize)} {
		if err := writeNewlinePacket(nil, payload); err == nil {
			t.Fatal("invalid frame size accepted")
		}
	}
}
