package launcher

import (
	"os"

	"wxbot/pkg/config"
)

// SocketPath returns the configured bridge socket, including the environment
// override shared with the Rust bridge.
func SocketPath(cfg *config.Config) string {
	if socket := os.Getenv("WX_BRIDGE_SOCKET"); socket != "" {
		return socket
	}
	if cfg != nil && cfg.Bot.IpcSocket != "" {
		return cfg.Bot.IpcSocket
	}
	return "/tmp/wx_bridge.sock"
}
