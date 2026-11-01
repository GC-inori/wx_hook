package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLauncherDefaults(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Launcher
	if !c.AutoStart || !c.UseXvfb || c.WeChatBin != "" || c.BridgeLib != "./librust_bridge.so" || c.Display != ":99" || c.XvfbScreen != "1280x1024x24" || c.QRPort != 8899 || c.QRBind != "0.0.0.0" || c.LoginTimeout != 120 {
		t.Fatalf("unexpected launcher defaults: %+v", c)
	}
}

func TestLauncherExplicitConfiguration(t *testing.T) {
	root := t.TempDir()
	data := "launcher:\n  auto_start: false\n  wechat_bin: /custom/wechat\n  bridge_lib: bridge.so\n  display: ':12.0'\n  use_xvfb: false\n  xvfb_screen: 800x600x16\n  qr_port: 0\n  qr_bind: 127.0.0.1\n  login_timeout: 5\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Launcher
	if c.AutoStart || c.UseXvfb || c.QRPort != 0 || c.LoginTimeout != 5 || c.WeChatBin != "/custom/wechat" || c.BridgeLib != "bridge.so" || c.Display != ":12.0" || c.XvfbScreen != "800x600x16" || c.QRBind != "127.0.0.1" {
		t.Fatalf("explicit launcher configuration lost: %+v", c)
	}
}

func TestInvalidLauncherConfiguration(t *testing.T) {
	for _, field := range []string{
		"qr_port: -1", "qr_port: 65536", "login_timeout: 0", "login_timeout: 86401",
		"bridge_lib: ''", "bridge_lib: 'bad library.so'", "bridge_lib: 'lib:other.so'",
		"display: 'remote:99'", "display: ':65536'", "display: ':99/../0'",
		"xvfb_screen: '1280x1024'", "xvfb_screen: '0x600x24'", "xvfb_screen: '99999x600x24'",
		"qr_bind: 'not-an-ip'",
	} {
		t.Run(field, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("launcher:\n  "+field+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(root); err == nil {
				t.Fatal("invalid launcher field accepted")
			}
		})
	}
}

func TestRelativeSocketResolvedFromRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("bot:\n  ipc_socket: runtime/bridge.sock\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.IpcSocket != filepath.Join(root, "runtime", "bridge.sock") {
		t.Fatalf("relative socket not rooted: %s", cfg.Bot.IpcSocket)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("bot:\n  ipc_socket: /tmp/bridge.sock\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(root)
	if err != nil || cfg.Bot.IpcSocket != "/tmp/bridge.sock" {
		t.Fatalf("absolute Linux socket changed during cross-platform config validation: %+v %v", cfg, err)
	}
}

func TestDisplayNumber(t *testing.T) {
	for _, display := range []string{":0", ":99", ":99.0", ":99.12"} {
		got, err := (LauncherConfig{Display: display}).DisplayNumber()
		if err != nil || (display != ":0" && got != 99) {
			t.Fatalf("display %s: %d, %v", display, got, err)
		}
	}
}
