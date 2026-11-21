"""Unified profile and blocklist configuration shared identically with Go `pkg/profile`."""

from __future__ import annotations

import dataclasses
import json
import os
import re
import tempfile
import time

from .paths import get_config_path, get_state_dir
from .sanitize import (
    DomainValidationError,
    ProcessValidationError,
    normalize_process_name,
    sanitize_domain,
    sanitize_domains,
)

DEFAULT_DAILY_GOAL_MINUTES = 120
MIN_DAILY_GOAL_MINUTES = 5
MAX_DAILY_GOAL_MINUTES = 1440

BUILTIN_PROFILE_NAMES: frozenset[str] = frozenset({"study", "dev", "hardcore"})
PROFILE_SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9_-]{0,31}$")

CANONICAL_SCHEDULE_DAYS: tuple[str, ...] = ("mon", "tue", "wed", "thu", "fri", "sat", "sun")
_DAY_ALIAS_TO_INDEX: dict[str, int] = {
    "mon": 0, "monday": 0,
    "tue": 1, "tues": 1, "tuesday": 1,
    "wed": 2, "weds": 2, "wednesday": 2,
    "thu": 3, "thur": 3, "thurs": 3, "thursday": 3,
    "fri": 4, "friday": 4,
    "sat": 5, "saturday": 5,
    "sun": 6, "sunday": 6,
}


def parse_schedule_days(spec: str | list[str] | tuple[str, ...]) -> list[str]:
    """Parse day specifications like 'mon-fri', 'mon,wed,fri', 'fri-mon', 'daily' into canonical mon..sun list."""
    if isinstance(spec, (list, tuple)):
        raw_str = ",".join(str(x) for x in spec if x is not None)
    else:
        raw_str = str(spec or "")
    trimmed = raw_str.strip().lower()
    if not trimmed:
        raise ValueError("empty schedule days specification")

    present = [False] * 7
    for part in trimmed.split(","):
        tok = part.strip()
        if not tok:
            continue
        if tok in ("daily", "all", "everyday", "*"):
            present = [True] * 7
            continue
        if tok in ("weekdays", "weekday"):
            for i in range(5):
                present[i] = True
            continue
        if tok in ("weekends", "weekend"):
            present[5] = True
            present[6] = True
            continue
        if "-" in tok:
            bounds = tok.split("-")
            if len(bounds) != 2:
                raise ValueError(f"invalid schedule day range {tok!r}")
            s_idx = _DAY_ALIAS_TO_INDEX.get(bounds[0].strip())
            e_idx = _DAY_ALIAS_TO_INDEX.get(bounds[1].strip())
            if s_idx is None or e_idx is None:
                raise ValueError(f"invalid schedule day range {tok!r}")
            idx = s_idx
            while True:
                present[idx] = True
                if idx == e_idx:
                    break
                idx = (idx + 1) % 7
            continue
        idx = _DAY_ALIAS_TO_INDEX.get(tok)
        if idx is None:
            raise ValueError(f"invalid schedule day token {tok!r}")
        present[idx] = True

    out = [CANONICAL_SCHEDULE_DAYS[i] for i, ok in enumerate(present) if ok]
    if not out:
        raise ValueError("no schedule days selected")
    return out


def _normalize_clock_time(raw: object) -> str:
    s = str(raw or "").strip()
    parts = s.split(":")
    if len(parts) != 2 or not (1 <= len(parts[0]) <= 2) or len(parts[1]) != 2:
        raise ValueError(f"invalid clock time {raw!r}")
    if not (parts[0].isascii() and parts[0].isdigit() and parts[1].isascii() and parts[1].isdigit()):
        raise ValueError(f"invalid clock time {raw!r}")
    hour = int(parts[0])
    minute = int(parts[1])
    if not (0 <= hour <= 23 and 0 <= minute <= 59):
        raise ValueError(f"clock time out of range {raw!r}")
    return f"{hour:02d}:{minute:02d}"


@dataclasses.dataclass
class ScheduleRule:
    id: str
    name: str
    days: list[str]
    start_time: str
    end_time: str
    profile: str
    enabled: bool = True

    def to_dict(self) -> dict[str, object]:
        return {
            "id": self.id,
            "name": self.name,
            "days": list(self.days),
            "start_time": self.start_time,
            "end_time": self.end_time,
            "profile": self.profile,
            "enabled": bool(self.enabled),
        }

    @classmethod
    def from_dict(cls, data: dict[str, object]) -> "ScheduleRule":
        raw_name_val = data.get("name")
        if not isinstance(raw_name_val, str):
            raise ValueError("schedule name must be a string")
        raw_name = raw_name_val.strip()
        if not raw_name or len(raw_name) > 64 or any(ord(c) < 32 or ord(c) == 127 for c in raw_name):
            raise ValueError(f"invalid schedule name {raw_name!r}")
        raw_days = data.get("days")
        if isinstance(raw_days, str):
            days = parse_schedule_days(raw_days)
        elif isinstance(raw_days, (list, tuple)):
            if not raw_days or not all(isinstance(x, str) for x in raw_days):
                raise ValueError("invalid schedule days")
            days = parse_schedule_days(list(raw_days))
        else:
            raise ValueError("invalid schedule days")
        raw_start = data.get("start_time")
        raw_end = data.get("end_time")
        if not isinstance(raw_start, str) or not isinstance(raw_end, str):
            raise ValueError("schedule start_time and end_time must be strings")
        start_time = _normalize_clock_time(raw_start)
        end_time = _normalize_clock_time(raw_end)
        if start_time == end_time:
            raise ValueError("schedule start_time and end_time cannot be identical")
        raw_prof = data.get("profile")
        if not isinstance(raw_prof, str):
            raise ValueError("schedule profile must be a string")
        prof = _resolve_profile_alias(raw_prof)
        if not prof:
            raise ValueError("schedule profile cannot be empty")
        raw_id_val = data.get("id")
        raw_id = raw_id_val.strip() if isinstance(raw_id_val, str) and raw_id_val.strip() else f"sch-{os.urandom(4).hex()}"
        raw_enabled = data.get("enabled", True)
        enabled = bool(raw_enabled) if isinstance(raw_enabled, bool) else True
        return cls(
            id=raw_id,
            name=raw_name,
            days=days,
            start_time=start_time,
            end_time=end_time,
            profile=prof,
            enabled=enabled,
        )


@dataclasses.dataclass
class HooksConfig:
    on_start: str = ""
    on_end: str = ""
    on_break: str = ""

    def to_dict(self) -> dict[str, object]:
        out: dict[str, object] = {
            "on_start": (self.on_start or "").strip(),
            "on_end": (self.on_end or "").strip(),
        }
        clean_break = (self.on_break or "").strip()
        if clean_break:
            out["on_break"] = clean_break
        return out

    @classmethod
    def from_dict(cls, data: object) -> "HooksConfig":
        if not isinstance(data, dict):
            return cls()
        raw_start = data.get("on_start")
        raw_end = data.get("on_end")
        raw_break = data.get("on_break")
        return cls(
            on_start=str(raw_start).strip() if isinstance(raw_start, str) else "",
            on_end=str(raw_end).strip() if isinstance(raw_end, str) else "",
            on_break=str(raw_break).strip() if isinstance(raw_break, str) else "",
        )


def _sanitize_schedule_rules(raw_rules: object) -> list[ScheduleRule]:
    if not isinstance(raw_rules, (list, tuple)):
        return []
    out: list[ScheduleRule] = []
    seen_ids: set[str] = set()
    for item in raw_rules:
        if isinstance(item, ScheduleRule):
            rule_data: dict[str, object] = item.to_dict()
        elif isinstance(item, dict):
            rule_data = item
        else:
            continue
        try:
            rule = ScheduleRule.from_dict(rule_data)
        except ValueError:
            continue
        if rule.id in seen_ids:
            rule.id = f"sch-{os.urandom(4).hex()}"
        seen_ids.add(rule.id)
        out.append(rule)
    return out


def normalize_daily_goal_minutes(value: int | float | str | None) -> int:
    """Default omitted or <= 0 daily goal values to 120 and clamp to 5..1440."""
    if value is None or isinstance(value, bool):
        return DEFAULT_DAILY_GOAL_MINUTES
    try:
        minutes = int(value)
    except (TypeError, ValueError):
        return DEFAULT_DAILY_GOAL_MINUTES
    if minutes <= 0:
        return DEFAULT_DAILY_GOAL_MINUTES
    return max(MIN_DAILY_GOAL_MINUTES, min(MAX_DAILY_GOAL_MINUTES, minutes))


def is_builtin_profile(name: str) -> bool:
    return (name or "").strip().lower() in BUILTIN_PROFILE_NAMES


def validate_profile_slug(name: str) -> str:
    slug = (name or "").strip()
    if not PROFILE_SLUG_RE.match(slug):
        raise ValueError(f"invalid profile slug {name!r} (must match ^[a-z0-9][a-z0-9_-]{{0,31}}$)")
    return slug


def validate_and_sanitize_domains(raw_domains: list[str] | tuple[str, ...]) -> list[str]:
    cleaned: list[str] = []
    for raw in raw_domains:
        trimmed = (raw or "").strip()
        if not trimmed:
            continue
        cleaned.append(sanitize_domain(trimmed))
    return sanitize_domains(cleaned)


def validate_and_sanitize_processes(raw_procs: list[str] | tuple[str, ...]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for raw in raw_procs:
        trimmed = (raw or "").strip()
        if not trimmed:
            continue
        norm = normalize_process_name(trimmed)
        if norm not in seen:
            seen.add(norm)
            out.append(norm)
    return out


@dataclasses.dataclass
class FocusProfile:
    name: str
    description: str
    blocked_domains: list[str]
    blocked_processes: list[str]
    default_work_minutes: int
    default_break_minutes: int

    def to_dict(self) -> dict[str, object]:
        return dataclasses.asdict(self)

    @classmethod
    def from_dict(cls, key: str, data: dict[str, object]) -> "FocusProfile":
        raw_domains = data.get("blocked_domains")
        raw_procs = data.get("blocked_processes")
        domains_list = [x for x in raw_domains if isinstance(x, str)] if isinstance(raw_domains, (list, tuple)) else []
        procs_list = [x for x in raw_procs if isinstance(x, str)] if isinstance(raw_procs, (list, tuple)) else []

        raw_work = data.get("default_work_minutes", 25)
        try:
            work_min = int(raw_work) if raw_work is not None and not isinstance(raw_work, bool) else 25
        except (TypeError, ValueError):
            work_min = 25
        if work_min < 1:
            work_min = 25
        elif work_min > 720:
            work_min = 720

        raw_break = data.get("default_break_minutes", 5)
        try:
            break_min = int(raw_break) if raw_break is not None and not isinstance(raw_break, bool) else 5
        except (TypeError, ValueError):
            break_min = 5
        if break_min < 0:
            break_min = 5
        elif break_min > 120:
            break_min = 120

        raw_name = data.get("name")
        clean_name = raw_name.strip().lower() if isinstance(raw_name, str) else ""
        raw_desc = data.get("description")
        clean_desc = raw_desc if isinstance(raw_desc, str) else ""

        return cls(
            name=clean_name or key,
            description=clean_desc,
            blocked_domains=sanitize_domains(domains_list),
            blocked_processes=_sanitize_process_list(procs_list),
            default_work_minutes=work_min,
            default_break_minutes=break_min,
        )


def _resolve_profile_alias(name: str) -> str:
    lower = (name or "").strip().lower()
    if lower == "coding":
        return "dev"
    if lower == "lockdown":
        return "hardcore"
    return lower


@dataclasses.dataclass
class ConfigStore:
    custom_domains: list[str]
    custom_processes: list[str]
    profiles: dict[str, FocusProfile]
    daily_goal_minutes: int = DEFAULT_DAILY_GOAL_MINUTES
    notifications_enabled: bool = True
    schedules: list[ScheduleRule] = dataclasses.field(default_factory=list)
    hooks: HooksConfig = dataclasses.field(default_factory=HooksConfig)

    def to_dict(self) -> dict[str, object]:
        return {
            "custom_domains": sanitize_domains(self.custom_domains),
            "custom_processes": _sanitize_process_list(self.custom_processes),
            "daily_goal_minutes": normalize_daily_goal_minutes(self.daily_goal_minutes),
            "notifications_enabled": bool(self.notifications_enabled),
            "schedules": [r.to_dict() for r in _sanitize_schedule_rules(self.schedules)],
            "hooks": self.hooks.to_dict() if isinstance(self.hooks, HooksConfig) else HooksConfig.from_dict(self.hooks).to_dict(),
            "profiles": {k: v.to_dict() for k, v in self.profiles.items()},
        }

    def sorted_profile_names(self) -> list[str]:
        builtins = [b for b in ("study", "dev", "hardcore") if b in self.profiles]
        custom = sorted(k for k in self.profiles if not is_builtin_profile(k))
        return [*builtins, *custom]

    def upsert_custom_profile(
        self,
        name: str,
        *,
        work_minutes: int | None = None,
        break_minutes: int | None = None,
        sites: list[str] | None = None,
        apps: list[str] | None = None,
        description: str | None = None,
        from_profile: str = "",
    ) -> tuple[FocusProfile, bool]:
        slug = validate_profile_slug(name)
        if is_builtin_profile(slug):
            raise ValueError(f"built-in profile {slug!r} cannot be overwritten")

        if not self.profiles:
            self.profiles = default_profiles()

        existing = self.profiles.get(slug)
        existed = existing is not None

        resolved_work = 25
        resolved_break = 5
        resolved_desc = "Custom focus profile"
        resolved_domains: list[str] = []
        resolved_procs: list[str] = []

        from_key = _resolve_profile_alias(from_profile)
        if from_key:
            src = self.profiles.get(from_key)
            if src is None:
                raise KeyError(f"source profile {from_profile!r} not found")
            resolved_work = src.default_work_minutes
            resolved_break = src.default_break_minutes
            resolved_desc = src.description
            resolved_domains = list(src.blocked_domains)
            resolved_procs = list(src.blocked_processes)
        elif existing is not None:
            resolved_work = existing.default_work_minutes
            resolved_break = existing.default_break_minutes
            resolved_desc = existing.description
            resolved_domains = list(existing.blocked_domains)
            resolved_procs = list(existing.blocked_processes)

        if work_minutes is not None:
            if work_minutes < 1 or work_minutes > 720:
                raise ValueError("work_minutes must be between 1 and 720")
            resolved_work = int(work_minutes)

        if break_minutes is not None:
            if break_minutes < 0 or break_minutes > 120:
                raise ValueError("break_minutes must be between 0 and 120")
            resolved_break = int(break_minutes)

        if description is not None:
            resolved_desc = description.strip() or "Custom focus profile"

        if sites is not None:
            clean_sites = validate_and_sanitize_domains(sites)
            resolved_domains = sanitize_domains([*resolved_domains, *clean_sites]) if from_key else clean_sites

        if apps is not None:
            clean_apps = validate_and_sanitize_processes(apps)
            resolved_procs = _sanitize_process_list([*resolved_procs, *clean_apps]) if from_key else clean_apps

        prof = FocusProfile(
            name=slug,
            description=resolved_desc,
            blocked_domains=resolved_domains,
            blocked_processes=resolved_procs,
            default_work_minutes=resolved_work,
            default_break_minutes=resolved_break,
        )
        self.profiles[slug] = prof
        return prof, not existed

    def delete_custom_profile(self, name: str) -> None:
        key = (name or "").strip().lower()
        if not key:
            raise KeyError("profile not found")
        if is_builtin_profile(key):
            raise ValueError(f"built-in profile {key!r} cannot be deleted")
        if key not in self.profiles:
            raise KeyError(f"profile {key!r} not found")
        del self.profiles[key]


def _sanitize_process_list(procs: list[str] | tuple[str, ...]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for raw in procs:
        try:
            norm = normalize_process_name(raw)
        except ProcessValidationError:
            continue
        if norm not in seen:
            seen.add(norm)
            out.append(norm)
    return out


def dev_profile() -> FocusProfile:
    return FocusProfile(
        name="dev",
        description="Developer Deep Work (Keeps GitHub & Docs open; blocks social media & gaming)",
        blocked_domains=[
            "twitter.com",
            "x.com",
            "instagram.com",
            "tiktok.com",
            "reddit.com",
            "facebook.com",
            "netflix.com",
            "twitch.tv",
        ],
        blocked_processes=[
            "discord.exe",
            "spotify.exe",
            "steam.exe",
            "epicgameslauncher.exe",
            "riotclientservices.exe",
        ],
        default_work_minutes=50,
        default_break_minutes=10,
    )


def study_profile() -> FocusProfile:
    return FocusProfile(
        name="study",
        description="Academic & Exam Preparation Mode (Zero entertainment)",
        blocked_domains=[
            "youtube.com",
            "instagram.com",
            "tiktok.com",
            "twitter.com",
            "x.com",
            "twitch.tv",
            "kick.com",
            "netflix.com",
            "disneyplus.com",
            "reddit.com",
        ],
        blocked_processes=[
            "steam.exe",
            "discord.exe",
            "spotify.exe",
            "telegram.exe",
            "whatsapp.exe",
            "riotclientservices.exe",
            "leagueclient.exe",
            "valorant.exe",
        ],
        default_work_minutes=25,
        default_break_minutes=5,
    )


def hardcore_profile() -> FocusProfile:
    base = study_profile()
    base.name = "hardcore"
    base.description = "Aegis Lockdown - Maximum discipline with Anti-Cheat task suppression"
    return base


def default_profiles() -> dict[str, FocusProfile]:
    return {
        "dev": dev_profile(),
        "study": study_profile(),
        "hardcore": hardcore_profile(),
    }


def save_config(cfg: ConfigStore, state_dir: str | None = None) -> None:
    resolved_dir = state_dir or get_state_dir()
    os.makedirs(resolved_dir, exist_ok=True)
    cfg_path = get_config_path(resolved_dir)

    if not cfg.profiles:
        cfg.profiles = default_profiles()
    cfg.custom_domains = sanitize_domains(cfg.custom_domains)
    cfg.custom_processes = _sanitize_process_list(cfg.custom_processes)
    cfg.daily_goal_minutes = normalize_daily_goal_minutes(cfg.daily_goal_minutes)
    cfg.notifications_enabled = bool(cfg.notifications_enabled)
    cfg.schedules = _sanitize_schedule_rules(cfg.schedules)
    if not isinstance(cfg.hooks, HooksConfig):
        cfg.hooks = HooksConfig.from_dict(cfg.hooks)

    payload = (json.dumps(cfg.to_dict(), indent=2, ensure_ascii=False) + "\n").encode("utf-8")
    fd, tmp_path = tempfile.mkstemp(prefix="config-", suffix=".tmp", dir=resolved_dir)
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(payload)
            f.flush()
            os.fsync(f.fileno())
        try:
            os.chmod(tmp_path, 0o644)
        except OSError:
            pass
        last_err: OSError | None = None
        for attempt in range(5):
            try:
                os.replace(tmp_path, cfg_path)
                return
            except OSError as exc:
                last_err = exc
                if attempt + 1 < 5:
                    time.sleep(0.025)
        if last_err is not None:
            raise last_err
    except Exception:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass
        raise


def _backup_corrupted_config(path: str, raw_bytes: bytes) -> None:
    if not raw_bytes:
        return
    bak_path = f"{path}.corrupt.bak"
    try:
        with open(bak_path, "wb") as f:
            f.write(raw_bytes)
    except OSError:
        pass


def load_config(state_dir: str | None = None) -> ConfigStore:
    resolved_dir = state_dir or get_state_dir()
    cfg_path = get_config_path(resolved_dir)

    def _init_default() -> ConfigStore:
        cfg_default = ConfigStore(
            custom_domains=[],
            custom_processes=[],
            profiles=default_profiles(),
            daily_goal_minutes=DEFAULT_DAILY_GOAL_MINUTES,
        )
        save_config(cfg_default, state_dir=resolved_dir)
        return cfg_default

    raw: dict[str, object] | None = None
    last_raw_bytes = b""
    last_read_err: OSError | None = None
    saw_empty = False
    saw_parse_error = False

    for attempt in range(3):
        try:
            with open(cfg_path, "rb") as f:
                raw_bytes = f.read()
        except FileNotFoundError:
            return _init_default()
        except OSError as exc:
            last_read_err = exc
            if attempt + 1 < 3:
                time.sleep(0.025)
            continue

        last_read_err = None
        last_raw_bytes = raw_bytes
        clean_bytes = raw_bytes[3:] if raw_bytes.startswith(b"\xef\xbb\xbf") else raw_bytes
        if not clean_bytes.strip():
            saw_empty = True
            saw_parse_error = False
            if attempt + 1 < 3:
                time.sleep(0.025)
            continue
        saw_empty = False

        try:
            parsed = json.loads(clean_bytes.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError):
            saw_parse_error = True
            if attempt + 1 < 3:
                time.sleep(0.025)
            continue

        if not isinstance(parsed, dict):
            saw_parse_error = True
            if attempt + 1 < 3:
                time.sleep(0.025)
            continue

        raw = parsed
        saw_parse_error = False
        break

    if last_read_err is not None:
        raise last_read_err
    if saw_empty:
        return _init_default()
    if saw_parse_error or raw is None:
        _backup_corrupted_config(cfg_path, last_raw_bytes)
        return _init_default()

    cd = raw.get("custom_domains")
    cs = raw.get("custom_sites")
    cp = raw.get("custom_processes")
    ca = raw.get("custom_apps")
    raw_domains = (list(cd) if isinstance(cd, (list, tuple)) else []) + (
        list(cs) if isinstance(cs, (list, tuple)) else []
    )
    raw_procs = (list(cp) if isinstance(cp, (list, tuple)) else []) + (
        list(ca) if isinstance(ca, (list, tuple)) else []
    )
    daily_goal = normalize_daily_goal_minutes(raw.get("daily_goal_minutes"))
    raw_notify = raw.get("notifications_enabled")
    notifications_enabled = raw_notify if isinstance(raw_notify, bool) else True
    schedules = _sanitize_schedule_rules(raw.get("schedules"))
    hooks = HooksConfig.from_dict(raw.get("hooks"))

    profiles_map: dict[str, FocusProfile] = {}
    raw_profiles = raw.get("profiles")
    if isinstance(raw_profiles, dict):
        for key, val in raw_profiles.items():
            clean_key = str(key).strip().lower()
            if clean_key and isinstance(val, dict):
                profiles_map[clean_key] = FocusProfile.from_dict(clean_key, val)

    defaults = default_profiles()
    for key, default_prof in defaults.items():
        if key not in profiles_map:
            profiles_map[key] = default_prof

    return ConfigStore(
        custom_domains=sanitize_domains([x for x in raw_domains if isinstance(x, str)]),
        custom_processes=_sanitize_process_list([x for x in raw_procs if isinstance(x, str)]),
        profiles=profiles_map,
        daily_goal_minutes=daily_goal,
        notifications_enabled=notifications_enabled,
        schedules=schedules,
        hooks=hooks,
    )


def load_or_default(name: str, state_dir: str | None = None) -> FocusProfile:
    lower = (name or "study").strip().lower()
    if lower == "coding":
        lower = "dev"
    elif lower == "lockdown":
        lower = "hardcore"

    try:
        cfg = load_config(state_dir=state_dir)
    except Exception:
        cfg = None

    if cfg is not None and lower in cfg.profiles:
        prof = cfg.profiles[lower]
        return FocusProfile(
            name=prof.name,
            description=prof.description,
            blocked_domains=sanitize_domains([*prof.blocked_domains, *cfg.custom_domains]),
            blocked_processes=_sanitize_process_list([*prof.blocked_processes, *cfg.custom_processes]),
            default_work_minutes=prof.default_work_minutes,
            default_break_minutes=prof.default_break_minutes,
        )

    if lower == "dev":
        fallback = dev_profile()
    elif lower == "hardcore":
        fallback = hardcore_profile()
    else:
        fallback = study_profile()

    if cfg is not None:
        fallback.blocked_domains = sanitize_domains([*fallback.blocked_domains, *cfg.custom_domains])
        fallback.blocked_processes = _sanitize_process_list([*fallback.blocked_processes, *cfg.custom_processes])
    return fallback
