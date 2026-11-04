package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wxbot/pkg/client"
	"wxbot/pkg/config"
	"wxbot/pkg/protocol"
)

func TestGatewayDefaultsAndClonePreserveQQCompat(t *testing.T) {
	cfg := gatewayDefaults(config.GatewayConfig{QQCompat: true})
	if !cfg.QQCompat {
		t.Fatal("gateway defaults disabled QQ compatibility")
	}
	cloned := cloneGatewayConfig(cfg)
	if !cloned.QQCompat {
		t.Fatal("gateway clone lost QQ compatibility")
	}
	configToValidate := validConfig("en-US")
	configToValidate.Gateway.QQCompat = cloned.QQCompat
	if err := configToValidate.Validate(); err != nil {
		t.Fatalf("configuration validation rejected QQ compatibility: %v", err)
	}
}

func TestConfiguredRequestTimeoutAndBodyLimit(t *testing.T) {
	var remaining time.Duration
	s := testServer(t, "en-US", fakeCaller{call: func(ctx context.Context, _ string, _ any) (json.RawMessage, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("caller context has no deadline")
		} else {
			remaining = time.Until(deadline)
		}
		return json.RawMessage(`{}`), nil
	}}, "")
	cfg := validConfig("en-US")
	cfg.Gateway.RequestTimeoutSec = 1
	cfg.Gateway.MaxRequestBodyMB = 1
	cfg.Gateway.APIToken = ""
	s.SetConfig(cfg, t.TempDir())

	status, response := request(t, s, http.MethodPost, `{"action":"get_login_info","params":{}}`)
	if status != http.StatusOK || response.Retcode != 0 {
		t.Fatalf("action with configured timeout failed: %+v", response)
	}
	if remaining <= 0 || remaining > time.Second {
		t.Fatalf("caller deadline remaining = %s, want at most one second", remaining)
	}

	body := strings.Repeat("x", 1024*1024+1)
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(body)))
	var oversized protocol.Response
	_ = json.Unmarshal(recorder.Body.Bytes(), &oversized)
	if recorder.Code != http.StatusOK || oversized.Retcode != 1400 {
		t.Fatalf("oversized body returned status %d retcode %d, want 200/1400", recorder.Code, oversized.Retcode)
	}
}

func TestWebSocketSettingsAndEventForwarding(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "")
	cfg := validConfig("en-US")
	cfg.Gateway.WSQueueSize = 16
	cfg.Gateway.WSPingIntervalSec = 5
	s.SetConfig(cfg, t.TempDir())
	if s.websocketQueueSize() != 16 || s.websocketPingInterval() != 5*time.Second {
		t.Fatalf("WebSocket options not applied: queue=%d ping=%s", s.websocketQueueSize(), s.websocketPingInterval())
	}

	peer := &subscriber{queue: make(chan []byte, s.websocketQueueSize()), actionQueue: make(chan []byte, s.websocketQueueSize()), done: make(chan struct{})}
	if !s.hub.add(peer) {
		t.Fatal("unable to register test subscriber")
	}
	for _, event := range []client.Event{
		{Event: "recvMsg", Data: json.RawMessage(`{"sender":"wxid_blocked","receiver":"room_allowed@chatroom"}`)},
		{Event: "recvMsg", Data: json.RawMessage(`{"sender":"friend","receiver":"room_denied@chatroom"}`)},
		{Event: "recvMsg", Data: json.RawMessage(`{"sender":"friend","receiver":"room_allowed@chatroom"}`)},
		{Event: "recvMsg", Data: json.RawMessage(`{"sender":"friend","receiver":"friend"}`)},
	} {
		s.Publish(event)
	}
	if len(peer.queue) != 4 {
		t.Fatalf("forwarded events = %d, want 4", len(peer.queue))
	}
}

func TestQQCompatWebhookReceivesTranslatedEvent(t *testing.T) {
	payloads := make(chan []byte, 1)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read webhook body: %v", err)
			return
		}
		payloads <- payload
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()

	server := testServer(t, "en-US", fakeCaller{}, "")
	cfg := validConfig("en-US")
	cfg.Gateway.QQCompat = true
	cfg.Bot.WeChatAccount = "wxid_self"
	cfg.Endpoints = []config.EndpointConfig{{ID: "compat-webhook", Type: "httpClient", URL: webhook.URL, Enabled: true}}
	server.SetConfig(cfg, t.TempDir())
	server.Publish(client.Event{Event: "recvMsg", Timestamp: "2026-10-31T12:00:00Z", Data: json.RawMessage(`{"msg_id":99,"from_wxid":"room@chatroom","final_from_wxid":"wxid_sender","text":"hello"}`)})

	select {
	case payload := <-payloads:
		var event map[string]any
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		if event["self_id"] != float64(server.idMapper.UserWxidToNum("wxid_self")) || event["user_id"] != float64(server.idMapper.UserWxidToNum("wxid_sender")) || event["group_id"] != float64(server.idMapper.GroupChatroomToNum("room@chatroom")) || event["message_id"] != float64(server.idMapper.MessageIDToNum(int64(99))) {
			t.Fatalf("webhook event IDs were not translated: %#v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook event")
	}
}

func TestOutboundDestinationIsForwardedWithoutBusinessFiltering(t *testing.T) {
	called := false
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		called = action == "sendText"
		return json.RawMessage(`{}`), nil
	}}, "")
	cfg := validConfig("en-US")
	cfg.Gateway.APIToken = ""
	server.SetConfig(cfg, t.TempDir())
	status, response := request(t, server, http.MethodPost, `{"action":"sendText","params":{"wxid":"wxid_target","msg":"hello"}}`)
	if status != http.StatusOK || response.Retcode != 0 || !called {
		t.Fatalf("destination was not forwarded: status=%d response=%+v called=%v", status, response, called)
	}
}

func TestConfigClonesAllowedOrigins(t *testing.T) {
	cfg := validConfig("en-US")
	cfg.Gateway.AllowedOrigins = []string{"https://allowed.example"}
	server := testServer(t, "en-US", fakeCaller{}, "")
	server.SetConfig(cfg, t.TempDir())
	cfg.Gateway.AllowedOrigins[0] = "https://mutated.example"
	stored := server.currentConfig()
	if stored.Gateway.AllowedOrigins[0] != "https://allowed.example" {
		t.Fatalf("stored config aliases caller slices: %+v", stored)
	}
	stored.Gateway.AllowedOrigins[0] = "https://read-mutated.example"
	if got := server.currentConfig().Gateway.AllowedOrigins[0]; got != "https://allowed.example" {
		t.Fatalf("returned config aliases server slices: %q", got)
	}
}

func TestGatewayDefaultsForDirectConstruction(t *testing.T) {
	s := New(nil, config.GatewayConfig{}, nil)
	defer s.Close()
	if s.requestTimeout() != 7*time.Second || s.requestBodyLimit() != 3*1024*1024 || s.websocketQueueSize() != 256 || s.websocketPingInterval() != 20*time.Second {
		t.Fatalf("unexpected direct-construction defaults: %+v", s.currentGatewayConfig())
	}
}
