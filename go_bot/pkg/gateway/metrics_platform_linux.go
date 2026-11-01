//go:build linux

package gateway

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func readSystemMemory() systemMemory {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return systemMemory{}
	}
	defer file.Close()
	values := make(map[string]uint64, 3)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if fields[0] != "MemTotal:" && fields[0] != "MemAvailable:" && fields[0] != "MemFree:" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			values[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	total := values["MemTotal"]
	available, ok := values["MemAvailable"]
	if !ok {
		available = values["MemFree"]
	}
	if total == 0 {
		return systemMemory{}
	}
	if available > total {
		available = total
	}
	return systemMemory{total: total, used: total - available}
}

func readCPUSnapshot() cpuSnapshot {
	var result cpuSnapshot
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		if len(fields) < 4 {
			return result
		}
		var total, idle uint64
		for i, field := range fields {
			value, parseErr := strconv.ParseUint(field, 10, 64)
			if parseErr != nil {
				return result
			}
			total += value
			if i == 3 || i == 4 { // idle and iowait
				idle += value
			}
		}
		result.total, result.idle = total, idle
		break
	}
	processData, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return result
	}
	closeParen := strings.LastIndexByte(string(processData), ')')
	if closeParen < 0 || closeParen+1 >= len(processData) {
		return result
	}
	fields := strings.Fields(string(processData[closeParen+1:]))
	// The suffix starts at field 3: utime and stime are indexes 11 and 12.
	if len(fields) < 13 {
		return result
	}
	user, errUser := strconv.ParseUint(fields[11], 10, 64)
	system, errSystem := strconv.ParseUint(fields[12], 10, 64)
	if errUser != nil || errSystem != nil {
		return result
	}
	result.proc = user + system
	result.valid = result.total > 0
	return result
}
