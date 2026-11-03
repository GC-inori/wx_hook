// Package config loads only the native gateway's configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type BotConfig struct {
	WeChatAccount        string `yaml:"wechat_account" json:"wechat_account"`
	IpcSocket            string `yaml:"ipc_socket" json:"ipc_socket"`
	ReconnectIntervalSec int    `yaml:"reconnect_interval_sec" json:"reconnect_interval_sec"`
}

type LoggingConfig struct {
	Dir           string `yaml:"dir" json:"dir"`
	Level         string `yaml:"level" json:"level"`
	ToConsole     bool   `yaml:"to_console" json:"to_console"`
	MaxSizeMB     int    `yaml:"max_size_mb" json:"max_size_mb"`
	MaxBackups    int    `yaml:"max_backups" json:"max_backups"`
	Compress      bool   `yaml:"compress" json:"compress"`
	LogBufferSize int    `yaml:"log_buffer_size" json:"log_buffer_size"`
}

type GatewayConfig struct {
	Listen            string   `yaml:"listen" json:"listen"`
	Locale            string   `yaml:"locale" json:"locale"`
	APIToken          string   `yaml:"api_token" json:"api_token"`
	AllowedOrigins    []string `yaml:"allowed_origins" json:"allowed_origins"`
	MaxConnections    int      `yaml:"max_connections" json:"max_connections"`
	RequestTimeoutSec int      `yaml:"request_timeout_sec" json:"request_timeout_sec"`
	MaxRequestBodyMB  int      `yaml:"max_request_body_mb" json:"max_request_body_mb"`
	WSQueueSize       int      `yaml:"ws_queue_size" json:"ws_queue_size"`
	WSPingIntervalSec int      `yaml:"ws_ping_interval_sec" json:"ws_ping_interval_sec"`
	QQCompat          bool     `yaml:"qq_compat" json:"qq_compat"`
}

type EndpointConfig struct {
	ID        string `json:"id" yaml:"id"`
	Name      string `json:"name" yaml:"name"`
	Type      string `json:"type" yaml:"type"` // "httpServer", "httpClient", "wsServer", "wsClient"
	URL       string `json:"url,omitempty" yaml:"url,omitempty"`
	Host      string `json:"host,omitempty" yaml:"host,omitempty"`
	Port      int    `json:"port,omitempty" yaml:"port,omitempty"`
	Token     string `json:"token,omitempty" yaml:"token,omitempty"`
	Heartbeat int    `json:"heartbeat,omitempty" yaml:"heartbeat,omitempty"`
	Enabled   bool   `json:"enabled" yaml:"enabled"`
}

type PolicyConfig struct {
	AtSender             bool     `json:"at_sender" yaml:"at_sender"`
	BlacklistWxids       []string `json:"blacklist_wxids" yaml:"blacklist_wxids"`
	WhitelistGroups      []string `json:"whitelist_groups" yaml:"whitelist_groups"`
	AntiRevoke           bool     `json:"anti_revoke" yaml:"anti_revoke"`
	AutoAgreeFriend      bool     `json:"auto_agree_friend" yaml:"auto_agree_friend"`
	FriendWelcomeEnabled bool     `json:"friend_welcome_enabled" yaml:"friend_welcome_enabled"`
	FriendWelcomeMessage string   `json:"friend_welcome_message" yaml:"friend_welcome_message"`
	AutoAgreeGroup       bool     `json:"auto_agree_group" yaml:"auto_agree_group"`
	FilterGh             bool     `json:"filter_gh" yaml:"filter_gh"`
	MediaDir             string   `json:"media_dir" yaml:"media_dir"`
	RetryCount           int      `json:"retry_count" yaml:"retry_count"`
	RateLimitEnabled     bool     `json:"rate_limit_enabled" yaml:"rate_limit_enabled"`
	MaxMessagesPerSec    int      `json:"max_messages_per_sec" yaml:"max_messages_per_sec"`
	TargetMinIntervalMs  int      `json:"target_min_interval_ms" yaml:"target_min_interval_ms"`
	JitterMinMs          int      `json:"jitter_min_ms" yaml:"jitter_min_ms"`
	JitterMaxMs          int      `json:"jitter_max_ms" yaml:"jitter_max_ms"`
	AntiLoopWindowSec    int      `json:"anti_loop_window_sec" yaml:"anti_loop_window_sec"`
}

// DefaultPolicyConfig returns the behavioral limits used for new installations.
func DefaultPolicyConfig() PolicyConfig {
	return PolicyConfig{
		AtSender:            true,
		RateLimitEnabled:    true,
		MaxMessagesPerSec:   5,
		TargetMinIntervalMs: 600,
		JitterMinMs:         200,
		JitterMaxMs:         800,
		AntiLoopWindowSec:   3,
	}
}

func applyPolicyDefaults(policy *PolicyConfig) {
	if policy.TargetMinIntervalMs == 0 {
		policy.TargetMinIntervalMs = 600
	}
	if policy.JitterMinMs == 0 {
		policy.JitterMinMs = 200
	}
	if policy.JitterMaxMs == 0 {
		policy.JitterMaxMs = 800
	}
	if policy.AntiLoopWindowSec == 0 {
		policy.AntiLoopWindowSec = 3
	}
}

// LauncherConfig controls the Linux runtime. Relative paths are resolved against RootDir.
type LauncherConfig struct {
	AutoStart         bool   `yaml:"auto_start" json:"auto_start"`
	WeChatBin         string `yaml:"wechat_bin" json:"wechat_bin"`
	BridgeLib         string `yaml:"bridge_lib" json:"bridge_lib"`
	Display           string `yaml:"display" json:"display"`
	UseXvfb           bool   `yaml:"use_xvfb" json:"use_xvfb"`
	XvfbScreen        string `yaml:"xvfb_screen" json:"xvfb_screen"`
	QRPort            int    `yaml:"qr_port" json:"qr_port"`
	QRBind            string `yaml:"qr_bind" json:"qr_bind"`
	LoginTimeout      int    `yaml:"login_timeout" json:"login_timeout"`
	BridgeWaitTimeout int    `yaml:"bridge_wait_timeout" json:"bridge_wait_timeout"`
	XvfbWaitTimeout   int    `yaml:"xvfb_wait_timeout" json:"xvfb_wait_timeout"`
}

type Config struct {
	Bot       BotConfig        `yaml:"bot" json:"bot"`
	Logging   LoggingConfig    `yaml:"logging" json:"logging"`
	Gateway   GatewayConfig    `yaml:"gateway" json:"gateway"`
	Launcher  LauncherConfig   `yaml:"launcher" json:"launcher"`
	Policy    PolicyConfig     `yaml:"policy" json:"policy"`
	Endpoints []EndpointConfig `yaml:"endpoints" json:"endpoints"`
	RootDir   string           `yaml:"-" json:"-"`
}

func LoadConfig(rootDir string) (*Config, error) {
	cfg := &Config{
		RootDir: rootDir,
		Bot:     BotConfig{ReconnectIntervalSec: 3},
		Logging: LoggingConfig{Dir: "logs", Level: "info", ToConsole: true, MaxSizeMB: 50, MaxBackups: 7, LogBufferSize: 500},
		Gateway: GatewayConfig{Listen: "127.0.0.1:8080", Locale: "zh-CN", MaxConnections: 128, RequestTimeoutSec: 7, MaxRequestBodyMB: 3, WSQueueSize: 256, WSPingIntervalSec: 20},
		Launcher: LauncherConfig{
			AutoStart: true, BridgeLib: "./librust_bridge.so", Display: ":99", UseXvfb: true,
			XvfbScreen: "1280x1024x24", QRPort: 8899, QRBind: "0.0.0.0", LoginTimeout: 120,
			BridgeWaitTimeout: 30, XvfbWaitTimeout: 5,
		},
		Policy: DefaultPolicyConfig(),
	}
	path := filepath.Join(rootDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	if err == nil {
		// Unknown fields from old installations are ignored, but malformed YAML is fatal.
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	applyConfigDefaults(cfg)
	applyPolicyDefaults(&cfg.Policy)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Bot.IpcSocket != "" && !filepath.IsAbs(cfg.Bot.IpcSocket) && !strings.HasPrefix(cfg.Bot.IpcSocket, "/") {
		cfg.Bot.IpcSocket = filepath.Join(rootDir, cfg.Bot.IpcSocket)
	}
	if token := os.Getenv("WX_GATEWAY_TOKEN"); token != "" {
		cfg.Gateway.APIToken = token
	}
	return cfg, nil
}

func applyConfigDefaults(cfg *Config) {
	if cfg.Bot.ReconnectIntervalSec == 0 {
		cfg.Bot.ReconnectIntervalSec = 3
	}
	if cfg.Logging.MaxSizeMB == 0 {
		cfg.Logging.MaxSizeMB = 50
	}
	if cfg.Logging.MaxBackups == 0 {
		cfg.Logging.MaxBackups = 7
	}
	if cfg.Logging.LogBufferSize == 0 {
		cfg.Logging.LogBufferSize = 500
	}
	if cfg.Gateway.RequestTimeoutSec == 0 {
		cfg.Gateway.RequestTimeoutSec = 7
	}
	if cfg.Gateway.MaxRequestBodyMB == 0 {
		cfg.Gateway.MaxRequestBodyMB = 3
	}
	if cfg.Gateway.WSQueueSize == 0 {
		cfg.Gateway.WSQueueSize = 256
	}
	if cfg.Gateway.WSPingIntervalSec == 0 {
		cfg.Gateway.WSPingIntervalSec = 20
	}
	if cfg.Launcher.BridgeWaitTimeout == 0 {
		cfg.Launcher.BridgeWaitTimeout = 30
	}
	if cfg.Launcher.XvfbWaitTimeout == 0 {
		cfg.Launcher.XvfbWaitTimeout = 5
	}
}

// Validate checks the fields required for the gateway and launcher to run.
func (cfg *Config) Validate() error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}
	if cfg.Gateway.Listen == "" {
		return fmt.Errorf("gateway.listen is empty")
	}
	if cfg.Gateway.Locale != "zh-CN" && cfg.Gateway.Locale != "en-US" {
		return fmt.Errorf("gateway.locale must be zh-CN or en-US")
	}
	if cfg.Gateway.MaxConnections < 1 || cfg.Gateway.MaxConnections > 4096 {
		return fmt.Errorf("gateway.max_connections must be between 1 and 4096")
	}
	if cfg.Gateway.RequestTimeoutSec < 1 {
		return fmt.Errorf("gateway.request_timeout_sec must be at least 1 second")
	}
	if cfg.Gateway.MaxRequestBodyMB < 1 {
		return fmt.Errorf("gateway.max_request_body_mb must be at least 1")
	}
	if cfg.Gateway.WSQueueSize < 16 {
		return fmt.Errorf("gateway.ws_queue_size must be at least 16")
	}
	if cfg.Gateway.WSPingIntervalSec < 5 {
		return fmt.Errorf("gateway.ws_ping_interval_sec must be at least 5 seconds")
	}
	if cfg.Bot.ReconnectIntervalSec < 1 {
		return fmt.Errorf("bot.reconnect_interval_sec must be at least 1 second")
	}
	if cfg.Logging.MaxSizeMB < 1 || cfg.Logging.MaxBackups < 0 || cfg.Logging.LogBufferSize < 1 {
		return fmt.Errorf("logging.max_size_mb and logging.log_buffer_size must be positive; logging.max_backups cannot be negative")
	}
	if strings.TrimSpace(cfg.Logging.Dir) == "" {
		return fmt.Errorf("logging.dir is empty")
	}
	if err := cfg.Launcher.Validate(); err != nil {
		return err
	}
	return nil
}

// SaveConfig serializes cfg as YAML and writes it to rootDir/config.yaml.
func SaveConfig(rootDir string, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("configuration root directory is empty")
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal configuration: %w", err)
	}
	if err := os.MkdirAll(rootDir, 0750); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	path := filepath.Join(rootDir, "config.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write configuration %s: %w", path, err)
	}
	return nil
}
