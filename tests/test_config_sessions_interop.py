"""Cross-runtime schema & state-directory interoperability tests between Go CLI and Python."""

from __future__ import annotations

import datetime
import json
import os
from pathlib import Path
import shutil
import subprocess

from focusflow_core.config import (
    ConfigStore,
    default_profiles,
    load_config,
    load_or_default,
    normalize_daily_goal_minutes,
    save_config,
    validate_profile_slug,
)
from focusflow_core.hosts import HostsManager, SessionMeta
from focusflow_core.paths import get_config_path, get_state_dir
from focusflow_core.sanitize import DomainValidationError, ProcessValidationError
from focusflow_core.storage import SessionStore


def _find_go_binary() -> str | None:
    found = shutil.which("go")
    if found:
        return found
    win_default = Path(r"C:\Program Files\Go\bin\go.exe")
    if win_default.exists():
        return str(win_default)
    return None


def test_config_migration_and_schema_roundtrip(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, _ = hermetic_focusflow_env
    assert Path(get_state_dir()) == state_dir

    # Write legacy keys and protected binaries; verify clean migration to custom_domains / custom_processes
    cfg_path = Path(get_config_path())
    cfg_path.write_text(
        json.dumps(
            {
                "custom_sites": ["https://www.DistractSite.com/news", "localhost", "127.0.0.1"],
                "custom_apps": ["EpicGamesLauncher", "explorer.exe", "pythonw.exe"],
            }
        ),
        encoding="utf-8",
    )

    cfg = load_config()
    assert cfg.custom_domains == ["distractsite.com"]
    assert cfg.custom_processes == ["epicgameslauncher.exe"]
    assert cfg.daily_goal_minutes == 120
    save_config(cfg)

    saved_raw = json.loads(cfg_path.read_text(encoding="utf-8"))
    assert set(saved_raw.keys()) == {"custom_domains", "custom_processes", "daily_goal_minutes", "profiles"}
    assert saved_raw["daily_goal_minutes"] == 120
    assert set(saved_raw["profiles"].keys()) >= {"study", "dev", "hardcore"}

    # Verify <= 0 defaults to 120 and out-of-range values clamp to 5..1440
    assert normalize_daily_goal_minutes(None) == 120
    assert normalize_daily_goal_minutes(0) == 120
    assert normalize_daily_goal_minutes(-15) == 120
    assert normalize_daily_goal_minutes(2) == 5
    assert normalize_daily_goal_minutes(9999) == 1440
    assert normalize_daily_goal_minutes(180) == 180

    dev_merged = load_or_default("dev")
    assert "distractsite.com" in dev_merged.blocked_domains
    assert "epicgameslauncher.exe" in dev_merged.blocked_processes


def test_python_custom_profiles_validation_and_crud(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    import pytest

    state_dir, _ = hermetic_focusflow_env
    cfg = load_config(state_dir=str(state_dir))

    for bad_slug in ("", "-bad", "_bad", "UpperCase", "has space", "a" * 33):
        with pytest.raises(ValueError):
            validate_profile_slug(bad_slug)

    for builtin in ("study", "dev", "hardcore"):
        with pytest.raises(ValueError):
            cfg.upsert_custom_profile(builtin, work_minutes=30)
        with pytest.raises(ValueError):
            cfg.delete_custom_profile(builtin)

    with pytest.raises(ProcessValidationError):
        cfg.upsert_custom_profile("unsafe", apps=["explorer.exe"])
    with pytest.raises(DomainValidationError):
        cfg.upsert_custom_profile("badhost", sites=["localhost"])

    prof, created = cfg.upsert_custom_profile(
        "writer-flow",
        from_profile="study",
        work_minutes=45,
        break_minutes=10,
        sites=["https://www.Medium.com/topic", "substack.com"],
        apps=["Slack"],
        description="Deep writing mode",
    )
    assert created is True
    assert prof.default_work_minutes == 45
    assert prof.default_break_minutes == 10
    assert "medium.com" in prof.blocked_domains
    assert "youtube.com" in prof.blocked_domains
    assert "slack.exe" in prof.blocked_processes
    save_config(cfg, state_dir=str(state_dir))

    loaded = load_or_default("writer-flow", state_dir=str(state_dir))
    assert loaded.name == "writer-flow"
    assert loaded.default_work_minutes == 45

    cfg.delete_custom_profile("writer-flow")
    save_config(cfg, state_dir=str(state_dir))
    reloaded = load_config(state_dir=str(state_dir))
    assert "writer-flow" not in reloaded.profiles


def test_go_and_python_full_state_interoperability(hermetic_focusflow_env: tuple[Path, Path]) -> None:
    state_dir, hosts_path = hermetic_focusflow_env
    go_bin = _find_go_binary()

    # Create config & session records from Python
    cfg = ConfigStore(
        custom_domains=["news.ycombinator.com"],
        custom_processes=["battle.net.exe"],
        profiles=default_profiles(),
    )
    save_config(cfg, state_dir=str(state_dir))

    store = SessionStore(state_dir=str(state_dir))
    now = datetime.datetime.now(datetime.timezone.utc)
    store.start_session("interop-1", "dev", 50, start_time=now - datetime.timedelta(minutes=50))
    store.finalize_session(
        "interop-1",
        "dev",
        50,
        completed=True,
        blocked_count=3,
        start_time=now - datetime.timedelta(minutes=50),
        end_time=now,
    )

    if go_bin is None:
        return

    repo_root = Path(__file__).resolve().parent.parent
    env = os.environ.copy()
    env["FOCUSFLOW_HOME"] = str(state_dir)
    env["FOCUSFLOW_HOSTS_PATH"] = str(hosts_path)
    env["CGO_ENABLED"] = "0"
    go_dir = str(Path(go_bin).parent)
    env["PATH"] = f"{go_dir}{os.pathsep}{env.get('PATH', '')}"

    # 1. Verify Go CLI reads Python-written config.json seamlessly
    res_list = subprocess.run(
        [go_bin, "run", "./cmd/focusflow", "list", "--json"],
        cwd=str(repo_root),
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    go_list = json.loads(res_list.stdout)
    assert go_list["custom_domains"] == ["news.ycombinator.com"]
    assert go_list["custom_processes"] == ["battle.net.exe"]

    # 2. Verify Go CLI computes identical stats from Python-written sessions.json
    res_stats = subprocess.run(
        [go_bin, "run", "./cmd/focusflow", "stats", "--days", "7", "--json"],
        cwd=str(repo_root),
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    go_stats = json.loads(res_stats.stdout)
    py_stats = store.compute_analytics(days=7)
    assert go_stats["completed_sprints"] == py_stats.completed_sprints == 1
    assert go_stats["total_minutes"] == py_stats.total_minutes == 50
    assert go_stats["active_streak_days"] == py_stats.active_streak_days == 1

    # 3. Simulate Python orphan crash and verify Go `doctor --json` auto-recovers hosts & sessions.json!
    store.start_session("py-crash-404", "hardcore", 25, start_time=now - datetime.timedelta(minutes=5))
    hm = HostsManager(hosts_path=str(hosts_path), state_dir=str(state_dir))
    hm.apply_blocklist(
        ["tiktok.com"],
        meta=SessionMeta(
            session_id="py-crash-404",
            pid=99999999,
            started_at=now - datetime.timedelta(minutes=5),
            expected_end_at=now + datetime.timedelta(minutes=20),
            profile="hardcore",
        ),
    )
    assert hm.is_active()

    res_doc = subprocess.run(
        [go_bin, "run", "./cmd/focusflow", "doctor", "--json"],
        cwd=str(repo_root),
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    doc_json = json.loads(res_doc.stdout)
    assert doc_json["recovery"]["recovered"] is True
    assert not hm.is_active()

    sessions = {s.id: s for s in store.load_sessions()}
    assert sessions["py-crash-404"].completed is False
    assert sessions["py-crash-404"].status == "orphaned"

    # 4. Bidirectional Go <-> Python custom profiles & daily_goal_minutes interoperability
    subprocess.run(
        [
            go_bin,
            "run",
            "./cmd/focusflow",
            "profile",
            "create",
            "go-architect",
            "--from",
            "dev",
            "--minutes",
            "60",
            "--break",
            "15",
            "--sites",
            "https://www.HackerNews.com/item",
            "--apps",
            "slack",
            "--desc",
            "System design sprint",
        ],
        cwd=str(repo_root),
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    subprocess.run(
        [go_bin, "run", "./cmd/focusflow", "goal", "--minutes", "200"],
        cwd=str(repo_root),
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )

    py_cfg = load_config(state_dir=str(state_dir))
    assert py_cfg.daily_goal_minutes == 200
    assert "go-architect" in py_cfg.profiles
    go_arch = py_cfg.profiles["go-architect"]
    assert go_arch.default_work_minutes == 60
    assert go_arch.default_break_minutes == 15
    assert "hackernews.com" in go_arch.blocked_domains
    assert "slack.exe" in go_arch.blocked_processes

    # Create a custom profile in Python and read it via Go `profile show --json` + compare goal progress!
    py_cfg.upsert_custom_profile(
        "py-research",
        work_minutes=40,
        break_minutes=8,
        sites=["arxiv-sanity.com"],
        apps=["telegram.exe"],
        description="Paper reading block",
    )
    save_config(py_cfg, state_dir=str(state_dir))

    res_show = subprocess.run(
        [go_bin, "run", "./cmd/focusflow", "profile", "show", "py-research", "--json"],
        cwd=str(repo_root),
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    shown_go = json.loads(res_show.stdout)
    assert shown_go["name"] == "py-research"
    assert shown_go["builtin"] is False
    assert shown_go["default_work_minutes"] == 40
    assert shown_go["default_break_minutes"] == 8
    assert shown_go["blocked_domains"] == ["arxiv-sanity.com"]
    assert shown_go["blocked_processes"] == ["telegram.exe"]

    res_status = subprocess.run(
        [go_bin, "run", "./cmd/focusflow", "status", "--json"],
        cwd=str(repo_root),
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    status_go = json.loads(res_status.stdout)
    py_goal = store.compute_daily_goal_progress()
    assert status_go["daily_goal_minutes"] == py_goal.daily_goal_minutes == 200
    assert status_go["today_focus_minutes"] == py_goal.today_focus_minutes == 50
    assert status_go["daily_goal_progress_pct"] == py_goal.daily_goal_progress_pct == 25.0
