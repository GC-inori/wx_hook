package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveConfigRoundTripsPolicyAndEndpoints(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{
		Gateway:  GatewayConfig{Listen: "127.0.0.1:8081", Locale: "en-US", MaxConnections: 64, QQCompat: true},
		Logging:  LoggingConfig{Dir: "logs", Level: "debug", ToConsole: false},
		Launcher: LauncherConfig{AutoStart: true, BridgeLib: "./bridge.so", Display: ":99", UseXvfb: true, XvfbScreen: "1280x1024x24", QRPort: 8899, QRBind: "127.0.0.1", LoginTimeout: 90},
		Policy: PolicyConfig{
			MediaDir:         "data/media",
			RateLimitEnabled: false, MaxMessagesPerSec: 0, TargetMinIntervalMs: 600, JitterMinMs: 200, JitterMaxMs: 800, AntiLoopWindowSec: 3,
		},
		Endpoints: []EndpointConfig{{ID: "webhook-1", Name: "Events", Type: "httpClient", URL: "http://127.0.0.1:9000/events", Token: "secret", Enabled: true}},
	}
	if err := SaveConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Gateway.QQCompat != cfg.Gateway.QQCompat {
		t.Fatalf("QQ compatibility flag did not round-trip: got %v, want %v", loaded.Gateway.QQCompat, cfg.Gateway.QQCompat)
	}
	loadedPolicy, originalPolicy := loaded.Policy, cfg.Policy

	if !reflect.DeepEqual(loadedPolicy, originalPolicy) || len(loaded.Endpoints) != 1 || loaded.Endpoints[0] != cfg.Endpoints[0] {
		t.Fatalf("saved configuration changed: policy=%+v endpoints=%+v", loaded.Policy, loaded.Endpoints)
	}
	data, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "policy:") || !strings.Contains(string(data), "endpoints:") || !strings.Contains(string(data), "qq_compat: true") {
		t.Fatalf("configuration sections or QQ compatibility flag missing from YAML: %s", data)
	}
}

func TestSaveConfigRejectsNilOrEmptyRoot(t *testing.T) {
	if err := SaveConfig(t.TempDir(), nil); err == nil {
		t.Fatal("nil configuration accepted")
	}
	if err := SaveConfig("", &Config{}); err == nil {
		t.Fatal("empty root directory accepted")
	}
}

func TestDefaultsAndEnvironmentToken(t *testing.T) {
	t.Setenv("WX_GATEWAY_TOKEN", "from-environment")
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gateway.Listen != "127.0.0.1:8080" || cfg.Gateway.Locale != "zh-CN" || cfg.Gateway.APIToken != "from-environment" || cfg.Gateway.MaxConnections != 128 || cfg.Gateway.QQCompat {
		t.Fatalf("bad defaults: %+v", cfg.Gateway)
	}
	if want := DefaultPolicyConfig(); !reflect.DeepEqual(cfg.Policy, want) {
		t.Fatalf("bad behavioral policy defaults: got %+v, want %+v", cfg.Policy, want)
	}
}

func TestPolicyDefaultsAndExplicitUnlimitedRate(t *testing.T) {
	root := t.TempDir()
	data := []byte("policy:\n  rate_limit_enabled: false\n  max_messages_per_sec: 0\n  target_min_interval_ms: 0\n  jitter_min_ms: 0\n  jitter_max_ms: 0\n  anti_loop_window_sec: 0\n")
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Policy.RateLimitEnabled || cfg.Policy.MaxMessagesPerSec != 0 || cfg.Policy.TargetMinIntervalMs != 600 || cfg.Policy.JitterMinMs != 200 || cfg.Policy.JitterMaxMs != 800 || cfg.Policy.AntiLoopWindowSec != 3 {
		t.Fatalf("unexpected policy defaults: %+v", cfg.Policy)
	}
}

func TestMalformedConfigurationIsRejected(t *testing.T) {
	for _, data := range []string{
		"gateway: [",
		"gateway:\n  locale: xx-YY\n",
		"gateway:\n  listen: ''\n",
		"gateway:\n  max_connections: 0\n",
	} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(root); err == nil {
			t.Fatalf("invalid configuration accepted: %q", data)
		}
	}
}

func TestLegacyApplicationSettingsAreNotLoaded(t *testing.T) {
	root := t.TempDir()
	data := []byte("bot:\n  wechat_account: wxid_test\n  plugins:\n    echo:\n      enabled: true\nllm:\n  api_key: removed\ngateway:\n  locale: en-US\n")
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.WeChatAccount != "wxid_test" || cfg.Gateway.Locale != "en-US" {
		t.Fatalf("configuration lost: %+v", cfg)
	}
}
