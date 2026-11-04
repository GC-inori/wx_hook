package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wxbot/pkg/client"
	"wxbot/pkg/config"
	"wxbot/pkg/i18n"
	"wxbot/pkg/protocol"
)

type fakeCaller struct {
	call func(context.Context, string, any) (json.RawMessage, error)
}

func (f fakeCaller) Call(ctx context.Context, action string, data any) (json.RawMessage, error) {
	if f.call == nil {
		return json.RawMessage(`{"ok":true}`), nil
	}
	return f.call(ctx, action, data)
}

func testServer(t *testing.T, language string, caller Caller, token string) *Server {
	t.Helper()
	catalog, err := i18n.Load(filepath.Join("..", "..", ".."), language)
	if err != nil {
		t.Fatal(err)
	}
	s := New(caller, config.GatewayConfig{Locale: language, APIToken: token, MaxConnections: 2}, catalog)
	t.Cleanup(s.Close)
	return s
}

func request(t *testing.T, s *Server, method, body string) (int, protocol.Response) {
	t.Helper()
	req := httptest.NewRequest(method, "/api", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, req)
	var response protocol.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid response %s: %v", recorder.Body.String(), err)
	}
	return recorder.Code, response
}

func TestEntireActionMatrixAndRawArguments(t *testing.T) {
	var expected string
	s := testServer(t, "en-US", fakeCaller{call: func(ctx context.Context, action string, data any) (json.RawMessage, error) {
		if action != expected {
			t.Errorf("action got %s want %s", action, expected)
		}
		raw, ok := data.(json.RawMessage)
		if !ok || (string(raw) != `{"wxid":"wxid_test","msg_id":18446744073709551615}` && string(raw) != `{"msg_id":18446744073709551615,"wxid":"wxid_test"}`) {
			t.Errorf("raw parameters changed: %s", raw)
		}
		return json.RawMessage(`{"msg_id":"18446744073709551615"}`), nil
	}}, "")
	if len(protocol.Actions) != 36 {
		t.Fatalf("action matrix changed: %d", len(protocol.Actions))
	}
	for _, action := range protocol.Actions {
		expected = action
		status, response := request(t, s, "POST", `{"action":"`+action+`","params":{"wxid":"wxid_test","msg_id":18446744073709551615}}`)
		if status != 200 || response.Retcode != 0 || response.Status != "ok" {
			t.Fatalf("%s: %+v", action, response)
		}
	}
}

func TestValidationAndMethodErrors(t *testing.T) {
	called := false
	s := testServer(t, "en-US", fakeCaller{call: func(context.Context, string, any) (json.RawMessage, error) { called = true; return nil, nil }}, "")
	for _, tc := range []struct {
		method, body string
		retcode      int
	}{
		{"GET", "", 1400},
		{"POST", "{", 1400},
		{"POST", `{"action":"sendText","params":[]}`, 1400},
		{"POST", `{"action":"sendText","params":null}`, 1400},
		{"POST", `{"action":"sendText"} {}`, 1400},
		{"POST", `{"action":"unknown","params":{}}`, 1404},
		{"POST", `{"action":"sendText","params":{"text":"` + strings.Repeat("a", int(s.requestBodyLimit())) + `"}}`, 1400},
	} {
		status, result := request(t, s, tc.method, tc.body)
		if status != 200 || result.Retcode != tc.retcode {
			t.Errorf("got status %d retcode %d want retcode %d", status, result.Retcode, tc.retcode)
		}
	}
	if called {
		t.Fatal("invalid request reached native bridge")
	}
}

func TestLocalesAndNativeErrors(t *testing.T) {
	for _, tc := range []struct{ language, message string }{{"zh-CN", "当前版本尚未支持此接口"}, {"en-US", "Action not supported in current version"}} {
		s := testServer(t, tc.language, fakeCaller{call: func(context.Context, string, any) (json.RawMessage, error) {
			return nil, &client.BridgeError{Status: 501, Code: 501, Message: "offset unresolved", Result: json.RawMessage(`{"implemented":false,"action":"createGroup"}`)}
		}}, "")
		status, result := request(t, s, "POST", `{"action":"createGroup","params":{}}`)
		if status != 200 || result.Retcode != 1405 || result.Message != tc.message {
			t.Fatalf("localized error: %+v", result)
		}
	}
}

func TestTimeoutAndBusyErrors(t *testing.T) {
	for _, tc := range []struct {
		err     error
		retcode int
	}{{context.DeadlineExceeded, 1500}, {context.Canceled, 1500}, {client.ErrBusy, 1409}, {client.ErrDisconnected, 1500}, {errors.New("I/O failure"), 1500}} {
		s := testServer(t, "en-US", fakeCaller{call: func(context.Context, string, any) (json.RawMessage, error) { return nil, tc.err }}, "")
		status, result := request(t, s, "POST", `{"action":"sendText","params":{}}`)
		if status != 200 || result.Retcode != tc.retcode {
			t.Fatalf("unexpected error response: %+v", result)
		}
	}
}

func validConfig(locale string) *config.Config {
	return &config.Config{
		Bot:      config.BotConfig{WeChatAccount: "wxid_test", IpcSocket: "/tmp/bridge.sock", ReconnectIntervalSec: 3},
		Logging:  config.LoggingConfig{Dir: "logs", Level: "info", ToConsole: true, MaxSizeMB: 50, MaxBackups: 7, LogBufferSize: 500},
		Gateway:  config.GatewayConfig{Listen: "127.0.0.1:8080", Locale: locale, APIToken: "test-token", MaxConnections: 8, RequestTimeoutSec: 7, MaxRequestBodyMB: 3, WSQueueSize: 256, WSPingIntervalSec: 20},
		Launcher: config.LauncherConfig{AutoStart: true, BridgeLib: "./librust_bridge.so", Display: ":99", UseXvfb: true, XvfbScreen: "1280x1024x24", QRPort: 8899, QRBind: "127.0.0.1", LoginTimeout: 120, BridgeWaitTimeout: 30, XvfbWaitTimeout: 5},
	}
}

func TestConfigGetAndPost(t *testing.T) {
	root := t.TempDir()
	s := testServer(t, "en-US", fakeCaller{}, "test-token")
	initial := validConfig("en-US")
	initial.RootDir = root
	s.SetConfig(initial, root)

	get := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	get.Header.Set("Authorization", "Bearer test-token")
	getRecorder := httptest.NewRecorder()
	s.ServeHTTP(getRecorder, get)
	var getResponse Response
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &getResponse); err != nil {
		t.Fatal(err)
	}
	if getRecorder.Code != http.StatusOK || getResponse.Code != http.StatusOK {
		t.Fatalf("GET /api/config failed: %d %s", getRecorder.Code, getRecorder.Body.String())
	}
	gotConfig, ok := getResponse.Result.(map[string]any)
	if !ok || gotConfig["gateway"] == nil || gotConfig["policy"] == nil {
		t.Fatalf("GET did not return the complete configuration: %+v", getResponse.Result)
	}

	updated := validConfig("en-US")
	updated.Gateway.APIToken = "next-token"
	updated.Gateway.AllowedOrigins = []string{"https://dashboard.example"}
	updated.Policy = config.PolicyConfig{
		MediaDir:         "media",
		RateLimitEnabled: true, MaxMessagesPerSec: 4, TargetMinIntervalMs: 40, JitterMinMs: 10, JitterMaxMs: 20, AntiLoopWindowSec: 2,
	}
	updated.Endpoints = []config.EndpointConfig{{ID: "ep-1", Name: "webhook", Type: "httpClient", URL: "http://127.0.0.1:9999/hook", Enabled: true}}
	body, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	post := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(string(body)))
	post.Header.Set("Authorization", "Bearer test-token")
	postRecorder := httptest.NewRecorder()
	s.ServeHTTP(postRecorder, post)
	var postResponse Response
	if err := json.Unmarshal(postRecorder.Body.Bytes(), &postResponse); err != nil {
		t.Fatal(err)
	}
	if postRecorder.Code != http.StatusOK || postResponse.Code != http.StatusOK {
		t.Fatalf("POST /api/config failed: %d %s", postRecorder.Code, postRecorder.Body.String())
	}
	if s.currentGatewayConfig().APIToken != "next-token" || len(s.currentConfig().Endpoints) != 1 {
		t.Fatalf("runtime configuration was not updated: %+v", s.currentConfig())
	}
	s.pacer.mu.Lock()
	activePolicy := s.pacer.policy
	s.pacer.mu.Unlock()
	if !reflect.DeepEqual(activePolicy, updated.Policy) {
		t.Fatalf("pacer policy was not updated: got %+v, want %+v", activePolicy, updated.Policy)
	}
	loaded, err := config.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Gateway.APIToken != "next-token" || loaded.Policy.MediaDir != "media" || len(loaded.Endpoints) != 1 {
		t.Fatalf("saved configuration was not updated: %+v", loaded)
	}

	unauthorized := httptest.NewRecorder()
	s.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("config endpoint did not enforce current token: %d", unauthorized.Code)
	}
}

func TestConfigMethodAndValidationErrors(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "")
	cfg := validConfig("en-US")
	cfg.Gateway.APIToken = ""
	s.SetConfig(cfg, t.TempDir())
	for _, tc := range []struct {
		method string
		body   string
		want   int
	}{{http.MethodDelete, "", http.StatusMethodNotAllowed}, {http.MethodPost, "{", http.StatusBadRequest}, {http.MethodPost, `{"gateway":{"listen":"127.0.0.1:8080","locale":"invalid","max_connections":2}}`, http.StatusBadRequest}} {
		recorder := httptest.NewRecorder()
		s.ServeHTTP(recorder, httptest.NewRequest(tc.method, "/api/config", strings.NewReader(tc.body)))
		if recorder.Code != tc.want {
			t.Errorf("%s /api/config returned %d, want %d", tc.method, recorder.Code, tc.want)
		}
	}
}

func TestWebhookPushesEventAsynchronously(t *testing.T) {
	received := make(chan struct {
		body  []byte
		token string
		type_ string
	}, 1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read webhook body: %v", err)
		}
		received <- struct {
			body  []byte
			token string
			type_ string
		}{body: body, token: r.Header.Get("Authorization"), type_: r.Header.Get("Content-Type")}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hook.Close()

	s := testServer(t, "en-US", fakeCaller{}, "")
	cfg := validConfig("en-US")
	cfg.Endpoints = []config.EndpointConfig{{ID: "hook-1", Type: "httpClient", URL: hook.URL, Token: "webhook-secret", Enabled: true}}
	s.SetConfig(cfg, t.TempDir())
	started := time.Now()
	s.Publish(client.Event{Event: "recvMsg", Data: json.RawMessage(`{"msg":"hello"}`), Timestamp: "2026-10-27T12:00:00Z"})
	if time.Since(started) > time.Second {
		t.Fatal("Publish blocked while sending webhook")
	}
	select {
	case result := <-received:
		if result.token != "Bearer webhook-secret" || result.type_ != "application/json" {
			t.Fatalf("unexpected webhook headers: %+v", result)
		}
		var event map[string]any
		if err := json.Unmarshal(result.body, &event); err != nil {
			t.Fatal(err)
		}
		if event["post_type"] != "message" || event["message_type"] != "private" || event["raw_message"] != "hello" {
			t.Fatalf("unexpected OneBot webhook event: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook event was not delivered")
	}
}

func TestAuthentication(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{call: func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{"wxid":"self","nickname":"me"}`), nil
	}}, "secret")
	req := httptest.NewRequest("POST", "/api", strings.NewReader(`{"action":"get_login_info"}`))
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, req)
	var authFailed protocol.Response
	_ = json.Unmarshal(recorder.Body.Bytes(), &authFailed)
	if recorder.Code != http.StatusOK || authFailed.Retcode != 1401 {
		t.Fatalf("missing token: status %d retcode %d", recorder.Code, authFailed.Retcode)
	}
	req = httptest.NewRequest("POST", "/api", strings.NewReader(`{"action":"get_login_info"}`))
	req.Header.Set("Authorization", "Bearer secret")
	recorder = httptest.NewRecorder()
	s.ServeHTTP(recorder, req)
	var authOK protocol.Response
	_ = json.Unmarshal(recorder.Body.Bytes(), &authOK)
	if recorder.Code != http.StatusOK || authOK.Retcode != 0 {
		t.Fatalf("valid token failed: status %d retcode %d", recorder.Code, authOK.Retcode)
	}

	for _, path := range []string{"/", "/dashboard", "/index.html", "/api/login/status", "/healthz"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		recorder = httptest.NewRecorder()
		s.ServeHTTP(recorder, req)
		if recorder.Code == http.StatusUnauthorized {
			t.Errorf("public WebUI path %s unexpectedly required a token", path)
		}
	}
}

func TestLoginStatusEnvelopeAndDisconnectedCaller(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "secret")
	s.SetLoginController(NewLoginState(":99", "/tmp/wx_bridge.sock", "127.0.0.1:8080"))
	req := httptest.NewRequest(http.MethodGet, "/api/login/status", nil)
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, req)
	var status struct {
		Code   int            `json:"code"`
		Msg    string         `json:"msg"`
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || status.Code != http.StatusOK || status.Msg != "success" || status.Result["status"] != "launching" {
		t.Fatalf("unexpected login status response: %s", recorder.Body.String())
	}

	s.SetCaller(disconnectedCaller{})
	req = httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{"action":"get_login_info","params":{}}`))
	req.Header.Set("Authorization", "Bearer secret")
	recorder = httptest.NewRecorder()
	s.ServeHTTP(recorder, req)
	var response protocol.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || response.Retcode != 1401 {
		t.Fatalf("disconnected caller was accepted: %+v", response)
	}
}

type disconnectedCaller struct{}

func (disconnectedCaller) Call(context.Context, string, any) (json.RawMessage, error) {
	return nil, errors.New("unexpected call")
}

func (disconnectedCaller) Connected() bool { return false }

func TestWebSocketAllEventsAndShutdown(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "secret")
	httpServer := httptest.NewServer(s)
	defer httpServer.Close()
	endpoint := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Authorization": []string{"Bearer secret"}})
	if err != nil {
		t.Fatalf("websocket upgrade: %v %+v", err, resp)
	}
	defer conn.Close()
	for _, name := range []string{"recvMsg", "revokeMsg", "friendReq", "transPay", "hongBao", "qrPay", "groupMemberChanges"} {
		s.Publish(client.Event{Event: name, Data: json.RawMessage(`{"msg_id":"18446744073709551615"}`)})
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var event map[string]any
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatal(err)
		}
		if event["post_type"] == nil || event["time"] == nil || event["self_id"] == nil {
			t.Fatalf("invalid OneBot event for %s: %+v", name, event)
		}
		wantPostType := "notice"
		if name == "recvMsg" {
			wantPostType = "message"
		} else if name == "friendReq" {
			wantPostType = "request"
		}
		if event["post_type"] != wantPostType {
			t.Fatalf("event %s has post_type %v, want %s", name, event["post_type"], wantPostType)
		}
	}
	s.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("shutdown left websocket open")
	}
}

func TestWebSocketRejectsOriginAndMissingToken(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "secret")
	server := httptest.NewServer(s)
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	for _, tc := range []struct {
		headers http.Header
		status  int
	}{
		{http.Header{}, 401},
		{http.Header{"Authorization": []string{"Bearer secret"}, "Origin": []string{"https://evil.example"}}, 403},
	} {
		conn, resp, err := websocket.DefaultDialer.Dial(endpoint, tc.headers)
		if conn != nil {
			_ = conn.Close()
		}
		if err == nil || resp == nil || resp.StatusCode != tc.status {
			t.Fatalf("unexpected upgrade: %v %+v", err, resp)
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
}

func TestSlowWebSocketConsumerIsBounded(t *testing.T) {
	hub := newEventHub(1)
	defer hub.close()
	queueSize := 16
	peer := &subscriber{queue: make(chan []byte, queueSize), done: make(chan struct{})}
	if !hub.add(peer) {
		t.Fatal("first subscriber rejected")
	}
	if hub.add(&subscriber{}) {
		t.Fatal("connection limit ignored")
	}
	for i := 0; i <= queueSize; i++ {
		hub.publish(client.Event{Event: "recvMsg", Data: json.RawMessage(`{}`)})
	}
	select {
	case <-peer.done:
	default:
		t.Fatal("slow consumer not disconnected")
	}
	if len(peer.queue) != queueSize {
		t.Fatal("queue bound changed")
	}
}
