# Changelog

All notable changes to FocusFlow are documented in this file.

## [2.2.1] - 2026-11-14

### Fixed
- **`focusflow stop` Stale `"running"` Sessions & Concurrent Re-Apply Race (`cmd/focusflow`, `pkg/storage`, `focusflow_core/storage.py`, `FocusFlowGUI.py`):** Fixed `focusflow stop` so it always finalizes any active `"running"` records in `sessions.json` as `"aborted"` with computed elapsed minutes—even when running in unprivileged `PROCESS-ONLY` mode or during a break with no active `hosts` journal. Active `focusflow start` CLI loops and the desktop HUD now detect external session termination via `IsSessionRunning` / `is_session_running` and exit cleanly without mistaking the rollback for `hosts` tampering or overwriting aborted sessions as `completed: true`.
- **UTF-8 BOM, Empty File & Corrupted JSON Resilience (`pkg/storage`, `pkg/profile`, `pkg/engine`, `focusflow_core/config.py`, `focusflow_core/storage.py`, `focusflow_core/hosts.py`):** Stripped UTF-8 BOM (`\xef\xbb\xbf`, emitted by Windows PowerShell `Set-Content -Encoding UTF8`) across `config.json`, `sessions.json`, `hosts.focusflow.journal.json`, and `gui_settings.json`, preventing `json.Unmarshal`/`json.load` failures and silent history wipes. Added 0-byte tolerance, `.corrupt.bak` preservation prior to self-healing malformed `config.json`/`sessions.json` files, and null-safe field coercion in Python `SessionRecord.from_dict` and `FocusProfile.from_dict`.
- **Domain Sanitization Bypasses & Unicode Port Validation (`pkg/engine/sanitize.go`, `focusflow_core/sanitize.py`):** Blocked `www.<reserved>` (`www.ip6-localhost`, `www.ip6-loopback`, `www.broadcasthost`, `www.127.0.0.1`), `home.arpa` / `*.home.arpa` / `*.invalid`, scheme-relative `//` URLs, consecutive trailing dots (`..`), and non-standard hex/octal IPv4 literals (`0x7f.0.0.0x1`) identically in Go and Python. Fixed Python `sanitize_domain` raising uncaught `ValueError` instead of `DomainValidationError` on non-ASCII Unicode superscript port digits (e.g. `reddit.com:\u00b2`).
- **Read-Only `hosts` Attribute Handling (`pkg/engine/fileio_windows.go`, `pkg/engine/fileio_posix.go`, `focusflow_core/hosts.py`):** Temporarily clear and restore `FILE_ATTRIBUTE_READONLY` around `ReplaceFileW` and fallback writes on Windows, and apply `0444` POSIX modes only after writing/fsyncing temporary `hosts` staging files so read-only `hosts` files can be engaged and rolled back cleanly.
- **CLI Subcommand Flag Ordering & Positional Argument Validation (`cmd/focusflow/main.go`):** Implemented FlagSet-aware interspersed flag parsing (`parseInterspersedFlags`) across all subcommands so flags placed before or after positional arguments (e.g. `focusflow start dev --minutes 30`, `focusflow profile create myprof --minutes 45`, `focusflow profile show dev --json`, `focusflow add site reddit.com --json`, `focusflow stop --json`) work consistently, unknown flags never swallow positional `<name>` tokens, and unexpected positional arguments (such as `focusflow goal 90`) fail fast instead of being silently ignored.
- **Desktop HUD Early Break Leak, Double-Counted Block Telemetry, Hardcore Break Lockdown & Friction Regeneration (`FocusFlowGUI.py`):**
  - Finalized active work sprints as `"aborted"` when taking an early manual break (`[TAKE BREAK]`), eliminating orphaned `"running"` records in `sessions.json`.
  - Fixed `DISTRACTIONS KILLED` KPI double-counting `session_blocked_count` after session completion or abort.
  - Kept `hosts` sinkhole, `ProcessSupervisor`, and `AntiCheatGuard` continuously active during Pomodoro breaks in `hardcore` mode (matching Go CLI behavior).
  - Regenerated fresh 16-character tokens and redrew the `FrictionChallengeDialog` canvas on mismatch, synchronized live custom blocklist edits during active sessions, and added automatic fallback to `study` when a selected custom profile is deleted externally.
- **Python JSON Export Local Timezone Alignment (`focusflow_core/storage.py`):** Fixed `SessionStore.export_json_string()` defaulting `now` to UTC instead of the local system timezone, aligning exported `today_focus_minutes`, `active_streak_days`, and `daily` buckets with `compute_analytics()` and Go `ExportJSON`.
- **Installer Checksum BOM & Uppercase Version Tag Handling (`install.ps1`, `install.sh`):** Stripped UTF-8 BOM and CRLF line endings when parsing `checksums.txt` in `install.ps1` and `install.sh`, and normalized uppercase `V` prefixes (`FOCUSFLOW_VERSION=V2.2.1`) in `install.sh`.

## [2.2.0] - 2026-11-14

### Added
- Custom Named Profiles (`focusflow profile list|show|create|delete [--json]`) with strict slug validation (`^[a-z0-9][a-z0-9_-]{0,31}$`), `--from` profile cloning, RFC-1123 domain sanitization, protected process denylist enforcement, and built-in profile deletion/overwrite guards across both Go (`pkg/profile`) and Python (`focusflow_core/config.py`).
- Dynamic custom profile loading and selection in `FocusFlowGUI.py` (Catppuccin Mocha zero-emoji HUD) synchronized seamlessly with `ConfigStore.profiles`.
- Daily Focus Goals (`daily_goal_minutes`, defaulting to `120` and clamped to `5..1440`) across Go and Python `config.json` stores, plus `focusflow goal [--minutes <N>] [--json]` CLI command and daily progress telemetry (`Today: X / Y min [Z%]`, `today_focus_minutes`, `daily_goal_minutes`, `daily_goal_progress_pct`) in `focusflow stats`, `focusflow status`, Python `SessionStore`, and the desktop HUD.
- Multi-Cycle Pomodoro Sprint Engine (`focusflow start --cycles <N> --break <min>`) supporting 1..24 sequential work/break cycles, `[CYCLE k/N]` TUI progress indicators, immediate per-sprint `sessions.json` persistence, automatic `hosts` shield pause during recovery breaks (kept continuous under `--strict hardcore`), and clean `SIGINT`/`SIGTERM` rollback at any work or break phase.
- Universal one-line installers `install.sh` (Linux and macOS `amd64`/`arm64`) and `install.ps1` (Windows `amd64`/`arm64`) with mandatory SHA-256 verification against `checksums.txt` before extraction.
- Expanded Go (`pkg/profile`, `pkg/storage`, `pkg/tui`, `cmd/focusflow`) and Python (`pytest`) test suites covering Go <-> Python custom profile & daily goal interoperability, multi-cycle Pomodoro execution, signal rollback, and SHA-256 installer verification.

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
