//go:build windows

package client

import (
	"fmt"
	"net"
	"os"
	"time"
)

type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

func ipcEndpoint(pid uint32) string {
	return fmt.Sprintf(`\\.\pipe\wxbot_ipc_%d`, pid)
}

type pipeConn struct {
	file *os.File
}

func dialPipe(pipePath string) (net.Conn, error) {
	f, err := os.OpenFile(pipePath, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &pipeConn{file: f}, nil
}

func (p *pipeConn) Read(b []byte) (n int, err error) {
	return p.file.Read(b)
}

func (p *pipeConn) Write(b []byte) (n int, err error) {
	return p.file.Write(b)
}

func (p *pipeConn) Close() error {
	return p.file.Close()
}

func (p *pipeConn) LocalAddr() net.Addr {
	return pipeAddr(p.file.Name())
}

func (p *pipeConn) RemoteAddr() net.Addr {
	return pipeAddr(p.file.Name())
}

func (p *pipeConn) SetDeadline(t time.Time) error      { return nil }
func (p *pipeConn) SetReadDeadline(t time.Time) error  { return nil }
func (p *pipeConn) SetWriteDeadline(t time.Time) error { return nil }
