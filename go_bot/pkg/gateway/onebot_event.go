package gateway

import (
	"encoding/json"
	"strings"
	"time"

	"wxbot/pkg/client"
	"wxbot/pkg/protocol"
)

func formatOneBotEvent(event client.Event, selfID string) map[string]any {
	fields := rawObject(event.Data)
	base := map[string]any{
		"time":    protocol.EventTime(event.Timestamp),
		"self_id": selfID,
	}
	switch event.Event {
	case "recvMsg":
		text := firstRawString(fields, "text", "msg")
		segments := protocol.FormatMessage(text)
		groupID := firstRawString(fields, "from_wxid", "group_id", "group_wxid", "receiver", "chat_id")
		if !strings.HasSuffix(groupID, "@chatroom") {
			groupID = firstRawString(fields, "group_id", "group_wxid", "receiver", "chat_id")
		}
		isGroup := strings.HasSuffix(groupID, "@chatroom") || rawParamBool(fields, "is_group")
		userID := firstRawString(fields, "final_from_wxid", "sender", "sender_wxid", "sender_id", "user_id", "wxid")
		if userID == "" {
			userID = firstRawString(fields, "from_wxid")
		}
		if strings.HasSuffix(userID, "@chatroom") {
			userID = firstRawString(fields, "final_from_wxid", "sender_wxid", "sender_id")
		}
		nickname := firstRawString(fields, "nickname", "name", "sender_name", "remark")
		messageID := rawMessageID(fields, "msg_id", "message_id", "sequence")
		base["post_type"] = "message"
		base["message_type"] = "private"
		base["sub_type"] = "normal"
		if isGroup {
			base["message_type"] = "group"
			base["group_id"] = groupID
		}
		base["message_id"] = messageID
		base["user_id"] = userID
		base["message"] = segments
		base["raw_message"] = protocol.FormatCQCode(segments)
		base["font"] = 0
		base["sender"] = map[string]any{"user_id": userID, "nickname": nickname, "card": ""}
	case "revokeMsg":
		groupID := firstRawString(fields, "session", "group_id", "group_wxid")
		base["post_type"] = "notice"
		base["notice_type"] = "friend_recall"
		if strings.HasSuffix(groupID, "@chatroom") {
			base["notice_type"] = "group_recall"
			base["group_id"] = groupID
		}
		base["message_id"] = rawMessageID(fields, "revoked_msg_id", "msg_id", "message_id")
		base["user_id"] = firstRawString(fields, "final_from_wxid", "sender", "user_id")
	case "friendReq":
		applicant := rawObject(fields["applicant"])
		userID := firstRawString(applicant, "wxid")
		if userID == "" {
			userID = firstRawString(fields, "final_from_wxid", "sender", "user_id")
		}
		base["post_type"] = "request"
		base["request_type"] = "friend"
		base["user_id"] = userID
		base["comment"] = firstRawString(applicant, "content")
		base["flag"] = firstRawString(applicant, "ticket")
	case "groupMemberChanges":
		groupID := firstRawString(fields, "group_wxid", "group_id", "from_wxid")
		noticeType := "group_increase"
		if strings.Contains(strings.ToLower(firstRawString(fields, "notification", "text", "msg")), "remove") || strings.Contains(firstRawString(fields, "notification", "text", "msg"), "移出") || strings.Contains(firstRawString(fields, "notification", "text", "msg"), "退出") {
			noticeType = "group_decrease"
		}
		base["post_type"] = "notice"
		base["notice_type"] = noticeType
		base["sub_type"] = "approve"
		base["group_id"] = groupID
		base["user_id"] = firstRawString(fields, "final_from_wxid", "user_id", "sender")
	case "log":
		base["post_type"] = "meta_event"
		base["meta_event_type"] = "lifecycle"
		base["sub_type"] = "log"
		base["level"] = firstRawString(fields, "level")
		base["message"] = firstRawString(fields, "message")
		base["log_time"] = firstRawString(fields, "time")
	case "transPay":
		base["post_type"] = "notice"
		base["notice_type"] = "wx_trans_pay"
		base["amount"] = firstRawString(fields, "amount")
		base["user_id"] = firstRawString(fields, "sender", "user_id")
	case "qrPay":
		base["post_type"] = "notice"
		base["notice_type"] = "wx_qr_pay"
		base["amount"] = firstRawString(fields, "amount")
		base["user_id"] = firstRawString(fields, "sender", "user_id")
	case "systemMsg":
		base["post_type"] = "notice"
		base["notice_type"] = "wx_system_message"
		base["message"] = firstRawString(fields, "notification", "text", "msg")
	default:
		base["post_type"] = "notice"
		base["notice_type"] = "wx_event"
		base["sub_type"] = event.Event
	}
	return base
}

func rawMessageID(fields map[string]json.RawMessage, keys ...string) any {
	for _, key := range keys {
		raw := fields[key]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var number json.Number
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if decoder.Decode(&number) == nil {
			if integer, err := number.Int64(); err == nil {
				return integer
			}
			return number
		}
		var text string
		if json.Unmarshal(raw, &text) == nil {
			if integer, err := json.Number(text).Int64(); err == nil {
				return integer
			}
			return text
		}
		return json.RawMessage(raw)
	}
	return time.Now().UnixNano()
}

func currentSelfID(s *Server) string {
	if caller := s.currentCaller(); caller != nil {
		if self, ok := caller.(interface{ SelfWxid() string }); ok {
			if id := self.SelfWxid(); id != "" {
				return id
			}
		}
	}
	return strings.TrimSpace(s.currentConfig().Bot.WeChatAccount)
}

func marshalWebsocketEvent(event client.Event, selfIDs ...string) ([]byte, error) {
	selfID := ""
	if len(selfIDs) != 0 {
		selfID = selfIDs[0]
	}
	return json.Marshal(formatOneBotEvent(event, selfID))
}
