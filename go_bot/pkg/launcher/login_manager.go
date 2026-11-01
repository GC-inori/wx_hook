package launcher

import (
	"errors"
	"sync"
	"time"

	"wxbot/pkg/gateway"
)

// LoginManager keeps the login state visible to the WebUI and coordinates
// lifecycle actions with the background runtime loop.
type LoginManager struct {
	mu        sync.RWMutex
	started   time.Time
	status    gateway.LoginStatus
	qr        []byte
	refresh   chan struct{}
	restart   chan struct{}
	logout    chan struct{}
	stopOwned func()
}

func NewLoginManager(display, socket, listen string) *LoginManager {
	return &LoginManager{
		started: time.Now(),
		status: gateway.LoginStatus{
			Status: "launching", Display: display, Socket: socket, Listen: listen,
		},
		refresh: make(chan struct{}, 1),
		restart: make(chan struct{}, 1),
		logout:  make(chan struct{}, 1),
	}
}

func (m *LoginManager) Snapshot() gateway.LoginStatus {
	if m == nil {
		return gateway.LoginStatus{Status: "stopped"}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.UptimeSecs = int64(time.Since(m.started).Seconds())
	return status
}

func (m *LoginManager) QRCode() ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.qr) == 0 {
		return nil, false
	}
	return append([]byte(nil), m.qr...), true
}

// Update atomically refreshes public login metadata. Empty values preserve
// metadata already learned from the launcher.
func (m *LoginManager) Update(status, wxid string, pid int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if status != "" {
		m.status.Status = status
	}
	if wxid != "" || status == "logged_in" || status == "stopped" || status == "launching" || status == "waiting_scan" {
		m.status.Wxid = wxid
	}
	if pid != 0 {
		m.status.PID = pid
	}
	if status == "logged_in" || status == "stopped" || status == "expired" || status == "launching" {
		m.qr = nil
	}
}

func (m *LoginManager) SetQRCode(png []byte) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.qr = append(m.qr[:0], png...)
	m.mu.Unlock()
}

func (m *LoginManager) Refresh() error {
	if m == nil {
		return errors.New("login manager is unavailable")
	}
	if m.Snapshot().Status == "logged_in" {
		return nil
	}
	return enqueueLoginAction(m.refresh)
}

func (m *LoginManager) Restart() error {
	if m == nil {
		return errors.New("login manager is unavailable")
	}
	return enqueueLoginAction(m.restart)
}

func (m *LoginManager) Logout() error {
	if m == nil {
		return errors.New("login manager is unavailable")
	}
	return enqueueLoginAction(m.logout)
}

func enqueueLoginAction(action chan struct{}) error {
	select {
	case action <- struct{}{}:
	default: // Coalesce repeated clicks while the action is already pending.
	}
	return nil
}

func (m *LoginManager) RefreshRequests() <-chan struct{} { return m.refresh }
func (m *LoginManager) RestartRequests() <-chan struct{} { return m.restart }
func (m *LoginManager) LogoutRequests() <-chan struct{}  { return m.logout }

func (m *LoginManager) setOwnedStop(stop func()) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.stopOwned = stop
	m.mu.Unlock()
}

// StopOwned stops only the Xvfb/WeChat processes launched by this runtime.
func (m *LoginManager) StopOwned() {
	if m == nil {
		return
	}
	m.mu.Lock()
	stop := m.stopOwned
	m.stopOwned = nil
	m.mu.Unlock()
	if stop != nil {
		stop()
	}
}
