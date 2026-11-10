# Changelog

All notable changes to FocusFlow are documented in this file.

## [2.1.0] - 2026-11-10

### Added
- Unified standard-library Python core package (`focusflow_core/`) shared with `FocusFlowGUI.py` and 100% interoperable with the Go CLI (`config.json`, `sessions.json`, `hosts.focusflow.journal.json`).
- Crash-safe write-ahead rollback journal (`hosts.focusflow.journal.json`) and automatic startup orphan recovery (`RecoverIfOrphaned`) across both Go CLI and Python HUD.
- Dual-stack IPv4 (`0.0.0.0`) + IPv6 (`::`) `hosts` sinkholing with expanded Anti-DoH endpoints (`dns.adguard-dns.com`, `dns.nextdns.io`, `doh.cleanbrowsing.org`, `doh.dns.sb`, `freedns.controld.com`) and mid-session tamper verification.
- Zero-dependency Python Win32 process supervisor using `ctypes` `CreateToolhelp32Snapshot` (`Process32FirstW`/`Process32NextW`) and `TerminateProcess`.
- Critical protected process and PID denylist across Go and Python (`csrss.exe`, `lsass.exe`, `svchost.exe`, `explorer.exe`, `python.exe`, `focusflow.exe`, `wt.exe`, etc.), plus `SystemInformer.exe` in hardcore `AntiCheatGuard`.
- New CLI commands and flags: `focusflow doctor`, `focusflow recover`, `focusflow export [--format json|csv] [--out <file>]`, `--elevate`, `stats --days`, and `--json` output on `status`, `list`, `stats`, `doctor`, and `recover`.
- Non-pasteable 16-character alphanumeric Friction Challenge in strict mode, `PER_MONITOR_AWARE_V2` High-DPI scaling, `[ELEVATE ADMIN]` UAC action, and JSON/CSV/PNG telemetry exports in the desktop HUD.
- Hermetic Go and Python (`pytest`) test suites and multi-OS CI matrix (`Go 1.23.x`/`1.24.x` and `Python 3.11`–`3.14`), plus `windows/arm64` release artifacts.

### Changed
- Replaced drifted sleep countdowns with `time.monotonic()` phase deadlines and paused modal transitions so break timers never burn in the background while dialogs are open.
- Preserved Windows `hosts` DACLs via `ReplaceFileW` and POSIX permissions with `fsync` and 5x/40ms sharing-violation retries.
- Fixed Linux `/proc` process scanning so `ps` is never spawned on zero-match ticks and 15-byte `TASK_COMM_LEN` truncated names resolve via `/proc/<pid>/exe` and `/proc/<pid>/cmdline`.
- Purged seeded placeholder stats in favor of live telemetry computed from `sessions.json`.
- Standardized CLI and HUD on a strict zero-emoji Catppuccin Mocha systems aesthetic.

### Removed
- Retired duplicate unmaintained Rust prototype (`Cargo.toml`, `src/`) and legacy V1 script (`legacy/FocusFlowV1.py`).

## [2.0.0] - 2026-11-08

### Added
- Native Go CLI (`cmd/focusflow`, `pkg/engine`, `pkg/profile`, `pkg/storage`, `pkg/tui`) and Catppuccin Mocha Tkinter desktop GUI (`FocusFlowGUI.py`).
- Built-in `study`, `dev`, and `hardcore` focus profiles and cross-platform release workflow.

## [1.0.0] - 2026-11-05

### Added
- Initial Pomodoro focus timer, process blocker, DNS/hosts blocking prototype, and weekly statistics view.
