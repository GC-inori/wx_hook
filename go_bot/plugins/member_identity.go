package plugins

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"

	"wxbot/pkg/model"
	"wxbot/pkg/plugin"
)

type MemberIdentityPlugin struct {
	enabled bool
	db      *sql.DB
	mu      sync.Mutex
}

func NewMemberIdentityPlugin() *MemberIdentityPlugin {
	return &MemberIdentityPlugin{enabled: true}
}

func (p *MemberIdentityPlugin) Name() string        { return "member_identity" }
func (p *MemberIdentityPlugin) Description() string { return "发送 /绑定 <昵称>，设置当前群个人称呼（SQLite持久化）" }
func (p *MemberIdentityPlugin) Enabled() bool       { return p.enabled }
func (p *MemberIdentityPlugin) SetEnabled(v bool)   { p.enabled = v }

func (p *MemberIdentityPlugin) Init(ctx *plugin.Context) error {
	dbFile := ctx.Config.Members.DatabaseFile
	if dbFile == "" {
		dbFile = ".runtime/member_data.sqlite3"
	}
	dbPath := filepath.Join(ctx.Config.RootDir, dbFile)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("打开 SQLite 失败: %w", err)
	}
	p.db = db

	// Create tables if not exist
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS member_bindings (
		group_id TEXT NOT NULL,
		wxid TEXT NOT NULL,
		alias TEXT NOT NULL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (group_id, wxid)
	);
	CREATE TABLE IF NOT EXISTS chat_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id TEXT NOT NULL,
		sender_id TEXT NOT NULL,
		alias TEXT NOT NULL,
		text TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(createTableSQL)
	return err
}

func (p *MemberIdentityPlugin) GetAlias(groupID, wxid string) (string, error) {
	if p.db == nil {
		return "", nil
	}
	var alias string
	err := p.db.QueryRow("SELECT alias FROM member_bindings WHERE group_id = ? AND wxid = ?", groupID, wxid).Scan(&alias)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return alias, err
}

func (p *MemberIdentityPlugin) BindAlias(groupID, wxid, alias string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	query := `
	INSERT INTO member_bindings (group_id, wxid, alias, updated_at) 
	VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(group_id, wxid) DO UPDATE SET alias=excluded.alias, updated_at=CURRENT_TIMESTAMP;
	`
	_, err := p.db.Exec(query, groupID, wxid, alias)
	return err
}

func (p *MemberIdentityPlugin) OnMessage(ctx *plugin.Context, msg *model.Message) (bool, error) {
	if msg.IsSelf {
		return false, nil
	}

	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(text, "/绑定") {
		parts := strings.Fields(text)
		if len(parts) < 2 {
			_ = ctx.Reply(msg, "用法: /绑定 你的昵称")
			return true, nil
		}

		alias := strings.TrimSpace(parts[1])
		if len(alias) > 30 {
			_ = ctx.Reply(msg, "绑定的昵称过长，请保持在 30 字符内。")
			return true, nil
		}

		err := p.BindAlias(msg.ChatID, msg.Sender, alias)
		if err != nil {
			_ = ctx.Reply(msg, fmt.Sprintf("绑定昵称失败: %v", err))
		} else {
			_ = ctx.Reply(msg, fmt.Sprintf("绑定成功！在当前群中你的昵称已设置为【%s】。", alias))
		}
		return true, nil
	}

	// Record chat history if in group and history is enabled
	if msg.IsGroup && ctx.Config.Members.HistoryEnabled && p.db != nil && text != "" {
		alias, _ := p.GetAlias(msg.ChatID, msg.Sender)
		if alias == "" {
			alias = msg.Sender
		}
		_, _ = p.db.Exec(
			"INSERT INTO chat_history (group_id, sender_id, alias, text) VALUES (?, ?, ?, ?)",
			msg.ChatID, msg.Sender, alias, text,
		)
	}

	return false, nil
}
