//go:build windows

package engine

import (
	"fmt"
	"syscall"
	"unsafe"
)

// ScanAndTerminate scans active Windows processes via Toolhelp32 snapshot,
// terminating any non-protected process whose executable name matches the blocklist.
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
		pid := int(entry.ProcessID)
		exeName := syscall.UTF16ToString(entry.ExeFile[:])

		if matchedName, ok := p.shouldTerminateProcess(pid, exeName); ok {
			hProc, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, entry.ProcessID)
			if err == nil && hProc != 0 {
				if err := syscall.TerminateProcess(hProc, 1); err == nil {
					terminated = append(terminated, fmt.Sprintf("%s (PID: %d)", matchedName, entry.ProcessID))
				}
				_ = syscall.CloseHandle(hProc)
			}
		}

		if err := syscall.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}

	return terminated
}
