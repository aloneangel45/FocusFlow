"""Unified profile and blocklist configuration shared identically with Go `pkg/profile`."""

from __future__ import annotations

import dataclasses
import json
import os
import tempfile

from .paths import get_config_path, get_state_dir
from .sanitize import ProcessValidationError, normalize_process_name, sanitize_domains


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
        return cls(
            name=str(data.get("name") or key),
            description=str(data.get("description") or ""),
            blocked_domains=sanitize_domains([str(x) for x in raw_domains]),
            blocked_processes=_sanitize_process_list([str(x) for x in raw_procs]),
            default_work_minutes=int(data.get("default_work_minutes", 25)),
            default_break_minutes=int(data.get("default_break_minutes", 5)),
        )


@dataclasses.dataclass
class ConfigStore:
    custom_domains: list[str]
    custom_processes: list[str]
    profiles: dict[str, FocusProfile]

    def to_dict(self) -> dict[str, object]:
        return {
            "custom_domains": sanitize_domains(self.custom_domains),
            "custom_processes": _sanitize_process_list(self.custom_processes),
            "profiles": {k: v.to_dict() for k, v in self.profiles.items()},
        }


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
        )
        save_config(cfg, state_dir=resolved_dir)
        return cfg

    with open(cfg_path, "r", encoding="utf-8") as f:
        raw = json.load(f)

    if not isinstance(raw, dict):
        raise ValueError("invalid config.json root object")

    raw_domains = list(raw.get("custom_domains") or []) + list(raw.get("custom_sites") or [])
    raw_procs = list(raw.get("custom_processes") or []) + list(raw.get("custom_apps") or [])

    profiles_map: dict[str, FocusProfile] = {}
    raw_profiles = raw.get("profiles")
    if isinstance(raw_profiles, dict):
        for key, val in raw_profiles.items():
            if isinstance(val, dict):
                profiles_map[str(key).lower()] = FocusProfile.from_dict(str(key).lower(), val)

    defaults = default_profiles()
    for key, default_prof in defaults.items():
        if key not in profiles_map:
            profiles_map[key] = default_prof

    return ConfigStore(
        custom_domains=sanitize_domains([str(x) for x in raw_domains]),
        custom_processes=_sanitize_process_list([str(x) for x in raw_procs]),
        profiles=profiles_map,
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
