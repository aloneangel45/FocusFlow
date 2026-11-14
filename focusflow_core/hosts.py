"""Crash-safe SHA-256 atomic hosts engine with write-ahead journal and dual-stack IPv4/IPv6."""

from __future__ import annotations

import ctypes
import dataclasses
import datetime
import hashlib
import json
import os
import sys
import tempfile
import time
from typing import Callable

from .paths import (
    BACKUP_FILENAME,
    JOURNAL_FILENAME,
    get_default_hosts_path,
    get_state_dir,
    system_default_hosts_path,
)
from .privileges import flush_dns_async
from .sanitize import sanitize_domains
from .supervisor import is_pid_alive

MARKER_BEGIN = "# --- BEGIN FOCUSFLOW MANAGED BLOCK ---"
MARKER_END = "# --- END FOCUSFLOW MANAGED BLOCK ---"
LEGACY_MARKER_BEGIN = "# --- FOCUSFLOW BLOCK START ---"
LEGACY_MARKER_END = "# --- FOCUSFLOW BLOCK END ---"

REPLACEFILE_IGNORE_MERGE_ERRORS = 0x00000002
FILE_ATTRIBUTE_READONLY = 0x00000001
INVALID_FILE_ATTRIBUTES = 0xFFFFFFFF
MAX_WRITE_RETRIES = 5
WRITE_RETRY_BACKOFF_SECONDS = 0.040
WIN32_SHARING_WINERRORS = frozenset({32, 33})

if sys.platform == "win32":
    from ctypes import wintypes

    _k32 = ctypes.WinDLL("kernel32", use_last_error=True)
    _k32.ReplaceFileW.argtypes = [
        wintypes.LPCWSTR,
        wintypes.LPCWSTR,
        wintypes.LPCWSTR,
        wintypes.DWORD,
        wintypes.LPVOID,
        wintypes.LPVOID,
    ]
    _k32.ReplaceFileW.restype = wintypes.BOOL
    _k32.GetFileAttributesW.argtypes = [wintypes.LPCWSTR]
    _k32.GetFileAttributesW.restype = wintypes.DWORD
    _k32.SetFileAttributesW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD]
    _k32.SetFileAttributesW.restype = wintypes.BOOL


@dataclasses.dataclass
class HostsJournal:
    original_sha256: str
    backup_path: str
    hosts_path: str
    session_id: str
    pid: int
    started_at: str
    expected_end_at: str
    profile: str

    def to_dict(self) -> dict[str, object]:
        return dataclasses.asdict(self)

    @classmethod
    def from_dict(cls, data: dict[str, object]) -> "HostsJournal":
        raw_pid = data.get("pid", 0)
        try:
            pid_val = int(raw_pid) if raw_pid is not None and not isinstance(raw_pid, bool) else 0
        except (TypeError, ValueError):
            pid_val = 0
        return cls(
            original_sha256=str(data.get("original_sha256") or ""),
            backup_path=str(data.get("backup_path") or ""),
            hosts_path=str(data.get("hosts_path") or ""),
            session_id=str(data.get("session_id") or ""),
            pid=pid_val,
            started_at=str(data.get("started_at") or ""),
            expected_end_at=str(data.get("expected_end_at") or ""),
            profile=str(data.get("profile") or "study"),
        )


@dataclasses.dataclass
class SessionMeta:
    session_id: str
    pid: int = 0
    started_at: datetime.datetime | None = None
    expected_end_at: datetime.datetime | None = None
    profile: str = "study"


@dataclasses.dataclass
class RecoveryResult:
    recovered: bool
    used_fallback: bool = False
    orphan_session_id: str = ""
    active_owner_pid: int = 0
    restored_checksum: str = ""
    reason: str = "clean"


def compute_bytes_sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def format_rfc3339_utc(dt: datetime.datetime) -> str:
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=datetime.timezone.utc)
    else:
        dt = dt.astimezone(datetime.timezone.utc)
    return dt.replace(microsecond=0).isoformat().replace("+00:00", "Z")


def parse_rfc3339(ts: str) -> datetime.datetime | None:
    s = (ts or "").strip()
    if not s:
        return None
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    try:
        dt = datetime.datetime.fromisoformat(s)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=datetime.timezone.utc)
        return dt.astimezone(datetime.timezone.utc)
    except ValueError:
        return None


def has_managed_markers(content: str) -> bool:
    return any(
        m in content
        for m in (MARKER_BEGIN, MARKER_END, LEGACY_MARKER_BEGIN, LEGACY_MARKER_END)
    )


def detect_line_ending(content: str) -> str:
    if "\r\n" in content:
        return "\r\n"
    if "\n" in content:
        return "\n"
    return "\r\n" if sys.platform == "win32" else "\n"


def _is_begin_marker_line(line: str) -> bool:
    t = line.strip()
    return (
        t == MARKER_BEGIN
        or t == LEGACY_MARKER_BEGIN
        or MARKER_BEGIN in line
        or LEGACY_MARKER_BEGIN in line
    )


def _is_end_marker_line(line: str) -> bool:
    t = line.strip()
    return (
        t == MARKER_END
        or t == LEGACY_MARKER_END
        or MARKER_END in line
        or LEGACY_MARKER_END in line
    )


def _is_likely_focusflow_sinkhole_line(line: str) -> bool:
    t = line.strip()
    if not t:
        return True
    if t.startswith("#"):
        lower = t.lower()
        return any(w in lower for w in ("focusflow", "managed", "sinkhole", "distraction"))
    parts = t.split()
    if len(parts) < 2:
        return False
    ip, host = parts[0], parts[1].lower()
    if ip in ("0.0.0.0", "::"):
        return True
    if ip == "127.0.0.1" and host not in ("localhost", "localhost.localdomain"):
        return True
    return False


def strip_managed_blocks(content: str) -> str:
    """Deterministically strip all current & legacy FocusFlow managed blocks.

    Safely handles truncated/missing end-markers without string-slice corruption, and
    returns `content` byte-for-byte unchanged when no markers are present.
    """
    if not has_managed_markers(content):
        return content

    eol = detect_line_ending(content)
    had_trailing_newline = content.endswith("\n")

    raw_lines = content.split("\n")
    if had_trailing_newline and raw_lines and raw_lines[-1] == "":
        raw_lines = raw_lines[:-1]

    lines = [l[:-1] if l.endswith("\r") else l for l in raw_lines]
    kept: list[str] = []
    i = 0
    n = len(lines)

    while i < n:
        line = lines[i]
        if _is_begin_marker_line(line):
            if kept and kept[-1].strip() == "":
                kept.pop()

            end_idx = -1
            for j in range(i + 1, n):
                if _is_end_marker_line(lines[j]):
                    end_idx = j
                    break
                if _is_begin_marker_line(lines[j]):
                    break

            if end_idx >= 0:
                i = end_idx + 1
                continue

            i += 1
            while i < n:
                if _is_begin_marker_line(lines[i]):
                    break
                if _is_end_marker_line(lines[i]):
                    i += 1
                    break
                if _is_likely_focusflow_sinkhole_line(lines[i]):
                    i += 1
                    continue
                break
            continue

        if _is_end_marker_line(line):
            i += 1
            continue

        kept.append(line)
        i += 1

    if not kept:
        return ""
    out = eol.join(kept)
    if had_trailing_newline:
        out += eol
    return out


def build_managed_block(sanitized_domains: list[str], eol: str) -> str:
    """Build dual-stack IPv4 (0.0.0.0) + IPv6 (::) sinkhole block."""
    rows: list[str] = [
        MARKER_BEGIN,
        "# FocusFlow v2.1 - Dual-Stack Managed Sinkhole",
    ]
    for dom in sanitized_domains:
        rows.append(f"0.0.0.0 {dom}")
        if not dom.startswith("www."):
            rows.append(f"0.0.0.0 www.{dom}")
        rows.append(f":: {dom}")
        if not dom.startswith("www."):
            rows.append(f":: www.{dom}")
    rows.append(MARKER_END)
    return eol.join(rows) + eol


def _is_transient_sharing_error(exc: BaseException) -> bool:
    if isinstance(exc, OSError):
        winerr = getattr(exc, "winerror", None)
        if winerr in WIN32_SHARING_WINERRORS:
            return True
    return False


def read_hosts_bytes_with_retry(hosts_path: str) -> bytes:
    last_exc: Exception | None = None
    for attempt in range(MAX_WRITE_RETRIES):
        try:
            with open(hosts_path, "rb") as f:
                return f.read()
        except OSError as exc:
            last_exc = exc
            if _is_transient_sharing_error(exc) and attempt + 1 < MAX_WRITE_RETRIES:
                time.sleep(WRITE_RETRY_BACKOFF_SECONDS)
                continue
            raise
    assert last_exc is not None
    raise last_exc


def _write_atomic_state_bytes(path: str, data: bytes) -> None:
    parent = os.path.dirname(path) or "."
    os.makedirs(parent, exist_ok=True)
    fd, tmp_path = tempfile.mkstemp(prefix="ff-state-", suffix=".tmp", dir=parent)
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        try:
            os.chmod(tmp_path, 0o644)
        except OSError:
            pass
        os.replace(tmp_path, path)
    except Exception:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass
        raise


def write_hosts_file_safe(hosts_path: str, content: bytes) -> None:
    """Write hosts file preserving Windows DACLs / POSIX modes, fsyncing, and retrying sharing locks."""
    parent = os.path.dirname(hosts_path) or "."
    os.makedirs(parent, exist_ok=True)

    target_exists = os.path.exists(hosts_path)
    target_mode = 0o644
    if target_exists and sys.platform != "win32":
        try:
            target_mode = os.stat(hosts_path).st_mode & 0o777 or 0o644
        except OSError:
            target_mode = 0o644

    fd, tmp_path = tempfile.mkstemp(prefix="focusflow-hosts-", suffix=".tmp", dir=parent)
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(content)
            f.flush()
            os.fsync(f.fileno())
        if sys.platform != "win32":
            os.chmod(tmp_path, target_mode)
    except Exception:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass
        raise

    if sys.platform == "win32" and target_exists:
        orig_attrs = _k32.GetFileAttributesW(hosts_path)
        cleared_readonly = False
        if orig_attrs != INVALID_FILE_ATTRIBUTES and (orig_attrs & FILE_ATTRIBUTE_READONLY):
            if bool(_k32.SetFileAttributesW(hosts_path, orig_attrs & ~FILE_ATTRIBUTE_READONLY)):
                cleared_readonly = True
        try:
            last_winerr = 0
            for attempt in range(MAX_WRITE_RETRIES):
                ok = bool(
                    _k32.ReplaceFileW(
                        hosts_path,
                        tmp_path,
                        None,
                        REPLACEFILE_IGNORE_MERGE_ERRORS,
                        None,
                        None,
                    )
                )
                if ok:
                    return
                last_winerr = ctypes.get_last_error()
                if attempt + 1 < MAX_WRITE_RETRIES:
                    time.sleep(WRITE_RETRY_BACKOFF_SECONDS)

            try:
                os.unlink(tmp_path)
            except OSError:
                pass

            last_exc: OSError | None = None
            for attempt in range(MAX_WRITE_RETRIES):
                try:
                    with open(hosts_path, "r+b") as f:
                        f.seek(0)
                        f.write(content)
                        f.truncate()
                        f.flush()
                        os.fsync(f.fileno())
                    return
                except OSError as exc:
                    last_exc = exc
                    if attempt + 1 < MAX_WRITE_RETRIES:
                        time.sleep(WRITE_RETRY_BACKOFF_SECONDS)
                        continue
                    raise
            if last_exc is not None:
                raise last_exc
            raise OSError(last_winerr, f"ReplaceFileW failed on {hosts_path}")
        finally:
            if cleared_readonly:
                _k32.SetFileAttributesW(hosts_path, orig_attrs)

    last_err: OSError | None = None
    for attempt in range(MAX_WRITE_RETRIES):
        try:
            os.replace(tmp_path, hosts_path)
            return
        except OSError as exc:
            last_err = exc
            if attempt + 1 < MAX_WRITE_RETRIES:
                time.sleep(WRITE_RETRY_BACKOFF_SECONDS)
                continue
            try:
                os.unlink(tmp_path)
            except OSError:
                pass
            raise
    if last_err is not None:
        raise last_err


class HostsManager:
    """Unified crash-safe SHA-256 hosts manager identical to Go `engine.HostsManager`."""

    def __init__(self, hosts_path: str | None = None, state_dir: str | None = None) -> None:
        resolved_hosts = os.path.normpath(hosts_path) if hosts_path else get_default_hosts_path()
        if state_dir:
            resolved_state = os.path.normpath(state_dir)
        elif os.environ.get("FOCUSFLOW_HOME", "").strip():
            resolved_state = get_state_dir()
        elif resolved_hosts != os.path.normpath(system_default_hosts_path()):
            resolved_state = os.path.dirname(resolved_hosts) or "."
        else:
            resolved_state = get_state_dir()

        self.hosts_path = resolved_hosts
        self.state_dir = resolved_state
        self.backup_path = os.path.join(resolved_state, BACKUP_FILENAME)
        self.journal_path = os.path.join(resolved_state, JOURNAL_FILENAME)

    def compute_sha256(self, path: str) -> str:
        with open(path, "rb") as f:
            return compute_bytes_sha256(f.read())

    def backup(self) -> str:
        if not os.path.exists(self.hosts_path):
            raise FileNotFoundError(f"hosts file not found at {self.hosts_path}")
        raw_text = read_hosts_bytes_with_retry(self.hosts_path).decode("utf-8", errors="surrogateescape")
        clean_text = strip_managed_blocks(raw_text)
        clean_bytes = clean_text.encode("utf-8", errors="surrogateescape")
        _write_atomic_state_bytes(self.backup_path, clean_bytes)
        return compute_bytes_sha256(clean_bytes)

    def load_journal(self) -> HostsJournal | None:
        if not os.path.exists(self.journal_path):
            return None
        with open(self.journal_path, "r", encoding="utf-8-sig") as f:
            raw = json.load(f)
        if not isinstance(raw, dict):
            raise ValueError("corrupt journal structure")
        return HostsJournal.from_dict(raw)

    def _write_journal(self, journal: HostsJournal) -> None:
        payload = (json.dumps(journal.to_dict(), indent=2) + "\n").encode("utf-8")
        _write_atomic_state_bytes(self.journal_path, payload)

    def apply_blocklist(
        self,
        domains: list[str] | tuple[str, ...],
        meta: SessionMeta | None = None,
    ) -> bool:
        clean_domains = sanitize_domains(domains)
        if not clean_domains:
            return False

        if not os.path.exists(self.hosts_path):
            raise FileNotFoundError(f"hosts file not found at {self.hosts_path}")

        raw_bytes = read_hosts_bytes_with_retry(self.hosts_path)
        raw_text = raw_bytes.decode("utf-8", errors="surrogateescape")
        clean_baseline = strip_managed_blocks(raw_text)
        clean_bytes = clean_baseline.encode("utf-8", errors="surrogateescape")
        baseline_sha = compute_bytes_sha256(clean_bytes)

        need_fresh_backup = True
        try:
            existing_journal = self.load_journal()
            if existing_journal and existing_journal.original_sha256 and os.path.exists(self.backup_path):
                with open(self.backup_path, "rb") as f:
                    bak_bytes = f.read()
                bak_text = bak_bytes.decode("utf-8", errors="surrogateescape")
                if not has_managed_markers(bak_text) and compute_bytes_sha256(bak_bytes) == existing_journal.original_sha256:
                    baseline_sha = existing_journal.original_sha256
                    need_fresh_backup = False
        except Exception:
            need_fresh_backup = True

        if need_fresh_backup:
            _write_atomic_state_bytes(self.backup_path, clean_bytes)

        now_utc = datetime.datetime.now(datetime.timezone.utc)
        if meta is None:
            meta = SessionMeta(
                session_id=os.urandom(8).hex(),
                pid=os.getpid(),
                started_at=now_utc,
                expected_end_at=now_utc + datetime.timedelta(hours=2),
                profile="study",
            )

        pid = meta.pid if meta.pid > 0 else os.getpid()
        started_at = meta.started_at or now_utc
        expected_end_at = meta.expected_end_at or (started_at + datetime.timedelta(minutes=60))
        session_id = meta.session_id or os.urandom(8).hex()
        profile = meta.profile or "study"

        journal = HostsJournal(
            original_sha256=baseline_sha,
            backup_path=self.backup_path,
            hosts_path=self.hosts_path,
            session_id=session_id,
            pid=pid,
            started_at=format_rfc3339_utc(started_at),
            expected_end_at=format_rfc3339_utc(expected_end_at),
            profile=profile,
        )

        # Write-ahead journal persisted BEFORE modifying system hosts
        self._write_journal(journal)

        eol = detect_line_ending(clean_baseline)
        block = build_managed_block(clean_domains, eol)
        trimmed_base = clean_baseline.rstrip("\r\n")
        new_text = f"{trimmed_base}{eol}{eol}{block}" if trimmed_base else block
        write_hosts_file_safe(self.hosts_path, new_text.encode("utf-8", errors="surrogateescape"))
        return True

    def verify_or_reapply(
        self,
        domains: list[str] | tuple[str, ...],
        meta: SessionMeta | None = None,
    ) -> bool:
        """Check mid-session hosts integrity and re-apply blocklist if tampered. Returns True if tampered."""
        clean_domains = sanitize_domains(domains)
        if not clean_domains:
            return False

        raw_text = read_hosts_bytes_with_retry(self.hosts_path).decode("utf-8", errors="surrogateescape")
        if MARKER_BEGIN not in raw_text or MARKER_END not in raw_text:
            self.apply_blocklist(clean_domains, meta=meta)
            return True

        for dom in clean_domains:
            if f"0.0.0.0 {dom}" not in raw_text or f":: {dom}" not in raw_text:
                self.apply_blocklist(clean_domains, meta=meta)
                return True
        return False

    def _restore_hosts_from_backup_or_strip(self) -> bool:
        """Restore hosts via SHA-256 verified backup or deterministic marker stripping fallback.

        Returns True if fallback marker stripping was used, False if .bak matched SHA-256.
        """
        journal: HostsJournal | None = None
        try:
            journal = self.load_journal()
        except Exception:
            journal = None

        bak_path = journal.backup_path if (journal and journal.backup_path) else self.backup_path
        if os.path.exists(bak_path):
            try:
                with open(bak_path, "rb") as f:
                    bak_bytes = f.read()
                bak_text = bak_bytes.decode("utf-8", errors="surrogateescape")
                bak_clean = not has_managed_markers(bak_text)
                hash_matches = True
                if journal and journal.original_sha256:
                    hash_matches = compute_bytes_sha256(bak_bytes) == journal.original_sha256
                if bak_clean and hash_matches:
                    write_hosts_file_safe(self.hosts_path, bak_bytes)
                    return False
            except OSError:
                pass

        if os.path.exists(self.hosts_path):
            raw_text = read_hosts_bytes_with_retry(self.hosts_path).decode("utf-8", errors="surrogateescape")
            clean_text = strip_managed_blocks(raw_text)
            write_hosts_file_safe(self.hosts_path, clean_text.encode("utf-8", errors="surrogateescape"))
            return True

        return False

    def _cleanup_state_artifacts(self) -> None:
        for p in (self.journal_path, self.backup_path, f"{self.hosts_path}.focusflow.bak"):
            try:
                if os.path.exists(p):
                    os.unlink(p)
            except OSError:
                pass

    def rollback(self) -> None:
        self._restore_hosts_from_backup_or_strip()
        self._cleanup_state_artifacts()

    def recover_if_orphaned(
        self,
        on_orphan_session: Callable[[str], None] | None = None,
        now: datetime.datetime | None = None,
        flush_async: bool = True,
    ) -> RecoveryResult:
        """Automatically recover crashed/rebooted/expired hosts blocks on startup."""
        now_utc = (now or datetime.datetime.now(datetime.timezone.utc)).astimezone(datetime.timezone.utc)

        has_corrupt_journal = False
        journal: HostsJournal | None = None
        if os.path.exists(self.journal_path):
            try:
                journal = self.load_journal()
            except Exception:
                has_corrupt_journal = True

        hosts_has_markers = False
        if os.path.exists(self.hosts_path):
            try:
                txt = read_hosts_bytes_with_retry(self.hosts_path).decode("utf-8", errors="surrogateescape")
                hosts_has_markers = has_managed_markers(txt)
            except OSError:
                hosts_has_markers = False

        if journal is None and not has_corrupt_journal and not hosts_has_markers:
            return RecoveryResult(recovered=False, reason="clean")

        reason = "stray_markers_without_journal"
        orphan_session_id = ""

        if journal is not None:
            orphan_session_id = journal.session_id
            exp_dt = parse_rfc3339(journal.expected_end_at)
            expired = exp_dt is not None and now_utc > exp_dt
            pid_alive = journal.pid > 0 and is_pid_alive(journal.pid)

            if pid_alive and not expired:
                return RecoveryResult(
                    recovered=False,
                    active_owner_pid=journal.pid,
                    reason="active_session_alive",
                )
            reason = f"owner_pid_{journal.pid}_dead" if not pid_alive else "session_lease_expired"
        elif has_corrupt_journal:
            reason = "corrupt_journal"

        used_fallback = self._restore_hosts_from_backup_or_strip()
        self._cleanup_state_artifacts()

        if on_orphan_session is not None:
            on_orphan_session(orphan_session_id)

        if flush_async:
            flush_dns_async()

        checksum = self.compute_sha256(self.hosts_path) if os.path.exists(self.hosts_path) else ""
        return RecoveryResult(
            recovered=True,
            used_fallback=used_fallback,
            orphan_session_id=orphan_session_id,
            restored_checksum=checksum,
            reason=reason,
        )

    def is_active(self) -> bool:
        if not os.path.exists(self.hosts_path):
            return False
        try:
            txt = read_hosts_bytes_with_retry(self.hosts_path).decode("utf-8", errors="surrogateescape")
            return MARKER_BEGIN in txt or LEGACY_MARKER_BEGIN in txt
        except OSError:
            return False
