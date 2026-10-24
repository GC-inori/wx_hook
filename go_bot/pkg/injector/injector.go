//go:build windows

package injector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DefaultBridgePath returns the first available Windows bridge DLL path.
func DefaultBridgePath(root string) string {
	candidates := []string{
		filepath.Join(root, "rust_bridge", "target", "release", "rust_bridge.dll"),
		filepath.Join(root, "rust_bridge", "target", "debug", "rust_bridge.dll"),
		filepath.Join(root, "rust_bridge.dll"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procCreateRemoteThread = kernel32.NewProc("CreateRemoteThread")
	procVirtualAllocEx     = kernel32.NewProc("VirtualAllocEx")
	procVirtualFreeEx      = kernel32.NewProc("VirtualFreeEx")
	procWriteProcessMemory = kernel32.NewProc("WriteProcessMemory")
	procGetExitCodeThread  = kernel32.NewProc("GetExitCodeThread")
)

// FindWeChatProcess finds the PID of Weixin.exe
func FindWeChatProcess() (uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot failed: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Process32First(snapshot, &entry)
	if err != nil {
		return 0, fmt.Errorf("Process32First failed: %w", err)
	}

	var pids []uint32
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(name, "Weixin.exe") || strings.EqualFold(name, "WeChat.exe") {
			pids = append(pids, entry.ProcessID)
		}
		err = windows.Process32Next(snapshot, &entry)
		if err != nil {
			break
		}
	}

	if len(pids) == 0 {
		return 0, fmt.Errorf("未找到运行中的微信进程 (Weixin.exe)")
	}

	// Returns the first PID found
	return pids[0], nil
}

// IsModuleLoaded checks if a dll is already loaded in the target process
func IsModuleLoaded(pid uint32, dllName string) bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE|windows.TH32CS_SNAPMODULE32, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ModuleEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Module32First(snapshot, &entry)
	if err != nil {
		return false
	}

	for {
		name := windows.UTF16ToString(entry.Module[:])
		if strings.EqualFold(name, dllName) {
			return true
		}
		err = windows.Module32Next(snapshot, &entry)
		if err != nil {
			break
		}
	}
	return false
}

// InjectDLL injects a DLL into the target process by PID using LoadLibraryW
func InjectDLL(pid uint32, dllPath string) error {
	absPath, err := filepath.Abs(dllPath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %w", err)
	}

	dllName := filepath.Base(absPath)
	if IsModuleLoaded(pid, dllName) {
		return nil // Already loaded
	}

	hProcess, err := windows.OpenProcess(
		windows.PROCESS_CREATE_THREAD|windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_OPERATION|windows.PROCESS_VM_WRITE|windows.PROCESS_VM_READ,
		false,
		pid,
	)
	if err != nil {
		return fmt.Errorf("打开目标进程失败 (PID=%d): %w", pid, err)
	}
	defer windows.CloseHandle(hProcess)

	pathUtf16, err := windows.UTF16FromString(absPath)
	if err != nil {
		return fmt.Errorf("路径转 UTF16 失败: %w", err)
	}
	pathBytes := len(pathUtf16) * 2

	// Allocate memory in target process
	addr, _, errAlloc := procVirtualAllocEx.Call(
		uintptr(hProcess),
		0,
		uintptr(pathBytes),
		windows.MEM_COMMIT|windows.MEM_RESERVE,
		windows.PAGE_READWRITE,
	)
	if addr == 0 {
		return fmt.Errorf("VirtualAllocEx 失败: %v", errAlloc)
	}
	defer procVirtualFreeEx.Call(uintptr(hProcess), addr, 0, windows.MEM_RELEASE)

	// Write DLL path into target process
	var bytesWritten uintptr
	resWrite, _, errWrite := procWriteProcessMemory.Call(
		uintptr(hProcess),
		addr,
		uintptr(unsafe.Pointer(&pathUtf16[0])),
		uintptr(pathBytes),
		uintptr(unsafe.Pointer(&bytesWritten)),
	)
	if resWrite == 0 {
		return fmt.Errorf("WriteProcessMemory 失败: %v", errWrite)
	}

	// Get address of LoadLibraryW in kernel32
	modKernel32, err := windows.LoadLibrary("kernel32.dll")
	if err != nil {
		return fmt.Errorf("加载 kernel32.dll 失败: %w", err)
	}
	procLoadLibraryW, err := windows.GetProcAddress(modKernel32, "LoadLibraryW")
	if err != nil {
		return fmt.Errorf("查找 LoadLibraryW 失败: %w", err)
	}

	// CreateRemoteThread to call LoadLibraryW(addr)
	hThread, _, errThread := procCreateRemoteThread.Call(
		uintptr(hProcess),
		0,
		0,
		procLoadLibraryW,
		addr,
		0,
		0,
	)
	if hThread == 0 {
		return fmt.Errorf("CreateRemoteThread 失败: %v", errThread)
	}
	defer windows.CloseHandle(windows.Handle(hThread))

	// Wait for thread to finish
	windows.WaitForSingleObject(windows.Handle(hThread), windows.INFINITE)

	var exitCode uint32
	procGetExitCodeThread.Call(uintptr(hThread), uintptr(unsafe.Pointer(&exitCode)))
	if exitCode == 0 {
		return fmt.Errorf("LoadLibraryW 执行失败 (返回值为 NULL)")
	}

	return nil
}
