//go:build darwin

package engine

import (
	"context"
	"os/exec"
)

// buildDarwinNotifyCommand uses `osascript` `on run argv` so title and body are passed
// as raw argv parameters rather than interpolated into AppleScript source code.
func buildDarwinNotifyCommand(ctx context.Context, title, body string) *exec.Cmd {
	return exec.CommandContext(
		ctx,
		"osascript",
		"-e", "on run argv",
		"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
		"-e", "end run",
		"--",
		SanitizeNotificationText(title, 120),
		SanitizeNotificationText(body, 256),
	)
}

func sendNativeNotification(ctx context.Context, title, body string) error {
	if _, err := exec.LookPath("osascript"); err != nil {
		return nil
	}
	cmd := buildDarwinNotifyCommand(ctx, title, body)
	_ = cmd.Run()
	return nil
}
