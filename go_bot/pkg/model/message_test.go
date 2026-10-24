package model

import (
	"testing"
)

func TestParseAtMembers(t *testing.T) {
	xmlCdata := `<msgsource><atuserlist><![CDATA[wxid_aaa,wxid_bbb]]></atuserlist></msgsource>`
	members := ParseAtMembers(xmlCdata)
	if len(members) != 2 || members[0] != "wxid_aaa" || members[1] != "wxid_bbb" {
		t.Fatalf("unexpected members: %v", members)
	}

	xmlPlain := `<msgsource><atuserlist>wxid_ccc</atuserlist></msgsource>`
	membersPlain := ParseAtMembers(xmlPlain)
	if len(membersPlain) != 1 || membersPlain[0] != "wxid_ccc" {
		t.Fatalf("unexpected members: %v", membersPlain)
	}

	if ParseAtMembers("") != nil {
		t.Fatalf("expected nil for empty")
	}
}

func TestBuildMessage(t *testing.T) {
	msg := BuildMessage(
		1,
		"wxid_sender",
		"123456@chatroom",
		"@bot hello",
		`<msgsource><atuserlist><![CDATA[wxid_bot]]></atuserlist></msgsource>`,
		100,
		"wxid_bot",
	)

	if !msg.IsGroup {
		t.Errorf("expected is_group to be true")
	}
	if msg.ChatID != "123456@chatroom" {
		t.Errorf("expected chat_id 123456@chatroom, got %s", msg.ChatID)
	}
	if !msg.IsAtMe("wxid_bot") {
		t.Errorf("expected is_at_me to be true")
	}
	if msg.IsAtMe("wxid_other") {
		t.Errorf("expected is_at_me for other to be false")
	}
}
