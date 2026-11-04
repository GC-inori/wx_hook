package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"wxbot/pkg/client"
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

func TestSetFriendAddRequestRejectDoesNotCallNative(t *testing.T) {
	called := false
	server := testServer(t, "en-US", fakeCaller{call: func(context.Context, string, any) (json.RawMessage, error) {
		called = true
		return nil, nil
	}}, "")
	server.Publish(client.Event{
		Event: "friendReq",
		Data:  json.RawMessage(`{"applicant":{"wxid":"wxid_requester","ticket":"ticket-reject"}}`),
	})

	status, response := request(t, server, http.MethodPost, `{"action":"set_friend_add_request","params":{"flag":"ticket-reject","approve":false}}`)
	if status != http.StatusOK || response.Retcode != 1405 {
		t.Fatalf("unsupported rejection response: status=%d response=%+v", status, response)
	}
	if called {
		t.Fatal("rejected friend request unexpectedly reached native bridge")
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
