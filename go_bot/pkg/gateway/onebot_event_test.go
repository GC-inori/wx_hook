package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"wxbot/pkg/client"
	"wxbot/pkg/protocol"
)

func TestFormatOneBotEventInjectsMentionSegments(t *testing.T) {
	tests := []struct {
		name        string
		mentionKey  string
		mentionList string
		text        string
		wantQQ      string
		wantAtCount int
	}{
		{
			name:        "plain atuserlist and quarter-space prefix",
			mentionKey:  "mention_source",
			mentionList: "<atuserlist>wxid_test,wxid_other</atuserlist>",
			text:        "@Bot\u2005hello",
			wantQQ:      "wxid_test",
			wantAtCount: 2,
		},
		{
			name:        "CDATA source and ASCII-space prefix",
			mentionKey:  "source",
			mentionList: "<atuserlist><![CDATA[wxid_test]]></atuserlist>",
			text:        "@Bot hello",
			wantQQ:      "wxid_test",
			wantAtCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]string{
				"text":          test.text,
				test.mentionKey: test.mentionList,
			})
			if err != nil {
				t.Fatal(err)
			}
			event := formatOneBotEvent(client.Event{Event: "recvMsg", Data: data}, "wxid_bot")

			segments, ok := event["message"].([]protocol.Segment)
			if !ok {
				t.Fatalf("message has unexpected type %T", event["message"])
			}
			wantAtCount := test.wantAtCount
			if len(segments) != wantAtCount+1 {
				t.Fatalf("got %d segments, want %d: %#v", len(segments), wantAtCount+1, segments)
			}
			for i := 0; i < wantAtCount; i++ {
				if segments[i].Type != "at" {
					t.Fatalf("segment %d type is %q, want at", i, segments[i].Type)
				}
				if i == 0 && segments[i].Data["qq"] != test.wantQQ {
					t.Fatalf("first mention target is %#v, want %q", segments[i].Data["qq"], test.wantQQ)
				}
			}
			textSegment := segments[len(segments)-1]
			if textSegment.Type != "text" || textSegment.Data["text"] != "hello" {
				t.Fatalf("text prefix was not stripped: %#v", textSegment)
			}
			if event["raw_message"] != protocol.FormatCQCode(segments) {
				t.Fatalf("raw_message was not regenerated: %#v", event["raw_message"])
			}
		})
	}
}

func TestFormatHongBaoEventIncludesRedPacketNoticeFields(t *testing.T) {
	data := json.RawMessage(`{"title":"新年快乐","send_id":"send-1","native_url":"wxpay://hongbao","sender":"wxid_sender","receiver":"room@chatroom","sub_type":"opened","amount":128,"receiveId":"rid-1"}`)
	event := formatOneBotEvent(client.Event{Event: "hongBao", Data: data}, "wxid_bot")
	for key, want := range map[string]any{
		"post_type": "notice", "notice_type": "wx_red_packet", "title": "新年快乐",
		"send_id": "send-1", "native_url": "wxpay://hongbao", "user_id": "wxid_sender",
		"group_id": "room@chatroom", "sub_type": "opened", "amount": "128", "receive_id": "rid-1",
	} {
		if got := event[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

func TestFormatTransPayEventIncludesConfirmationFields(t *testing.T) {
	data := json.RawMessage(`{"transfer_id":"tr-1","transaction_id":"tx-1","amount":"￥1.00","pay_subtype":"1","invalid_time":"1730000000","transfer_attach":"opaque","sender":"wxid_sender"}`)
	event := formatOneBotEvent(client.Event{Event: "transPay", Data: data}, "wxid_bot")
	for key, want := range map[string]any{
		"post_type":       "notice",
		"notice_type":     "wx_trans_pay",
		"transfer_id":     "tr-1",
		"transaction_id":  "tx-1",
		"amount":          "￥1.00",
		"pay_subtype":     "1",
		"invalid_time":    "1730000000",
		"transfer_attach": "opaque",
		"user_id":         "wxid_sender",
	} {
		if got := event[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

func TestParseMentionTargetsMapsEveryone(t *testing.T) {
	fields := map[string]json.RawMessage{
		"atuserlist": json.RawMessage(`"<atuserlist>wxid_test,announcement@all,notify@all</atuserlist>"`),
	}
	got := parseMentionTargets(fields)
	want := []string{"wxid_test", "all"}
	if len(got) != len(want) {
		t.Fatalf("got mention targets %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got mention targets %#v, want %#v", got, want)
		}
	}
}

func TestOneBotListActionsReturnEmptyOnNativeFailure(t *testing.T) {
	tests := []struct {
		action       string
		nativeAction string
		params       string
	}{
		{action: "get_group_list", nativeAction: "getGroupList", params: `{}`},
		{action: "get_friend_list", nativeAction: "getFriendList", params: `{}`},
		{action: "get_group_member_list", nativeAction: "getMemberList", params: `{"group_id":"123@chatroom"}`},
	}
	for _, test := range tests {
		t.Run(test.action, func(t *testing.T) {
			calledAction := ""
			server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, _ any) (json.RawMessage, error) {
				calledAction = action
				return nil, errors.New("native action is unsupported")
			}}, "")

			_, response := request(t, server, "POST", `{"action":"`+test.action+`","params":`+test.params+`}`)
			items, ok := response.Data.([]any)
			if response.Retcode != 0 || !ok || len(items) != 0 {
				t.Fatalf("unexpected fallback for %s: %+v", test.action, response)
			}
			if calledAction != test.nativeAction {
				t.Fatalf("native action is %q, want %q", calledAction, test.nativeAction)
			}
		})
	}
}
