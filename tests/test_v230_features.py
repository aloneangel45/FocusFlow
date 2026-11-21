"""v2.3.0 Go <-> Python schema round-trip, schedules/hooks/notifications preservation, and heatmap tests."""

from __future__ import annotations

import datetime
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import FocusFlowGUI
import focusflow_core
from focusflow_core.config import (
    HooksConfig,
    ScheduleRule,
    load_config,
    parse_schedule_days,
    save_config,
)
from focusflow_core.paths import get_config_path
from focusflow_core.storage import SessionStore


def _find_go_binary() -> str | None:
    found = shutil.which("go")
    if found:
        return found
    win_default = Path(r"C:\Program Files\Go\bin\go.exe")
    if win_default.exists():
        return str(win_default)
    return None


class FocusFlowV230Tests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory(prefix="focusflow-v230-")
        self.base_dir = Path(self._tmp.name)
        self.state_dir = self.base_dir / "state"
        self.state_dir.mkdir(parents=True, exist_ok=True)
        self.hosts_path = self.base_dir / "hosts"
        self.hosts_path.write_text("127.0.0.1 localhost\n::1 localhost\n", encoding="utf-8")

        self._prev_env = {
            k: os.environ.get(k)
            for k in ("FOCUSFLOW_HOME", "FOCUSFLOW_HOSTS_PATH", "FOCUSFLOW_NO_NOTIFY")
        }
        os.environ["FOCUSFLOW_HOME"] = str(self.state_dir)
        os.environ["FOCUSFLOW_HOSTS_PATH"] = str(self.hosts_path)
        os.environ["FOCUSFLOW_NO_NOTIFY"] = "1"

    def tearDown(self) -> None:
        for k, v in self._prev_env.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v
        self._tmp.cleanup()

    def test_python_config_preserves_schedules_hooks_and_notifications(self) -> None:
        cfg_path = Path(get_config_path(str(self.state_dir)))
        initial_payload = {
            "custom_domains": ["reddit.com"],
            "custom_processes": ["discord.exe"],
            "daily_goal_minutes": 150,
            "notifications_enabled": False,
            "schedules": [
                {
                    "id": "sch-morning",
                    "name": "Weekday Deep Work",
                    "days": ["mon", "tue", "wed", "thu", "fri"],
                    "start_time": "09:00",
                    "end_time": "12:00",
                    "profile": "dev",
                    "enabled": True,
                },
                {
                    "id": "sch-night",
                    "name": "Overnight Lockdown",
                    "days": ["sun"],
                    "start_time": "22:30",
                    "end_time": "01:30",
                    "profile": "hardcore",
                    "enabled": False,
                },
            ],
            "hooks": {
                "on_start": "echo starting-focus",
                "on_end": "echo ending-focus",
                "on_break": "echo break-time",
            },
            "profiles": {},
        }
        cfg_path.write_text(json.dumps(initial_payload, indent=2), encoding="utf-8")

        # Simulate Python HUD loading config, updating daily_goal_minutes, and saving back
        cfg = load_config(state_dir=str(self.state_dir))
        self.assertFalse(cfg.notifications_enabled)
        self.assertEqual(len(cfg.schedules), 2)
        self.assertEqual(cfg.schedules[0].id, "sch-morning")
        self.assertEqual(cfg.schedules[1].end_time, "01:30")
        self.assertFalse(cfg.schedules[1].enabled)
        self.assertEqual(cfg.hooks.on_start, "echo starting-focus")
        self.assertEqual(cfg.hooks.on_end, "echo ending-focus")
        self.assertEqual(cfg.hooks.on_break, "echo break-time")

        cfg.daily_goal_minutes = 180
        save_config(cfg, state_dir=str(self.state_dir))

        reloaded = load_config(state_dir=str(self.state_dir))
        self.assertEqual(reloaded.daily_goal_minutes, 180)
        self.assertFalse(reloaded.notifications_enabled)
        self.assertEqual(len(reloaded.schedules), 2)
        self.assertEqual(reloaded.schedules[0].name, "Weekday Deep Work")
        self.assertEqual(reloaded.hooks.on_start, "echo starting-focus")
        self.assertEqual(reloaded.hooks.on_end, "echo ending-focus")
        self.assertEqual(reloaded.hooks.on_break, "echo break-time")

        # Verify schedule days parser handles wrap-around and aliases identically to Go
        self.assertEqual(parse_schedule_days("mon-fri"), ["mon", "tue", "wed", "thu", "fri"])
        self.assertEqual(parse_schedule_days("fri-mon"), ["mon", "fri", "sat", "sun"])
        self.assertEqual(parse_schedule_days("weekends"), ["sat", "sun"])

    def test_go_and_python_v230_config_roundtrip_and_heatmap_alignment(self) -> None:
        go_bin = _find_go_binary()
        if go_bin is None:
            self.skipTest("Go toolchain not available")

        repo_root = Path(__file__).resolve().parent.parent
        env = os.environ.copy()
        env["FOCUSFLOW_HOME"] = str(self.state_dir)
        env["FOCUSFLOW_HOSTS_PATH"] = str(self.hosts_path)
        env["FOCUSFLOW_NO_NOTIFY"] = "1"
        env["CGO_ENABLED"] = "0"
        go_dir = str(Path(go_bin).parent)
        env["PATH"] = f"{go_dir}{os.pathsep}{env.get('PATH', '')}"

        # 1. Create a schedule via Go CLI
        subprocess.run(
            [
                go_bin, "run", "./cmd/focusflow",
                "schedule", "add",
                "--name", "Morning Engineering",
                "--days", "mon-fri",
                "--from", "09:00",
                "--to", "12:00",
                "--profile", "dev",
            ],
            cwd=str(repo_root),
            env=env,
            capture_output=True,
            text=True,
            check=True,
        )

        # 2. Load in Python, attach hooks + toggle notifications_enabled=False + change daily goal, save back!
        py_cfg = load_config(state_dir=str(self.state_dir))
        self.assertEqual(len(py_cfg.schedules), 1)
        self.assertEqual(py_cfg.schedules[0].name, "Morning Engineering")
        py_cfg.notifications_enabled = False
        py_cfg.daily_goal_minutes = 160
        py_cfg.hooks = HooksConfig(on_start="echo go-py-start", on_end="echo go-py-end")
        py_cfg.schedules.append(
            ScheduleRule(
                id="sch-py-night",
                name="Night Study",
                days=["sat", "sun"],
                start_time="23:00",
                end_time="01:00",
                profile="study",
                enabled=True,
            )
        )
        save_config(py_cfg, state_dir=str(self.state_dir))

        # 3. Verify Go CLI reads BOTH schedules, hooks, and notifications_enabled=false without any loss!
        res_list = subprocess.run(
            [go_bin, "run", "./cmd/focusflow", "list", "--json"],
            cwd=str(repo_root),
            env=env,
            capture_output=True,
            text=True,
            check=True,
        )
        go_cfg = json.loads(res_list.stdout)
        self.assertFalse(go_cfg["notifications_enabled"])
        self.assertEqual(go_cfg["daily_goal_minutes"], 160)
        self.assertEqual(go_cfg["hooks"]["on_start"], "echo go-py-start")
        self.assertEqual(go_cfg["hooks"]["on_end"], "echo go-py-end")
        sched_names = [s["name"] for s in go_cfg["schedules"]]
        self.assertEqual(sched_names, ["Morning Engineering", "Night Study"])

        # 4. Verify Go CLI and Python compute identical 90-day analytics & longest_streak_days
        store = SessionStore(state_dir=str(self.state_dir))
        local_today = datetime.datetime.now().astimezone().replace(hour=10, minute=0, second=0, microsecond=0)
        for offset in (0, 1, 10, 11, 12, 13):
            day_dt = local_today - datetime.timedelta(days=offset)
            store.finalize_session(
                f"hm-{offset}",
                "dev",
                80,
                completed=True,
                blocked_count=1,
                start_time=day_dt,
                end_time=day_dt + datetime.timedelta(minutes=80),
            )

        py_summary = store.compute_analytics(days=90)
        self.assertEqual(len(py_summary.daily), 90)
        self.assertEqual(py_summary.active_streak_days, 2)
        self.assertEqual(py_summary.longest_streak_days, 4)

        res_stats = subprocess.run(
            [go_bin, "run", "./cmd/focusflow", "stats", "--heatmap", "--json"],
            cwd=str(repo_root),
            env=env,
            capture_output=True,
            text=True,
            check=True,
        )
        go_stats = json.loads(res_stats.stdout)
        self.assertEqual(len(go_stats["daily"]), 90)
        self.assertEqual(go_stats["active_streak_days"], py_summary.active_streak_days)
        self.assertEqual(go_stats["longest_streak_days"], py_summary.longest_streak_days)

    def test_gui_heatmap_intensity_and_version_sync(self) -> None:
        self.assertEqual(focusflow_core.__version__, "2.3.0")
        self.assertEqual(FocusFlowGUI.compute_heatmap_intensity(0, 120), 0)
        self.assertEqual(FocusFlowGUI.compute_heatmap_intensity(25, 120), 1)
        self.assertEqual(FocusFlowGUI.compute_heatmap_intensity(60, 120), 2)
        self.assertEqual(FocusFlowGUI.compute_heatmap_intensity(95, 120), 3)
        self.assertEqual(FocusFlowGUI.compute_heatmap_intensity(120, 120), 4)
        self.assertEqual(FocusFlowGUI.compute_heatmap_intensity(240, 120), 4)


if __name__ == "__main__":
    unittest.main()
