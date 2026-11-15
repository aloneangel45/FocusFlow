"""Unittest-compatible v2.2.2 regression suite (discoverable by both `unittest` and `pytest`)."""

from __future__ import annotations

import builtins
import datetime
import io
import json
import os
from pathlib import Path
import re
import tempfile
import unittest
from unittest import mock

import focusflow_core
from focusflow_core.config import ConfigStore, default_profiles, load_config, save_config
from focusflow_core.hosts import (
    ALIVE_PID_LEASE_GRACE,
    LEGACY_MARKER_BEGIN,
    MARKER_BEGIN,
    MARKER_END,
    HostsManager,
    SessionMeta,
    strip_managed_blocks,
)
from focusflow_core.paths import get_gui_settings_path, get_state_dir
from focusflow_core.privileges import build_posix_sudo_command, elevate_self
from focusflow_core.sanitize import (
    ProcessValidationError,
    is_protected_process,
    normalize_process_name,
)
from focusflow_core.storage import SessionStore
from focusflow_core.supervisor import ProcessSupervisor, resolve_procfs_names
import FocusFlowGUI


class FocusFlowV222RegressionTests(unittest.TestCase):
    """Comprehensive unittest suite verifying all FocusFlow v2.2.2 hardening fixes."""

    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory()
        self.base_dir = Path(self._tmp.name)
        self.state_dir = self.base_dir / "state"
        self.state_dir.mkdir(parents=True, exist_ok=True)
        self.hosts_path = self.base_dir / "hosts"
        self.hosts_path.write_text("127.0.0.1 localhost\n::1 localhost\n", encoding="utf-8")
        self._env_patch = mock.patch.dict(
            os.environ,
            {
                "FOCUSFLOW_HOME": str(self.state_dir),
                "FOCUSFLOW_HOSTS_PATH": str(self.hosts_path),
            },
        )
        self._env_patch.start()

    def tearDown(self) -> None:
        self._env_patch.stop()
        self._tmp.cleanup()

    def test_verify_or_reapply_blocks_comment_www_and_marker_bypasses(self) -> None:
        hm = HostsManager(hosts_path=str(self.hosts_path), state_dir=str(self.state_dir))
        now = datetime.datetime.now(datetime.timezone.utc)
        meta = SessionMeta(
            session_id="u-tamper-1",
            pid=os.getpid(),
            started_at=now,
            expected_end_at=now + datetime.timedelta(minutes=30),
            profile="hardcore",
        )
        domains = ["youtube.com"]
        self.assertTrue(hm.apply_blocklist(domains, meta=meta))
        self.assertFalse(hm.verify_or_reapply(domains, meta=meta))

        tamper_payloads = [
            # 1. Commented-out IPv4 line
            (
                "127.0.0.1 localhost\n"
                f"{MARKER_BEGIN}\n"
                "# 0.0.0.0 youtube.com\n"
                "0.0.0.0 www.youtube.com\n"
                ":: youtube.com\n"
                ":: www.youtube.com\n"
                f"{MARKER_END}\n"
            ),
            # 2. Deleted www.youtube.com IPv4 mapping
            (
                "127.0.0.1 localhost\n"
                f"{MARKER_BEGIN}\n"
                "0.0.0.0 youtube.com\n"
                ":: youtube.com\n"
                ":: www.youtube.com\n"
                f"{MARKER_END}\n"
            ),
            # 3. Deleted www.youtube.com IPv6 mapping
            (
                "127.0.0.1 localhost\n"
                f"{MARKER_BEGIN}\n"
                "0.0.0.0 youtube.com\n"
                "0.0.0.0 www.youtube.com\n"
                ":: youtube.com\n"
                f"{MARKER_END}\n"
            ),
            # 4. Inverted MarkerEnd before MarkerBegin
            (
                "127.0.0.1 localhost\n"
                f"{MARKER_END}\n"
                "0.0.0.0 youtube.com\n"
                "0.0.0.0 www.youtube.com\n"
                ":: youtube.com\n"
                ":: www.youtube.com\n"
                f"{MARKER_BEGIN}\n"
            ),
            # 5. Commented decoy outside empty managed block
            (
                "# 0.0.0.0 youtube.com\n"
                "# 0.0.0.0 www.youtube.com\n"
                "# :: youtube.com\n"
                "# :: www.youtube.com\n"
                f"{MARKER_BEGIN}\n{MARKER_END}\n"
            ),
        ]

        for payload in tamper_payloads:
            self.hosts_path.write_text(payload, encoding="utf-8")
            self.assertTrue(hm.verify_or_reapply(domains, meta=meta))
            self.assertFalse(hm.verify_or_reapply(domains, meta=meta))

    def test_truncated_block_preserves_custom_127_and_non_focusflow_managed_comments(self) -> None:
        raw = (
            "127.0.0.1 localhost\n\n"
            f"{LEGACY_MARKER_BEGIN}\n"
            "# FocusFlow Managed Header\n"
            "0.0.0.0 reddit.com\n"
            ":: reddit.com\n"
            "# Puppet managed custom cluster\n"
            "127.0.0.1 local.dev.cluster\n"
        )
        cleaned = strip_managed_blocks(raw)
        self.assertEqual(
            cleaned,
            "127.0.0.1 localhost\n# Puppet managed custom cluster\n127.0.0.1 local.dev.cluster\n",
        )

    def test_transient_read_retries_prevent_false_corrupt_reset(self) -> None:
        store = SessionStore(state_dir=str(self.state_dir))
        now = datetime.datetime.now(datetime.timezone.utc)
        store.finalize_session("retry-s1", "dev", 45, completed=True, start_time=now, end_time=now)

        cfg = ConfigStore(
            custom_domains=["example.com"],
            custom_processes=["slack.exe"],
            profiles=default_profiles(),
            daily_goal_minutes=180,
        )
        save_config(cfg, state_dir=str(self.state_dir))

        real_open = builtins.open
        sess_path_norm = os.path.normcase(os.path.abspath(store.path))
        sess_calls = [0]

        def flaky_sess_open(file, mode="r", *args, **kwargs):
            if isinstance(file, (str, os.PathLike)) and os.path.normcase(os.path.abspath(file)) == sess_path_norm and "r" in mode:
                sess_calls[0] += 1
                if sess_calls[0] == 1:
                    return io.BytesIO(b'{"sessions": [') if "b" in mode else io.StringIO('{"sessions": [')
            return real_open(file, mode, *args, **kwargs)

        with mock.patch("builtins.open", side_effect=flaky_sess_open):
            sessions = store.load_sessions()
        self.assertEqual(len(sessions), 1)
        self.assertEqual(sessions[0].id, "retry-s1")
        self.assertFalse(Path(f"{store.path}.corrupt.bak").exists())

        cfg_path = self.state_dir / "config.json"
        cfg_path_norm = os.path.normcase(os.path.abspath(str(cfg_path)))
        cfg_calls = [0]

        def flaky_cfg_open(file, mode="r", *args, **kwargs):
            if isinstance(file, (str, os.PathLike)) and os.path.normcase(os.path.abspath(file)) == cfg_path_norm and "r" in mode:
                cfg_calls[0] += 1
                if cfg_calls[0] == 1:
                    return io.BytesIO(b'{"daily_goal_minutes": ') if "b" in mode else io.StringIO('{"daily_goal_minutes": ')
            return real_open(file, mode, *args, **kwargs)

        with mock.patch("builtins.open", side_effect=flaky_cfg_open):
            loaded_cfg = load_config(state_dir=str(self.state_dir))
        self.assertEqual(loaded_cfg.daily_goal_minutes, 180)
        self.assertEqual(loaded_cfg.custom_domains, ["example.com"])
        self.assertFalse(Path(f"{cfg_path}.corrupt.bak").exists())

    def test_atomic_save_gui_prefs_and_self_export_safety(self) -> None:
        prefs_path = get_gui_settings_path(str(self.state_dir))
        sample_prefs: dict[str, object] = {
            "mode": "pomodoro",
            "profile": "hardcore",
            "pomo_work": 50,
            "pomo_break": 10,
            "strict_mode": True,
        }
        FocusFlowGUI._write_gui_prefs_atomic(prefs_path, sample_prefs)
        parsed_prefs = json.loads(Path(prefs_path).read_text(encoding="utf-8"))
        self.assertEqual(parsed_prefs["profile"], "hardcore")
        self.assertEqual(parsed_prefs["pomo_work"], 50)
        # Ensure no leftover temp files exist in state_dir
        leftovers = list(self.state_dir.glob("gui-settings-*.tmp"))
        self.assertEqual(leftovers, [])

        # Exporting directly to sessions.json path must serialize first without wiping sessions!
        store = SessionStore(state_dir=str(self.state_dir))
        now = datetime.datetime.now(datetime.timezone.utc)
        store.finalize_session("self-exp-1", "study", 30, completed=True, start_time=now, end_time=now)
        out_file = store.export_to_file(store.path, fmt="json", days=7, now=now)
        exported_json = json.loads(Path(out_file).read_text(encoding="utf-8"))
        self.assertEqual(len(exported_json["sessions"]), 1)
        self.assertEqual(exported_json["sessions"][0]["id"], "self-exp-1")

    def test_alive_pid_orphan_grace_window_vs_dead_pid(self) -> None:
        hm = HostsManager(hosts_path=str(self.hosts_path), state_dir=str(self.state_dir))
        t0 = datetime.datetime.now(datetime.timezone.utc)
        lease_end = t0 + datetime.timedelta(minutes=25)

        hm.apply_blocklist(
            ["reddit.com"],
            meta=SessionMeta(
                session_id="alive-grace-1",
                pid=os.getpid(),
                started_at=t0,
                expected_end_at=lease_end,
                profile="hardcore",
            ),
        )
        # 5 minutes past ExpectedEndAt (< 15m grace) with live PID -> NOT recovered
        within_grace = hm.recover_if_orphaned(
            now=lease_end + datetime.timedelta(minutes=5),
            flush_async=False,
        )
        self.assertFalse(within_grace.recovered)
        self.assertEqual(within_grace.reason, "active_session_alive")

        # Past 15-minute grace window -> recovered
        past_grace = hm.recover_if_orphaned(
            now=lease_end + ALIVE_PID_LEASE_GRACE + datetime.timedelta(minutes=1),
            flush_async=False,
        )
        self.assertTrue(past_grace.recovered)
        self.assertEqual(past_grace.reason, "session_lease_expired")

    def test_posix_sudo_state_dir_and_elevation_env_forwarding(self) -> None:
        cmd = build_posix_sudo_command(
            "/usr/bin/python3",
            "/opt/focusflow/FocusFlowGUI.py",
            ["--profile", "dev"],
            state_dir="/home/alice/.config/focusflow",
            hosts_override="/tmp/hosts",
        )
        self.assertEqual(cmd[0:2], ["sudo", "env"])
        self.assertTrue(any(arg.startswith("FOCUSFLOW_HOME=") for arg in cmd))
        self.assertTrue(any(arg.startswith("FOCUSFLOW_HOSTS_PATH=") for arg in cmd))

        with (
            mock.patch.object(focusflow_core.privileges.sys, "platform", "linux"),
            mock.patch("subprocess.Popen") as mock_popen,
        ):
            mock_proc = mock.MagicMock()
            mock_proc.pid = 43210
            mock_popen.return_value = mock_proc
            ok = elevate_self(script_path="/opt/focusflow/FocusFlowGUI.py", extra_args=[], executable="/usr/bin/python3")
            self.assertTrue(ok)
            called_cmd = mock_popen.call_args[0][0]
            self.assertEqual(called_cmd[0:2], ["sudo", "env"])
            self.assertTrue(any(tok.startswith("FOCUSFLOW_HOME=") for tok in called_cmd))

        # Verify SUDO_USER fallback in get_state_dir() when running as root without FOCUSFLOW_HOME
        fake_alice_home = self.base_dir / "alice_home"
        fake_alice_home.mkdir(parents=True, exist_ok=True)
        with (
            mock.patch.dict(os.environ, {"FOCUSFLOW_HOME": "", "SUDO_USER": "alice"}, clear=False),
            mock.patch.object(focusflow_core.paths.sys, "platform", "linux"),
            mock.patch.object(focusflow_core.paths.os, "geteuid", create=True, return_value=0),
            mock.patch(
                "focusflow_core.paths._resolve_sudo_user_home",
                return_value=str(fake_alice_home),
            ),
        ):
            resolved = get_state_dir()
            self.assertEqual(resolved, os.path.join(str(fake_alice_home), ".config", "focusflow"))

    def test_posix_protected_denylist_and_unconditional_proc_exe_resolution(self) -> None:
        posix_protected = (
            "sh",
            "bash",
            "zsh",
            "dash",
            "fish",
            "sshd",
            "sudo",
            "login",
            "dbus-daemon",
            "gnome-shell",
            "plasmashell",
            "xorg",
            "xwayland",
            "windowserver",
        )
        for name in posix_protected:
            self.assertTrue(is_protected_process(name), f"expected {name} to be protected")
            self.assertTrue(is_protected_process(f"/usr/bin/{name}"))
            with self.assertRaises(ProcessValidationError):
                normalize_process_name(name)

        # Short comm (< 15 chars) with /proc/<pid>/exe symlink to protected binary
        proc_root = self.base_dir / "fake_proc"
        pid_dir = proc_root / "24680"
        pid_dir.mkdir(parents=True, exist_ok=True)
        (pid_dir / "comm").write_text("worker\n", encoding="utf-8")

        with mock.patch("focusflow_core.supervisor.os.readlink", return_value="/usr/bin/zsh"):
            cands = resolve_procfs_names(str(proc_root), "24680")
        self.assertIn("worker", cands)
        self.assertIn("zsh", cands)
        sup = ProcessSupervisor(["worker.exe"])
        self.assertIsNone(sup.should_terminate(24680, cands))

    def test_v222_version_and_workflow_node24_alignment(self) -> None:
        repo_root = Path(__file__).resolve().parent.parent
        self.assertEqual(focusflow_core.__version__, "2.2.2")
        self.assertIn("v2.2.2", (repo_root / "FocusFlowGUI.py").read_text(encoding="utf-8"))
        self.assertIn("v2.2.2", (repo_root / "Launch_FocusFlow_GUI.bat").read_text(encoding="utf-8"))
        self.assertIn('Version = "2.2.2"', (repo_root / "cmd" / "focusflow" / "main.go").read_text(encoding="utf-8"))
        self.assertIn('DefaultVersion = "2.2.2"', (repo_root / "pkg" / "tui" / "hud.go").read_text(encoding="utf-8"))
        self.assertIn("## [2.2.2]", (repo_root / "CHANGELOG.md").read_text(encoding="utf-8"))

        for wf_name in ("ci.yml", "release.yml"):
            wf_text = (repo_root / ".github" / "workflows" / wf_name).read_text(encoding="utf-8")
            self.assertIn('FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: "true"', wf_text)

        release_text = (repo_root / ".github" / "workflows" / "release.yml").read_text(encoding="utf-8")
        self.assertIn("if ! command -v zip >/dev/null 2>&1; then", release_text)

        ps1_text = (repo_root / "install.ps1").read_text(encoding="utf-8")
        self.assertIn("PSModulePath", ps1_text)
        self.assertIsNone(re.search(r"(?m)^\s*exit\b", ps1_text))


if __name__ == "__main__":
    unittest.main()
