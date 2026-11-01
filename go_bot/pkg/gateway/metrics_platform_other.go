//go:build !linux && !windows

package gateway

func readSystemMemory() systemMemory { return systemMemory{} }
func readCPUSnapshot() cpuSnapshot   { return cpuSnapshot{} }
