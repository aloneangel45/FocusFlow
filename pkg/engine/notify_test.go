package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNotificationEscapingAndSanitization(t *testing.T) {
	raw := "Title\r\n$(calc.exe)\x00'\"<&>"
	clean := SanitizeNotificationText(raw, 200)
	if strings.ContainsAny(clean, "\r\n\x00") {
		t.Errorf("control characters leaked into sanitized text: %q", clean)
	}

	ps := EscapePowerShellSingleQuoted("hello 'world'; $(whoami)")
	if !strings.Contains(ps, "''world''") {
		t.Errorf("expected single quotes doubled in PowerShell escape: %q", ps)
	}

	as := EscapeAppleScriptString(`say "hi" \ test`)
	if as != `say \"hi\" \\ test` {
		t.Errorf("unexpected AppleScript escape: %q", as)
	}

	xmlEsc := EscapeXMLText(`<script>&"'</script>`)
	if strings.ContainsAny(xmlEsc, "<>'\"") || !strings.Contains(xmlEsc, "&lt;script&gt;") {
		t.Errorf("unexpected XML escape: %q", xmlEsc)
	}
}

func TestNotifier_DisabledAndEnvSuppression(t *testing.T) {
	called := false
	n := &Notifier{
		Enabled: false,
		Timeout: time.Second,
		sendFn: func(ctx context.Context, title, body string) error {
			called = true
			return nil
		},
	}
	if err := n.Send(context.Background(), "T", "B"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if called {
		t.Errorf("disabled notifier must not invoke sendFn")
	}

	t.Setenv("FOCUSFLOW_NO_NOTIFY", "1")
	n.Enabled = true
	if err := n.Send(context.Background(), "T", "B"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if called {
		t.Errorf("FOCUSFLOW_NO_NOTIFY=1 must suppress sendFn")
	}
}

func TestNotifier_NonBlockingAsyncAndTimeout(t *testing.T) {
	t.Setenv("FOCUSFLOW_NO_NOTIFY", "")

	var mu sync.Mutex
	var events []string
	done := make(chan struct{}, 3)

	n := &Notifier{
		Enabled: true,
		Timeout: 2 * time.Second,
		sendFn: func(ctx context.Context, title, body string) error {
			mu.Lock()
			events = append(events, title+"|"+body)
			mu.Unlock()
			done <- struct{}{}
			return errors.New("simulated notification failure")
		},
	}

	n.NotifySprintStart("dev", 50, 1, 2)
	n.NotifyBreakStart(10, 1, 2)
	n.NotifySessionComplete("dev", 2, 100)

	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for async notification %d", i+1)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 3 {
		t.Fatalf("expected 3 notifications dispatched, got %d", len(events))
	}

	// Verify context timeout is strictly enforced
	slow := &Notifier{
		Enabled: true,
		Timeout: 80 * time.Millisecond,
		sendFn: func(ctx context.Context, title, body string) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	start := time.Now()
	err := slow.Send(context.Background(), "TimeoutTest", "Body")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("timeout took too long: %v", time.Since(start))
	}
}
