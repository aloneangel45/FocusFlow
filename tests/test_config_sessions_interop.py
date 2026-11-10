"""Cross-runtime schema & state-directory interoperability tests between Go CLI and Python."""

from __future__ import annotations

import datetime
import json
import os
from pathlib import Path
import shutil
import subprocess

from focusflow_core.config import ConfigStore, default_profiles, load_config, load_or_default, save_config
from focusflow_core.hosts import HostsManager, SessionMeta
from focusflow_core.paths import get_config_path, get_state_dir
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
    save_config(cfg)

    saved_raw = json.loads(cfg_path.read_text(encoding="utf-8"))
    assert set(saved_raw.keys()) == {"custom_domains", "custom_processes", "profiles"}
    assert set(saved_raw["profiles"].keys()) >= {"study", "dev", "hardcore"}

    dev_merged = load_or_default("dev")
    assert "distractsite.com" in dev_merged.blocked_domains
    assert "epicgameslauncher.exe" in dev_merged.blocked_processes


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
