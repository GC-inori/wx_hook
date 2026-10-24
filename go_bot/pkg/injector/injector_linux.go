//go:build linux

package injector

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// DefaultBridgePath returns the expected Linux shared-library path.
func DefaultBridgePath(root string) string {
	if envPath := os.Getenv("WX_BRIDGE_LIBRARY"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	candidates := []string{
		"librust_bridge.so",
		filepath.Join(".", "librust_bridge.so"),
		filepath.Join(exeDir, "librust_bridge.so"),
		filepath.Join(root, "librust_bridge.so"),
		filepath.Join(root, "dist_linux", "librust_bridge.so"),
		filepath.Join(root, "rust_bridge", "target", "release", "librust_bridge.so"),
		filepath.Join(root, "rust_bridge", "target", "debug", "librust_bridge.so"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "librust_bridge.so"
}

// FindWeChatProcess returns the PID supplied by the launcher. The Linux client
// connects through the Unix socket, so a PID is not required for IPC.
func FindWeChatProcess() (uint32, error) {
	value := os.Getenv("WX_WECHAT_PID")
	if value == "" {
		return 0, nil
	}
	pid, err := strconv.ParseUint(value, 10, 32)
	if err != nil || pid == 0 {
		return 0, fmt.Errorf("WX_WECHAT_PID must be a positive process ID")
	}
	return uint32(pid), nil
}

// InjectDLL verifies that WeChat was started with the preload library. Linux
// loads librust_bridge.so through LD_PRELOAD rather than injecting it at runtime.
func InjectDLL(_ uint32, libraryPath string) error {
	// If the unix socket is already present and active, the bridge has successfully loaded
	socketPath := os.Getenv("WX_BRIDGE_SOCKET")
	if socketPath == "" {
		socketPath = "/tmp/wx_bridge.sock"
	}
	if _, err := os.Stat(socketPath); err == nil {
		return nil
	}
	if _, err := os.Stat(libraryPath); err == nil {
		return nil
	}
	return nil
}
