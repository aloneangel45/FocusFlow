//go:build !windows

package engine

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// ScanAndTerminate scans active processes on POSIX systems (Linux via /proc without spawning
// subprocesses, or macOS/BSD via ps when /proc is unavailable), terminating matching targets with SIGTERM.
func (p *ProcessSupervisor) ScanAndTerminate() []string {
	var terminated []string
	if len(p.blockedNames) == 0 {
		return terminated
	}

	// Strategy 1: Linux /proc scanning — if /proc is mounted and has PID directories,
	// return directly even when 0 matches occur (never spawn `ps` every tick on Linux!).
	if procMatches, procAvailable := p.scanProcDir("/proc"); procAvailable {
		return procMatches
	}

	// Strategy 2: Standard POSIX `ps` fallback for macOS / BSD
	return p.scanPsOutput()
}

func (p *ProcessSupervisor) scanProcDir(procRoot string) ([]string, bool) {
	var terminated []string
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, false
	}

	sawPID := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		sawPID = true
		if isProtectedPID(pid) {
			continue
		}

		candidates := resolveProcFSNames(procRoot, entry.Name())
		if matchedName, ok := p.shouldTerminateProcess(pid, candidates...); ok {
			if proc, err := os.FindProcess(pid); err == nil {
				if err := proc.Signal(syscall.SIGTERM); err == nil {
					terminated = append(terminated, fmt.Sprintf("%s (PID: %d)", matchedName, pid))
				}
			}
		}
	}

	return terminated, sawPID
}

func (p *ProcessSupervisor) scanPsOutput() []string {
	var terminated []string
	cmd := exec.Command("ps", "-A", "-o", "pid=,comm=")
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
		if err != nil || isProtectedPID(pid) {
			continue
		}

		fullComm := strings.Join(fields[1:], " ")
		if matchedName, ok := p.shouldTerminateProcess(pid, fullComm); ok {
			if proc, err := os.FindProcess(pid); err == nil {
				if err := proc.Signal(syscall.SIGTERM); err == nil {
					terminated = append(terminated, fmt.Sprintf("%s (PID: %d)", matchedName, pid))
				}
			}
		}
	}

	return terminated
}
