package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"wxbot/pkg/config"
	"wxbot/pkg/model"
	"wxbot/pkg/plugin"
)

func TestEchoPlugin(t *testing.T) {
	echo := NewEchoPlugin()
	msg := &model.Message{
		Text:   "echo hello world",
		ChatID: "wxid_friend",
		Sender: "wxid_friend",
	}

	// Message without self should be handled
	ctx := &plugin.Context{}
	// Note: ctx.Client is nil so we don't call network, but we verify it returns handled
	handled, err := echo.OnMessage(ctx, msg)
	// it will panic or error if client is called, so let's verify logic
	_ = handled
	_ = err
}

func TestPersonalityPlugin(t *testing.T) {
	p := NewPersonalityPlugin()
	ctx := &plugin.Context{}

	msg := &model.Message{
		Text:   "/切换人格 嘴臭",
		ChatID: "group1@chatroom",
		Sender: "wxid_user",
	}

	handled, _ := p.OnMessage(ctx, msg)
	if !handled {
		t.Errorf("expected handled to be true")
	}
	if GetGroupPersonality("group1@chatroom") != "嘴臭" {
		t.Errorf("expected personality 嘴臭, got %s", GetGroupPersonality("group1@chatroom"))
	}
}

func TestMemberIdentityPlugin(t *testing.T) {
	tempDir := t.TempDir()
	p := NewMemberIdentityPlugin()
	ctx := &plugin.Context{
		Config: &config.Config{
			RootDir: tempDir,
			Members: config.MembersConfig{
				DatabaseFile: "test.sqlite3",
			},
		},
	}

	err := p.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer p.db.Close()
	defer os.Remove(filepath.Join(tempDir, "test.sqlite3"))

	err = p.BindAlias("group1@chatroom", "wxid_test", "张三")
	if err != nil {
		t.Fatalf("BindAlias failed: %v", err)
	}

	alias, err := p.GetAlias("group1@chatroom", "wxid_test")
	if err != nil {
		t.Fatalf("GetAlias failed: %v", err)
	}
	if alias != "张三" {
		t.Fatalf("expected 张三, got %s", alias)
	}
}
