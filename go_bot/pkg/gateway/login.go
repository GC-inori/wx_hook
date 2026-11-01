package gateway

import (
	"errors"
	"sync"
	"time"
)

// LoginStatus is the public, stable snapshot exposed by /api/login/status.
type LoginStatus struct {
	Status          string  `json:"status"`
	Wxid            string  `json:"wxid"`
	UptimeSecs      int64   `json:"uptime_secs"`
	Display         string  `json:"display"`
	PID             int     `json:"pid"`
	Socket          string  `json:"socket"`
	Listen          string  `json:"listen"`
	CPUUsageSystem  float64 `json:"cpu_usage_system"`
	CPUUsageProcess float64 `json:"cpu_usage_process"`
	MemTotalMB      uint64  `json:"mem_total_mb"`
	MemUsedMB       uint64  `json:"mem_used_mb"`
	MemProcessMB    uint64  `json:"mem_process_mb"`
	MemProcessSysMB uint64  `json:"mem_process_sys_mb"`
	Goroutines      int     `json:"goroutines"`
	RecvCount       uint64  `json:"recv_count"`
	SendCount       uint64  `json:"send_count"`
	WSConnections   int     `json:"ws_connections"`
	DroppedEvents   uint64  `json:"dropped_events"`
}

// LoginController provides the login state and lifecycle actions used by the WebUI.
type LoginController interface {
	Snapshot() LoginStatus
	QRCode() ([]byte, bool)
	Refresh() error
	Restart() error
	Logout() error
}

// LoginState is a concurrency-safe state holder. Lifecycle handlers are injected by
// the application so HTTP handlers do not own launcher or IPC implementation details.
type LoginState struct {
	mu        sync.RWMutex
	started   time.Time
	status    LoginStatus
	qr        []byte
	refreshFn func() error
	restartFn func() error
	logoutFn  func() error
}

func NewLoginState(display, socket, listen string) *LoginState {
	return &LoginState{
		started: time.Now(),
		status:  LoginStatus{Status: "launching", Display: display, Socket: socket, Listen: listen},
	}
}

func (s *LoginState) Snapshot() LoginStatus {
	if s == nil {
		return LoginStatus{Status: "stopped"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.status
	status.UptimeSecs = int64(time.Since(s.started).Seconds())
	return status
}

func (s *LoginState) QRCode() ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.qr) == 0 {
		return nil, false
	}
	return append([]byte(nil), s.qr...), true
}

// Update changes the public runtime fields atomically. Empty strings preserve the
// existing metadata; a zero PID preserves the latest known process identifier.
func (s *LoginState) Update(status, wxid, display, socket, listen string, pid int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if status != "" {
		s.status.Status = status
	}
	if wxid != "" || status == "logged_in" {
		s.status.Wxid = wxid
	}
	if display != "" {
		s.status.Display = display
	}
	if socket != "" {
		s.status.Socket = socket
	}
	if listen != "" {
		s.status.Listen = listen
	}
	if pid != 0 {
		s.status.PID = pid
	}
	if status == "logged_in" || status == "stopped" || status == "expired" {
		s.qr = nil
	}
}

func (s *LoginState) SetQRCode(png []byte) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.qr = append(s.qr[:0], png...)
	s.mu.Unlock()
}

func (s *LoginState) SetActions(refresh, restart, logout func() error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.refreshFn, s.restartFn, s.logoutFn = refresh, restart, logout
	s.mu.Unlock()
}

func (s *LoginState) Refresh() error { return s.invoke(func() func() error { return s.refreshFn }) }
func (s *LoginState) Restart() error { return s.invoke(func() func() error { return s.restartFn }) }
func (s *LoginState) Logout() error  { return s.invoke(func() func() error { return s.logoutFn }) }

func (s *LoginState) invoke(selectHandler func() func() error) error {
	if s == nil {
		return errors.New("login manager is unavailable")
	}
	s.mu.RLock()
	handler := selectHandler()
	s.mu.RUnlock()
	if handler == nil {
		return errors.New("login action is unavailable")
	}
	return handler()
}
