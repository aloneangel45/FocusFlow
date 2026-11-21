package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultHookTimeout = 10 * time.Second
	maxHookOutputBytes = 4096

	HookEventStart = "start"
	HookEventBreak = "break"
	HookEventEnd   = "end"
)

var (
	ErrRootHookRefused    = errors.New("refusing to execute lifecycle hook as root without SUDO_UID; pass --allow-root-hooks to override")
	ErrHookTimeout        = errors.New("lifecycle hook timed out")
	ErrInvalidHookCommand = errors.New("lifecycle hook command contains invalid NUL byte")
)

// HooksConfig persists user-configured shell hooks in config.json.
type HooksConfig struct {
	OnStart string `json:"on_start"`
	OnEnd   string `json:"on_end"`
	OnBreak string `json:"on_break,omitempty"`
}

// HookContext carries structured FocusFlow lifecycle context exported as environment variables.
type HookContext struct {
	Event     string
	Profile   string
	SessionID string
	Minutes   int
}

// HookOptions controls execution timeout and root-hook security policy.
type HookOptions struct {
	Timeout        time.Duration
	AllowRootHooks bool
}

// ResolveEffectiveHooks merges persistent config.json hooks with CLI overrides.
// Non-empty CLI overrides take precedence over config.json values.
func ResolveEffectiveHooks(cfgHooks HooksConfig, cliOnStart, cliOnBreak, cliOnEnd string) HooksConfig {
	out := HooksConfig{
		OnStart: strings.TrimSpace(cfgHooks.OnStart),
		OnBreak: strings.TrimSpace(cfgHooks.OnBreak),
		OnEnd:   strings.TrimSpace(cfgHooks.OnEnd),
	}
	if strings.TrimSpace(cliOnStart) != "" {
		out.OnStart = strings.TrimSpace(cliOnStart)
	}
	if strings.TrimSpace(cliOnBreak) != "" {
		out.OnBreak = strings.TrimSpace(cliOnBreak)
	}
	if strings.TrimSpace(cliOnEnd) != "" {
		out.OnEnd = strings.TrimSpace(cliOnEnd)
	}
	return out
}

// ResolvePosixHookPrivilege enforces the POSIX privilege-drop security policy for hooks:
//   - Non-root (euid != 0): execute normally as the current user.
//   - Root under sudo (euid == 0 with valid non-zero SUDO_UID & SUDO_GID): drop privileges to SUDO_UID/SUDO_GID.
//   - Pure root (euid == 0 without valid non-zero SUDO_UID/SUDO_GID): refuse unless allowRootHooks is true.
func ResolvePosixHookPrivilege(euid int, sudoUID, sudoGID string, allowRootHooks bool) (drop bool, uid uint32, gid uint32, err error) {
	if euid != 0 {
		return false, 0, 0, nil
	}
	su := strings.TrimSpace(sudoUID)
	sg := strings.TrimSpace(sudoGID)
	if su != "" && sg != "" {
		u64, uErr := strconv.ParseUint(su, 10, 32)
		g64, gErr := strconv.ParseUint(sg, 10, 32)
		if uErr == nil && gErr == nil && u64 > 0 && g64 > 0 && u64 < math.MaxUint32 && g64 < math.MaxUint32 {
			return true, uint32(u64), uint32(g64), nil
		}
	}
	if !allowRootHooks {
		return false, 0, 0, ErrRootHookRefused
	}
	return false, 0, 0, nil
}

func buildHookEnv(baseEnv []string, hctx HookContext) []string {
	overrides := map[string]string{
		"FOCUSFLOW_EVENT":      hctx.Event,
		"FOCUSFLOW_PROFILE":    hctx.Profile,
		"FOCUSFLOW_SESSION_ID": hctx.SessionID,
		"FOCUSFLOW_MINUTES":    strconv.Itoa(hctx.Minutes),
	}
	out := make([]string, 0, len(baseEnv)+len(overrides))
	for _, entry := range baseEnv {
		eqIdx := strings.IndexByte(entry, '=')
		if eqIdx > 0 {
			if _, replaced := overrides[entry[:eqIdx]]; replaced {
				continue
			}
		}
		out = append(out, entry)
	}
	out = append(out,
		"FOCUSFLOW_EVENT="+hctx.Event,
		"FOCUSFLOW_PROFILE="+hctx.Profile,
		"FOCUSFLOW_SESSION_ID="+hctx.SessionID,
		"FOCUSFLOW_MINUTES="+strconv.Itoa(hctx.Minutes),
	)
	return out
}

type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	origLen := len(p)
	if l.buf.Len() < l.max {
		rem := l.max - l.buf.Len()
		if len(p) > rem {
			p = p[:rem]
		}
		_, _ = l.buf.Write(p)
	}
	return origLen, nil
}

func (l *limitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// RunLifecycleHook executes a single lifecycle hook command with strict timeout enforcement,
// structured FOCUSFLOW_* environment variables, and POSIX root privilege dropping.
func RunLifecycleHook(ctx context.Context, command string, hctx HookContext, opts HookOptions) (string, error) {
	if strings.ContainsRune(command, '\x00') {
		return "", ErrInvalidHookCommand
	}
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultHookTimeout
	}

	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := buildPlatformHookCommand(tctx, trimmed, opts)
	if err != nil {
		return "", err
	}

	baseEnv := cmd.Env
	if len(baseEnv) == 0 {
		baseEnv = os.Environ()
	}
	cmd.Env = buildHookEnv(baseEnv, hctx)

	outBuf := &limitedBuffer{max: maxHookOutputBytes}
	cmd.Stdout = outBuf
	cmd.Stderr = outBuf
	cmd.WaitDelay = 250 * time.Millisecond

	runErr := cmd.Run()
	output := strings.TrimSpace(outBuf.String())

	if errors.Is(tctx.Err(), context.DeadlineExceeded) {
		return output, fmt.Errorf("%w (%s)", ErrHookTimeout, timeout)
	}
	if runErr != nil {
		if output != "" {
			return output, fmt.Errorf("%w: %s", runErr, output)
		}
		return output, runErr
	}
	return output, nil
}

// RunHookSafe invokes RunLifecycleHook and reports any failure as a non-fatal warning,
// guaranteeing that a failing or timed-out hook can never abort session flow or hosts rollback.
func RunHookSafe(command string, hctx HookContext, opts HookOptions, warnWriter io.Writer) {
	if strings.TrimSpace(command) == "" {
		return
	}
	if _, err := RunLifecycleHook(context.Background(), command, hctx, opts); err != nil && warnWriter != nil {
		_, _ = fmt.Fprintf(warnWriter, "  [WARN] Lifecycle hook (%s) failed: %v\n", hctx.Event, err)
	}
}
