//go:build !windows

package engine

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ScanAndTerminate scans active processes on POSIX systems (Linux via /proc, or macOS/BSD via ps),
// terminating any process whose name matches the supervisor's blocklist with SIGTERM.
func (p *ProcessSupervisor) ScanAndTerminate() []string {
	var terminated []string
	if len(p.blockedNames) == 0 {
		return terminated
	}

	// Strategy 1: Linux /proc scanning (efficient and direct)
	if _, err := os.Stat("/proc"); err == nil {
		procMatches := p.scanProcDir()
		if len(procMatches) > 0 {
			return procMatches
		}
	}

	// Strategy 2: Standard POSIX `ps` tool (macOS, BSD, and Linux fallback)
	psMatches := p.scanPsOutput()
	if len(psMatches) > 0 {
		return psMatches
	}

	return terminated
}

func (p *ProcessSupervisor) scanProcDir() []string {
	var terminated []string
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return terminated
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 {
			continue // skip non-numeric dirs and init process
		}

		// Read /proc/<pid>/comm (process name)
		commPath := filepath.Join("/proc", entry.Name(), "comm")
		commBytes, err := os.ReadFile(commPath)
		var procName string
		if err == nil {
			procName = strings.TrimSpace(string(commBytes))
		} else {
			// Fallback: check /proc/<pid>/exe symbolic link target
			exePath := filepath.Join("/proc", entry.Name(), "exe")
			target, err := os.Readlink(exePath)
			if err == nil {
				procName = filepath.Base(target)
			}
		}

		if procName == "" {
			continue
		}

		procLower := strings.ToLower(procName)
		if _, exists := p.blockedNames[procLower]; exists {
			proc, err := os.FindProcess(pid)
			if err == nil {
				if err := proc.Signal(syscall.SIGTERM); err == nil {
					terminated = append(terminated, fmt.Sprintf("%s (PID: %d)", procName, pid))
				}
			}
		}
	}

	return terminated
}

func (p *ProcessSupervisor) scanPsOutput() []string {
	var terminated []string
	cmd := exec.Command("ps", "-A", "-o", "pid,comm")
	out, err := cmd.Output()
	if err != nil {
		return terminated
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(strings.ToUpper(line), "PID") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 1 {
			continue
		}

		fullComm := strings.Join(fields[1:], " ")
		procName := filepath.Base(fullComm)
		procLower := strings.ToLower(procName)

		if _, exists := p.blockedNames[procLower]; exists {
			proc, err := os.FindProcess(pid)
			if err == nil {
				if err := proc.Signal(syscall.SIGTERM); err == nil {
					terminated = append(terminated, fmt.Sprintf("%s (PID: %d)", procName, pid))
				}
			}
		}
	}

	return terminated
}
