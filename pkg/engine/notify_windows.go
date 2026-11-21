//go:build windows

package engine

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// windowsNotifyScript reads title and body strictly from process environment variables
// (never interpolated into PowerShell source code) and XML-escapes them before rendering
// a WinRT toast, falling back to a WinForms BalloonTip if WinRT Toast fails.
const windowsNotifyScript = `
$ErrorActionPreference = 'Stop'
$title = $env:FOCUSFLOW_NOTIFY_TITLE
$body = $env:FOCUSFLOW_NOTIFY_BODY
if ([string]::IsNullOrWhiteSpace($title)) { $title = 'FocusFlow' }
if ([string]::IsNullOrWhiteSpace($body)) { $body = $title }
try {
    [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
    [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
    $escTitle = [System.Security.SecurityElement]::Escape($title)
    $escBody = [System.Security.SecurityElement]::Escape($body)
    $xmlStr = "<toast><visual><binding template='ToastText02'><text id='1'>$escTitle</text><text id='2'>$escBody</text></binding></visual></toast>"
    $doc = New-Object Windows.Data.Xml.Dom.XmlDocument
    $doc.LoadXml($xmlStr)
    $toast = New-Object Windows.UI.Notifications.ToastNotification $doc
    $notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('FocusFlow')
    $notifier.Show($toast)
    exit 0
} catch {
    try {
        Add-Type -AssemblyName System.Windows.Forms
        Add-Type -AssemblyName System.Drawing
        $ni = New-Object System.Windows.Forms.NotifyIcon
        $ni.Icon = [System.Drawing.SystemIcons]::Information
        $ni.BalloonTipTitle = $title
        $ni.BalloonTipText = $body
        $ni.Visible = $true
        $ni.ShowBalloonTip(3000)
        Start-Sleep -Milliseconds 600
        $ni.Dispose()
    } catch {
        # Best-effort only
    }
}
`

func replaceEnvCaseInsensitive(baseEnv []string, overrides map[string]string) []string {
	out := make([]string, 0, len(baseEnv)+len(overrides))
	for _, entry := range baseEnv {
		eqIdx := strings.IndexByte(entry, '=')
		if eqIdx > 0 {
			if _, ok := overrides[strings.ToUpper(entry[:eqIdx])]; ok {
				continue
			}
		}
		out = append(out, entry)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

func buildWindowsNotifyCommand(ctx context.Context, title, body string) *exec.Cmd {
	cmd := exec.CommandContext(
		ctx,
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass",
		"-Command", windowsNotifyScript,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.WaitDelay = 250 * time.Millisecond
	cmd.Env = replaceEnvCaseInsensitive(os.Environ(), map[string]string{
		"FOCUSFLOW_NOTIFY_TITLE": SanitizeNotificationText(title, 120),
		"FOCUSFLOW_NOTIFY_BODY":  SanitizeNotificationText(body, 256),
	})
	return cmd
}

func sendNativeNotification(ctx context.Context, title, body string) error {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		return nil
	}
	cmd := buildWindowsNotifyCommand(ctx, title, body)
	_ = cmd.Run()
	return nil
}
