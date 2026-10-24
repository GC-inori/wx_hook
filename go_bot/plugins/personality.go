package plugins

import (
	"fmt"
	"strings"
	"sync"

	"wxbot/pkg/model"
	"wxbot/pkg/plugin"
)

var (
	personalityMu     sync.RWMutex
	groupPersonality = make(map[string]string)
)

func GetGroupPersonality(chatID string) string {
	personalityMu.RLock()
	defer personalityMu.RUnlock()
	return groupPersonality[chatID]
}

type PersonalityPlugin struct {
	enabled bool
}

func NewPersonalityPlugin() *PersonalityPlugin {
	return &PersonalityPlugin{enabled: true}
}

func (p *PersonalityPlugin) Name() string        { return "personality" }
func (p *PersonalityPlugin) Description() string { return "按群切换机器人的嘴臭、卑微、夸夸人格" }
func (p *PersonalityPlugin) Enabled() bool       { return p.enabled }
func (p *PersonalityPlugin) SetEnabled(v bool)   { p.enabled = v }
func (p *PersonalityPlugin) Init(ctx *plugin.Context) error { return nil }

func (p *PersonalityPlugin) OnMessage(ctx *plugin.Context, msg *model.Message) (bool, error) {
	if msg.IsSelf {
		return false, nil
	}

	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(text, "/切换人格") {
		parts := strings.Fields(text)
		if len(parts) < 2 {
			_ = ctx.Reply(msg, "用法: /切换人格 嘴臭|卑微|夸夸|取消")
			return true, nil
		}

		style := parts[1]
		personalityMu.Lock()
		switch style {
		case "取消", "默认", "normal":
			delete(groupPersonality, msg.ChatID)
			_ = ctx.Reply(msg, "已恢复当前群的默认交流人格。")
		case "嘴臭", "卑微", "夸夸":
			groupPersonality[msg.ChatID] = style
			_ = ctx.Reply(msg, fmt.Sprintf("已成功为当前群切换为人格【%s】。", style))
		default:
			_ = ctx.Reply(msg, "未知人格。可选：嘴臭、卑微、夸夸、取消")
		}
		personalityMu.Unlock()
		return true, nil
	}

	return false, nil
}
