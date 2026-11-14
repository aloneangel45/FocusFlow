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


def test_export_json_string_local_timezone_parity(
    hermetic_focusflow_env: tuple[Path, Path],
) -> None:
    state_dir, _ = hermetic_focusflow_env
    store = SessionStore(state_dir=str(state_dir))

    # 2026-06-15 21:30:00 UTC-5 == 2026-06-16 02:30:00Z (crosses UTC midnight, same local day!)
    tz_minus5 = datetime.timezone(datetime.timedelta(hours=-5))
    local_start = datetime.datetime(2026, 6, 15, 21, 0, 0, tzinfo=tz_minus5)
    local_now = datetime.datetime(2026, 6, 15, 21, 45, 0, tzinfo=tz_minus5)

    store.finalize_session(
        "tz-1",
        "dev",
        45,
        completed=True,
        blocked_count=2,
        start_time=local_start,
        end_time=local_now,
    )
    direct = store.compute_analytics(days=7, now=local_now)
    exported = json.loads(store.export_json_string(days=7, now=local_now))
    assert direct.today_focus_minutes == 45
    assert exported["summary"]["today_focus_minutes"] == 45
    assert exported["summary"]["daily"][-1]["date"] == "2026-06-15"

    # Also verify default `now=None` uses local timezone consistently in both methods
    today_local = datetime.datetime.now().astimezone().replace(hour=0, minute=5, second=0, microsecond=0)
    store.finalize_session(
        "tz-today",
        "study",
        25,
        completed=True,
        blocked_count=1,
        start_time=today_local,
        end_time=today_local + datetime.timedelta(minutes=25),
    )
    default_summary = store.compute_analytics(days=7)
    default_export = json.loads(store.export_json_string(days=7))
    assert default_export["summary"]["today_focus_minutes"] == default_summary.today_focus_minutes
    assert default_export["summary"]["daily"][-1]["date"] == default_summary.daily[-1].date


def test_gui_early_break_blocked_kpi_hardcore_break_and_token_regen(
    hermetic_focusflow_env: tuple[Path, Path],
    monkeypatch,
) -> None:
    import pytest

    tk = pytest.importorskip("tkinter")
    import FocusFlowGUI
    from focusflow_core.config import load_config, save_config

    state_dir, _ = hermetic_focusflow_env
    cfg = load_config(state_dir=str(state_dir))
    cfg.upsert_custom_profile("temp-custom", work_minutes=30, break_minutes=5, sites=["medium.com"])
    save_config(cfg, state_dir=str(state_dir))

    try:
        root = tk.Tk()
        root.withdraw()
    except tk.TclError:
        pytest.skip("Tkinter display unavailable in headless environment")

    app = FocusFlowGUI.FocusFlowGUI(root)
    try:
        # 1. FrictionChallengeDialog regenerates fresh token & redraws Canvas on mismatch
        dlg = FocusFlowGUI.FrictionChallengeDialog(root, reason_label="TEST REGEN")
        try:
            first_token = dlg.token
            dlg.entry.insert(0, "WRONGCODE1234567")
            dlg._on_submit()
            assert dlg.passed is False
            assert dlg.token != first_token
            canvas_text = dlg.canvas.itemcget(dlg._token_text_id, "text").replace(" - ", "")
            assert canvas_text == dlg.token
        finally:
            dlg.destroy()

        # 2. Early manual break (`give_break`) finalizes WORK session (zero leaked "running" records)
        #    and does NOT double-count DISTRACTIONS KILLED KPI!
        monkeypatch.setattr(app, "_verify_friction_challenge", lambda reason="": True)
        app.profile_var.set("study")
        app.on_profile_change()
        app.start_session()
        app.session_blocked_count = 4
        app.refresh_analytics_ui()
        assert app.kpi_labels["blocked"].cget("text") == "4"

        app.give_break()
        assert app.pomo_state == "BREAK"
        assert app.active_session_id == ""
        assert app.session_blocked_count == 0
        sessions_after_break = app.session_store.load_sessions()
        assert len(sessions_after_break) == 1
        assert sessions_after_break[0].status == "aborted"
        assert sessions_after_break[0].completed is False
        assert sessions_after_break[0].blocked_count == 4
        # KPI must equal 4 (not doubled to 8!)
        assert app.kpi_labels["blocked"].cget("text") == "4"

        app.stop_session(verified=True)
        assert app.kpi_labels["blocked"].cget("text") == "4"

        # 3. Hardcore mode keeps hosts shield continuously engaged during Pomodoro break
        app.profile_var.set("hardcore")
        app.on_profile_change()
        app.start_session()
        assert app.hosts_engaged is True
        monkeypatch.setattr(FocusFlowGUI.messagebox, "showinfo", lambda *a, **kw: None)
        app._on_phase_expired()
        assert app.pomo_state == "BREAK"
        assert app.hosts_engaged is True
        assert app.hosts_manager.is_active() is True
        app.stop_session(verified=True)

        # 4. External deletion of selected custom profile falls back cleanly to "study" on tab switch
        app.profile_var.set("temp-custom")
        app.on_profile_change()
        assert app.current_profile == "temp-custom"

        ext_cfg = load_config(state_dir=str(state_dir))
        ext_cfg.delete_custom_profile("temp-custom")
        save_config(ext_cfg, state_dir=str(state_dir))

        app.notebook.select(app.tab_timer)
        app._on_tab_changed(None)
        assert app.current_profile == "study"
        assert app.profile_var.get() == "study"
    finally:
        app.worker_running = False
        try:
            root.destroy()
        except Exception:
            pass

