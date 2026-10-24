package plugins

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wxbot/pkg/model"
	"wxbot/pkg/plugin"
)

type MentionChatPlugin struct {
	enabled bool
}

func NewMentionChatPlugin() *MentionChatPlugin {
	return &MentionChatPlugin{enabled: true}
}

func (p *MentionChatPlugin) Name() string        { return "mention_chat" }
func (p *MentionChatPlugin) Description() string { return "在群聊中@机器人时，结合当前群人格与大模型进行对话回复" }
func (p *MentionChatPlugin) Enabled() bool       { return p.enabled }
func (p *MentionChatPlugin) SetEnabled(v bool)   { p.enabled = v }
func (p *MentionChatPlugin) Init(ctx *plugin.Context) error { return nil }

func (p *MentionChatPlugin) OnMessage(ctx *plugin.Context, msg *model.Message) (bool, error) {
	if msg.IsSelf {
		return false, nil
	}

	// Trigger if bot is @-mentioned in group, or in private chat if not a command
	selfWxid := ctx.Client.SelfWxid()
	isAt := msg.IsAtMe(selfWxid)

	if !isAt && msg.IsGroup {
		return false, nil
	}

	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "echo ") || strings.EqualFold(text, "test") {
		return false, nil
	}

	// Clean out @nickname or @bot text
	cleanedText := text
	if idx := strings.Index(cleanedText, "\u2005"); idx != -1 {
		cleanedText = strings.TrimSpace(cleanedText[idx+len("\u2005"):])
	}

	if cleanedText == "" {
		_ = ctx.ReplyAt(msg, "找我有什么事吗？", msg.Sender)
		return true, nil
	}

	if !ctx.LLM.IsConfigured() {
		_ = ctx.ReplyAt(msg, "大模型接口尚未配置，请在 llm_config.json 中填入 api_key 与 base_url 后启用。", msg.Sender)
		return true, nil
	}

	personality := GetGroupPersonality(msg.ChatID)
	systemPrompt := "你是一个微信群聊智能助手，回答简洁自然，口语化，适合聊天软件场景。禁止使用长篇大论。"
	switch personality {
	case "嘴臭":
		systemPrompt += " 你的当前性格设定：说话刻薄、毒舌、嘲讽、不耐烦，但不要使用敏感违规脏字。"
	case "卑微":
		systemPrompt += " 你的当前性格设定：极度卑微、讨好型人格、称呼对方为大佬、小心翼翼。"
	case "夸夸":
		systemPrompt += " 你的当前性格设定：疯狂赞美、无脑夸奖、热情洋溢、极度捧场。"
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	reply, err := ctx.LLM.Chat(reqCtx, systemPrompt, cleanedText)
	if err != nil {
		_ = ctx.ReplyAt(msg, fmt.Sprintf("对话生成失败: %v", err), msg.Sender)
		return true, err
	}

	if msg.IsGroup {
		_ = ctx.ReplyAt(msg, reply, msg.Sender)
	} else {
		_ = ctx.Reply(msg, reply)
	}

	return true, nil
}
