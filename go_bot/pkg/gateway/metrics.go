package gateway

import (
	"math"
	"runtime"
	"sync"
	"time"
)

type cpuSnapshot struct {
	total uint64
	idle  uint64
	proc  uint64
	valid bool
}

type systemMemory struct {
	total uint64
	used  uint64
}

type resourceMetrics struct {
	CPUUsageSystem  float64 `json:"cpu_usage_system"`
	CPUUsageProcess float64 `json:"cpu_usage_process"`
	MemTotalMB      uint64  `json:"mem_total_mb"`
	MemUsedMB       uint64  `json:"mem_used_mb"`
	MemProcessMB    uint64  `json:"mem_process_mb"`
	MemProcessSysMB uint64  `json:"mem_process_sys_mb"`
	Goroutines      int     `json:"goroutines"`
	UptimeSecs      int64   `json:"uptime_secs"`
	RecvCount       uint64  `json:"recv_count"`
	SendCount       uint64  `json:"send_count"`
	WSConnections   int     `json:"ws_connections"`
	DroppedEvents   uint64  `json:"dropped_events"`
}

type metricsCollector struct {
	mu   sync.Mutex
	last cpuSnapshot
}

func (m *metricsCollector) sampleCPU() (system, process float64) {
	now := readCPUSnapshot()
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.last
	m.last = now
	if !now.valid || !previous.valid || now.total <= previous.total {
		return 0, 0
	}
	totalDelta := now.total - previous.total
	idleDelta := uint64(0)
	if now.idle >= previous.idle {
		idleDelta = now.idle - previous.idle
	}
	procDelta := uint64(0)
	if now.proc >= previous.proc {
		procDelta = now.proc - previous.proc
	}
	system = 100 * float64(totalDelta-idleDelta) / float64(totalDelta)
	process = 100 * float64(procDelta) / float64(totalDelta)
	return clampPercent(system), clampPercent(process)
}

func clampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func (s *Server) collectMetrics() resourceMetrics {
	var memoryStats runtime.MemStats
	runtime.ReadMemStats(&memoryStats)
	memory := readSystemMemory()
	if memory.total == 0 {
		// Fall back to runtime-managed memory on platforms where querying total
		// physical memory is unavailable.
		memory.total = memoryStats.Sys
		memory.used = memoryStats.Alloc
	}
	cpuSystem, cpuProcess := s.metrics.sampleCPU()
	connections := s.hub.count()
	s.wsClientsMu.Lock()
	for _, endpoint := range s.wsClients {
		if endpoint.connection() {
			connections++
		}
	}
	s.wsClientsMu.Unlock()
	var dropped uint64
	if caller := s.currentCaller(); caller != nil {
		if state, ok := caller.(interface{ DroppedEvents() uint64 }); ok {
			dropped = state.DroppedEvents()
		}
	}
	return resourceMetrics{
		CPUUsageSystem: cpuSystem, CPUUsageProcess: cpuProcess,
		MemTotalMB: memory.total / (1024 * 1024), MemUsedMB: memory.used / (1024 * 1024),
		MemProcessMB: memoryStats.Alloc / (1024 * 1024), MemProcessSysMB: memoryStats.Sys / (1024 * 1024),
		Goroutines: runtime.NumGoroutine(), UptimeSecs: int64(time.Since(s.started).Seconds()),
		RecvCount: s.recvCount.Load(), SendCount: s.sendCount.Load(),
		WSConnections: connections, DroppedEvents: dropped,
	}
}
