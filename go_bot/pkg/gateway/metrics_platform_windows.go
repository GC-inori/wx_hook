//go:build windows

package gateway

import (
	"syscall"
	"unsafe"
)

var (
	kernel32Metrics       = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemTimes    = kernel32Metrics.NewProc("GetSystemTimes")
	procGetProcessTimes   = kernel32Metrics.NewProc("GetProcessTimes")
	procGetCurrentProcess = kernel32Metrics.NewProc("GetCurrentProcess")
	procGlobalMemStatus   = kernel32Metrics.NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func fileTimeValue(value syscall.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}

func readSystemMemory() systemMemory {
	status := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	result, _, _ := procGlobalMemStatus.Call(uintptr(unsafe.Pointer(&status)))
	if result == 0 {
		return systemMemory{}
	}
	return systemMemory{total: status.TotalPhys, used: status.TotalPhys - status.AvailPhys}
}

func readCPUSnapshot() cpuSnapshot {
	var idle, kernel, user syscall.Filetime
	result, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)),
	)
	if result == 0 {
		return cpuSnapshot{}
	}
	total := fileTimeValue(kernel) + fileTimeValue(user)
	currentProcess, _, _ := procGetCurrentProcess.Call()
	var created, exited, processKernel, processUser syscall.Filetime
	processResult, _, _ := procGetProcessTimes.Call(
		currentProcess, uintptr(unsafe.Pointer(&created)), uintptr(unsafe.Pointer(&exited)),
		uintptr(unsafe.Pointer(&processKernel)), uintptr(unsafe.Pointer(&processUser)),
	)
	if processResult == 0 {
		return cpuSnapshot{}
	}
	return cpuSnapshot{
		total: total, idle: fileTimeValue(idle),
		proc: fileTimeValue(processKernel) + fileTimeValue(processUser), valid: total > 0,
	}
}
