package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wxbot/pkg/client"
	"wxbot/pkg/config"
	"wxbot/pkg/protocol"
)

func TestWebSocketActionCallsEchoAndLargePayload(t *testing.T) {
	type callRecord struct {
		action string
		params string
	}
	calls := make(chan callRecord, 2)
	s := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, args any) (json.RawMessage, error) {
		params, _ := args.(json.RawMessage)
		calls <- callRecord{action: action, params: string(params)}
		return json.RawMessage(`{"ok":true}`), nil
	}}, "secret")
	server := httptest.NewServer(s)
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, response, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Authorization": []string{"Bearer secret"}})
	if err != nil {
		t.Fatalf("websocket upgrade failed: %v (%+v)", err, response)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"send_text","params":{"msg_id":18446744073709551615},"echo":"onebot-echo"}`)); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status  string `json:"status"`
		Retcode int    `json:"retcode"`
		Echo    string `json:"echo"`
	}
	if err := conn.ReadJSON(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.Retcode != 0 || result.Echo != "onebot-echo" {
		t.Fatalf("unexpected OneBot response: %+v", result)
	}
	first := <-calls
	if first.action != "sendText" || first.params != `{"msg_id":18446744073709551615}` {
		t.Fatalf("OneBot request was not canonicalized/preserved: %+v", first)
	}

	largeData := strings.Repeat("x", 2048)
	payload := []byte(`{"action":"sendFile","params":{"file_data":"` + largeData + `"},"echo":"ws-echo"}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	var wsResult protocol.Response
	if err := conn.ReadJSON(&wsResult); err != nil {
		t.Fatal(err)
	}
	if wsResult.Status != "ok" || wsResult.Retcode != 0 || wsResult.Echo != "ws-echo" {
		t.Fatalf("unexpected ws response: %+v", wsResult)
	}
	second := <-calls
	if second.action != "sendFile" || !strings.Contains(second.params, largeData) {
		t.Fatalf("large request payload was changed: action=%q payload bytes=%d", second.action, len(second.params))
	}
	if got := s.sendCount.Load(); got != 2 {
		t.Fatalf("send count got %d, want 2", got)
	}
}

func TestReverseWSClientPublishesEventsAndExecutesActions(t *testing.T) {
	connected := make(chan *websocket.Conn, 1)
	authorization := make(chan string, 1)
	holdConnection := make(chan struct{})
	upgrader := websocket.Upgrader{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connected <- conn
		<-holdConnection
		_ = conn.Close()
	}))

	calls := make(chan string, 1)
	s := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, _ any) (json.RawMessage, error) {
		calls <- action
		return json.RawMessage(`{"accepted":true}`), nil
	}}, "")
	cfg := validConfig("en-US")
	cfg.Endpoints = []config.EndpointConfig{{ID: "reverse-1", Type: "wsClient", URL: "ws" + strings.TrimPrefix(remote.URL, "http"), Token: "reverse-secret", Heartbeat: 200, Enabled: true}}
	s.SetConfig(cfg, t.TempDir())

	var conn *websocket.Conn
	select {
	case conn = <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("reverse WebSocket did not connect")
	}
	defer func() {
		_ = conn.Close()
		s.Close()
		close(holdConnection)
		remote.Close()
	}()
	select {
	case got := <-authorization:
		if got != "Bearer reverse-secret" {
			t.Fatalf("Authorization header got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("reverse WebSocket handshake was not observed")
	}
	waitForReverseConnection(t, s)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	s.Publish(client.Event{Event: "recvMsg", Data: json.RawMessage(`{"text":"event"}`)})
	var event struct {
		PostType    string `json:"post_type"`
		MessageType string `json:"message_type"`
		RawMessage  string `json:"raw_message"`
	}
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("read forwarded event: %v", err)
	}
	if event.PostType != "message" || event.MessageType != "private" || event.RawMessage != "event" {
		t.Fatalf("unexpected reverse WebSocket event: %+v", event)
	}

	if err := conn.WriteJSON(map[string]any{"action": "get_self_info", "params": map[string]any{}, "echo": "remote-echo"}); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Status  string `json:"status"`
		Retcode int    `json:"retcode"`
		Echo    string `json:"echo"`
	}
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatalf("read reverse action response: %v", err)
	}
	if response.Status != "ok" || response.Retcode != 0 || response.Echo != "remote-echo" {
		t.Fatalf("unexpected reverse action response: %+v", response)
	}
	if action := <-calls; action != "getSelfInfo" {
		t.Fatalf("remote action got %q, want getSelfInfo", action)
	}

	cfg.Endpoints = nil
	s.SetConfig(cfg, t.TempDir())
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("removed reverse WebSocket endpoint remained connected")
	}
}

func waitForReverseConnection(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.wsClientsMu.Lock()
		connected := false
		for _, endpoint := range s.wsClients {
			connected = connected || endpoint.connection()
		}
		s.wsClientsMu.Unlock()
		if connected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("reverse WebSocket session did not become active")
}

func TestStatusEndpointsExposeRuntimeMetrics(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "")
	s.SetLoginController(NewLoginState(":99", "/tmp/bridge.sock", "127.0.0.1:8080"))
	s.Publish(client.Event{Event: "recvMsg", Data: json.RawMessage(`{}`)})
	if status, _ := request(t, s, http.MethodPost, `{"action":"get_login_info","params":{}}`); status != http.StatusOK {
		t.Fatalf("action request returned %d", status)
	}

	for _, endpoint := range []string{"/api/status", "/api/login/status"} {
		recorder := httptest.NewRecorder()
		s.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, endpoint, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d", endpoint, recorder.Code)
		}
		var envelope struct {
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"cpu_usage_system", "cpu_usage_process", "mem_total_mb", "mem_used_mb", "mem_process_mb", "goroutines", "recv_count", "send_count", "ws_connections", "dropped_events"} {
			if _, ok := envelope.Result[key]; !ok {
				t.Errorf("GET %s omitted %q: %+v", endpoint, key, envelope.Result)
			}
		}
		if envelope.Result["recv_count"] != float64(1) || envelope.Result["send_count"] != float64(1) {
			t.Errorf("GET %s returned incorrect counters: %+v", endpoint, envelope.Result)
		}
	}
}
