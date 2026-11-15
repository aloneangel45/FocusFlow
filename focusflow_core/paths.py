"""Unified state directory and system file path resolution shared with the Go CLI."""

from __future__ import annotations

import os
import sys

DEFAULT_WINDOWS_HOSTS_PATH = r"C:\Windows\System32\drivers\etc\hosts"
DEFAULT_POSIX_HOSTS_PATH = "/etc/hosts"

BACKUP_FILENAME = "hosts.focusflow.bak"
JOURNAL_FILENAME = "hosts.focusflow.journal.json"
CONFIG_FILENAME = "config.json"
SESSIONS_FILENAME = "sessions.json"
GUI_SETTINGS_FILENAME = "gui_settings.json"


def _resolve_sudo_user_home(sudo_user: str) -> str:
    """Resolve invoking non-root user's home directory when running under POSIX sudo."""
    username = (sudo_user or "").strip()
    if not username or username == "root" or "/" in username or "\\" in username or ":" in username:
        return ""

    try:
        import pwd  # type: ignore[import-not-found]

        pw_dir = pwd.getpwnam(username).pw_dir
        if pw_dir and os.path.isabs(pw_dir):
            return os.path.normpath(pw_dir)
    except Exception:
        pass

    try:
        with open("/etc/passwd", "r", encoding="utf-8", errors="replace") as f:
            for raw_line in f:
                line = raw_line.strip()
                if not line or line.startswith("#"):
                    continue
                fields = line.split(":")
                if len(fields) >= 6 and fields[0] == username:
                    cand = fields[5].strip()
                    if cand and os.path.isabs(cand):
                        return os.path.normpath(cand)
    except OSError:
        pass

    expanded = os.path.expanduser(f"~{username}")
    if expanded and not expanded.startswith("~") and os.path.isabs(expanded):
        return os.path.normpath(expanded)

    fallback = os.path.join("/Users" if sys.platform == "darwin" else "/home", username)
    if os.path.isdir(fallback):
        return os.path.normpath(fallback)
    return ""


def get_state_dir() -> str:
    """Resolve FocusFlow state directory identically to Go `storage.GetStateDir()`.

    Priority:
      1. FOCUSFLOW_HOME environment override
      2. Windows: %APPDATA%\\FocusFlow (or ~/AppData/Roaming/FocusFlow)
      3. POSIX under sudo (euid == 0 & SUDO_USER set): ~SUDO_USER/.config/focusflow
      4. POSIX: $XDG_CONFIG_HOME/focusflow (or ~/.config/focusflow)
      5. Fallback: .focusflow
    """
    override = os.environ.get("FOCUSFLOW_HOME", "").strip()
    if override:
        return os.path.normpath(override)

    if sys.platform == "win32":
        appdata = os.environ.get("APPDATA", "").strip()
        if appdata:
            return os.path.join(appdata, "FocusFlow")
        home = os.path.expanduser("~")
        if home and home != "~":
            return os.path.join(home, "AppData", "Roaming", "FocusFlow")
        return ".focusflow"

    try:
        is_root = os.geteuid() == 0
    except AttributeError:
        is_root = False

    if is_root:
        sudo_home = _resolve_sudo_user_home(os.environ.get("SUDO_USER", ""))
        if sudo_home:
            return os.path.join(sudo_home, ".config", "focusflow")

    xdg = os.environ.get("XDG_CONFIG_HOME", "").strip()
    if xdg:
        return os.path.join(xdg, "focusflow")
    home = os.path.expanduser("~")
    if home and home != "~":
        return os.path.join(home, ".config", "focusflow")
    return ".focusflow"


def system_default_hosts_path() -> str:
    if sys.platform == "win32":
        return DEFAULT_WINDOWS_HOSTS_PATH
    return DEFAULT_POSIX_HOSTS_PATH


def get_default_hosts_path() -> str:
    override = os.environ.get("FOCUSFLOW_HOSTS_PATH", "").strip()
    if override:
        return os.path.normpath(override)
    return system_default_hosts_path()


def get_config_path(state_dir: str | None = None) -> str:
    return os.path.join(state_dir or get_state_dir(), CONFIG_FILENAME)


def get_sessions_path(state_dir: str | None = None) -> str:
    return os.path.join(state_dir or get_state_dir(), SESSIONS_FILENAME)


def get_journal_path(state_dir: str | None = None) -> str:
    return os.path.join(state_dir or get_state_dir(), JOURNAL_FILENAME)


def get_backup_path(state_dir: str | None = None) -> str:
    return os.path.join(state_dir or get_state_dir(), BACKUP_FILENAME)


def get_gui_settings_path(state_dir: str | None = None) -> str:
    return os.path.join(state_dir or get_state_dir(), GUI_SETTINGS_FILENAME)
