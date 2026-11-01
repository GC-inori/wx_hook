package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wxbot/pkg/protocol"
)

func TestLogsEndpointKeepsMostRecentEntriesAndIsPublic(t *testing.T) {
	s := testServer(t, "en-US", nil, "secret")
	for i := 0; i < maxLogEntries+5; i++ {
		s.AddLog("info", fmt.Sprintf("message-%03d", i))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, req)
	var response struct {
		Code   int        `json:"code"`
		Result []LogEntry `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || response.Code != http.StatusOK {
		t.Fatalf("logs endpoint status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(response.Result) != maxLogEntries {
		t.Fatalf("got %d log entries, want %d", len(response.Result), maxLogEntries)
	}
	if response.Result[0].Msg != "message-005" || response.Result[len(response.Result)-1].Msg != "message-504" {
		t.Fatalf("buffer did not retain newest entries: first=%q last=%q", response.Result[0].Msg, response.Result[len(response.Result)-1].Msg)
	}
	if response.Result[0].Level != "INFO" {
		t.Fatalf("unexpected normalized level: %q", response.Result[0].Level)
	}
	if _, err := time.Parse("15:04:05", response.Result[0].Time); err != nil {
		t.Fatalf("invalid log time %q: %v", response.Result[0].Time, err)
	}
}

func TestBotInfoEndpointIsPublicAndUsesLoginSnapshot(t *testing.T) {
	s := testServer(t, "en-US", nil, "secret")
	login := NewLoginState(":99", "/tmp/wx_bridge.sock", "127.0.0.1:8080")
	login.Update("logged_in", "wxid_test", "", "", "", 4321)
	s.SetLoginController(login)

	req := httptest.NewRequest(http.MethodGet, "/api/bot/info", nil)
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, req)
	var response struct {
		Code   int `json:"code"`
		Result struct {
			Name         string `json:"name"`
			Version      string `json:"version"`
			Wxid         string `json:"wxid"`
			UptimeSecs   int64  `json:"uptime_secs"`
			Display      string `json:"display"`
			PID          int    `json:"pid"`
			Socket       string `json:"socket"`
			Listen       string `json:"listen"`
			ActionsCount int    `json:"actions_count"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || response.Code != http.StatusOK {
		t.Fatalf("bot info endpoint status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	info := response.Result
	if info.Name != "WeChat Native Hook" || info.Version != "4.1.15.55" || info.Wxid != "wxid_test" ||
		info.Display != ":99" || info.PID != 4321 || info.Socket != "/tmp/wx_bridge.sock" ||
		info.Listen != "127.0.0.1:8080" || info.ActionsCount != len(protocol.Actions) || info.UptimeSecs < 0 {
		t.Fatalf("unexpected bot info: %+v", info)
	}
}

func TestAddLogPublishesWebSocketLogEvent(t *testing.T) {
	s := testServer(t, "en-US", nil, "")
	httpServer := httptest.NewServer(s)
	defer httpServer.Close()
	endpoint := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		t.Fatalf("websocket upgrade: %v", err)
	}
	defer conn.Close()

	s.AddLog("WARN", "log streaming works")
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event["post_type"] != "meta_event" || event["meta_event_type"] != "lifecycle" || event["sub_type"] != "log" || event["level"] != "WARN" || event["message"] != "log streaming works" {
		t.Fatalf("unexpected OneBot log event: %+v", event)
	}
	if _, err := time.Parse("15:04:05", event["log_time"].(string)); err != nil {
		t.Fatalf("invalid log event time %q: %v", event["log_time"], err)
	}
}
