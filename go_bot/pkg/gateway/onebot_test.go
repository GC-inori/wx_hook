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
)

func TestOneBotHTTPEnvelopeAndPathActions(t *testing.T) {
	calls := make(chan struct {
		action string
		params map[string]json.RawMessage
	}, 4)
	s := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		var params map[string]json.RawMessage
		_ = json.Unmarshal(data.(json.RawMessage), &params)
		calls <- struct {
			action string
			params map[string]json.RawMessage
		}{action, params}
		return json.RawMessage(`{"sequence":123}`), nil
	}}, "")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"action":"send_msg","params":{"user_id":"wxid_friend","message":"hello"},"echo":"echo-1"}`))
	s.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST / returned %d", recorder.Code)
	}
	var response struct {
		Status  string         `json:"status"`
		Retcode int            `json:"retcode"`
		Data    map[string]any `json:"data"`
		Echo    string         `json:"echo"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "ok" || response.Retcode != 0 || response.Data["message_id"] != float64(123) || response.Echo != "echo-1" {
		t.Fatalf("unexpected OneBot response: %s", recorder.Body.String())
	}
	first := <-calls
	if first.action != "sendText" || rawField(first.params, "target") != "wxid_friend" || rawField(first.params, "text") != "hello" {
		t.Fatalf("unexpected native text call: %s %#v", first.action, first.params)
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/send_group_msg", strings.NewReader(`{"group_id":"12345@chatroom","message":[{"type":"text","data":{"text":"Hi "}},{"type":"at","data":{"qq":"wxid_mentioned"}}]}`))
	s.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /send_group_msg returned %d", recorder.Code)
	}
	second := <-calls
	if second.action != "sendText2" || rawField(second.params, "target") != "12345@chatroom" || rawField(second.params, "text") != "Hi @wxid_mentioned\u2005" {
		t.Fatalf("unexpected native mention call: %s %#v", second.action, second.params)
	}
	var members []string
	if err := json.Unmarshal(second.params["members"], &members); err != nil || len(members) != 1 || members[0] != "wxid_mentioned" {
		t.Fatalf("mention list got %v, err=%v", members, err)
	}
}

func TestOneBotHTTPErrorEnvelopeAndAliases(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "")
	for _, tc := range []struct {
		path string
		body string
		want int
	}{{"/unknown_action", `{}`, 1404}, {"/send_msg", `{"user_id":"x"}`, 1400}} {
		recorder := httptest.NewRecorder()
		s.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
		var response map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusOK || response["status"] != "failed" || response["retcode"] != float64(tc.want) {
			t.Errorf("%s: code=%d body=%s", tc.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestOneBotEventFormattingFromNativeEvents(t *testing.T) {
	message := client.Event{Event: "recvMsg", Timestamp: "2026-10-28T10:00:00Z", Data: json.RawMessage(`{"msg_id":99,"from_wxid":"room@chatroom","final_from_wxid":"wxid_sender","text":"hello [CQ:at,qq=wxid_target]","nickname":"Alice"}`)}
	encoded, err := marshalWebsocketEvent(message, "wxid_self")
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(encoded, &event); err != nil {
		t.Fatal(err)
	}
	if event["post_type"] != "message" || event["message_type"] != "group" || event["self_id"] != "wxid_self" || event["group_id"] != "room@chatroom" || event["user_id"] != "wxid_sender" || event["message_id"] != float64(99) {
		t.Fatalf("unexpected OneBot message event: %s", encoded)
	}
	if event["raw_message"] != "hello [CQ:at,qq=wxid_target]" {
		t.Fatalf("raw message = %v", event["raw_message"])
	}
	segments, ok := event["message"].([]any)
	if !ok || len(segments) != 2 {
		t.Fatalf("event segments = %#v", event["message"])
	}

	recall := client.Event{Event: "revokeMsg", Data: json.RawMessage(`{"revoked_msg_id":"55","session":"room@chatroom"}`)}
	encoded, err = marshalWebsocketEvent(recall, "wxid_self")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &event); err != nil {
		t.Fatal(err)
	}
	if event["post_type"] != "notice" || event["notice_type"] != "group_recall" || event["message_id"] != float64(55) {
		t.Fatalf("unexpected recall event: %s", encoded)
	}
}

func TestOneBotWebSocketFullDuplexAction(t *testing.T) {
	calls := make(chan string, 1)
	s := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, _ any) (json.RawMessage, error) {
		calls <- action
		return json.RawMessage(`{"sequence":321}`), nil
	}}, "")
	server := httptest.NewServer(s)
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, response, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		t.Fatalf("websocket upgrade failed: %v (%+v)", err, response)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := conn.WriteJSON(map[string]any{"action": "send_private_msg", "params": map[string]any{"user_id": "wxid_friend", "message": "hi"}, "echo": "ws-1"}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status  string         `json:"status"`
		Retcode int            `json:"retcode"`
		Data    map[string]any `json:"data"`
		Echo    string         `json:"echo"`
	}
	if err := conn.ReadJSON(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.Retcode != 0 || result.Echo != "ws-1" || result.Data["message_id"] != float64(321) {
		t.Fatalf("unexpected full-duplex response: %+v", result)
	}
	if got := <-calls; got != "sendText" {
		t.Fatalf("native action = %q", got)
	}
}

func rawField(params map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(params[key], &value)
	return value
}
