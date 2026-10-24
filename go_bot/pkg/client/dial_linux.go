//go:build linux

package client

import (
	"net"
	"os"
)

const defaultSocketPath = "/tmp/wx_bridge.sock"

func ipcEndpoint(_ uint32) string {
	if path := os.Getenv("WX_BRIDGE_SOCKET"); path != "" {
		return path
	}
	return defaultSocketPath
}

func dialPipe(socketPath string) (net.Conn, error) {
	return net.Dial("unix", socketPath)
}
