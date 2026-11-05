//go:build windows

package engine

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

// ScanAndTerminate scans active Windows processes via Toolhelp32 snapshot,
// terminating any process whose executable name matches the supervisor's blocklist.
func (p *ProcessSupervisor) ScanAndTerminate() []string {
	var terminated []string
	if len(p.blockedNames) == 0 {
		return terminated
	}

	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil || snapshot == syscall.InvalidHandle {
		return terminated
	}
	defer syscall.CloseHandle(snapshot)

	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := syscall.Process32First(snapshot, &entry); err != nil {
		return terminated
	}

	for {
		exeName := syscall.UTF16ToString(entry.ExeFile[:])
		exeLower := strings.ToLower(exeName)

		if _, exists := p.blockedNames[exeLower]; exists {
			hProc, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, entry.ProcessID)
			if err == nil && hProc != 0 {
				if err := syscall.TerminateProcess(hProc, 1); err == nil {
					terminated = append(terminated, fmt.Sprintf("%s (PID: %d)", exeName, entry.ProcessID))
				}
				syscall.CloseHandle(hProc)
			}
		}

		if err := syscall.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}

	return terminated
}
