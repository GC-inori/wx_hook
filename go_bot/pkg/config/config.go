package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type BotConfig struct {
	WeChatAccount string                  `json:"wechat_account" yaml:"wechat_account"`
	IpcSocket     string                  `json:"ipc_socket" yaml:"ipc_socket"`
	TextDelay     TextDelayConfig         `json:"text_delay" yaml:"text_delay"`
	Plugins       map[string]PluginConfig `json:"plugins" yaml:"plugins"`
}

type TextDelayConfig struct {
	Enabled  bool    `json:"enabled" yaml:"enabled"`
	MinDelay float64 `json:"min_delay" yaml:"min_delay"`
	MaxDelay float64 `json:"max_delay" yaml:"max_delay"`
}

type PluginConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type LoggingConfig struct {
	Dir       string `json:"dir" yaml:"dir"`
	Level     string `json:"level" yaml:"level"`
	ToConsole bool   `json:"to_console" yaml:"to_console"`
}

type MembersConfig struct {
	DatabaseFile   string              `json:"database_file" yaml:"database_file"`
	HistoryEnabled bool                `json:"history_enabled" yaml:"history_enabled"`
	DailyClear     bool                `json:"daily_clear" yaml:"daily_clear"`
	Groups         map[string][]string `json:"groups" yaml:"groups"`
}

type LLMConfig struct {
	APIKey  string `json:"api_key" yaml:"api_key"`
	BaseURL string `json:"base_url" yaml:"base_url"`
	Model   string `json:"model" yaml:"model"`
}

type YamlRootConfig struct {
	Bot     BotConfig     `yaml:"bot"`
	Logging LoggingConfig `yaml:"logging"`
	Members MembersConfig `yaml:"members"`
	LLM     LLMConfig     `yaml:"llm"`
}

type Config struct {
	Bot     BotConfig
	Logging LoggingConfig
	Members MembersConfig
	LLM     LLMConfig
	RootDir string
}

func LoadConfig(rootDir string) (*Config, error) {
	cfg := &Config{
		RootDir: rootDir,
		Logging: LoggingConfig{
			Dir:       "logs",
			Level:     "info",
			ToConsole: true,
		},
	}

	// 1. 优先尝试从 config.yaml 加载全套配置
	yamlCandidates := []string{
		filepath.Join(rootDir, "config.yaml"),
		"config.yaml",
	}

	for _, yPath := range yamlCandidates {
		if data, err := os.ReadFile(yPath); err == nil {
			var yRoot YamlRootConfig
			if err := yaml.Unmarshal(data, &yRoot); err == nil {
				cfg.Bot = yRoot.Bot
				if yRoot.Logging.Dir != "" {
					cfg.Logging = yRoot.Logging
				}
				cfg.Members = yRoot.Members
				cfg.LLM = yRoot.LLM
				return cfg, nil
			}
		}
	}

	// 2. 回退模式：兼容以往旧版的独立 json 配置文件
	botPath := filepath.Join(rootDir, "bot_config.json")
	if data, err := os.ReadFile(botPath); err == nil {
		_ = json.Unmarshal(data, &cfg.Bot)
	}

	membersPath := filepath.Join(rootDir, "members_config.json")
	if data, err := os.ReadFile(membersPath); err == nil {
		_ = json.Unmarshal(data, &cfg.Members)
	}

	llmPath := filepath.Join(rootDir, "llm_config.json")
	if data, err := os.ReadFile(llmPath); err == nil {
		_ = json.Unmarshal(data, &cfg.LLM)
	}

	return cfg, nil
}
