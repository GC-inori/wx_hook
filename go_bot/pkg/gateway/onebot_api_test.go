package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"wxbot/pkg/client"
)

func TestQQCompatActionsReverseIDsAndIgnoreReplies(t *testing.T) {
	type nativeCall struct {
		action string
		params json.RawMessage
	}
	calls := make(chan nativeCall, 1)
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		calls <- nativeCall{action: action, params: data.(json.RawMessage)}
		return json.RawMessage(`{"sequence":123}`), nil
	}}, "")
	cfg := server.currentConfig()
	cfg.Gateway.QQCompat = true
	server.SetConfig(&cfg, t.TempDir())
	userID := server.idMapper.UserWxidToNum("wxid_destination")
	messageID := server.idMapper.MessageIDToNum("native-reply-id")
	if userID != firstUserID || messageID != firstMessageID {
		t.Fatalf("unexpected initial IDs: user=%d message=%d", userID, messageID)
	}

	status, response := request(t, server, http.MethodPost, `{"action":"send_msg","params":{"user_id":100000001,"message":[{"type":"reply","data":{"id":1000001}},{"type":"text","data":{"text":"hello"}}]}}`)
	if status != http.StatusOK || response.Retcode != 0 {
		t.Fatalf("reply segment caused send failure: status=%d response=%+v", status, response)
	}
	call := <-calls
	var params map[string]json.RawMessage
	if err := json.Unmarshal(call.params, &params); err != nil {
		t.Fatal(err)
	}
	var target, text string
	_ = json.Unmarshal(params["target"], &target)
	_ = json.Unmarshal(params["text"], &text)
	if call.action != "sendText" || target != "wxid_destination" || text != "hello" {
		t.Fatalf("QQ-compatible action was not reversed correctly: action=%s params=%s", call.action, call.params)
	}
	if response.Data.(map[string]any)["message_id"] != float64(firstMessageID+1) {
		t.Fatalf("send_msg response message ID was not mapped: %#v", response.Data)
	}

	for _, action := range []string{"get_group_root_files", "get_group_files_by_folder", "get_group_file_url"} {
		status, response = request(t, server, http.MethodPost, `{"action":"`+action+`","params":{}}`)
		data, ok := response.Data.(map[string]any)
		files, filesOK := data["files"].([]any)
		folders, foldersOK := data["folders"].([]any)
		if status != http.StatusOK || response.Retcode != 0 || !ok || !filesOK || !foldersOK || len(files) != 0 || len(folders) != 0 {
			t.Fatalf("unexpected %s response: status=%d response=%+v", action, status, response)
		}
	}
}

func TestContactAndPaymentActionsForwardToNativeBridge(t *testing.T) {
	type nativeCall struct {
		action string
		params map[string]json.RawMessage
	}
	calls := make(chan nativeCall, 8)
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		var params map[string]json.RawMessage
		if err := json.Unmarshal(data.(json.RawMessage), &params); err != nil {
			t.Errorf("invalid native parameters: %v", err)
		}
		calls <- nativeCall{action: action, params: params}
		return json.RawMessage(`{"ok":true}`), nil
	}}, "")

	for _, tc := range []struct {
		action string
		params string
		want   string
	}{
		{"confirm_trans", `{"transfer_id":"tr-1","invalid_time":"1730000000"}`, "confirmTrans"},
		{"confirmTrans", `{"trans_id":"tr-2","invalid_time":"1730000001"}`, "confirmTrans"},
		{"receive_hong_bao", `{"send_id":"send-1","channel_id":"1","native_url":"wxpay://hb","session_wxid":"wxid_sender","in_way":1}`, "receiveHongBao"},
		{"openHongBao", `{"channel_id":"1","native_url":"wxpay://hb","session_wxid":"wxid_sender","timing_identifier":"time-1","nick_name":"Bot"}`, "openHongBao"},
		{"delete_friend", `{"user_id":"wxid_target"}`, "delFriend"},
		{"del_friend", `{"user_id":"wxid_target"}`, "delFriend"},
		{"delFriend", `{"wxid":"wxid_target"}`, "delFriend"},
	} {
		status, response := request(t, server, http.MethodPost, `{"action":"`+tc.action+`","params":`+tc.params+`}`)
		if status != http.StatusOK || response.Retcode != 0 {
			t.Fatalf("%s returned status=%d response=%+v", tc.action, status, response)
		}
		call := <-calls
		if call.action != tc.want {
			t.Errorf("%s dispatched %q, want %q", tc.action, call.action, tc.want)
		}
		if tc.want == "delFriend" && rawParamString(call.params, "wxid") != "wxid_target" {
			t.Errorf("%s did not map user_id to wxid: %#v", tc.action, call.params)
		}
	}
}

func TestDatabaseAndHistoryActionsRouteThroughNativeQueryObject(t *testing.T) {
	type nativeCall struct {
		action string
		params map[string]json.RawMessage
	}
	calls := make(chan nativeCall, 8)
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		params := make(map[string]json.RawMessage)
		if err := json.Unmarshal(data.(json.RawMessage), &params); err != nil {
			return nil, err
		}
		calls <- nativeCall{action: action, params: params}
		return json.RawMessage(`{"key":"aabb","page_size":4096}`), nil
	}}, "")
	groupID := server.idMapper.GroupChatroomToNum("room@chatroom")
	userID := server.idMapper.UserWxidToNum("wxid_friend")
	messageID := server.idMapper.MessageIDToNum("native-msg-1")

	result := server.dispatchOneBotAction(context.Background(), "get_db_key", json.RawMessage(`{}`))
	if result.retcode != 0 {
		t.Fatalf("get_db_key returned retcode %d: %s", result.retcode, result.message)
	}
	if call := <-calls; call.action != "get_db_key" {
		t.Fatalf("get_db_key dispatched %q", call.action)
	}

	for _, test := range []struct {
		action string
		params string
		chatID string
		group  string
		user   string
	}{
		{"get_history_msg", `{"chat_id":200000001,"count":20,"message_id":1000001}`, "room@chatroom", "", ""},
		{"get_chat_history", `{"user_id":100000001,"count":10,"message_id":1000001}`, "wxid_friend", "", "wxid_friend"},
		{"get_group_msg_history", `{"group_id":200000001,"count":5,"message_id":1000001}`, "room@chatroom", "room@chatroom", ""},
	} {
		result := server.dispatchOneBotAction(context.Background(), test.action, json.RawMessage(test.params))
		if result.retcode != 0 {
			t.Fatalf("%s returned retcode %d: %s", test.action, result.retcode, result.message)
		}
		call := <-calls
		if call.action != "queryObj" {
			t.Fatalf("%s dispatched %q, want queryObj", test.action, call.action)
		}
		if got := rawParamString(call.params, "chat_id"); got != test.chatID {
			t.Errorf("%s chat_id = %q, want %q", test.action, got, test.chatID)
		}
		if test.group != "" && rawParamString(call.params, "group_id") != test.group {
			t.Errorf("%s group_id = %q, want %q", test.action, rawParamString(call.params, "group_id"), test.group)
		}
		if test.user != "" && rawParamString(call.params, "user_id") != test.user {
			t.Errorf("%s user_id = %q, want %q", test.action, rawParamString(call.params, "user_id"), test.user)
		}
		if got := rawParamString(call.params, "message_id"); got != "native-msg-1" {
			t.Errorf("%s message_id = %q, want native-msg-1", test.action, got)
		}
		if _, ok := call.params["count"]; !ok {
			t.Errorf("%s did not forward count", test.action)
		}
	}
	if groupID != firstGroupID || userID != firstUserID || messageID != firstMessageID {
		t.Fatalf("unexpected test ID allocation: group=%d user=%d message=%d", groupID, userID, messageID)
	}
}

func TestOneBotGroupAndRemarkActionsDispatchToNativeAtomics(t *testing.T) {
	type nativeCall struct {
		action string
		params map[string]any
	}
	calls := make(chan nativeCall, 7)
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		var params map[string]any
		if err := json.Unmarshal(data.(json.RawMessage), &params); err != nil {
			t.Errorf("invalid native parameters: %v", err)
		}
		calls <- nativeCall{action: action, params: params}
		return json.RawMessage(`{"ok":true}`), nil
	}}, "")

	for _, tc := range []struct {
		action string
		params string
		want   nativeCall
	}{
		{"set_group_kick", `{"group_id":"room@chatroom","user_id":"wxid_member"}`, nativeCall{"delMembers", map[string]any{"chatroom_name": "room@chatroom", "user_list": []any{"wxid_member"}}}},
		{"set_group_leave", `{"group_id":"room@chatroom"}`, nativeCall{"quitGroup", map[string]any{"chatroom_name": "room@chatroom"}}},
		{"set_group_card", `{"group_id":"room@chatroom","card":"Alice"}`, nativeCall{"editSelfMemberNick", map[string]any{"chatroom_name": "room@chatroom", "nick": "Alice"}}},
		{"set_friend_remark", `{"user_id":"wxid_friend","remark":"Friend"}`, nativeCall{"editObjRemark", map[string]any{"wxid": "wxid_friend", "remark": "Friend"}}},
		{"set_friend_add_request", `{"wxid":"wxid_friend","remark":"Friend"}`, nativeCall{"editObjRemark", map[string]any{"wxid": "wxid_friend", "remark": "Friend"}}},
		{"create_group", `{"user_list":["wxid_a","wxid_b"],"topic":"team"}`, nativeCall{"createGroup", map[string]any{"user_list": []any{"wxid_a", "wxid_b"}, "topic": "team"}}},
		{"add_members", `{"group_id":"room@chatroom","user_list":["wxid_a"]}`, nativeCall{"addMembers", map[string]any{"chatroom_name": "room@chatroom", "group_id": "room@chatroom", "user_list": []any{"wxid_a"}}}},
	} {
		status, response := request(t, server, http.MethodPost, `{"action":"`+tc.action+`","params":`+tc.params+`}`)
		if status != http.StatusOK || response.Retcode != 0 {
			t.Fatalf("%s returned status=%d response=%+v", tc.action, status, response)
		}
		got := <-calls
		if got.action != tc.want.action || !reflect.DeepEqual(got.params, tc.want.params) {
			t.Errorf("%s dispatched %#v, want %#v", tc.action, got, tc.want)
		}
	}
}

func TestOneBotNumericActionIDsReverseWithoutQQCompat(t *testing.T) {
	type nativeCall struct {
		action string
		params map[string]json.RawMessage
	}
	calls := make(chan nativeCall, 7)
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		var params map[string]json.RawMessage
		if err := json.Unmarshal(data.(json.RawMessage), &params); err != nil {
			return nil, err
		}
		calls <- nativeCall{action: action, params: params}
		return json.RawMessage(`{"sequence":123}`), nil
	}}, "")
	userID := server.idMapper.UserWxidToNum("wxid_target")
	groupID := server.idMapper.GroupChatroomToNum("room@chatroom")
	if userID != firstUserID || groupID != firstGroupID {
		t.Fatalf("unexpected test IDs: user=%d group=%d", userID, groupID)
	}
	if server.currentGatewayConfig().QQCompat {
		t.Fatal("test requires QQ compatibility mode to be disabled")
	}

	tests := []struct {
		action       string
		params       string
		nativeAction string
		wantTarget   string
		wantGroup    string
		wantUser     string
	}{
		{"send_msg", `{"user_id":100000001,"message":"hello"}`, "sendText", "wxid_target", "", ""},
		{"send_group_msg", `{"group_id":200000001,"message":"hello"}`, "sendText", "room@chatroom", "", ""},
		{"send_private_msg", `{"user_id":100000001,"message":"hello"}`, "sendText", "wxid_target", "", ""},
		{"set_group_kick", `{"group_id":200000001,"user_id":100000001}`, "delMembers", "", "room@chatroom", "wxid_target"},
		{"set_group_card", `{"group_id":200000001,"card":"Target"}`, "editSelfMemberNick", "", "room@chatroom", ""},
		{"set_group_leave", `{"group_id":200000001}`, "quitGroup", "", "room@chatroom", ""},
		{"delete_friend", `{"user_id":100000001}`, "delFriend", "", "", "wxid_target"},
	}
	for _, test := range tests {
		t.Run(test.action, func(t *testing.T) {
			result := server.dispatchOneBotAction(context.Background(), test.action, json.RawMessage(test.params))
			if result.retcode != 0 {
				t.Fatalf("dispatch returned retcode %d: %s", result.retcode, result.message)
			}
			call := <-calls
			if call.action != test.nativeAction {
				t.Fatalf("native action = %q, want %q", call.action, test.nativeAction)
			}
			if test.wantTarget != "" && rawParamString(call.params, "target") != test.wantTarget {
				t.Errorf("native target = %q, want %q", rawParamString(call.params, "target"), test.wantTarget)
			}
			if test.wantGroup != "" && rawParamString(call.params, "chatroom_name") != test.wantGroup {
				t.Errorf("native chatroom = %q, want %q", rawParamString(call.params, "chatroom_name"), test.wantGroup)
			}
			if test.wantUser != "" {
				if call.action == "delMembers" {
					var users []string
					if err := json.Unmarshal(call.params["user_list"], &users); err != nil || len(users) != 1 || users[0] != test.wantUser {
						t.Errorf("native user list = %#v, err=%v; want [%q]", users, err, test.wantUser)
					}
				} else if got := rawParamString(call.params, "wxid"); got != test.wantUser {
					t.Errorf("native wxid = %q, want %q", got, test.wantUser)
				}
			}
		})
	}
}

func TestOneBotListAndRevokeActionsAdaptDataAndReverseIDs(t *testing.T) {
	type nativeCall struct {
		action string
		params map[string]json.RawMessage
	}
	calls := make(chan nativeCall, 4)
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		params := make(map[string]json.RawMessage)
		if err := json.Unmarshal(data.(json.RawMessage), &params); err != nil {
			return nil, err
		}
		calls <- nativeCall{action: action, params: params}
		switch action {
		case "getGroupList":
			return json.RawMessage(`[{"chatroom_name":"room@chatroom","wxid":"room@chatroom"}]`), nil
		case "getFriendList":
			return json.RawMessage(`[{"wxid":"wxid_friend","nickname":"Friend"}]`), nil
		case "getMemberList":
			return json.RawMessage(`{"chatroom_name":"room@chatroom","owner_wxid":"wxid_owner","members":[{"wxid":"wxid_owner","nickname":"Owner","is_owner":true},{"wxid":"wxid_member","nickname":"Member","is_owner":false}]}`), nil
		default:
			return json.RawMessage(`{"revoked":true}`), nil
		}
	}}, "")
	cfg := server.currentConfig()
	server.SetConfig(&cfg, t.TempDir())
	groupID := server.idMapper.GroupChatroomToNum("room@chatroom")
	friendID := server.idMapper.UserWxidToNum("wxid_friend")
	ownerID := server.idMapper.UserWxidToNum("wxid_owner")
	memberID := server.idMapper.UserWxidToNum("wxid_member")
	messageID := server.idMapper.MessageIDToNum("native-msg-42")
	server.rememberIncomingMessage(client.Event{
		Event: "recvMsg",
		Data:  json.RawMessage(`{"msg_id":"native-msg-42","client_msg_id":"client-msg-42","from_wxid":"room@chatroom","final_from_wxid":"wxid_owner","svr_msg_id":90042,"create_time":1730000000}`),
	}, "wxid_self")

	_, response := request(t, server, http.MethodPost, `{"action":"get_group_list","params":{}}`)
	groupCall := <-calls
	if groupCall.action != "getGroupList" {
		t.Fatalf("get_group_list dispatched %q", groupCall.action)
	}
	groups, ok := response.Data.([]any)
	if !ok || len(groups) != 1 || groups[0].(map[string]any)["group_id"] != float64(groupID) || groups[0].(map[string]any)["group_name"] != "room@chatroom" || groups[0].(map[string]any)["chatroom_name"] != "room@chatroom" {
		t.Fatalf("unexpected OneBot group list response: %#v", response.Data)
	}

	_, response = request(t, server, http.MethodPost, `{"action":"get_friend_list","params":{}}`)
	friendCall := <-calls
	if friendCall.action != "getFriendList" {
		t.Fatalf("get_friend_list dispatched %q", friendCall.action)
	}
	friends, ok := response.Data.([]any)
	if !ok || len(friends) != 1 || friends[0].(map[string]any)["user_id"] != float64(friendID) || friends[0].(map[string]any)["nickname"] != "Friend" || friends[0].(map[string]any)["wxid"] != "wxid_friend" {
		t.Fatalf("unexpected OneBot friend list response: %#v", response.Data)
	}

	_, response = request(t, server, http.MethodPost, `{"action":"get_group_member_list","params":{"group_id":200000001}}`)
	memberCall := <-calls
	if memberCall.action != "getMemberList" || rawParamString(memberCall.params, "group_id") != "room@chatroom" {
		t.Fatalf("group ID was not reversed for member lookup: %#v", memberCall)
	}
	members, ok := response.Data.([]any)
	if !ok || len(members) != 2 {
		t.Fatalf("unexpected OneBot member list response: %#v", response.Data)
	}
	owner := members[0].(map[string]any)
	member := members[1].(map[string]any)
	if owner["group_id"] != float64(groupID) || owner["user_id"] != float64(ownerID) || owner["wxid"] != "wxid_owner" || owner["role"] != "owner" || member["user_id"] != float64(memberID) || member["wxid"] != "wxid_member" || member["role"] != "member" {
		t.Fatalf("member IDs or roles were not adapted: %#v", members)
	}

	_, response = request(t, server, http.MethodPost, `{"action":"delete_msg","params":{"message_id":1000001}}`)
	revokeCall := <-calls
	if revokeCall.action != "revoke_msg" || rawParamString(revokeCall.params, "to_user") != "room@chatroom" || rawParamString(revokeCall.params, "client_msg_id") != "client-msg-42" || rawParamString(revokeCall.params, "svr_msg_id") != "90042" || rawParamString(revokeCall.params, "create_time") != "1730000000" {
		t.Fatalf("delete_msg did not produce native revoke parameters: action=%s params=%s", revokeCall.action, marshalParams(revokeCall.params))
	}
	if response.Retcode != 0 || messageID != 1000001 {
		t.Fatalf("delete_msg failed: response=%+v message_id=%d", response, messageID)
	}
}

func TestOneBotAPIPathAndVersionInfo(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "")
	for _, path := range []string{"/api", "/get_version_info"} {
		body := `{"action":"get_version_info","params":{},"echo":"version"}`
		if path != "/api" {
			body = `{}`
		}
		recorder := httptest.NewRecorder()
		s.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		var response struct {
			Status  string         `json:"status"`
			Retcode int            `json:"retcode"`
			Echo    string         `json:"echo"`
			Data    map[string]any `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusOK || response.Status != "ok" || response.Retcode != 0 || response.Data["app_name"] != "wxbot-onebot" || response.Data["protocol_version"] != "v11" {
			t.Fatalf("unexpected response at %s: %s", path, recorder.Body.String())
		}
		if path == "/api" && response.Echo != "version" {
			t.Fatalf("echo was not preserved: %s", recorder.Body.String())
		}
	}
}
