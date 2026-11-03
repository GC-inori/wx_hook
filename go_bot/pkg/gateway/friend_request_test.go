package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"wxbot/pkg/client"
	"wxbot/pkg/config"
)

func TestSetFriendAddRequestUsesCachedRequestDetails(t *testing.T) {
	var action string
	var raw json.RawMessage
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, calledAction string, data any) (json.RawMessage, error) {
		action = calledAction
		raw, _ = data.(json.RawMessage)
		return json.RawMessage(`{"agreed":true}`), nil
	}}, "")
	server.Publish(client.Event{
		Event: "friendReq",
		Data:  json.RawMessage(`{"applicant":{"wxid":"wxid_requester","ticket":"ticket-1","encryptusername":"encrypt-1","scene":"7"}}`),
	})

	status, response := request(t, server, http.MethodPost, `{"action":"set_friend_add_request","params":{"flag":"ticket-1","remark":"welcome"}}`)
	if status != http.StatusOK || response.Retcode != 0 {
		t.Fatalf("set_friend_add_request failed: status=%d response=%+v", status, response)
	}
	if action != "agreeFriendReq" {
		t.Fatalf("native action is %q, want agreeFriendReq", action)
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"wxid": "wxid_requester", "ticket": "ticket-1", "encryptusername": "encrypt-1",
		"remark": "welcome", "scene": float64(7),
	} {
		if params[key] != want {
			t.Errorf("native parameter %s = %#v, want %#v", key, params[key], want)
		}
	}
}

func TestAutoAgreeFriendRequestSendsWelcomeAfterSuccess(t *testing.T) {
	type call struct {
		action string
		data   json.RawMessage
	}
	calls := make(chan call, 4)
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		raw, _ := data.(json.RawMessage)
		calls <- call{action: action, data: raw}
		return json.RawMessage(`{"ok":true}`), nil
	}}, "")
	cfg := config.Config{
		Gateway: config.GatewayConfig{Locale: "en-US", MaxConnections: 2, RequestTimeoutSec: 2, WSQueueSize: 16, WSPingIntervalSec: 20},
		Policy: config.PolicyConfig{
			AutoAgreeFriend: true, FriendWelcomeEnabled: true, FriendWelcomeMessage: "欢迎添加！",
			RateLimitEnabled: false,
		},
	}
	server.SetConfig(&cfg, t.TempDir())
	server.Publish(client.Event{
		Event: "friendReq",
		Data:  json.RawMessage(`{"applicant":{"wxid":"wxid_new_friend","ticket":"ticket-2","encryptusername":"encrypt-2","scene":"6"}}`),
	})

	seenAgree, seenWelcome := false, false
	deadline := time.After(3 * time.Second)
	for !seenAgree || !seenWelcome {
		select {
		case got := <-calls:
			var params map[string]any
			if err := json.Unmarshal(got.data, &params); err != nil {
				t.Fatal(err)
			}
			switch got.action {
			case "agreeFriendReq":
				seenAgree = true
				if params["wxid"] != "wxid_new_friend" || params["ticket"] != "ticket-2" || params["encryptusername"] != "encrypt-2" || params["scene"] != float64(6) {
					t.Errorf("unexpected auto-agree arguments: %#v", params)
				}
			case "sendText":
				seenWelcome = true
				if params["target"] != "wxid_new_friend" || params["text"] != "欢迎添加！" {
					t.Errorf("unexpected welcome message: %#v", params)
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for agree and welcome calls; agree=%v welcome=%v", seenAgree, seenWelcome)
		}
	}
}

func TestFriendAddedSystemMessageDetectionAndOneBotNotice(t *testing.T) {
	event := client.Event{
		Event: "systemMsg",
		Data:  json.RawMessage(`{"msg_type":10000,"sender":"wxid_bot","receiver":"wxid_new","notification":"你已添加了好友，现在可以开始聊天了"}`),
	}
	wxid, matched := friendAddedSystemMessage(event, "wxid_bot")
	if !matched || wxid != "wxid_new" {
		t.Fatalf("friend-added system message = (%q, %v), want (wxid_new, true)", wxid, matched)
	}
	notice := formatOneBotEvent(client.Event{Event: "friendAdded", Data: json.RawMessage(`{"wxid":"wxid_new"}`)}, "wxid_bot")
	if notice["post_type"] != "notice" || notice["notice_type"] != "friend_add" || notice["user_id"] != "wxid_new" {
		t.Fatalf("unexpected OneBot friend_add notice: %#v", notice)
	}
}

func TestRawParamSceneAcceptsJSONAndStringNumbers(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`8`), json.RawMessage(`"8"`)} {
		got, ok := rawParamScene(raw)
		if !ok || got != 8 {
			t.Errorf("rawParamScene(%s) = (%d, %v), want (8, true)", raw, got, ok)
		}
	}
}
