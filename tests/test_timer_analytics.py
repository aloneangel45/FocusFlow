"""Tests for monotonic deadline timer, friction challenge, real analytics, and zero-emoji compliance."""

from __future__ import annotations

import datetime
import json
from pathlib import Path
import re

from focusflow_core.storage import SessionStore
from focusflow_core.timer import FRICTION_ALPHABET, MonotonicPhaseTimer, generate_friction_token


def test_monotonic_timer_zero_drift_and_no_modal_break_burn() -> None:
    fake_now = [1000.0]
    timer = MonotonicPhaseTimer(clock_fn=lambda: fake_now[0])

    timer.start(1500.0, phase="WORK")
    assert timer.remaining_seconds() == 1500
    assert timer.poll_expired() is False

    fake_now[0] += 750.0
    assert timer.remaining_seconds() == 750
    assert abs(timer.progress_ratio() - 0.5) < 1e-6

    # Pause & resume without drift
    timer.pause()
    fake_now[0] += 300.0
    assert timer.remaining_seconds() == 750
    timer.resume()

    # Expire WORK phase
    fake_now[0] += 750.0
    assert timer.poll_expired() is True
    assert timer.awaiting_transition is True
    # Subsequent polls while modal dialog is open must NOT fire duplicate expirations!
    fake_now[0] += 600.0  # User leaves modal open for 10 minutes
    assert timer.poll_expired() is False

    # User clicks OK on modal -> 5-minute break starts fresh with full 300s!
    timer.start(300.0, phase="BREAK")
    assert timer.remaining_seconds() == 300


def test_friction_token_properties() -> None:
    tokens = {generate_friction_token(16) for _ in range(25)}
    assert len(tokens) == 25
    for tok in tokens:
        assert len(tok) == 16
        assert all(ch in FRICTION_ALPHABET for ch in tok)
        assert not any(amb in tok for amb in ("0", "O", "1", "I"))


def test_real_analytics_no_fake_seed_and_streak_calculation(
    hermetic_focusflow_env: tuple[Path, Path],
) -> None:
    state_dir, _ = hermetic_focusflow_env
    store = SessionStore(state_dir=str(state_dir))

    # Empty store must report 0 streak, 0 minutes, 0% completion
    empty = store.compute_analytics(days=7)
    assert empty.active_streak_days == 0
    assert empty.total_minutes == 0
    assert empty.completed_sprints == 0
    assert empty.completion_rate == 0.0
    assert len(empty.daily) == 7
    assert all(d.minutes == 0 for d in empty.daily)

    ref = datetime.datetime(2026, 11, 10, 18, 0, 0, tzinfo=datetime.timezone.utc)
    d2 = ref - datetime.timedelta(days=2)
    d1 = ref - datetime.timedelta(days=1)

    store.finalize_session("s1", "study", 25, completed=True, blocked_count=1, start_time=d2, end_time=d2)
    store.finalize_session("s2", "dev", 50, completed=True, blocked_count=2, start_time=d1, end_time=d1)
    sid3 = store.start_session("s3", "study", 45, start_time=ref - datetime.timedelta(minutes=45))
    store.finalize_session(
        sid3,
        "study",
        45,
        completed=True,
        blocked_count=4,
        start_time=ref - datetime.timedelta(minutes=45),
        end_time=ref,
    )
    # Aborted session recorded as completed=False
    store.finalize_session(
        "s4",
        "hardcore",
        10,
        completed=False,
        blocked_count=3,
        start_time=ref - datetime.timedelta(minutes=10),
        end_time=ref,
    )

    summary = store.compute_analytics(days=7, now=ref)
    assert summary.completed_sprints == 3
    assert summary.attempted_sprints == 4
    assert summary.aborted_sprints == 1
    assert summary.completion_rate == 75.0
    assert summary.total_minutes == 120
    assert summary.total_hours == 2.0
    assert summary.active_streak_days == 3
    assert summary.total_blocked == 10

    exported = json.loads(store.export_json_string(days=7, now=ref))
    assert exported["summary"]["active_streak_days"] == 3
    csv_text = store.export_csv_string()
    assert "s1,study" in csv_text
    assert "s4,hardcore" in csv_text


def test_zero_emoji_and_zero_shortcode_across_code_and_docs() -> None:
    repo_root = Path(__file__).resolve().parent.parent
    targets = [
        "FocusFlowGUI.py",
        "cmd/focusflow/main.go",
        "pkg/tui/hud.go",
        "install.sh",
        "install.ps1",
        "README.md",
        "README.tr.md",
        "SECURITY.md",
        "CHANGELOG.md",
        "CONTRIBUTING.md",
    ]
    shortcode_re = re.compile(r":[a-z0-9_+-]+:")

    for rel in targets:
        p = repo_root / rel
        text = p.read_text(encoding="utf-8")
        for ch in text:
            cp = ord(ch)
            is_emoji = (
                (0x1F000 <= cp <= 0x1FAFF)
                or (0x2600 <= cp <= 0x27BF)
                or cp == 0xFE0F
            )
            assert not is_emoji, f"Forbidden emoji/dingbat U+{cp:04X} ({ch!r}) found in {rel}"

        # Exclude URL schemes like https: or port specs by checking GitHub emoji shortcodes
        for match in shortcode_re.finditer(text):
            token = match.group(0)
            assert token not in (":white_check_mark:", ":x:", ":warning:", ":rocket:", ":fire:"), (
                f"Forbidden emoji shortcode {token} found in {rel}"
            )


def test_gui_profile_switching_strict_close_and_single_prompt_unlock(
    hermetic_focusflow_env: tuple[Path, Path],
    monkeypatch,
) -> None:
    import pytest

    tk = pytest.importorskip("tkinter")
    import FocusFlowGUI
    from focusflow_core.config import load_config, save_config

    state_dir, _ = hermetic_focusflow_env
    pre_cfg = load_config(state_dir=str(state_dir))
    pre_cfg.daily_goal_minutes = 150
    pre_cfg.upsert_custom_profile(
        "writer",
        work_minutes=45,
        break_minutes=10,
        sites=["medium.com"],
        apps=["slack.exe"],
        description="Custom writing profile",
    )
    save_config(pre_cfg, state_dir=str(state_dir))

    try:
        root = tk.Tk()
        root.withdraw()
    except tk.TclError:
        pytest.skip("Tkinter display unavailable in headless environment")

    app = FocusFlowGUI.FocusFlowGUI(root)
    try:
        # 0. Custom profile from config.json is dynamically loaded and selectable in GUI
        assert "writer" in app.config_store.profiles
        assert app.profile_combo is not None
        app.profile_var.set("writer")
        app.on_profile_change()
        assert app.current_profile == "writer"
        assert app.gui_prefs["pomo_work"] == 45
        assert app.gui_prefs["pomo_break"] == 10
        assert "medium.com" in app.active_profile_obj.blocked_domains
        assert "Today: 0 / 150 min [0%]" in app.lbl_mini_goal.cget("text")
        # 1. Profile switching dynamically updates intervals, rules, and strictness
        app.profile_var.set("dev")
        app.on_profile_change()
        assert app.current_profile == "dev"
        assert app.gui_prefs["pomo_work"] == 50
        assert app.gui_prefs["pomo_break"] == 10
        assert "youtube.com" not in app.active_profile_obj.blocked_domains

        app.profile_var.set("hardcore")
        app.on_profile_change()
        assert app.current_profile == "hardcore"
        assert app.is_strict_active() is True
        assert app.current_strictness_level() == "hardcore"
        assert "youtube.com" in app.active_profile_obj.blocked_domains

        # 2. Start session -> StartTime persisted immediately in sessions.json
        app.start_session()
        assert app.is_active is True
        running = app.session_store.load_sessions()
        assert len(running) == 1
        assert running[0].status == "running"

        # 3. Window close (X) in strict mode MUST invoke Friction Challenge and block close on failure
        challenge_calls = []

        def fake_fail_challenge(reason: str = "") -> bool:
            challenge_calls.append(reason)
            return False

        monkeypatch.setattr(app, "_verify_friction_challenge", fake_fail_challenge)
        app.on_close()
        assert len(challenge_calls) == 1
        assert app.is_active is True  # Window close bypass blocked!

        # 4. Emergency unlock invokes Friction Challenge ONCE (no double-prompt bug!) and records completed=False
        prompt_count = [0]

        def fake_pass_challenge(reason: str = "") -> bool:
            prompt_count[0] += 1
            return True

        monkeypatch.setattr(app, "_verify_friction_challenge", fake_pass_challenge)
        app.prompt_emergency()
        assert prompt_count[0] == 1
        assert app.is_active is False

        finalized = app.session_store.load_sessions()
        assert len(finalized) == 1
        assert finalized[0].completed is False
        assert finalized[0].status == "aborted"
    finally:
        app.worker_running = False
        try:
            root.destroy()
        except Exception:
            pass

