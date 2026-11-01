package gateway

import (
	"encoding/json"
	"strings"

	"wxbot/pkg/client"
)

func (s *Server) configuredDestinationBlocked(action string, data json.RawMessage) string {
	if _, outbound := outboundMessageActions[action]; !outbound {
		return ""
	}
	policy := s.currentConfig().Policy
	if len(policy.BlacklistWxids) == 0 {
		return ""
	}
	target, _ := messageIdentity(data)
	for _, blocked := range policy.BlacklistWxids {
		if target != "" && target == strings.TrimSpace(blocked) {
			return target
		}
	}
	return ""
}

func eventAllowedByPolicy(event client.Event, policyBlacklist, groupWhitelist []string) bool {
	if len(policyBlacklist) == 0 && len(groupWhitelist) == 0 {
		return true
	}
	var fields map[string]json.RawMessage
	if len(event.Data) != 0 && json.Unmarshal(event.Data, &fields) != nil {
		return true
	}

	for _, key := range []string{"final_from_wxid", "sender", "sender_wxid", "sender_id", "from_wxid", "wxid"} {
		sender := eventString(fields[key])
		if sender != "" && containsWxid(policyBlacklist, sender) {
			return false
		}
	}
	if len(groupWhitelist) == 0 {
		return true
	}

	group := eventBool(fields["is_group"])
	groupID := ""
	for _, key := range []string{"chat_id", "group_id", "group_wxid", "room_wxid", "receiver", "from_wxid", "sender"} {
		value := eventString(fields[key])
		if strings.HasSuffix(value, "@chatroom") {
			group = true
			if groupID == "" {
				groupID = value
			}
		}
	}
	if !group {
		return true
	}
	return groupID != "" && containsWxid(groupWhitelist, groupID)
}

func eventString(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func eventBool(raw json.RawMessage) bool {
	var value bool
	return len(raw) != 0 && json.Unmarshal(raw, &value) == nil && value
}

func containsWxid(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
