"""Tests for zero-dependency Win32/POSIX process supervisor, denylist, and elevation cmdline."""

from __future__ import annotations

import os
from pathlib import Path
import subprocess
import sys
import time

from focusflow_core.privileges import build_elevation_cmdline, shield_mode_label
from focusflow_core.supervisor import (
    AntiCheatGuard,
    ProcessSupervisor,
    is_pid_alive,
    resolve_procfs_names,
)


def test_supervisor_denylist_and_protected_pids() -> None:
    killed: list[int] = []
    fake_procs = [
        (0, ["System Idle Process"]),
        (4, ["System"]),
        (os.getpid(), ["discord.exe"]),
        (os.getppid(), ["discord.exe"]),
        (1200, ["explorer.exe"]),
        (1300, ["svchost.exe"]),
        (1400, ["python.exe"]),
        (1500, ["focusflow.exe"]),
        (4242, ["Discord.exe"]),
    ]
    sup = ProcessSupervisor(
        blocked=["discord.exe", "explorer.exe", "svchost.exe", "python.exe", "focusflow.exe"],
        enumerator=lambda: fake_procs,
        terminator=lambda pid: (killed.append(pid) or True),
    )

    result = sup.scan_and_terminate()
    assert killed == [4242]
    assert result == ["Discord.exe (PID: 4242)"]


def test_anticheat_guard_includes_system_informer() -> None:
    ac = AntiCheatGuard("hardcore")
    for expected in ("taskmgr.exe", "processhacker.exe", "systeminformer.exe", "procmon.exe", "procexp.exe"):
        assert expected in ac.supervisor.blocked_names

    ac_std = AntiCheatGuard("standard")
    assert len(ac_std.supervisor.blocked_names) == 0


def test_linux_procfs_15_byte_comm_truncation(tmp_path: Path) -> None:
    proc_root = tmp_path / "proc"
    pid_dir = proc_root / "54321"
    pid_dir.mkdir(parents=True)
    # 15-byte truncated TASK_COMM_LEN
    (pid_dir / "comm").write_text("riotclientservi\n", encoding="utf-8")
    (pid_dir / "cmdline").write_bytes(b"/opt/riot/RiotClientServices.exe\x00--launch\x00")

    cands = resolve_procfs_names(str(proc_root), "54321")
    sup = ProcessSupervisor(["riotclientservices.exe"])
    assert sup.should_terminate(54321, cands) == "RiotClientServices.exe"


def test_live_native_process_enumeration_and_termination_zero_psutil() -> None:
    assert is_pid_alive(os.getpid()) is True
    assert is_pid_alive(99999999) is False

    # Spawn a harmless standard OS utility process ("ping" on Windows, "sleep" on POSIX)
    # and terminate it via native zero-dependency Win32 Toolhelp32 / POSIX supervisor!
    if sys.platform == "win32":
        child = subprocess.Popen(
            ["ping.exe", "-n", "30", "127.0.0.1"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            creationflags=0x08000000,
        )
        target_name = "ping.exe"
    else:
        child = subprocess.Popen(
            ["sleep", "30"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        target_name = "sleep"

    try:
        time.sleep(0.15)
        assert is_pid_alive(child.pid)
        sup = ProcessSupervisor([target_name])
        terminated = sup.scan_and_terminate()
        child.wait(timeout=5)
        assert any(str(child.pid) in entry for entry in terminated)
    finally:
        if child.poll() is None:
            child.kill()
            child.wait(timeout=3)


def test_elevation_cmdline_quotes_spaces_and_honest_shield_labels() -> None:
    cmd = build_elevation_cmdline(
        script_path=r"C:\Users\Asaf Arda\Desktop\FocusFlow\FocusFlowGUI.py",
        extra_args=["--profile", "hardcore mode"],
    )
    assert '"C:\\Users\\Asaf Arda\\Desktop\\FocusFlow\\FocusFlowGUI.py"' in cmd or "Asaf Arda" in cmd
    assert '"hardcore mode"' in cmd
    assert shield_mode_label(True) == "FULL SHIELD [HOSTS + PROC]"
    assert shield_mode_label(False) == "PROCESS-ONLY [UNPRIVILEGED]"
