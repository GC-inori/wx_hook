package plugin

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"wxbot/pkg/client"
	"wxbot/pkg/config"
	"wxbot/pkg/llm"
	"wxbot/pkg/model"
)

type Context struct {
	Client *client.Client
	Config *config.Config
	LLM    *llm.Client
}

func (ctx *Context) Reply(msg *model.Message, text string) error {
	if text == "" {
		return nil
	}
	if ctx.Client == nil {
		return fmt.Errorf("Client 未初始化")
	}
	target := msg.ChatID
	if target == "" {
		target = msg.Sender
	}
	return ctx.Client.SendText(target, text)
}

func (ctx *Context) ReplyAt(msg *model.Message, text string, atMembers ...string) error {
	if text == "" {
		return nil
	}
	if ctx.Client == nil {
		return fmt.Errorf("Client 未初始化")
	}
	target := msg.ChatID
	if target == "" {
		target = msg.Receiver
	}

	if len(atMembers) == 0 {
		return ctx.Reply(msg, text)
	}

	membersStr := strings.Join(atMembers, ",")
	return ctx.Client.SendAtText(target, text, membersStr)
}

type Plugin interface {
	Name() string
	Description() string
	Enabled() bool
	SetEnabled(bool)
	Init(ctx *Context) error
	OnMessage(ctx *Context, msg *model.Message) (bool, error)
}

type Manager struct {
	mu      sync.RWMutex
	plugins []Plugin
	ctx     *Context
}

func NewManager(ctx *Context) *Manager {
	return &Manager{
		ctx: ctx,
	}
}

func (m *Manager) Register(p Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if configured enabled/disabled in bot_config.json
	if pConf, ok := m.ctx.Config.Bot.Plugins[p.Name()]; ok {
		p.SetEnabled(pConf.Enabled)
	}

	if err := p.Init(m.ctx); err != nil {
		log.Printf("[Plugin] 初始化插件 %s 警告: %v", p.Name(), err)
	}

	m.plugins = append(m.plugins, p)
}

func (m *Manager) List() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]Plugin, len(m.plugins))
	copy(res, m.plugins)
	return res
}

func (m *Manager) SetEnabled(name string, enabled bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.plugins {
		if strings.EqualFold(p.Name(), name) {
			p.SetEnabled(enabled)
			return true
		}
	}
	return false
}

func (m *Manager) Dispatch(msg *model.Message) {
	m.mu.RLock()
	plugins := make([]Plugin, len(m.plugins))
	copy(plugins, m.plugins)
	m.mu.RUnlock()

	for _, p := range plugins {
		if !p.Enabled() {
			continue
		}
		handled, err := p.OnMessage(m.ctx, msg)
		if err != nil {
			log.Printf("[Plugin] 插件 %s 处理消息错误: %v", p.Name(), err)
		}
		if handled {
			break
		}
	}
}
