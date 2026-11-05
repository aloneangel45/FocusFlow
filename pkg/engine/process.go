package engine

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

type ProcessSupervisor struct {
	blockedNames map[string]struct{}
}

func NewProcessSupervisor(blocked []string) *ProcessSupervisor {
	m := make(map[string]struct{})
	for _, b := range blocked {
		m[strings.ToLower(strings.TrimSpace(b))] = struct{}{}
	}
	return &ProcessSupervisor{blockedNames: m}
}

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
