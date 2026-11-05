# ⚡ FocusFlow V2

<p align="center">
  <img src="https://raw.githubusercontent.com/aloneangel45/FocusFlow/main/assets/banner.png" alt="FocusFlow V2 Banner" width="700" onerror="this.style.display='none'"/>
</p>

<p align="center">
  <strong>High-Performance Deep Work Discipline Engine, Atomic Hosts Blocker & Catppuccin Mocha Desktop HUD.</strong><br>
  Engineered for developers, students, and professionals who demand uncompromising focus without digital sabotage.
</p>

<p align="center">
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Engine-Go%20Native-00ADD8.svg?style=flat-square&logo=go" alt="Go Native"/></a>
  <a href="https://python.org/"><img src="https://img.shields.io/badge/GUI-Catppuccin%20Mocha%20High--DPI-cba6f7.svg?style=flat-square" alt="Modern GUI"/></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square" alt="License: MIT"/></a>
  <a href="#"><img src="https://img.shields.io/badge/Platform-Windows%20Win32-0078D6?style=flat-square&logo=windows" alt="Platform"/></a>
  <a href="#"><img src="https://img.shields.io/badge/Memory%20Footprint-%3C4MB-brightgreen?style=flat-square" alt="Memory Footprint"/></a>
  <a href="#"><img src="https://img.shields.io/badge/Rollback-SHA--256%20Atomic-purple?style=flat-square" alt="Atomic Rollback"/></a>
  <a href="#"><img src="https://img.shields.io/badge/Anti--DoH-Auto--Flushed-success?style=flat-square" alt="Anti-DoH"/></a>
</p>

---

## ⚡ Why FocusFlow V2? (The Paradigm Shift)

Traditional site blockers and browser extensions are easily bypassed:
1. **Easy Browser Disabling:** Extensions can be disabled in two clicks via incognito mode or developer settings.
2. **DNS-over-HTTPS (DoH) Leaks:** Modern browsers (Chrome, Brave, Edge) bypass local `hosts` files by resolving DNS through encrypted cloud resolvers (`cloudflare-dns.com`, `dns.google`).
3. **Hosts File Corruption:** Primitive scripts write directly to `C:\Windows\System32\drivers\etc\hosts`. If the system crashes, reboots, or loses power, internet connectivity is permanently broken.
4. **Resource Hogging:** Electron/Python blockers often consume 200MB+ RAM and 5-10% CPU constantly iterating process trees.

**FocusFlow V2 solves all of this at the native operating system layer:**
* 🛡️ **SHA-256 Atomic Rollback:** Backs up the hosts file before every sprint. Atomic file swapping guarantees your original network configuration is restored 100% cleanly, even during unexpected shutdowns.
* ⚡ **Zero-Overhead Win32 Scanning:** Replaces high-overhead pollers with low-level Win32 `CreateToolhelp32Snapshot` calls, consuming **<4 MB RAM** and **0.0% idle CPU**.
* 🎨 **Catppuccin Mocha High-DPI Desktop GUI:** A sleek, modern dark HUD with real-time Pomodoro timers, weekly bar charts, custom blocklists, and motivation cards.
* 🌐 **Automatic Anti-DoH & DNS Flushing:** Automatically flushes the local resolver cache (`ipconfig /flushdns`) and restricts known DoH bypass endpoints during focus sessions.
* 📊 **Persistent Analytics:** Tracks completed sprints, streak days, total deep work hours, and distraction blocks locally.

---

## 🏗️ Architecture

```text
┌──────────────────────────────────────────────────────────────┐
│           FocusFlow V2 Desktop GUI / CLI Terminal            │
└──────────────────────────────┬───────────────────────────────┘
                               │
        ┌──────────────────────┼───────────────────────┐
        ▼                      ▼                       ▼
 ┌──────────────┐       ┌──────────────┐       ┌─────────────────┐
 │ Hosts Engine │       │ Win32 Killer │       │ Session Metrics │
 │ (Atomic/SHA) │       │ (<4MB RAM)   │       │ (%APPDATA% JSON)│
 └──────┬───────┘       └──────┬───────┘       └─────────────────┘
        │                      │
        ▼                      ▼
 [System Hosts File]     [Target Procs]
 (Auto-Rollback)         (Terminated)
```

---

## 🚀 Quick Start

### 1. Graphical Interface (GUI)
Simply launch `Launch_FocusFlow_GUI.bat` or run:
```bash
pythonw FocusFlowGUI.py
```

### 2. Standalone Native Binary (CLI)
Compile directly from source with Go:
```bash
go build -ldflags="-s -w" -o focusflow.exe ./cmd/focusflow
```

---

## 💻 CLI Usage

> **Note:** Modifying system hosts files requires administrative privileges. Run terminal as **Administrator**.

```bash
# Launch interactive terminal TUI menu
focusflow menu

# Start a 50-minute Developer sprint (keeps GitHub & Docs open)
focusflow start --profile dev --minutes 50

# Start a 25-minute Academic Study sprint (YKS/KPSS/Exam mode)
focusflow start --profile study --minutes 25

# Check current blocker status
focusflow status

# Immediate rollback & restore internet
focusflow stop

# Purge local DNS cache immediately
focusflow flush

# View lifetime productivity metrics
focusflow stats
```

---

## 🎯 Built-in Profiles

| Profile | Target Audience | Allowed Resources | Blocked Distractions |
| :--- | :--- | :--- | :--- |
| **`dev`** | Software Engineers | GitHub, GitLab, StackOverflow, Docs.rs, MDN | Twitter/X, Instagram, TikTok, Reddit, Netflix, Twitch, Discord, Steam |
| **`study`** | Students & Researchers | Educational portals, LMS, Academic sites | YouTube, TikTok, Kick, Netflix, Gaming launchers, Chat clients |
| **`hardcore`** | Deep Work Purists | Specified whitelist only | All distractions + Task Manager / Terminal kill prevention |

---

## 🛡️ Safety & Integrity Guarantee

FocusFlow V2 is engineered with an atomic safety-first philosophy:
* Before any modification, the original `hosts` file is copied to `hosts.focusflow.bak` and verified via a SHA-256 checksum.
* On session completion, interrupt (`Ctrl+C`), or `focusflow stop`, the backup file is restored and the DNS resolver cache is cleared immediately.

---

## 📄 License

This project is open-source under the [MIT License](LICENSE).  
Created with passion by [aloneangel45](https://github.com/aloneangel45).
