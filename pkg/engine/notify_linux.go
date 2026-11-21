//go:build !windows && !darwin

package engine

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

func buildLinuxNotifyCommand(ctx context.Context, binPath, title, body string) *exec.Cmd {
	// Escape XML/Pango markup in body since libnotify/notify-send parses body as Pango markup.
	cmd := exec.CommandContext(
		ctx,
		binPath,
		"--app-name=FocusFlow",
		"--",
		SanitizeNotificationText(title, 120),
		EscapeXMLText(SanitizeNotificationText(body, 256)),
	)
	cmd.WaitDelay = 250 * time.Millisecond
	return cmd
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
