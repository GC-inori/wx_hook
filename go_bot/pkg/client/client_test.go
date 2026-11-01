package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

func testClient(t *testing.T, callback EventCallback) (*Client, net.Conn) {
	t.Helper()
	local, remote := net.Pipe()
	c := newClient("unused-test-endpoint", local, callback)
	t.Cleanup(func() { c.Close(); _ = remote.Close() })
	return c, remote
}

func TestConcurrentCallsPreserveIDsAndData(t *testing.T) {
	c, peer := testClient(t, nil)
	const count = 40
	peerError := make(chan error, 1)
	go func() {
		reader := newPacketReader(peer)
		packets := make([]ipcPacket, 0, count)
		for i := 0; i < count; i++ {
			raw, err := reader.ReadPacket()
			if err != nil {
				peerError <- err
				return
			}
			var pkt ipcPacket
			if err := json.Unmarshal(raw, &pkt); err != nil {
				peerError <- err
				return
			}
			packets = append(packets, pkt)
		}
		for i := len(packets) - 1; i >= 0; i-- {
			pkt := packets[i]
			raw := []byte(fmt.Sprintf(`{"id":%d,"status":0,"code":200,"data":%s}`, *pkt.ID, pkt.Data))
			if err := writePacket(peer, raw); err != nil {
				peerError <- err
				return
			}
		}
		peerError <- nil
	}()
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(number int) {
			defer wg.Done()
			data, err := c.Call(context.Background(), "queryObj", map[string]int{"number": number})
			if err != nil {
				t.Errorf("call: %v", err)
				return
			}
			if string(data) != fmt.Sprintf(`{"number":%d}`, number) {
				t.Errorf("wrong response: %s", data)
			}
		}(i)
	}
	wg.Wait()
	if err := <-peerError; err != nil {
		t.Fatal(err)
	}
}

func TestBridgeErrorRetainsStructuredResult(t *testing.T) {
	c, peer := testClient(t, nil)
	go func() {
		reader := newPacketReader(peer)
		raw, err := reader.ReadPacket()
		if err != nil {
			return
		}
		var request ipcPacket
		_ = json.Unmarshal(raw, &request)
		_ = writePacket(peer, []byte(fmt.Sprintf(`{"id":%d,"status":501,"code":501,"error":"offset unresolved","result":{"action":"createGroup","implemented":false,"msg_id":18446744073709551615}}`, *request.ID)))
	}()
	_, err := c.Call(context.Background(), "createGroup", map[string]any{})
	var native *BridgeError
	if !errors.As(err, &native) {
		t.Fatalf("expected BridgeError, got %v", err)
	}
	if native.Code != 501 || native.Status != 501 || native.Message != "offset unresolved" {
		t.Fatalf("lost native error: %+v", native)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(native.Result, &result); err != nil {
		t.Fatal(err)
	}
	if string(result["msg_id"]) != "18446744073709551615" {
		t.Fatalf("message ID precision lost: %s", native.Result)
	}
}

func TestCancellationAndCloseReleasePendingCalls(t *testing.T) {
	c, peer := testClient(t, nil)
	received := make(chan struct{})
	go func() { _, _ = newPacketReader(peer).ReadPacket(); close(received) }()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := c.Call(ctx, "sendText", map[string]string{"target": "a", "text": "b"})
		result <- err
	}()
	<-received
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	c.pendingMu.Lock()
	pending := len(c.pending)
	c.pendingMu.Unlock()
	if pending != 0 {
		t.Fatalf("pending requests leaked: %d", pending)
	}
	c.Close()
	if _, err := c.Call(context.Background(), "getSelfInfo", nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed client accepted request: %v", err)
	}
}

func TestDisconnectFailsWithoutReplaying(t *testing.T) {
	c, peer := testClient(t, nil)
	go func() { _, _ = newPacketReader(peer).ReadPacket(); _ = peer.Close() }()
	start := time.Now()
	_, err := c.Call(context.Background(), "sendText", map[string]string{"text": "hello"})
	if !errors.Is(err, ErrDisconnected) {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("disconnect did not fail pending request promptly")
	}
}

func TestEventNamesAndRawPayload(t *testing.T) {
	events := make(chan Event, 7)
	c, peer := testClient(t, func(event Event) { events <- event })
	_ = c
	names := []string{"recv_msg", "revokeMsg", "friendReq", "transPay", "qrPay", "groupMemberChanges", "mmmojo_read"}
	for _, name := range names {
		if err := writePacket(peer, []byte(fmt.Sprintf(`{"event":%q,"data":{"msg_id":18446744073709551615}}`, name))); err != nil {
			t.Fatal(err)
		}
		select {
		case event := <-events:
			if event.Event != canonicalEvent(name) || string(event.Data) != `{"msg_id":18446744073709551615}` {
				t.Fatalf("invalid event: %+v", event)
			}
			if _, err := time.Parse(time.RFC3339Nano, event.Timestamp); err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("event not delivered")
		}
	}
}

func TestSelfAccountConcurrentAccess(t *testing.T) {
	c, _ := testClient(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.SetSelfWxid("wxid_test"); _ = c.SelfWxid() }()
	}
	wg.Wait()
}

func TestCapacityIsBounded(t *testing.T) {
	c, _ := testClient(t, nil)
	for i := 0; i < maxPendingRequests; i++ {
		c.slots <- struct{}{}
	}
	if _, err := c.Call(context.Background(), "sendText", nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected busy, got %v", err)
	}
}
