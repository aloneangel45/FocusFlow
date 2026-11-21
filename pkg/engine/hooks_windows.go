//go:build windows

package engine

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func normalizeWindowsHookCommand(command string) string {
	normalized := strings.ReplaceAll(command, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if !strings.Contains(normalized, "\n") {
		return normalized
	}
	lines := strings.Split(normalized, "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, " & ")
}

func killWindowsProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	killCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tk := exec.CommandContext(killCtx, "taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid))
	tk.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return tk.Run()
}

func buildPlatformHookCommand(ctx context.Context, command string, _ HookOptions) (*exec.Cmd, error) {
	safeCmd := normalizeWindowsHookCommand(command)
	cmd := exec.CommandContext(ctx, "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		CmdLine:    `cmd.exe /D /S /C "` + safeCmd + `"`,
	}
	// On timeout/cancellation, kill the entire child process tree (/T /F) so nested
	// children (e.g. ping, powershell, scripts) do not linger or keep pipes open.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		_ = killWindowsProcessTree(cmd.Process.Pid)
		return cmd.Process.Kill()
	}
	cmd.Env = os.Environ()
	return cmd, nil
}
