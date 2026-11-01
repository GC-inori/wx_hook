package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfigurationDefaultsAndZeroValues(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	assertNewDefaults(t, cfg)

	root := t.TempDir()
	contents := []byte(`bot:
  reconnect_interval_sec: 0
logging:
  max_size_mb: 0
  max_backups: 0
  log_buffer_size: 0
gateway:
  request_timeout_sec: 0
  max_request_body_mb: 0
  ws_queue_size: 0
  ws_ping_interval_sec: 0
launcher:
  bridge_wait_timeout: 0
  xvfb_wait_timeout: 0
`)
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	assertNewDefaults(t, cfg)
}

func assertNewDefaults(t *testing.T, cfg *Config) {
	t.Helper()
	if cfg.Bot.ReconnectIntervalSec != 3 {
		t.Errorf("reconnect interval = %d, want 3", cfg.Bot.ReconnectIntervalSec)
	}
	if cfg.Logging.MaxSizeMB != 50 || cfg.Logging.MaxBackups != 7 || cfg.Logging.Compress || cfg.Logging.LogBufferSize != 500 {
		t.Errorf("logging defaults = %+v", cfg.Logging)
	}
	if cfg.Gateway.RequestTimeoutSec != 7 || cfg.Gateway.MaxRequestBodyMB != 3 || cfg.Gateway.WSQueueSize != 256 || cfg.Gateway.WSPingIntervalSec != 20 {
		t.Errorf("gateway defaults = %+v", cfg.Gateway)
	}
	if cfg.Launcher.BridgeWaitTimeout != 30 || cfg.Launcher.XvfbWaitTimeout != 5 {
		t.Errorf("launcher defaults = %+v", cfg.Launcher)
	}
	if !cfg.Policy.AtSender || len(cfg.Policy.BlacklistWxids) != 0 || len(cfg.Policy.WhitelistGroups) != 0 {
		t.Errorf("policy defaults = %+v", cfg.Policy)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("default configuration did not validate: %v", err)
	}
}

func TestNewConfigurationOptionsLoadAndValidate(t *testing.T) {
	root := t.TempDir()
	contents := []byte(`bot:
  reconnect_interval_sec: 9
logging:
  max_size_mb: 25
  max_backups: 4
  compress: true
  log_buffer_size: 32
gateway:
  request_timeout_sec: 12
  max_request_body_mb: 8
  ws_queue_size: 64
  ws_ping_interval_sec: 30
launcher:
  auto_start: false
  bridge_wait_timeout: 45
  xvfb_wait_timeout: 2
policy:
  at_sender: false
  blacklist_wxids: [wxid_blocked]
  whitelist_groups: [room_allowed@chatroom]
`)
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.ReconnectIntervalSec != 9 || cfg.Logging.MaxSizeMB != 25 || cfg.Logging.MaxBackups != 4 || !cfg.Logging.Compress || cfg.Logging.LogBufferSize != 32 {
		t.Fatalf("custom bot/logging options were not loaded: %+v %+v", cfg.Bot, cfg.Logging)
	}
	if cfg.Gateway.RequestTimeoutSec != 12 || cfg.Gateway.MaxRequestBodyMB != 8 || cfg.Gateway.WSQueueSize != 64 || cfg.Gateway.WSPingIntervalSec != 30 {
		t.Fatalf("custom gateway options were not loaded: %+v", cfg.Gateway)
	}
	if cfg.Launcher.BridgeWaitTimeout != 45 || cfg.Launcher.XvfbWaitTimeout != 2 {
		t.Fatalf("custom launcher wait options were not loaded: %+v", cfg.Launcher)
	}
	if cfg.Policy.AtSender || len(cfg.Policy.BlacklistWxids) != 1 || cfg.Policy.BlacklistWxids[0] != "wxid_blocked" || len(cfg.Policy.WhitelistGroups) != 1 || cfg.Policy.WhitelistGroups[0] != "room_allowed@chatroom" {
		t.Fatalf("custom policy options were not loaded: %+v", cfg.Policy)
	}
}

func TestNewConfigurationMinimumsAreValidated(t *testing.T) {
	checks := []struct {
		name string
		set  func(*Config)
	}{
		{"request timeout", func(c *Config) { c.Gateway.RequestTimeoutSec = 0 }},
		{"request body", func(c *Config) { c.Gateway.MaxRequestBodyMB = 0 }},
		{"WebSocket queue", func(c *Config) { c.Gateway.WSQueueSize = 15 }},
		{"WebSocket ping", func(c *Config) { c.Gateway.WSPingIntervalSec = 4 }},
		{"reconnect interval", func(c *Config) { c.Bot.ReconnectIntervalSec = 0 }},
		{"bridge wait", func(c *Config) { c.Launcher.BridgeWaitTimeout = 4 }},
		{"Xvfb wait", func(c *Config) { c.Launcher.XvfbWaitTimeout = 0 }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			cfg, err := LoadConfig(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			check.set(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("invalid %s value passed validation", check.name)
			}
		})
	}
}
