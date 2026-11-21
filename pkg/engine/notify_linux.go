//go:build !windows && !darwin

package engine

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

func buildLinuxNotifyCommand(ctx context.Context, binPath, title, body string) *exec.Cmd {
	return exec.CommandContext(
		ctx,
		binPath,
		"--app-name=FocusFlow",
		"--",
		SanitizeNotificationText(title, 120),
		SanitizeNotificationText(body, 256),
	)
}

func sendNativeNotification(ctx context.Context, title, body string) error {
	// Silently no-op in headless/SSH environments without a graphical session
	if strings.TrimSpace(os.Getenv("DISPLAY")) == "" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == "" {
		return nil
	}
	binPath, err := exec.LookPath("notify-send")
	if err != nil {
		return nil
	}
	cmd := buildLinuxNotifyCommand(ctx, binPath, title, body)
	_ = cmd.Run()
	return nil
}
