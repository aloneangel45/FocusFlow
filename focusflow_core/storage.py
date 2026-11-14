"""Unified sessions.json telemetry store, real streak/analytics engine, and JSON/CSV exporters."""

from __future__ import annotations

import csv
import dataclasses
import datetime
import io
import json
import os
import secrets
import tempfile
import threading

from .config import (
    DEFAULT_DAILY_GOAL_MINUTES,
    normalize_daily_goal_minutes,
)
from .hosts import format_rfc3339_utc
from .paths import get_config_path, get_sessions_path, get_state_dir


def calculate_daily_goal_progress_pct(today_minutes: int, daily_goal_minutes: int) -> float:
    goal = normalize_daily_goal_minutes(daily_goal_minutes)
    if today_minutes <= 0 or goal <= 0:
        return 0.0
    return round((today_minutes / goal) * 100.0, 2)


@dataclasses.dataclass
class DailyGoalProgress:
    today_focus_minutes: int
    daily_goal_minutes: int
    daily_goal_progress_pct: float

    def to_dict(self) -> dict[str, object]:
        return {
            "today_focus_minutes": self.today_focus_minutes,
            "daily_goal_minutes": self.daily_goal_minutes,
            "daily_goal_progress_pct": round(self.daily_goal_progress_pct, 2),
        }


def generate_session_id() -> str:
    return secrets.token_hex(8)


@dataclasses.dataclass
class SessionRecord:
    profile: str
    start_time: str
    duration_minutes: int
    completed: bool
    blocked_count: int = 0
    id: str = ""
    end_time: str = ""
    planned_minutes: int = 0
    status: str = ""

    def to_dict(self) -> dict[str, object]:
        out: dict[str, object] = {}
        if self.id:
            out["id"] = self.id
        out["profile"] = self.profile
        out["start_time"] = self.start_time
        if self.end_time:
            out["end_time"] = self.end_time
        out["duration_minutes"] = int(self.duration_minutes)
        if self.planned_minutes > 0:
            out["planned_minutes"] = int(self.planned_minutes)
        out["completed"] = bool(self.completed)
        if self.status:
            out["status"] = self.status
        out["blocked_count"] = int(self.blocked_count)
        return out

    @classmethod
    def from_dict(cls, data: dict[str, object]) -> "SessionRecord":
        return cls(
            id=str(data.get("id", "")),
            profile=str(data.get("profile", "study")),
            start_time=str(data.get("start_time", "")),
            end_time=str(data.get("end_time", "")),
            duration_minutes=int(data.get("duration_minutes", 0)),
            planned_minutes=int(data.get("planned_minutes", 0)),
            completed=bool(data.get("completed", False)),
            status=str(data.get("status", "")),
            blocked_count=int(data.get("blocked_count", 0)),
        )


@dataclasses.dataclass
class DailyStat:
    date: str
    minutes: int
    sprints: int

    def to_dict(self) -> dict[str, object]:
        return dataclasses.asdict(self)


@dataclasses.dataclass
class AnalyticsSummary:
    completed_sprints: int
    attempted_sprints: int
    aborted_sprints: int
    completion_rate: float
    total_minutes: int
    total_hours: float
    total_blocked: int
    active_streak_days: int
    daily: list[DailyStat]
    today_focus_minutes: int = 0
    daily_goal_minutes: int = DEFAULT_DAILY_GOAL_MINUTES
    daily_goal_progress_pct: float = 0.0

    def to_dict(self) -> dict[str, object]:
        return {
            "completed_sprints": self.completed_sprints,
            "attempted_sprints": self.attempted_sprints,
            "aborted_sprints": self.aborted_sprints,
            "completion_rate": round(self.completion_rate, 2),
            "total_minutes": self.total_minutes,
            "total_hours": round(self.total_hours, 2),
            "total_blocked": self.total_blocked,
            "active_streak_days": self.active_streak_days,
            "today_focus_minutes": self.today_focus_minutes,
            "daily_goal_minutes": self.daily_goal_minutes,
            "daily_goal_progress_pct": round(self.daily_goal_progress_pct, 2),
            "daily": [d.to_dict() for d in self.daily],
        }


def _parse_session_local_date(ts: str, tz: datetime.tzinfo | None) -> str | None:
    s = (ts or "").strip()
    if not s:
        return None
    if len(s) == 10 and s[4] == "-" and s[7] == "-":
        return s
    iso = s[:-1] + "+00:00" if s.endswith("Z") else s
    try:
        dt = datetime.datetime.fromisoformat(iso)
        if dt.tzinfo is not None and tz is not None:
            dt = dt.astimezone(tz)
        return dt.strftime("%Y-%m-%d")
    except ValueError:
        if len(s) >= 10 and s[4] == "-" and s[7] == "-":
            return s[:10]
        return None


class SessionStore:
    """Thread-safe atomic `sessions.json` store shared identically between Go CLI and Python GUI."""

    def __init__(self, state_dir: str | None = None) -> None:
        self.state_dir = state_dir or get_state_dir()
        os.makedirs(self.state_dir, exist_ok=True)
        self.path = get_sessions_path(self.state_dir)
        self._lock = threading.Lock()

    def _read_locked(self) -> list[SessionRecord]:
        if not os.path.exists(self.path):
            return []
        try:
            with open(self.path, "r", encoding="utf-8") as f:
                raw = json.load(f)
            if isinstance(raw, dict) and isinstance(raw.get("sessions"), list):
                return [SessionRecord.from_dict(item) for item in raw["sessions"] if isinstance(item, dict)]
        except Exception:
            return []
        return []

    def _write_locked(self, sessions: list[SessionRecord]) -> None:
        os.makedirs(self.state_dir, exist_ok=True)
        payload_obj = {"sessions": [s.to_dict() for s in sessions]}
        payload = (json.dumps(payload_obj, indent=2, ensure_ascii=False) + "\n").encode("utf-8")

        fd, tmp_path = tempfile.mkstemp(prefix="sessions-", suffix=".tmp", dir=self.state_dir)
        try:
            with os.fdopen(fd, "wb") as f:
                f.write(payload)
                f.flush()
                os.fsync(f.fileno())
            try:
                os.chmod(tmp_path, 0o644)
            except OSError:
                pass
            os.replace(tmp_path, self.path)
        except Exception:
            try:
                os.unlink(tmp_path)
            except OSError:
                pass
            raise

    def load_sessions(self) -> list[SessionRecord]:
        with self._lock:
            return list(self._read_locked())

    def start_session(
        self,
        session_id: str,
        profile: str,
        planned_minutes: int,
        start_time: datetime.datetime | None = None,
    ) -> str:
        """Record `start_time` immediately when a focus sprint begins."""
        sid = session_id or generate_session_id()
        started = start_time or datetime.datetime.now(datetime.timezone.utc)
        rec = SessionRecord(
            id=sid,
            profile=profile,
            start_time=format_rfc3339_utc(started),
            duration_minutes=0,
            planned_minutes=max(0, int(planned_minutes)),
            completed=False,
            status="running",
            blocked_count=0,
        )
        with self._lock:
            sessions = self._read_locked()
            sessions.append(rec)
            self._write_locked(sessions)
        return sid

    def finalize_session(
        self,
        session_id: str,
        profile: str,
        duration_minutes: int,
        completed: bool,
        blocked_count: int = 0,
        status: str = "",
        start_time: datetime.datetime | None = None,
        end_time: datetime.datetime | None = None,
    ) -> None:
        """Update running session upon completion or early abort."""
        ended = end_time or datetime.datetime.now(datetime.timezone.utc)
        resolved_status = status or ("completed" if completed else "aborted")

        with self._lock:
            sessions = self._read_locked()
            updated = False
            if session_id:
                for idx in range(len(sessions) - 1, -1, -1):
                    if sessions[idx].id == session_id:
                        sessions[idx].duration_minutes = max(0, int(duration_minutes))
                        sessions[idx].completed = bool(completed)
                        sessions[idx].status = resolved_status
                        sessions[idx].blocked_count = max(0, int(blocked_count))
                        sessions[idx].end_time = format_rfc3339_utc(ended)
                        if not sessions[idx].start_time and start_time is not None:
                            sessions[idx].start_time = format_rfc3339_utc(start_time)
                        updated = True
                        break

            if not updated:
                started = start_time or (ended - datetime.timedelta(minutes=max(0, int(duration_minutes))))
                sessions.append(
                    SessionRecord(
                        id=session_id,
                        profile=profile,
                        start_time=format_rfc3339_utc(started),
                        end_time=format_rfc3339_utc(ended),
                        duration_minutes=max(0, int(duration_minutes)),
                        completed=bool(completed),
                        status=resolved_status,
                        blocked_count=max(0, int(blocked_count)),
                    )
                )
            self._write_locked(sessions)

    def mark_orphaned_session(
        self,
        session_id: str = "",
        now: datetime.datetime | None = None,
    ) -> int:
        """Mark crashed/orphaned running sessions as completed=False, status='orphaned'."""
        ended = now or datetime.datetime.now(datetime.timezone.utc)
        marked = 0
        with self._lock:
            sessions = self._read_locked()
            for rec in sessions:
                if (session_id and rec.id == session_id) or rec.status == "running":
                    if rec.status == "running" or (session_id and rec.id == session_id and (rec.completed or not rec.status)):
                        rec.completed = False
                        rec.status = "orphaned"
                        if not rec.end_time:
                            rec.end_time = format_rfc3339_utc(ended)
                        marked += 1
            if marked > 0:
                self._write_locked(sessions)
        return marked

    def _read_configured_daily_goal(self) -> int:
        cfg_path = get_config_path(self.state_dir)
        if not os.path.exists(cfg_path):
            return DEFAULT_DAILY_GOAL_MINUTES
        try:
            with open(cfg_path, "r", encoding="utf-8") as f:
                raw = json.load(f)
            if isinstance(raw, dict):
                return normalize_daily_goal_minutes(raw.get("daily_goal_minutes"))
        except Exception:
            pass
        return DEFAULT_DAILY_GOAL_MINUTES

    def compute_daily_goal_progress(
        self,
        daily_goal_minutes: int | None = None,
        now: datetime.datetime | None = None,
    ) -> DailyGoalProgress:
        summary = self.compute_analytics(days=1, now=now, daily_goal_minutes=daily_goal_minutes)
        return DailyGoalProgress(
            today_focus_minutes=summary.today_focus_minutes,
            daily_goal_minutes=summary.daily_goal_minutes,
            daily_goal_progress_pct=summary.daily_goal_progress_pct,
        )

    def compute_analytics(
        self,
        days: int = 7,
        now: datetime.datetime | None = None,
        daily_goal_minutes: int | None = None,
    ) -> AnalyticsSummary:
        """Compute genuine streak, total focus hours, daily goal progress, completion rate, and N-day bar chart."""
        window = days if days > 0 else 7
        ref_now = now or datetime.datetime.now().astimezone()
        tz = ref_now.tzinfo
        today = ref_now.date()

        if daily_goal_minutes is None or daily_goal_minutes <= 0:
            goal_minutes = self._read_configured_daily_goal()
        else:
            goal_minutes = normalize_daily_goal_minutes(daily_goal_minutes)

        sessions = self.load_sessions()
        daily_min: dict[str, int] = {}
        daily_sprints: dict[str, int] = {}
        completed_dates: set[str] = set()

        completed_count = 0
        attempted_count = 0
        aborted_count = 0
        total_minutes = 0
        total_blocked = 0

        for rec in sessions:
            total_blocked += max(0, rec.blocked_count)
            if rec.status == "running":
                continue
            attempted_count += 1
            if rec.completed:
                completed_count += 1
                total_minutes += max(0, rec.duration_minutes)
                d_str = _parse_session_local_date(rec.start_time, tz)
                if d_str:
                    daily_min[d_str] = daily_min.get(d_str, 0) + max(0, rec.duration_minutes)
                    daily_sprints[d_str] = daily_sprints.get(d_str, 0) + 1
                    if rec.duration_minutes > 0:
                        completed_dates.add(d_str)
            else:
                aborted_count += 1

        completion_rate = (completed_count / attempted_count * 100.0) if attempted_count > 0 else 0.0

        streak = 0
        today_str = today.strftime("%Y-%m-%d")
        yesterday = today - datetime.timedelta(days=1)
        yesterday_str = yesterday.strftime("%Y-%m-%d")

        cursor: datetime.date | None = None
        if today_str in completed_dates:
            cursor = today
        elif yesterday_str in completed_dates:
            cursor = yesterday

        while cursor is not None and cursor.strftime("%Y-%m-%d") in completed_dates:
            streak += 1
            cursor = cursor - datetime.timedelta(days=1)

        daily: list[DailyStat] = []
        for offset in range(window - 1, -1, -1):
            d_str = (today - datetime.timedelta(days=offset)).strftime("%Y-%m-%d")
            daily.append(
                DailyStat(
                    date=d_str,
                    minutes=daily_min.get(d_str, 0),
                    sprints=daily_sprints.get(d_str, 0),
                )
            )

        today_focus_minutes = daily_min.get(today_str, 0)
        goal_progress_pct = calculate_daily_goal_progress_pct(today_focus_minutes, goal_minutes)

        return AnalyticsSummary(
            completed_sprints=completed_count,
            attempted_sprints=attempted_count,
            aborted_sprints=aborted_count,
            completion_rate=completion_rate,
            total_minutes=total_minutes,
            total_hours=total_minutes / 60.0,
            total_blocked=total_blocked,
            active_streak_days=streak,
            daily=daily,
            today_focus_minutes=today_focus_minutes,
            daily_goal_minutes=goal_minutes,
            daily_goal_progress_pct=goal_progress_pct,
        )

    def export_json_string(
        self,
        days: int = 7,
        now: datetime.datetime | None = None,
    ) -> str:
        ref = now or datetime.datetime.now(datetime.timezone.utc)
        sessions = self.load_sessions()
        summary = self.compute_analytics(days=days, now=ref)
        payload = {
            "generated_at": format_rfc3339_utc(ref),
            "summary": summary.to_dict(),
            "sessions": [s.to_dict() for s in sessions],
        }
        return json.dumps(payload, indent=2, ensure_ascii=False) + "\n"

    def export_csv_string(self) -> str:
        sessions = sorted(self.load_sessions(), key=lambda r: r.start_time)
        buf = io.StringIO()
        writer = csv.writer(buf, lineterminator="\n")
        writer.writerow(
            [
                "id",
                "profile",
                "start_time",
                "end_time",
                "duration_minutes",
                "planned_minutes",
                "completed",
                "status",
                "blocked_count",
            ]
        )
        for rec in sessions:
            status = rec.status or ("completed" if rec.completed else "aborted")
            writer.writerow(
                [
                    rec.id,
                    rec.profile,
                    rec.start_time,
                    rec.end_time,
                    rec.duration_minutes,
                    rec.planned_minutes,
                    str(rec.completed).lower(),
                    status,
                    rec.blocked_count,
                ]
            )
        return buf.getvalue()

    def export_to_file(
        self,
        out_path: str,
        fmt: str = "json",
        days: int = 7,
        now: datetime.datetime | None = None,
    ) -> str:
        clean_path = os.path.abspath(out_path)
        os.makedirs(os.path.dirname(clean_path) or ".", exist_ok=True)
        fmt_lower = fmt.strip().lower()
        if fmt_lower == "json":
            content = self.export_json_string(days=days, now=now)
        elif fmt_lower == "csv":
            content = self.export_csv_string()
        else:
            raise ValueError(f"unsupported export format: {fmt}")
        with open(clean_path, "w", encoding="utf-8", newline="") as f:
            f.write(content)
        return clean_path
