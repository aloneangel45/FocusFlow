# ⚡ FocusFlow V2

<p align="center">
  <img src="https://raw.githubusercontent.com/aloneangel45/FocusFlow/main/assets/banner.png" alt="FocusFlow V2 Banner" width="700" onerror="this.style.display='none'"/>
</p>

<p align="center">
  <strong>Kernel-level deep work discipline engine, atomic hosts blocker, and low-overhead Win32 process supervisor in Rust.</strong><br>
  Built for developers, students, and professionals who require uncompromising focus without digital sabotage.
</p>

<p align="center">
  <a href="https://www.rust-lang.org/"><img src="https://img.shields.io/badge/Rust-1.75+-orange.svg?style=flat-square&logo=rust" alt="Rust Version"/></a>
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
4. **Resource Hogging:** Python/Electron blockers consume 150MB+ RAM and 5-10% CPU constantly iterating process trees.

**FocusFlow V2 solves all of this at the native operating system layer:**
* 🛡️ **SHA-256 Atomic Rollback:** Backs up hosts file before every sprint. Atomic file swapping guarantees your original network configuration is restored 100% cleanly, even during unexpected shutdowns.
* ⚡ **Zero-Overhead Win32 Scanning:** Replaces high-overhead pollers with low-level Win32 `CreateToolhelp32Snapshot` calls, consuming **<4 MB RAM** and **0.0% idle CPU**.
* 🌐 **Automatic Anti-DoH & DNS Flushing:** Automatically flushes the local resolver cache (`ipconfig /flushdns`) and restricts known DoH bypass endpoints during focus sessions.
* 📊 **Embedded SQLite Analytics:** Tracks completed sprints, total deep work hours, and distraction blocks locally.

---

## 🏗️ Architecture

```text
┌──────────────────────────────────────────────────────────────┐
│                       FocusFlow V2 CLI                       │
└──────────────────────────────┬───────────────────────────────┘
                               │
       ┌───────────────────────┼───────────────────────┐
       ▼                       ▼                       ▼
┌──────────────┐       ┌──────────────┐       ┌─────────────────┐
│ Hosts Engine │       │ Win32 Killer │       │ SQLite Storage  │
│ (Atomic/SHA) │       │ (<4MB RAM)   │       │ (Local Metrics) │
└──────┬───────┘       └──────┬───────┘       └─────────────────┘
       │                      │
       ▼                      ▼
[System Hosts File]     [Target Procs]
(Auto-Rollback)         (Terminated)
```

---

## 🚀 Quick Start

### Installation

Compile directly from source with Rust:

```bash
git clone https://github.com/aloneangel45/FocusFlow.git
cd FocusFlow
cargo build --release
```

The self-contained binary will be available at `./target/release/focusflow.exe`.

---

## 💻 Usage

> **Note:** Modifying system hosts files requires administrative privileges. Run your terminal as **Administrator**.

### 1. Start a Deep Work Sprint
```bash
# Start a 50-minute Developer sprint (keeps GitHub & Docs open)
focusflow start --profile dev --minutes 50

# Start a 25-minute Academic Study sprint (YKS/KPSS/Exam mode)
focusflow start --profile study --minutes 25

# Hardcore Aegis mode (enforces Anti-Cheat task suppression)
focusflow start --profile hardcore --strictness hardcore
```

### 2. Check Guard Status
```bash
focusflow status
```

### 3. Emergency / Immediate Rollback
```bash
focusflow stop
```

### 4. View Lifetime Productivity Metrics
```bash
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
