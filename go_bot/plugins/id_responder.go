package plugins

import (
	"fmt"
	"strings"

	"wxbot/pkg/model"
	"wxbot/pkg/plugin"
)

type IDResponderPlugin struct {
	enabled bool
}

func NewIDResponderPlugin() *IDResponderPlugin {
	return &IDResponderPlugin{enabled: true}
}

func (p *IDResponderPlugin) Name() string        { return "id_responder" }
func (p *IDResponderPlugin) Description() string { return "发送 test，回复当前会话 ID 和发送者 ID" }
func (p *IDResponderPlugin) Enabled() bool       { return p.enabled }
func (p *IDResponderPlugin) SetEnabled(v bool)   { p.enabled = v }
func (p *IDResponderPlugin) Init(ctx *plugin.Context) error { return nil }

func (p *IDResponderPlugin) OnMessage(ctx *plugin.Context, msg *model.Message) (bool, error) {
	if msg.IsSelf {
		return false, nil
	}
	text := strings.TrimSpace(msg.Text)
	if strings.EqualFold(text, "test") {
		reply := fmt.Sprintf("会话 ID: %s\n发送者: %s", msg.ChatID, msg.Sender)
		if msg.IsGroup {
			reply += fmt.Sprintf("\n群聊: %s", msg.Receiver)
		}
		_ = ctx.Reply(msg, reply)
		return true, nil
	}
	return false, nil
}
