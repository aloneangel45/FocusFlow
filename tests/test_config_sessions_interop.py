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
    today_start = datetime.datetime.now().astimezone().replace(hour=0, minute=0, second=0, microsecond=0)
    store.start_session("interop-1", "dev", 50, start_time=today_start)
    store.finalize_session(
        "interop-1",
        "dev",
        50,
        completed=True,
        blocked_count=3,
        start_time=today_start,
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


def test_utf8_bom_zero_byte_corrupt_backup_and_abort_active_sessions(
    hermetic_focusflow_env: tuple[Path, Path],
) -> None:
    state_dir, _ = hermetic_focusflow_env
    cfg_path = Path(get_config_path(str(state_dir)))

    # 1. UTF-8 BOM in config.json + null fields in profile
    bom_cfg = b'\xef\xbb\xbf{"daily_goal_minutes": 165, "custom_domains": ["https://reddit.com:443/r/python"], "profiles": {"custom1": {"name": null, "description": null, "default_work_minutes": null, "default_break_minutes": -3, "blocked_domains": ["x.com"]}}}\n'
    cfg_path.write_bytes(bom_cfg)
    cfg = load_config(state_dir=str(state_dir))
    assert cfg.daily_goal_minutes == 165
    assert cfg.custom_domains == ["reddit.com"]
    assert cfg.profiles["custom1"].default_work_minutes == 25
    assert cfg.profiles["custom1"].default_break_minutes == 5

    # 2. 0-byte config.json self-heals without .corrupt.bak
    cfg_path.write_bytes(b"")
    cfg_empty = load_config(state_dir=str(state_dir))
    assert cfg_empty.daily_goal_minutes == 120
    assert not Path(f"{cfg_path}.corrupt.bak").exists()

    # 3. Corrupted config.json backs up to config.json.corrupt.bak and self-heals
    corrupt_cfg = b'{"daily_goal_minutes": 90, "profiles": [BROKEN'
    cfg_path.write_bytes(corrupt_cfg)
    cfg_healed = load_config(state_dir=str(state_dir))
    assert cfg_healed.daily_goal_minutes == 120
    assert Path(f"{cfg_path}.corrupt.bak").read_bytes() == corrupt_cfg

    # 4. UTF-8 BOM + null fields in sessions.json preserves history
    store = SessionStore(state_dir=str(state_dir))
    sess_path = Path(store.path)
    bom_sess = b'\xef\xbb\xbf{"sessions": [{"id": "s-bom", "profile": null, "start_time": "2026-06-15T10:00:00Z", "duration_minutes": null, "completed": true, "blocked_count": null}]}\n'
    sess_path.write_bytes(bom_sess)
    loaded = store.load_sessions()
    assert len(loaded) == 1
    assert loaded[0].id == "s-bom"
    assert loaded[0].profile == "study"
    assert loaded[0].duration_minutes == 0
    assert loaded[0].blocked_count == 0

    t0 = datetime.datetime(2026, 6, 15, 12, 0, 0, tzinfo=datetime.timezone.utc)
    store.start_session("s-run", "dev", 50, start_time=t0)
    assert len(store.load_sessions()) == 2
    assert store.is_session_running("s-run") is True

    # 5. abort_active_sessions finalizes running record and blocks late completed overwrite
    aborted = store.abort_active_sessions(now=t0 + datetime.timedelta(minutes=14, seconds=20))
    assert aborted == 1
    assert store.is_session_running("s-run") is False
    store.finalize_session("s-run", "dev", 50, completed=True, status="completed", start_time=t0, end_time=t0 + datetime.timedelta(minutes=50))
    by_id = {r.id: r for r in store.load_sessions()}
    assert by_id["s-run"].completed is False
    assert by_id["s-run"].status == "aborted"
    assert by_id["s-run"].duration_minutes == 14

    # 6. Corrupted sessions.json backs up to sessions.json.corrupt.bak and self-heals
    corrupt_sess = b'{"sessions": [{"id": "truncated"'
    sess_path.write_bytes(corrupt_sess)
    assert store.load_sessions() == []
    assert Path(f"{sess_path}.corrupt.bak").read_bytes() == corrupt_sess
