"""Zero-dependency Win32 (Toolhelp32 ctypes) & POSIX (/proc + ps) process supervisor."""

from __future__ import annotations

import ctypes
import errno
import os
import signal
import subprocess
import sys
from typing import Callable

from .sanitize import is_protected_process

TH32CS_SNAPPROCESS = 0x00000002
PROCESS_TERMINATE = 0x00000001
PROCESS_QUERY_LIMITED_INFORMATION = 0x00001000
STILL_ACTIVE = 259
ERROR_ACCESS_DENIED = 5

HARDCORE_ANTICHEAT_BINARIES: tuple[str, ...] = (
    "taskmgr.exe",
    "processhacker.exe",
    "SystemInformer.exe",
    "procmon.exe",
    "procmon64.exe",
    "procexp.exe",
    "procexp64.exe",
)

if sys.platform == "win32":
    from ctypes import wintypes

    INVALID_HANDLE_VALUE = ctypes.c_void_p(-1).value

    class PROCESSENTRY32W(ctypes.Structure):
        _fields_ = [
            ("dwSize", wintypes.DWORD),
            ("cntUsage", wintypes.DWORD),
            ("th32ProcessID", wintypes.DWORD),
            ("th32DefaultHeapID", ctypes.c_size_t),
            ("th32ModuleID", wintypes.DWORD),
            ("cntThreads", wintypes.DWORD),
            ("th32ParentProcessID", wintypes.DWORD),
            ("pcPriClassBase", wintypes.LONG),
            ("dwFlags", wintypes.DWORD),
            ("szExeFile", wintypes.WCHAR * 260),
        ]

    _kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)

    _kernel32.CreateToolhelp32Snapshot.argtypes = [wintypes.DWORD, wintypes.DWORD]
    _kernel32.CreateToolhelp32Snapshot.restype = wintypes.HANDLE

    _kernel32.Process32FirstW.argtypes = [wintypes.HANDLE, ctypes.POINTER(PROCESSENTRY32W)]
    _kernel32.Process32FirstW.restype = wintypes.BOOL

    _kernel32.Process32NextW.argtypes = [wintypes.HANDLE, ctypes.POINTER(PROCESSENTRY32W)]
    _kernel32.Process32NextW.restype = wintypes.BOOL

    _kernel32.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    _kernel32.OpenProcess.restype = wintypes.HANDLE

    _kernel32.TerminateProcess.argtypes = [wintypes.HANDLE, wintypes.UINT]
    _kernel32.TerminateProcess.restype = wintypes.BOOL

    _kernel32.GetExitCodeProcess.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)]
    _kernel32.GetExitCodeProcess.restype = wintypes.BOOL

    _kernel32.CloseHandle.argtypes = [wintypes.HANDLE]
    _kernel32.CloseHandle.restype = wintypes.BOOL


def is_protected_pid(pid: int) -> bool:
    """Never allow terminating kernel/init PIDs (<= 4), current PID, or parent PID."""
    if pid <= 4:
        return True
    if pid == os.getpid() or pid == os.getppid():
        return True
    return False


def is_pid_alive(pid: int) -> bool:
    """Return True if a process with `pid` is currently running."""
    if pid <= 0:
        return False
    if pid == os.getpid():
        return True

    if sys.platform == "win32":
        from ctypes import wintypes

        handle = _kernel32.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, False, pid)
        if not handle:
            return ctypes.get_last_error() == ERROR_ACCESS_DENIED
        try:
            exit_code = wintypes.DWORD(0)
            if not _kernel32.GetExitCodeProcess(handle, ctypes.byref(exit_code)):
                return False
            return exit_code.value == STILL_ACTIVE
        finally:
            _kernel32.CloseHandle(handle)

    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    except OSError as exc:
        return exc.errno == errno.EPERM


def _clean_basename(path_or_name: str) -> str:
    s = path_or_name.strip()
    if not s:
        return ""
    return s.replace("\\", "/").rsplit("/", 1)[-1]


def resolve_procfs_names(proc_root: str, pid_str: str) -> list[str]:
    """Resolve candidate executable names from /proc/<pid>, handling 15-byte comm truncation."""
    candidates: list[str] = []
    pid_dir = os.path.join(proc_root, pid_str)
    comm_name = ""

    try:
        with open(os.path.join(pid_dir, "comm"), "r", encoding="utf-8", errors="replace") as f:
            comm_name = f.read().strip()
        if comm_name:
            candidates.append(comm_name)
    except OSError:
        pass

    if not comm_name or len(comm_name) >= 15:
        try:
            exe_target = os.readlink(os.path.join(pid_dir, "exe"))
            if exe_target.endswith(" (deleted)"):
                exe_target = exe_target[: -len(" (deleted)")]
            base = _clean_basename(exe_target)
            if base:
                candidates.append(base)
        except OSError:
            pass

        try:
            with open(os.path.join(pid_dir, "cmdline"), "rb") as f:
                raw_cmd = f.read(4096)
            if raw_cmd:
                argv0 = raw_cmd.split(b"\x00", 1)[0].decode("utf-8", errors="replace").strip()
                base = _clean_basename(argv0)
                if base:
                    candidates.append(base)
        except OSError:
            pass

    return candidates


def enumerate_processes_win32() -> list[tuple[int, list[str]]]:
    """Enumerate live Windows processes natively via CreateToolhelp32Snapshot (zero psutil)."""
    results: list[tuple[int, list[str]]] = []
    snap = _kernel32.CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
    if not snap or snap == INVALID_HANDLE_VALUE:
        return results

    try:
        entry = PROCESSENTRY32W()
        entry.dwSize = ctypes.sizeof(PROCESSENTRY32W)
        if not _kernel32.Process32FirstW(snap, ctypes.byref(entry)):
            return results

        while True:
            pid = int(entry.th32ProcessID)
            exe_name = str(entry.szExeFile)
            if exe_name:
                results.append((pid, [exe_name]))
            if not _kernel32.Process32NextW(snap, ctypes.byref(entry)):
                break
    finally:
        _kernel32.CloseHandle(snap)

    return results


def terminate_pid_win32(pid: int) -> bool:
    """Terminate a Windows process by PID via OpenProcess + TerminateProcess."""
    if is_protected_pid(pid):
        return False
    handle = _kernel32.OpenProcess(PROCESS_TERMINATE, False, pid)
    if not handle:
        return False
    try:
        return bool(_kernel32.TerminateProcess(handle, 1))
    finally:
        _kernel32.CloseHandle(handle)


def enumerate_processes_posix(proc_root: str = "/proc") -> list[tuple[int, list[str]]]:
    """Enumerate POSIX processes via /proc (without spawning ps) or fall back to ps on macOS/BSD."""
    results: list[tuple[int, list[str]]] = []
    if os.path.isdir(proc_root):
        try:
            entries = os.listdir(proc_root)
        except OSError:
            entries = []

        saw_pid = False
        for name in entries:
            if not name.isdigit():
                continue
            saw_pid = True
            pid = int(name)
            cands = resolve_procfs_names(proc_root, name)
            if cands:
                results.append((pid, cands))
        if saw_pid:
            return results

    try:
        proc = subprocess.run(
            ["ps", "-A", "-o", "pid=,comm="],
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
        if proc.returncode == 0:
            for line in proc.stdout.splitlines():
                parts = line.strip().split(None, 1)
                if len(parts) < 2 or not parts[0].isdigit():
                    continue
                pid = int(parts[0])
                comm = _clean_basename(parts[1])
                if comm:
                    results.append((pid, [comm]))
    except Exception:
        pass

    return results


def terminate_pid_posix(pid: int) -> bool:
    if is_protected_pid(pid):
        return False
    try:
        os.kill(pid, signal.SIGTERM)
        return True
    except OSError:
        return False


class ProcessSupervisor:
    """Cross-platform zero-dependency process supervisor with strict system/host denylist."""

    def __init__(
        self,
        blocked: list[str] | tuple[str, ...] | None = None,
        enumerator: Callable[[], list[tuple[int, list[str]]]] | None = None,
        terminator: Callable[[int], bool] | None = None,
    ) -> None:
        self.blocked_names: set[str] = set()
        for raw in blocked or ():
            if not isinstance(raw, str):
                continue
            cleaned = _clean_basename(raw).lower()
            if not cleaned or is_protected_process(cleaned):
                continue
            self.blocked_names.add(cleaned)
            if cleaned.endswith(".exe"):
                base = cleaned[:-4]
                if base and not is_protected_process(base):
                    self.blocked_names.add(base)
            else:
                with_exe = f"{cleaned}.exe"
                if not is_protected_process(with_exe):
                    self.blocked_names.add(with_exe)

        if enumerator is not None:
            self._enumerator = enumerator
        elif sys.platform == "win32":
            self._enumerator = enumerate_processes_win32
        else:
            self._enumerator = enumerate_processes_posix

        if terminator is not None:
            self._terminator = terminator
        elif sys.platform == "win32":
            self._terminator = terminate_pid_win32
        else:
            self._terminator = terminate_pid_posix

    def should_terminate(self, pid: int, candidate_names: list[str] | tuple[str, ...]) -> str | None:
        """Return matched executable name if process should be terminated, else None."""
        if is_protected_pid(pid) or not self.blocked_names:
            return None

        for cand in candidate_names:
            base = _clean_basename(cand)
            if base and is_protected_process(base):
                return None

        for cand in candidate_names:
            base = _clean_basename(cand)
            if base and base.lower() in self.blocked_names:
                return base

        return None

    def scan_and_terminate(self) -> list[str]:
        """Scan active OS processes and terminate matching non-protected targets."""
        if not self.blocked_names:
            return []

        terminated: list[str] = []
        for pid, candidates in self._enumerator():
            matched = self.should_terminate(pid, candidates)
            if matched is not None and self._terminator(pid):
                terminated.append(f"{matched} (PID: {pid})")
        return terminated


class AntiCheatGuard:
    """Hardcore task-killer / process-inspector suppression guard."""

    def __init__(self, strictness: str = "standard") -> None:
        self.strictness = strictness.strip().lower()
        targets = list(HARDCORE_ANTICHEAT_BINARIES) if self.strictness == "hardcore" else []
        self.supervisor = ProcessSupervisor(targets)

    def enforce(self) -> list[str]:
        if self.strictness == "hardcore":
            return self.supervisor.scan_and_terminate()
        return []
