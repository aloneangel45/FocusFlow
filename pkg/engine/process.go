package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

type ProcessSupervisor struct {
	blockedNames map[string]struct{}
}

func NewProcessSupervisor(blocked []string) *ProcessSupervisor {
	m := make(map[string]struct{})
	for _, b := range blocked {
		trimmed := strings.ToLower(strings.TrimSpace(b))
		if trimmed == "" {
			continue
		}
		if idx := strings.LastIndexAny(trimmed, `/\`); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		if trimmed == "" || IsProtectedProcess(trimmed) {
			continue
		}
		m[trimmed] = struct{}{}
		if strings.HasSuffix(trimmed, ".exe") {
			base := strings.TrimSuffix(trimmed, ".exe")
			if base != "" && !IsProtectedProcess(base) {
				m[base] = struct{}{}
			}
		} else {
			withExe := trimmed + ".exe"
			if !IsProtectedProcess(withExe) {
				m[withExe] = struct{}{}
			}
		}
	}
	return &ProcessSupervisor{blockedNames: m}
}

func isProtectedPID(pid int) bool {
	if pid <= 4 {
		return true
	}
	if pid == os.Getpid() || pid == os.Getppid() {
		return true
	}
	return false
}

func cleanBasename(pathOrName string) string {
	s := strings.TrimSpace(pathOrName)
	if s == "" {
		return ""
	}
	if idx := strings.LastIndexAny(s, `/\`); idx >= 0 {
		s = s[idx+1:]
	}
	return s
}

func (p *ProcessSupervisor) shouldTerminateProcess(pid int, candidateNames ...string) (string, bool) {
	if isProtectedPID(pid) || len(p.blockedNames) == 0 {
		return "", false
	}
	for _, cand := range candidateNames {
		base := cleanBasename(cand)
		if base == "" {
			continue
		}
		if IsProtectedProcess(base) {
			return "", false
		}
	}
	for _, cand := range candidateNames {
		base := cleanBasename(cand)
		if base == "" {
			continue
		}
		lower := strings.ToLower(base)
		if _, exists := p.blockedNames[lower]; exists {
			return base, true
		}
	}
	return "", false
}

// resolveProcFSNames resolves process executable candidate names from a Linux /proc/<pid> entry.
// When /proc/<pid>/comm hits the 15-byte Linux TASK_COMM_LEN truncation limit (or is unreadable),
// it also resolves /proc/<pid>/exe symlink target and /proc/<pid>/cmdline argv[0] basename.
func resolveProcFSNames(procRoot string, pidStr string) []string {
	var candidates []string
	pidDir := filepath.Join(procRoot, pidStr)

	commBytes, commErr := os.ReadFile(filepath.Join(pidDir, "comm"))
	var commName string
	if commErr == nil {
		commName = strings.TrimSpace(string(commBytes))
		if commName != "" {
			candidates = append(candidates, commName)
		}
	}

	// Linux kernel TASK_COMM_LEN is 16 bytes (max 15 visible chars in /proc/<pid>/comm).
	if commName == "" || len(commName) >= 15 {
		if exeTarget, err := os.Readlink(filepath.Join(pidDir, "exe")); err == nil {
			exeTarget = strings.TrimSuffix(exeTarget, " (deleted)")
			if base := cleanBasename(exeTarget); base != "" {
				candidates = append(candidates, base)
			}
		}
		if cmdBytes, err := os.ReadFile(filepath.Join(pidDir, "cmdline")); err == nil && len(cmdBytes) > 0 {
			argv0 := cmdBytes
			if nulIdx := bytes.IndexByte(cmdBytes, 0); nulIdx >= 0 {
				argv0 = cmdBytes[:nulIdx]
			}
			argStr := strings.TrimSpace(string(argv0))
			if argStr != "" {
				// Handle Wine / proton style Windows paths or POSIX paths in cmdline
				if base := cleanBasename(argStr); base != "" {
					candidates = append(candidates, base)
				}
			}
		}
	}

	return candidates
}

// KillProcessesByNames terminates running processes matching any of the specified names.
func KillProcessesByNames(names []string) error {
	sup := NewProcessSupervisor(names)
	_ = sup.ScanAndTerminate()
	return nil
}

// KillProcessesByNames terminates processes for this supervisor.
func (p *ProcessSupervisor) KillProcessesByNames(names []string) error {
	sup := NewProcessSupervisor(names)
	_ = sup.ScanAndTerminate()
	return nil
}
