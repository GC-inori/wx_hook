package plugins

import (
	"strings"

	"wxbot/pkg/model"
	"wxbot/pkg/plugin"
)

type EchoPlugin struct {
	enabled bool
}

func NewEchoPlugin() *EchoPlugin {
	return &EchoPlugin{enabled: true}
}

func (e *EchoPlugin) Name() string        { return "echo" }
func (e *EchoPlugin) Description() string { return "发送 echo <文字>，原样回复后面的文字" }
func (e *EchoPlugin) Enabled() bool       { return e.enabled }
func (e *EchoPlugin) SetEnabled(v bool)   { e.enabled = v }
func (e *EchoPlugin) Init(ctx *plugin.Context) error { return nil }

func (e *EchoPlugin) OnMessage(ctx *plugin.Context, msg *model.Message) (bool, error) {
	if msg.IsSelf {
		return false, nil
	}
	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(strings.ToLower(text), "echo ") {
		replyText := strings.TrimSpace(text[5:])
		if replyText != "" {
			_ = ctx.Reply(msg, replyText)
			return true, nil
		}
	}
	return false, nil
}
