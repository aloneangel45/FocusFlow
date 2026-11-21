package engine

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolvePosixHookPrivilege(t *testing.T) {
	// 1. Non-root user executes normally without dropping
	drop, uid, gid, err := ResolvePosixHookPrivilege(1000, "", "", false)
	if err != nil || drop || uid != 0 || gid != 0 {
		t.Fatalf("non-root: got drop=%v uid=%d gid=%d err=%v", drop, uid, gid, err)
	}

	// 2. Root under sudo drops privileges to invoking SUDO_UID / SUDO_GID
	drop, uid, gid, err = ResolvePosixHookPrivilege(0, "1001", "1002", false)
	if err != nil || !drop || uid != 1001 || gid != 1002 {
		t.Fatalf("sudo drop: got drop=%v uid=%d gid=%d err=%v", drop, uid, gid, err)
	}

	// 3. Pure root without SUDO_UID is REFUSED by default
	_, _, _, err = ResolvePosixHookPrivilege(0, "", "", false)
	if !errors.Is(err, ErrRootHookRefused) {
		t.Fatalf("pure root without allowRootHooks: expected ErrRootHookRefused, got %v", err)
	}

	// 4. Root with SUDO_UID=0 is ALSO refused by default (cannot "drop" to root!)
	_, _, _, err = ResolvePosixHookPrivilege(0, "0", "0", false)
	if !errors.Is(err, ErrRootHookRefused) {
		t.Fatalf("SUDO_UID=0 without allowRootHooks: expected ErrRootHookRefused, got %v", err)
	}

	// 5. Pure root WITH explicit --allow-root-hooks is permitted
	drop, _, _, err = ResolvePosixHookPrivilege(0, "", "", true)
	if err != nil || drop {
		t.Fatalf("pure root with allowRootHooks=true: got drop=%v err=%v", drop, err)
	}
}

func TestRunLifecycleHook_EnvVarsTimeoutAndSafeWarning(t *testing.T) {
	var echoCmd, sleepCmd, failCmd string
	if runtime.GOOS == "windows" {
		echoCmd = `echo %FOCUSFLOW_EVENT%:%FOCUSFLOW_PROFILE%:%FOCUSFLOW_SESSION_ID%:%FOCUSFLOW_MINUTES%`
		sleepCmd = `ping -n 6 127.0.0.1 >nul`
		failCmd = `exit /b 7`
	} else {
		echoCmd = `echo "$FOCUSFLOW_EVENT:$FOCUSFLOW_PROFILE:$FOCUSFLOW_SESSION_ID:$FOCUSFLOW_MINUTES"`
		sleepCmd = `sleep 5`
		failCmd = `exit 7`
	}

	// 1. Structured FOCUSFLOW_* context env vars passed accurately
	out, err := RunLifecycleHook(context.Background(), echoCmd, HookContext{
		Event:     HookEventStart,
		Profile:   "dev",
		SessionID: "sess-abc123",
		Minutes:   50,
	}, HookOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("RunLifecycleHook echo failed: %v", err)
	}
	if !strings.Contains(out, "start:dev:sess-abc123:50") {
		t.Errorf("unexpected hook output %q, expected FOCUSFLOW env vars", out)
	}

	// 2. Timeout strictly enforced
	start := time.Now()
	_, err = RunLifecycleHook(context.Background(), sleepCmd, HookContext{
		Event: HookEventEnd,
	}, HookOptions{Timeout: 250 * time.Millisecond})
	if !errors.Is(err, ErrHookTimeout) {
		t.Errorf("expected ErrHookTimeout, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("hook timeout enforcement took too long: %v", time.Since(start))
	}

	// 3. Failing hook logs warning cleanly without panicking or blocking
	var warnBuf bytes.Buffer
	RunHookSafe(failCmd, HookContext{Event: HookEventEnd, Profile: "study"}, HookOptions{Timeout: 3 * time.Second}, &warnBuf)
	if !strings.Contains(warnBuf.String(), "[WARN] Lifecycle hook (end) failed") {
		t.Errorf("expected warning in warnBuf, got %q", warnBuf.String())
	}

	// 4. Effective hooks merge CLI overrides over config.json
	eff := ResolveEffectiveHooks(
		HooksConfig{OnStart: "cfg-start", OnEnd: "cfg-end"},
		"cli-start",
		"",
		"",
	)
	if eff.OnStart != "cli-start" || eff.OnEnd != "cfg-end" {
		t.Errorf("unexpected effective hooks: %+v", eff)
	}
}
