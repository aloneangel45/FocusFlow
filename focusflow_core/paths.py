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


def get_state_dir() -> str:
    """Resolve FocusFlow state directory identically to Go `storage.GetStateDir()`.

    Priority:
      1. FOCUSFLOW_HOME environment override
      2. Windows: %APPDATA%\\FocusFlow (or ~/AppData/Roaming/FocusFlow)
      3. POSIX: $XDG_CONFIG_HOME/focusflow (or ~/.config/focusflow)
      4. Fallback: .focusflow
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
