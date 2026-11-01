package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"wxbot/pkg/client"
	"wxbot/pkg/protocol"
)

const maxLogEntries = 500

// LogEntry is a recent standard logger line exposed to the dashboard.
type LogEntry struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

type logBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	limit   int
}

func newLogBuffer(capacity int) *logBuffer {
	if capacity <= 0 {
		capacity = maxLogEntries
	}
	return &logBuffer{entries: make([]LogEntry, 0, capacity), limit: capacity}
}

func (b *logBuffer) setLimit(capacity int) {
	if capacity <= 0 {
		capacity = maxLogEntries
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.limit = capacity
	if len(b.entries) > capacity {
		b.entries = append([]LogEntry(nil), b.entries[len(b.entries)-capacity:]...)
	}
}

func (b *logBuffer) add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) >= b.limit {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = entry
		return
	}
	b.entries = append(b.entries, entry)
}

func (b *logBuffer) snapshot() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]LogEntry(nil), b.entries...)
}

// AddLog stores a log line for the dashboard and publishes it to WebSocket clients.
func (s *Server) AddLog(level, msg string) {
	level = strings.ToUpper(strings.TrimSpace(level))
	switch level {
	case "INFO", "WARN", "ERROR":
	default:
		level = "INFO"
	}
	entry := LogEntry{Time: time.Now().Format("15:04:05"), Level: level, Msg: msg}
	s.logs.add(entry)
	data, _ := json.Marshal(struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		Time    string `json:"time"`
	}{Level: entry.Level, Message: entry.Msg, Time: entry.Time})
	s.Publish(client.Event{Event: "log", Data: data})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeLoginJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	s.writeLoginJSON(w, http.StatusOK, s.logs.snapshot())
}

type botInfo struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Wxid         string `json:"wxid"`
	UptimeSecs   int64  `json:"uptime_secs"`
	Display      string `json:"display"`
	PID          int    `json:"pid"`
	Socket       string `json:"socket"`
	Listen       string `json:"listen"`
	ActionsCount int    `json:"actions_count"`
}

func (s *Server) handleBotInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeLoginJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	info := botInfo{
		Name: "WeChat Native Hook", Version: "4.1.15.55",
		UptimeSecs: int64(time.Since(s.started).Seconds()),
		Listen:     s.currentGatewayConfig().Listen, ActionsCount: len(protocol.Actions),
	}
	if s.login != nil {
		status := s.login.Snapshot()
		info.Wxid = status.Wxid
		info.UptimeSecs = status.UptimeSecs
		info.Display = status.Display
		info.PID = status.PID
		info.Socket = status.Socket
		info.Listen = status.Listen
	}
	s.writeLoginJSON(w, http.StatusOK, info)
}
