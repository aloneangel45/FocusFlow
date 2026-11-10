# Security Policy

## Supported Versions

| Version | Status               |
| :------ | :------------------- |
| 2.1.x   | Supported            |
| 2.0.x   | Critical fixes only  |
| < 2.0   | Unsupported          |

## Security Architecture Notes

FocusFlow operates at the operating-system boundary by managing DNS sinkhole blocks in the system `hosts` file and terminating user-configured distraction executables:

- **Strict RFC-1123 Domain Validation:** Rejects control characters, embedded newlines/whitespace, IPv4/IPv6 literals, `localhost`, `localdomain`, and `*.local` domains to prevent `hosts` injection.
- **Protected System Binary Denylist:** Prevents adding or terminating PIDs `0`, `4`, current/parent PIDs, and critical OS/host binaries (`csrss.exe`, `smss.exe`, `lsass.exe`, `services.exe`, `svchost.exe`, `wininit.exe`, `winlogon.exe`, `dwm.exe`, `explorer.exe`, `sihost.exe`, `fontdrvhost.exe`, `focusflow.exe`, `python.exe`, `pythonw.exe`, `conhost.exe`, `wt.exe`).
- **ACL-Preserving Crash Recovery:** Uses Win32 `ReplaceFileW` and POSIX mode preservation alongside a SHA-256 write-ahead rollback journal (`hosts.focusflow.journal.json`) so network resolution permissions are never degraded.

## Reporting a Vulnerability

Please do **not** open a public GitHub issue for security vulnerabilities. Report suspected vulnerabilities privately via `security@aloneangel.dev` or through a GitHub Security Advisory on this repository. Reports are triaged within 48 hours.
