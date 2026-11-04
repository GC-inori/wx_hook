package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"wxbot/pkg/client"
)

func TestIDMapperStableMappingsPersistAcrossRestart(t *testing.T) {
	root := t.TempDir()
	mapper, err := NewIDMapper(root)
	if err != nil {
		t.Fatal(err)
	}
	userID := mapper.UserWxidToNum("wxid_alice")
	groupID := mapper.GroupChatroomToNum("123456@chatroom")
	messageID := mapper.MessageIDToNum("18446744073709551615")
	if userID != firstUserID || groupID != firstGroupID || messageID != firstMessageID {
		t.Fatalf("unexpected allocated IDs: user=%d group=%d message=%d", userID, groupID, messageID)
	}
	if _, err := os.Stat(filepath.Join(root, "id_mapping.json")); err != nil {
		t.Fatalf("mapping file was not persisted: %v", err)
	}

	restarted, err := NewIDMapper(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.UserWxidToNum("wxid_alice"); got != userID {
		t.Fatalf("user ID changed after restart: got %d, want %d", got, userID)
	}
	if got := restarted.GroupChatroomToNum("123456@chatroom"); got != groupID {
		t.Fatalf("group ID changed after restart: got %d, want %d", got, groupID)
	}
	if got := restarted.MessageIDToNum("18446744073709551615"); got != messageID {
		t.Fatalf("message ID changed after restart: got %d, want %d", got, messageID)
	}
	if got := restarted.NumToUserWxid(userID); got != "wxid_alice" {
		t.Fatalf("user reverse lookup failed: %q", got)
	}
	if got := restarted.NumToGroupChatroom(groupID); got != "123456@chatroom" {
		t.Fatalf("group reverse lookup failed: %q", got)
	}
	if got := restarted.NumToMessageID(messageID); got != "18446744073709551615" {
		t.Fatalf("message reverse lookup failed: %q", got)
	}
}

func TestIDMapperAllocationsAreConcurrentAndUnique(t *testing.T) {
	mapper := newIDMapper()
	const count = 64
	ids := make(chan int64, count)
	var wait sync.WaitGroup
	for i := 0; i < count; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ids <- mapper.UserWxidToNum("wxid_same")
		}()
	}
	wait.Wait()
	close(ids)
	var expected int64
	for id := range ids {
		if expected == 0 {
			expected = id
		}
		if id != expected {
			t.Fatalf("concurrent requests allocated different IDs: %d and %d", expected, id)
		}
	}

	unique := make(map[int64]struct{}, count)
	for i := 0; i < count; i++ {
		id := mapper.UserWxidToNum(fmt.Sprintf("wxid_%d", i))
		if _, exists := unique[id]; exists {
			t.Fatalf("duplicate numeric user ID %d", id)
		}
		unique[id] = struct{}{}
	}
}

func TestIDMapperTranslatesGroupActionListsAndChatroomNames(t *testing.T) {
	mapper := newIDMapper()
	userID := mapper.UserWxidToNum("wxid_member")
	groupID := mapper.GroupChatroomToNum("room@chatroom")
	raw := json.RawMessage(fmt.Sprintf(`{"group_id":%d,"user_list":[%d]}`, groupID, userID))
	translated := mapper.TranslateActionParams("add_members", raw)
	var params map[string]any
	if err := json.Unmarshal(translated, &params); err != nil {
		t.Fatal(err)
	}
	if params["group_id"] != "room@chatroom" || !reflect.DeepEqual(params["user_list"], []any{"wxid_member"}) {
		t.Fatalf("group action IDs were not reversed: %#v", params)
	}

	commaList := json.RawMessage(fmt.Sprintf(`{"member_list":"%d"}`, userID))
	translated = mapper.TranslateActionParams("create_group", commaList)
	if err := json.Unmarshal(translated, &params); err != nil {
		t.Fatal(err)
	}
	if params["member_list"] != "wxid_member" {
		t.Fatalf("comma-delimited member ID was not reversed: %#v", params)
	}

	response := mapper.TranslateActionResponseData("create_group", map[string]any{
		"chatroom_name": "room@chatroom",
	}).(map[string]any)
	if response["chatroom_name"] != groupID {
		t.Fatalf("chatroom_name was not mapped to numeric group ID: %#v", response)
	}
}

func TestIDMapperTranslatesEventsAndCQMentions(t *testing.T) {
	mapper := newIDMapper()
	selfID := mapper.UserWxidToNum("wxid_self")
	userID := mapper.UserWxidToNum("wxid_sender")
	groupID := mapper.GroupChatroomToNum("room@chatroom")
	messageID := mapper.MessageIDToNum("native-message-id")
	event := map[string]any{
		"self_id": "wxid_self", "user_id": "wxid_sender", "group_id": "room@chatroom",
		"message_id":  "native-message-id",
		"sender":      map[string]any{"user_id": "wxid_sender", "nickname": "Alice"},
		"message":     []any{map[string]any{"type": "at", "data": map[string]any{"qq": "wxid_sender"}}},
		"raw_message": "hello [CQ:at,qq=wxid_sender]",
	}
	translated := mapper.TranslateEvent(event)
	if got := translated["self_id"]; got != selfID {
		t.Fatalf("self_id was not mapped: %#v", got)
	}
	if got := translated["user_id"]; got != userID {
		t.Fatalf("user_id was not mapped: %#v", got)
	}
	if got := translated["group_id"]; got != groupID {
		t.Fatalf("group_id was not mapped: %#v", got)
	}
	if got := translated["message_id"]; got != messageID {
		t.Fatalf("message_id was not mapped: %#v", got)
	}
	sender := translated["sender"].(map[string]any)
	if got := sender["user_id"]; got != userID {
		t.Fatalf("sender.user_id was not mapped: %#v", got)
	}
	segments := translated["message"].([]any)
	mention := segments[0].(map[string]any)["data"].(map[string]any)["qq"]
	if mention != userID {
		t.Fatalf("message segment mention was not mapped: %#v", mention)
	}
	if got := translated["raw_message"]; got != "hello [CQ:at,qq="+strconv.FormatInt(userID, 10)+"]" {
		t.Fatalf("raw CQ mention was not mapped: %v", got)
	}
	if event["user_id"] != "wxid_sender" {
		t.Fatalf("TranslateEvent mutated its input: %#v", event["user_id"])
	}
}

func TestIDMapperTranslatesActionParamsBothWays(t *testing.T) {
	mapper := newIDMapper()
	userID := mapper.UserWxidToNum("wxid_target")
	groupID := mapper.GroupChatroomToNum("room@chatroom")
	messageID := mapper.MessageIDToNum("native-message-id")
	if userID != firstUserID || groupID != firstGroupID || messageID != firstMessageID {
		t.Fatalf("unexpected initial mapping IDs: user=%d group=%d message=%d", userID, groupID, messageID)
	}
	raw := json.RawMessage(`{"user_id":100000001,"group_id":200000001,"message_id":1000001,"message":"[CQ:at,qq=100000001]","segments":[{"type":"at","data":{"qq":100000001}},{"type":"reply","data":{"id":1000001}}]}`)
	translated := mapper.TranslateActionParams("send_msg", raw)
	decoder := json.NewDecoder(bytes.NewReader(translated))
	decoder.UseNumber()
	var got map[string]any
	if err := decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["user_id"] != "wxid_target" || got["group_id"] != "room@chatroom" || got["message_id"] != "native-message-id" {
		t.Fatalf("top-level action IDs were not reversed: %#v", got)
	}
	if got["message"] != "[CQ:at,qq=wxid_target]" {
		t.Fatalf("CQ mention was not reversed: %#v", got["message"])
	}
	segments := got["segments"].([]any)
	mention := segments[0].(map[string]any)["data"].(map[string]any)["qq"]
	replyID := segments[1].(map[string]any)["data"].(map[string]any)["id"]
	if mention != "wxid_target" || replyID != "native-message-id" {
		t.Fatalf("segment IDs were not reversed: mention=%#v reply=%#v", mention, replyID)
	}
}

func TestIDMapperTranslatesActionResponseData(t *testing.T) {
	mapper := newIDMapper()
	userID := mapper.UserWxidToNum("wxid_friend")
	groupID := mapper.GroupChatroomToNum("room@chatroom")
	messageID := mapper.MessageIDToNum("native-message-id")
	data := map[string]any{
		"user_id": "wxid_friend", "group_id": "room@chatroom", "message_id": "native-message-id",
		"friends": []any{map[string]any{"wxid": "wxid_friend"}},
		"message": []any{map[string]any{"type": "at", "data": map[string]any{"qq": "wxid_friend"}}},
	}
	translated := mapper.TranslateActionResponseData("get_msg", data).(map[string]any)
	if translated["user_id"] != userID || translated["group_id"] != groupID || translated["message_id"] != messageID {
		t.Fatalf("action response IDs were not mapped: %#v", translated)
	}
	friends := translated["friends"].([]any)
	if got := friends[0].(map[string]any)["wxid"]; got != userID {
		t.Fatalf("friend wxid was not mapped: %#v", got)
	}
}

func TestServerQQCompatPublishesTranslatedEvent(t *testing.T) {
	server := testServer(t, "en-US", fakeCaller{}, "")
	cfg := server.currentConfig()
	cfg.Gateway.QQCompat = true
	cfg.Bot.WeChatAccount = "wxid_self"
	server.SetConfig(&cfg, t.TempDir())

	peer := &subscriber{queue: make(chan []byte, 1), actionQueue: make(chan []byte, 1), done: make(chan struct{})}
	if !server.hub.add(peer) {
		t.Fatal("could not add event subscriber")
	}
	defer server.hub.remove(peer)
	server.Publish(client.Event{
		Event: "recvMsg", Timestamp: "2026-10-31T12:00:00Z",
		Data: json.RawMessage(`{"msg_id":99,"from_wxid":"room@chatroom","final_from_wxid":"wxid_sender","text":"hello [CQ:at,qq=wxid_target]"}`),
	})

	var event map[string]any
	if err := json.Unmarshal(<-peer.queue, &event); err != nil {
		t.Fatal(err)
	}
	expectedSelf := server.idMapper.UserWxidToNum("wxid_self")
	expectedSender := server.idMapper.UserWxidToNum("wxid_sender")
	expectedTarget := server.idMapper.UserWxidToNum("wxid_target")
	expectedGroup := server.idMapper.GroupChatroomToNum("room@chatroom")
	expectedMessage := server.idMapper.MessageIDToNum(int64(99))
	if event["self_id"] != float64(expectedSelf) || event["user_id"] != float64(expectedSender) || event["group_id"] != float64(expectedGroup) || event["message_id"] != float64(expectedMessage) {
		t.Fatalf("event IDs were not translated for QQ compatibility: %#v", event)
	}
	if event["raw_message"] != "hello [CQ:at,qq="+strconv.FormatInt(expectedTarget, 10)+"]" {
		t.Fatalf("event CQ mention was not translated: %#v", event["raw_message"])
	}
}

func TestIDMapperRejectsInvalidPersistenceFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "id_mapping.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewIDMapper(root); err == nil {
		t.Fatal("malformed ID mapping file was accepted")
	}
}
