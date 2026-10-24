package model

import (
	"regexp"
	"strings"
	"time"
)

var (
	atUserListCdataRegex = regexp.MustCompile(`<atuserlist><!\[CDATA\[(.*?)\]\]></atuserlist>`)
	atUserListRegex      = regexp.MustCompile(`<atuserlist>(.*?)</atuserlist>`)
)

// Message represents an incoming or outgoing chat message
type Message struct {
	MsgType       uint32    `json:"msg_type"`
	Sender        string    `json:"sender"`
	Receiver      string    `json:"receiver"`
	Text          string    `json:"text"`
	MentionSource string    `json:"mention_source,omitempty"`
	Sequence      u64       `json:"sequence"`
	ReceivedAt    time.Time `json:"received_at"`

	// Derived fields
	ChatID    string   `json:"chat_id"`
	IsGroup   bool     `json:"is_group"`
	IsSelf    bool     `json:"is_self"`
	AtMembers []string `json:"at_members"`
}

type u64 = uint64

// ParseAtMembers extracts mentioned wxids from msgsource XML
func ParseAtMembers(source string) []string {
	if source == "" {
		return nil
	}

	var rawList string
	if m := atUserListCdataRegex.FindStringSubmatch(source); len(m) > 1 {
		rawList = m[1]
	} else if m := atUserListRegex.FindStringSubmatch(source); len(m) > 1 {
		rawList = m[1]
	}

	if rawList == "" {
		return nil
	}

	parts := strings.Split(rawList, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// BuildMessage converts a raw IPC event payload into a processed Message
func BuildMessage(msgType uint32, sender, receiver, text, source string, seq uint64, selfWxid string) *Message {
	isGroup := strings.HasSuffix(receiver, "@chatroom") || strings.HasSuffix(sender, "@chatroom")
	
	chatID := receiver
	if !isGroup && sender != selfWxid && sender != "" {
		chatID = sender
	}
	if strings.HasSuffix(sender, "@chatroom") {
		chatID = sender
	}

	atMembers := ParseAtMembers(source)
	isSelf := (sender == selfWxid)

	return &Message{
		MsgType:       msgType,
		Sender:        sender,
		Receiver:      receiver,
		Text:          text,
		MentionSource: source,
		Sequence:      seq,
		ReceivedAt:    time.Now(),
		ChatID:        chatID,
		IsGroup:       isGroup,
		IsSelf:        isSelf,
		AtMembers:     atMembers,
	}
}

// IsAtMe returns true if this message actually @-mentioned the bot's wxid
func (m *Message) IsAtMe(selfWxid string) bool {
	if selfWxid == "" {
		return false
	}
	for _, mem := range m.AtMembers {
		if mem == selfWxid {
			return true
		}
	}
	return false
}
