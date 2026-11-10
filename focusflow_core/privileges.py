"""Privilege inspection, UAC/sudo self-elevation, and non-blocking DNS cache flushing."""

from __future__ import annotations

import ctypes
import os
import subprocess
import sys
import threading

CREATE_NO_WINDOW = 0x08000000 if sys.platform == "win32" else 0


def is_elevated() -> bool:
    """Return True if the current process holds Administrator (Windows) or root (POSIX) privileges."""
    if sys.platform == "win32":
        try:
            shell32 = ctypes.WinDLL("shell32", use_last_error=True)
            shell32.IsUserAnAdmin.argtypes = []
            shell32.IsUserAnAdmin.restype = ctypes.wintypes.BOOL if hasattr(ctypes, "wintypes") else ctypes.c_int
            return bool(shell32.IsUserAnAdmin())
        except Exception:
            return False
    try:
        return os.geteuid() == 0
    except AttributeError:
        return False


def shield_mode_label(elevated: bool | None = None) -> str:
    """Return honest shield capability badge string."""
    if elevated is None:
        elevated = is_elevated()
    return "FULL SHIELD [HOSTS + PROC]" if elevated else "PROCESS-ONLY [UNPRIVILEGED]"


def build_elevation_cmdline(script_path: str | None = None, extra_args: list[str] | None = None) -> str:
    """Build safely quoted Win32 command-line argument string preserving paths with spaces."""
    target = os.path.abspath(script_path or sys.argv[0])
    args = list(extra_args) if extra_args is not None else [a for a in sys.argv[1:] if a != "--elevate"]
    return subprocess.list2cmdline([target, *args])


def elevate_self(
    script_path: str | None = None,
    extra_args: list[str] | None = None,
    executable: str | None = None,
) -> bool:
    """Relaunch the current Python entrypoint with Administrator/root privileges.

    Uses ShellExecuteW(None, "runas", ...) with subprocess.list2cmdline on Windows
    so workspace/user paths containing spaces never break.
    """
    exe = executable or sys.executable
    params = build_elevation_cmdline(script_path=script_path, extra_args=extra_args)
    cwd = os.getcwd()

    if sys.platform == "win32":
        from ctypes import wintypes

        # Prefer pythonw.exe when launching GUI without console flash if running from FocusFlowGUI.py
        target_name = os.path.basename(script_path or sys.argv[0]).lower()
        if executable is None and target_name.endswith("gui.py") and exe.lower().endswith("python.exe"):
            candidate_w = exe[:-10] + "pythonw.exe"
            if os.path.exists(candidate_w):
                exe = candidate_w

        shell32 = ctypes.WinDLL("shell32", use_last_error=True)
        shell32.ShellExecuteW.argtypes = [
            wintypes.HWND,
            wintypes.LPCWSTR,
            wintypes.LPCWSTR,
            wintypes.LPCWSTR,
            wintypes.LPCWSTR,
            ctypes.c_int,
        ]
        shell32.ShellExecuteW.restype = ctypes.c_void_p

        res = shell32.ShellExecuteW(None, "runas", exe, params, cwd, 1)
        return int(res or 0) > 32

    target = os.path.abspath(script_path or sys.argv[0])
    args = list(extra_args) if extra_args is not None else [a for a in sys.argv[1:] if a != "--elevate"]
    try:
        proc = subprocess.Popen(["sudo", exe, target, *args], cwd=cwd)
        return proc.pid > 0
    except Exception:
        return False


def flush_dns() -> bool:
    """Synchronously flush local OS DNS resolver cache."""
    try:
        if sys.platform == "win32":
            res = subprocess.run(
                ["ipconfig", "/flushdns"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                creationflags=CREATE_NO_WINDOW,
                timeout=10,
                check=False,
            )
            return res.returncode == 0
        if sys.platform == "darwin":
            subprocess.run(
                ["dscacheutil", "-flushcache"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                timeout=10,
                check=False,
            )
            subprocess.run(
                ["killall", "-HUP", "mDNSResponder"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                timeout=10,
                check=False,
            )
            return True
        res = subprocess.run(
            ["resolvectl", "flush-caches"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=10,
            check=False,
        )
        if res.returncode != 0:
            subprocess.run(
                ["systemd-resolve", "--flush-caches"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                timeout=10,
                check=False,
            )
        return True
    except Exception:
        return False


def flush_dns_async() -> threading.Thread:
    """Offload DNS cache purge to a daemon thread so GUI event loops never stall."""
    t = threading.Thread(target=flush_dns, name="FocusFlowDNSFlush", daemon=True)
    t.start()
    return t
