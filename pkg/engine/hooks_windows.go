//go:build windows

package engine

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

func buildPlatformHookCommand(ctx context.Context, command string, _ HookOptions) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		CmdLine:    `cmd.exe /D /S /C "` + command + `"`,
	}
	cmd.Env = os.Environ()
	return cmd, nil
}
