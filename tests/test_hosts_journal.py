"""Tests for crash-safe SHA-256 hosts engine, write-ahead journal, ACLs, and orphan recovery."""

from __future__ import annotations

import datetime
import os
from pathlib import Path
import subprocess
import sys

from focusflow_core.hosts import (
    LEGACY_MARKER_BEGIN,
    LEGACY_MARKER_END,
    MARKER_BEGIN,
    MARKER_END,
    HostsManager,
    SessionMeta,
    has_managed_markers,
    strip_managed_blocks,
)
from focusflow_core.storage import SessionStore


def test_atomic_dual_stack_apply_journal_and_rollback(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))
    initial_sha = hm.compute_sha256(str(hosts_path))

    now = datetime.datetime.now(datetime.timezone.utc)
    meta = SessionMeta(
        session_id="py-sess-001",
        pid=os.getpid(),
        started_at=now,
        expected_end_at=now + datetime.timedelta(minutes=25),
        profile="study",
    )

    assert hm.apply_blocklist(["https://www.Instagram.com/explore", "tiktok.com:443"], meta=meta)
    assert hm.is_active()

    journal = hm.load_journal()
    assert journal is not None
    assert journal.original_sha256 == initial_sha
    assert journal.session_id == "py-sess-001"
    assert journal.pid == os.getpid()
    assert journal.profile == "study"
    assert os.path.exists(journal.backup_path)

    content = hosts_path.read_text(encoding="utf-8")
    for expected in (
        MARKER_BEGIN,
        "0.0.0.0 instagram.com",
        "0.0.0.0 www.instagram.com",
        ":: instagram.com",
        ":: www.instagram.com",
        "0.0.0.0 tiktok.com",
        "0.0.0.0 www.tiktok.com",
        ":: tiktok.com",
        ":: www.tiktok.com",
        MARKER_END,
    ):
        assert expected in content

    hm.rollback()
    assert not hm.is_active()
    assert not os.path.exists(hm.journal_path)
    assert not os.path.exists(hm.backup_path)
    assert hm.compute_sha256(str(hosts_path)) == initial_sha


def test_never_backup_contaminated_hosts(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    clean_base = "127.0.0.1 localhost\r\n192.168.1.99 workstation.lan\r\n"
    contaminated = (
        clean_base
        + "\r\n"
        + f"{LEGACY_MARKER_BEGIN}\r\n127.0.0.1 youtube.com\r\n{LEGACY_MARKER_END}\r\n\r\n"
        + f"{MARKER_BEGIN}\r\n0.0.0.0 reddit.com\r\n{MARKER_END}\r\n"
    )
    hosts_path.write_bytes(contaminated.encode("utf-8"))

    hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))
    hm.apply_blocklist(["x.com"])

    bak_content = Path(hm.backup_path).read_bytes().decode("utf-8")
    assert not has_managed_markers(bak_content)
    assert bak_content == clean_base

    hm.rollback()
    assert hosts_path.read_bytes().decode("utf-8") == clean_base


def test_missing_end_marker_safe_stripping_no_corruption() -> None:
    truncated_current = (
        "127.0.0.1 localhost\n::1 localhost\n\n"
        f"{MARKER_BEGIN}\n# FocusFlow v2.1 - Dual-Stack Managed Sinkhole\n"
        "0.0.0.0 youtube.com\n:: youtube.com\n"
    )
    assert strip_managed_blocks(truncated_current) == "127.0.0.1 localhost\n::1 localhost\n"

    truncated_legacy_with_custom = (
        "127.0.0.1 localhost\r\n\r\n"
        f"{LEGACY_MARKER_BEGIN}\r\n127.0.0.1 instagram.com\r\n"
        "10.0.0.25 internal.corp\r\n"
    )
    assert strip_managed_blocks(truncated_legacy_with_custom) == "127.0.0.1 localhost\r\n10.0.0.25 internal.corp\r\n"


def test_crlf_and_lf_line_ending_preservation(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    crlf_base = b"127.0.0.1 localhost\r\n::1 localhost\r\n"
    hosts_path.write_bytes(crlf_base)

    hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))
    hm.apply_blocklist(["reddit.com"])
    raw_after = hosts_path.read_bytes()
    assert b"\r\n0.0.0.0 reddit.com\r\n" in raw_after
    assert b"\r\n:: reddit.com\r\n" in raw_after

    hm.rollback()
    assert hosts_path.read_bytes() == crlf_base


def test_recover_if_orphaned_dead_pid_expired_and_corrupt_bak_fallback(
    hermetic_focusflow_env: tuple[Path, Path],
) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    clean_text = "127.0.0.1 localhost\n::1 localhost\n"
    hosts_path.write_text(clean_text, encoding="utf-8")

    hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))
    store = SessionStore(state_dir=str(state_dir))
    now = datetime.datetime.now(datetime.timezone.utc)

    # 1. Active current PID with future lease must not be clobbered
    hm.apply_blocklist(
        ["reddit.com"],
        meta=SessionMeta(
            session_id="live-1",
            pid=os.getpid(),
            started_at=now,
            expected_end_at=now + datetime.timedelta(minutes=30),
            profile="dev",
        ),
    )
    res_live = hm.recover_if_orphaned(now=now, flush_async=False)
    assert not res_live.recovered
    assert res_live.active_owner_pid == os.getpid()

    # 2. Dead PID + tampered .bak triggers deterministic marker-stripping fallback + marks session orphaned
    store.start_session("dead-sess-88", "hardcore", 25, start_time=now - datetime.timedelta(minutes=10))
    hm.apply_blocklist(
        ["reddit.com", "twitch.tv"],
        meta=SessionMeta(
            session_id="dead-sess-88",
            pid=99999999,
            started_at=now - datetime.timedelta(minutes=10),
            expected_end_at=now + datetime.timedelta(minutes=15),
            profile="hardcore",
        ),
    )
    Path(hm.backup_path).write_text("corrupted-backup-contents", encoding="utf-8")

    res_dead = hm.recover_if_orphaned(
        on_orphan_session=lambda sid: store.mark_orphaned_session(sid, now=now),
        now=now,
        flush_async=False,
    )
    assert res_dead.recovered
    assert res_dead.used_fallback
    assert res_dead.orphan_session_id == "dead-sess-88"
    assert hosts_path.read_text(encoding="utf-8") == clean_text

    sessions = store.load_sessions()
    assert len(sessions) == 1
    assert sessions[0].id == "dead-sess-88"
    assert sessions[0].completed is False
    assert sessions[0].status == "orphaned"


def test_verify_or_reapply_mid_session_watchdog(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    clean_text = "127.0.0.1 localhost\n"
    hosts_path.write_text(clean_text, encoding="utf-8")

    hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))
    domains = ["youtube.com", "netflix.com"]
    hm.apply_blocklist(domains)
    assert hm.verify_or_reapply(domains) is False

    # Simulate external tampering mid-sprint
    hosts_path.write_text(clean_text, encoding="utf-8")
    assert hm.verify_or_reapply(domains) is True
    assert hm.is_active()

    hm.rollback()
    assert hosts_path.read_text(encoding="utf-8") == clean_text


def test_permissions_and_windows_acl_preservation(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    hosts_path.write_bytes(b"127.0.0.1 localhost\r\n")
    hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))

    if sys.platform == "win32":
        subprocess.run(["icacls", str(hosts_path), "/grant", "*S-1-5-32-545:(R)"], check=False, capture_output=True)
        before = subprocess.run(["icacls", str(hosts_path)], check=True, capture_output=True, text=True).stdout
        hm.apply_blocklist(["example.com"])
        after = subprocess.run(["icacls", str(hosts_path)], check=True, capture_output=True, text=True).stdout
        assert before == after
    else:
        os.chmod(hosts_path, 0o644)
        hm.apply_blocklist(["example.com"])
        assert (os.stat(hosts_path).st_mode & 0o777) == 0o644


def test_readonly_hosts_apply_and_rollback(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    clean_text = "127.0.0.1 localhost\n::1 localhost\n"
    hosts_path.write_text(clean_text, encoding="utf-8")
    os.chmod(hosts_path, 0o444)
    try:
        hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))
        assert hm.apply_blocklist(["reddit.com"]) is True
        assert hm.is_active() is True
        assert (os.stat(hosts_path).st_mode & 0o200) == 0

        hm.rollback()
        assert hm.is_active() is False
        assert hosts_path.read_text(encoding="utf-8") == clean_text
    finally:
        os.chmod(hosts_path, 0o644)


def test_journal_utf8_bom_tolerance(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))
    Path(hm.journal_path).write_bytes(
        b'\xef\xbb\xbf{"session_id": "bom-j-1", "pid": 4321, "profile": "dev"}\n'
    )
    loaded = hm.load_journal()
    assert loaded is not None
    assert loaded.session_id == "bom-j-1"
    assert loaded.pid == 4321
    assert loaded.profile == "dev"
