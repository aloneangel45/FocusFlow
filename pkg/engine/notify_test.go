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
	// PowerShell treats Unicode smart single quotes U+2018..U+201B as string terminators!
	psSmart := EscapePowerShellSingleQuoted("a\u2018b\u2019c\u201ad\u201be")
	if strings.ContainsAny(psSmart, "\u2018\u2019\u201a\u201b") || strings.Count(psSmart, "''") != 4 {
		t.Errorf("expected Unicode smart single quotes escaped to '': %q", psSmart)
	}

	// C1 controls, Unicode line/paragraph separators, and XML-illegal U+FFFE/U+FFFF
	ctrl := SanitizeNotificationText("A\u2028B\u2029C\u0085\u009b\ufffe\uffffD`$(id)`", 200)
	if strings.ContainsAny(ctrl, "\u2028\u2029\u0085\u009b\ufffe\uffff") {
		t.Errorf("unsafe Unicode control/separator leaked: %q", ctrl)
	}
	if !strings.Contains(ctrl, "A B C") {
		t.Errorf("expected Unicode line/paragraph separators collapsed to space: %q", ctrl)
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

func TestNotifier_BoundedConcurrencyDropsExcessBurst(t *testing.T) {
	t.Setenv("FOCUSFLOW_NO_NOTIFY", "")

	release := make(chan struct{})
	var mu sync.Mutex
	cur := 0
	maxSeen := 0
	totalStarted := 0
	startedCh := make(chan struct{}, 10)

	n := &Notifier{
		Enabled:       true,
		Timeout:       2 * time.Second,
		MaxConcurrent: 2,
		sendFn: func(ctx context.Context, title, body string) error {
			mu.Lock()
			cur++
			totalStarted++
			if cur > maxSeen {
				maxSeen = cur
			}
			mu.Unlock()
			startedCh <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
			mu.Lock()
			cur--
			mu.Unlock()
			return nil
		},
	}

	for i := 0; i < 15; i++ {
		n.SendAsync("Burst", "Payload")
	}

	// Wait for the 2 allowed slots to enter sendFn
	for i := 0; i < 2; i++ {
		select {
		case <-startedCh:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for slot %d", i+1)
		}
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		inFlight := cur
		mu.Unlock()
		if inFlight == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if maxSeen > 2 {
		t.Errorf("max concurrent notifications exceeded limit 2: got %d", maxSeen)
	}
	if totalStarted > 2 {
		t.Errorf("expected burst beyond MaxConcurrent=2 to be dropped while slots busy, got %d started", totalStarted)
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
