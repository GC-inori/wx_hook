// Package launcher manages the Linux desktop, preloaded WeChat process and login
// lifecycle used by the gateway. It never invokes a shell or Python interpreter.
package launcher

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	probeTimeout = 1500 * time.Millisecond
	maxIPCBytes  = 3 * 1024 * 1024
	maxQRBytes   = 8 * 1024 * 1024
)

// probeLogin uses one bounded connection, discards asynchronous bridge events,
// and accepts only a successful response with a nonempty account identifier.
func probeLogin(ctx context.Context, socket string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return queryLogin(ctx, conn)
}

func queryLogin(ctx context.Context, conn net.Conn) (string, error) {
	deadline := time.Now().Add(probeTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if _, err := io.WriteString(conn, "{\"id\":1,\"action\":\"get_self_info\",\"data\":{}}\n"); err != nil {
		return "", err
	}
	reader := bufio.NewReader(io.LimitReader(conn, maxIPCBytes+1))
	for i := 0; i < 128; i++ {
		line, err := reader.ReadBytes('\n')
		if len(line) > maxIPCBytes {
			return "", errors.New("launcher IPC response exceeds size limit")
		}
		if err != nil {
			return "", err
		}
		var reply struct {
			ID     uint64          `json:"id"`
			Status *int32          `json:"status"`
			Data   json.RawMessage `json:"data"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(line, &reply); err != nil {
			return "", fmt.Errorf("decode launcher IPC response: %w", err)
		}
		if reply.ID != 1 {
			continue
		}
		if reply.Status == nil || *reply.Status != 0 {
			return "", errors.New("WeChat is not logged in or the bridge is not ready")
		}
		data := reply.Result
		if len(data) == 0 || string(data) == "null" {
			data = reply.Data
		}
		var info struct {
			Wxid string `json:"wxid"`
		}
		if err := json.Unmarshal(data, &info); err != nil {
			return "", err
		}
		return strings.TrimSpace(info.Wxid), nil
	}
	return "", errors.New("too many unrelated IPC messages during login probe")
}

type qrImage struct{ data []byte }

type qrPreview struct {
	image atomic.Pointer[qrImage]
	srv   *http.Server
	addr  string
	done  chan error
}

// The preview has exactly one route and no access to project files or directory
// listings. Images remain only in memory and are removed when login finishes.
func startQRPreview(bind string, port int) (*qrPreview, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	p := &qrPreview{addr: listener.Addr().String(), done: make(chan error, 1)}
	p.srv = &http.Server{
		Handler: p, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 4096,
	}
	limited := &qrListener{Listener: listener, slots: make(chan struct{}, 32), done: make(chan struct{})}
	go func() { p.done <- p.srv.Serve(limited) }()
	return p, nil
}

// Bound accepted connections, including slow clients that never finish sending
// HTTP headers, rather than only limiting work after a request is parsed.
type qrListener struct {
	net.Listener
	slots chan struct{}
	done  chan struct{}
	once  sync.Once
}

func (l *qrListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}
	conn, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	return &qrConnection{Conn: conn, release: func() { <-l.slots }}, nil
}

func (l *qrListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type qrConnection struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *qrConnection) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}

func (p *qrPreview) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	if r.URL.Path != "/qrcode.png" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	image := p.image.Load()
	if image == nil {
		http.Error(w, "QR image not available yet", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(image.data)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(image.data)
	}
}

func (p *qrPreview) close() {
	p.image.Store(nil)
	_ = p.srv.Close()
	<-p.done
}

func waitInterval(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
