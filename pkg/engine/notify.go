package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

const DefaultNotifyTimeout = 5 * time.Second

// Notifier dispatches non-blocking native OS desktop notifications.
// Notification failures are always swallowed so focus sessions are never blocked or aborted.
type Notifier struct {
	Enabled bool
	Timeout time.Duration
	sendFn  func(ctx context.Context, title, body string) error
}

// NewNotifier creates a native OS notifier. Passing enabled=false disables all notifications.
func NewNotifier(enabled bool) *Notifier {
	return &Notifier{
		Enabled: enabled,
		Timeout: DefaultNotifyTimeout,
		sendFn:  sendNativeNotification,
	}
}

func isNotifySuppressedByEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FOCUSFLOW_NO_NOTIFY")))
	return v == "1" || v == "true" || v == "yes"
}

// SanitizeNotificationText strips NUL and control characters, collapses whitespace newlines,
// and bounds length to maxRunes to ensure safe native OS notification arguments.
func SanitizeNotificationText(s string, maxRunes int) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\r' || r == '\n' || r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			continue
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if maxRunes > 0 {
		runes := []rune(cleaned)
		if len(runes) > maxRunes {
			cleaned = string(runes[:maxRunes])
		}
	}
	return cleaned
}

// EscapePowerShellSingleQuoted sanitizes control characters and escapes single quotes
// for safe embedding inside a PowerShell single-quoted literal.
func EscapePowerShellSingleQuoted(s string) string {
	clean := SanitizeNotificationText(s, 256)
	return strings.ReplaceAll(clean, "'", "''")
}

// EscapeAppleScriptString sanitizes control characters and escapes backslashes and double
// quotes for safe embedding inside an AppleScript double-quoted string literal.
func EscapeAppleScriptString(s string) string {
	clean := SanitizeNotificationText(s, 256)
	clean = strings.ReplaceAll(clean, `\`, `\\`)
	clean = strings.ReplaceAll(clean, `"`, `\"`)
	return clean
}

// EscapeXMLText sanitizes control characters and escapes XML special characters.
func EscapeXMLText(s string) string {
	clean := SanitizeNotificationText(s, 256)
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(clean)
}

// Send dispatches a notification synchronously with a bounded timeout.
func (n *Notifier) Send(ctx context.Context, title, body string) error {
	if n == nil || !n.Enabled || isNotifySuppressedByEnv() {
		return nil
	}
	cleanTitle := SanitizeNotificationText(title, 120)
	cleanBody := SanitizeNotificationText(body, 256)
	if cleanTitle == "" && cleanBody == "" {
		return nil
	}
	if cleanTitle == "" {
		cleanTitle = "FocusFlow"
	}

	timeout := n.Timeout
	if timeout <= 0 {
		timeout = DefaultNotifyTimeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fn := n.sendFn
	if fn == nil {
		fn = sendNativeNotification
	}
	return fn(tctx, cleanTitle, cleanBody)
}

// SendAsync dispatches a notification in a detached goroutine without blocking the caller.
func (n *Notifier) SendAsync(title, body string) {
	if n == nil || !n.Enabled || isNotifySuppressedByEnv() {
		return
	}
	go func() {
		_ = n.Send(context.Background(), title, body)
	}()
}

// NotifySprintStart sends a non-blocking sprint start notification.
func (n *Notifier) NotifySprintStart(profileName string, durationMin, cycle, totalCycles int) {
	var body string
	if totalCycles > 1 {
		body = fmt.Sprintf("Cycle %d/%d (%s) started for %d min.", cycle, totalCycles, profileName, durationMin)
	} else {
		body = fmt.Sprintf("Focus sprint (%s) started for %d min.", profileName, durationMin)
	}
	n.SendAsync("FocusFlow — Sprint Started", body)
}

// NotifyBreakStart sends a non-blocking recovery break notification.
func (n *Notifier) NotifyBreakStart(breakMin, cycle, totalCycles int) {
	body := fmt.Sprintf("Cycle %d/%d complete. Enjoy a %d min recovery break.", cycle, totalCycles, breakMin)
	n.SendAsync("FocusFlow — Recovery Break", body)
}

// NotifySessionComplete sends a non-blocking session completion notification.
func (n *Notifier) NotifySessionComplete(profileName string, totalCycles, totalMinutes int) {
	var body string
	if totalCycles > 1 {
		body = fmt.Sprintf("Completed %d cycles (%d min total) on profile '%s'.", totalCycles, totalMinutes, profileName)
	} else {
		body = fmt.Sprintf("Completed %d min sprint on profile '%s'. Shield rolled back.", totalMinutes, profileName)
	}
	n.SendAsync("FocusFlow — Session Complete", body)
}
