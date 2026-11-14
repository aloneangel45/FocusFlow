"""Unified profile and blocklist configuration shared identically with Go `pkg/profile`."""

from __future__ import annotations

import dataclasses
import json
import os
import re
import tempfile

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
        raw_domains = data.get("blocked_domains") or []
        raw_procs = data.get("blocked_processes") or []
        work_min = int(data.get("default_work_minutes", 25))
        break_min = int(data.get("default_break_minutes", 5))
        return cls(
            name=str(data.get("name") or key).strip().lower() or key,
            description=str(data.get("description") or ""),
            blocked_domains=sanitize_domains([str(x) for x in raw_domains]),
            blocked_processes=_sanitize_process_list([str(x) for x in raw_procs]),
            default_work_minutes=work_min if work_min > 0 else 25,
            default_break_minutes=break_min if break_min >= 0 else 5,
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

    def to_dict(self) -> dict[str, object]:
        return {
            "custom_domains": sanitize_domains(self.custom_domains),
            "custom_processes": _sanitize_process_list(self.custom_processes),
            "daily_goal_minutes": normalize_daily_goal_minutes(self.daily_goal_minutes),
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
        os.replace(tmp_path, cfg_path)
    except Exception:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass
        raise


def load_config(state_dir: str | None = None) -> ConfigStore:
    resolved_dir = state_dir or get_state_dir()
    cfg_path = get_config_path(resolved_dir)

    if not os.path.exists(cfg_path):
        cfg = ConfigStore(
            custom_domains=[],
            custom_processes=[],
            profiles=default_profiles(),
            daily_goal_minutes=DEFAULT_DAILY_GOAL_MINUTES,
        )
        save_config(cfg, state_dir=resolved_dir)
        return cfg

    with open(cfg_path, "r", encoding="utf-8") as f:
        raw = json.load(f)

    if not isinstance(raw, dict):
        raise ValueError("invalid config.json root object")

    raw_domains = list(raw.get("custom_domains") or []) + list(raw.get("custom_sites") or [])
    raw_procs = list(raw.get("custom_processes") or []) + list(raw.get("custom_apps") or [])
    daily_goal = normalize_daily_goal_minutes(raw.get("daily_goal_minutes"))

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
        custom_domains=sanitize_domains([str(x) for x in raw_domains]),
        custom_processes=_sanitize_process_list([str(x) for x in raw_procs]),
        profiles=profiles_map,
        daily_goal_minutes=daily_goal,
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
