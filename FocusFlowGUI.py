"""
FocusFlow v2.2.1 - Native Desktop Deep Work & Distraction Shield HUD
Theme: Catppuccin Mocha Minimal Systems Architecture (Zero-Emoji)
High-DPI Per-Monitor v2, Crash-Safe SHA-256 Hosts Engine, Win32 Toolhelp32 Supervisor
"""

from __future__ import annotations

import ctypes
import datetime
import json
import os
import random
import sys
import threading
import time
import tkinter as tk
from tkinter import filedialog, messagebox, simpledialog, ttk

from focusflow_core import (
    DOH_BYPASS_DOMAINS,
    AntiCheatGuard,
    DomainValidationError,
    HostsManager,
    MonotonicPhaseTimer,
    ProcessSupervisor,
    ProcessValidationError,
    SessionMeta,
    SessionStore,
    __version__,
    elevate_self,
    flush_dns_async,
    generate_friction_token,
    generate_session_id,
    get_gui_settings_path,
    get_state_dir,
    is_elevated,
    load_config,
    load_or_default,
    normalize_daily_goal_minutes,
    normalize_process_name,
    sanitize_domain,
    save_config,
    shield_mode_label,
)

# Optional Pillow support for visual PNG analytics card export
try:
    from PIL import Image, ImageDraw

    HAS_PIL = True
except ImportError:
    HAS_PIL = False


def configure_windows_dpi() -> None:
    """Enable crisp Per-Monitor DPI Awareness v2 on Windows 10/11."""
    if sys.platform != "win32":
        return
    try:
        # DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
        ctypes.windll.user32.SetProcessDpiAwarenessContext(ctypes.c_void_p(-4))
        return
    except Exception:
        pass
    try:
        # PROCESS_PER_MONITOR_DPI_AWARE = 2
        ctypes.windll.shcore.SetProcessDpiAwareness(2)
        return
    except Exception:
        pass
    try:
        ctypes.windll.shcore.SetProcessDpiAwareness(1)
    except Exception:
        pass


# Catppuccin Mocha Palette
THEME: dict[str, str] = {
    "crust": "#11111b",
    "mantle": "#181825",
    "base": "#1e1e2e",
    "surface0": "#313244",
    "surface1": "#45475a",
    "surface2": "#585b70",
    "text": "#cdd6f4",
    "subtext": "#a6adc8",
    "overlay": "#6c7086",
    "green": "#a6e3a1",
    "blue": "#89b4fa",
    "sapphire": "#74c7ec",
    "yellow": "#f9e2af",
    "peach": "#fab387",
    "red": "#f38ba8",
    "mauve": "#cba6f7",
    "teal": "#94e2d5",
}

DEFAULT_QUOTES: list[str] = [
    "Discipline consistently outperforms fleeting motivation.",
    "One hour of uncompromising deep work prevents a lifetime of regret.",
    "Focus is the art of saying NO to every trivial distraction.",
    "Muhtac oldugun kudret, damarlarindaki asil kanda mevcuttur. - M. Kemal Ataturk",
    "Basari, disiplinin ve tavizsiz odaklanmanin eseridir.",
    "Her saniye geri gelmeyen bir sermayedir; dikkatini koru.",
    "Systems beat willpower when friction is engineered on your side.",
    "Protect your attention as fiercely as your production infrastructure.",
]


class FrictionChallengeDialog(tk.Toplevel):
    """High-friction non-pasteable 16-character alphanumeric unlock modal."""

    def __init__(self, parent: tk.Misc, reason_label: str = "STRICT SESSION UNLOCK") -> None:
        super().__init__(parent)
        self.title("FocusFlow Friction Challenge")
        self.configure(bg=THEME["base"])
        self.resizable(False, False)
        self.transient(parent)
        self.grab_set()

        self.passed = False
        self.token = generate_friction_token(16)

        container = tk.Frame(self, bg=THEME["base"], padx=28, pady=24)
        container.pack(fill="both", expand=True)

        tk.Label(
            container,
            text=f"[CHALLENGE] {reason_label}",
            font=("Segoe UI Bold", 12),
            fg=THEME["red"],
            bg=THEME["base"],
        ).pack(anchor="w")

        tk.Label(
            container,
            text="Clipboard paste is disabled. Type all 16 characters manually (case-insensitive, without spaces):",
            font=("Segoe UI", 9),
            fg=THEME["subtext"],
            bg=THEME["base"],
            wraplength=460,
            justify="left",
        ).pack(anchor="w", pady=(6, 14))

        # Render challenge code onto a Canvas so text cannot be highlighted or copied
        grouped = " - ".join(self.token[i : i + 4] for i in range(0, 16, 4))
        self.canvas = tk.Canvas(
            container,
            width=460,
            height=56,
            bg=THEME["crust"],
            highlightthickness=1,
            highlightbackground=THEME["surface1"],
        )
        self.canvas.pack(fill="x", pady=(0, 14))
        self._token_text_id = self.canvas.create_text(
            230,
            28,
            text=grouped,
            fill=THEME["yellow"],
            font=("Consolas", 16, "bold"),
        )

        self.entry = tk.Entry(
            container,
            bg=THEME["surface0"],
            fg=THEME["text"],
            insertbackground=THEME["text"],
            font=("Consolas", 14),
            relief="flat",
            highlightthickness=1,
            highlightbackground=THEME["surface2"],
        )
        self.entry.pack(fill="x", ipady=7, pady=(0, 8))

        # Block all paste bindings and middle/right-click paste shortcuts
        for seq in ("<Control-v>", "<Control-V>", "<Shift-Insert>", "<<Paste>>", "<Button-2>", "<Button-3>"):
            self.entry.bind(seq, lambda _e: "break")
        self.entry.bind("<Return>", lambda _e: self._on_submit())
        self.entry.bind("<Escape>", lambda _e: self._on_cancel())

        self.lbl_error = tk.Label(
            container,
            text="",
            font=("Segoe UI Semibold", 9),
            fg=THEME["red"],
            bg=THEME["base"],
        )
        self.lbl_error.pack(anchor="w", pady=(0, 10))

        btn_row = tk.Frame(container, bg=THEME["base"])
        btn_row.pack(fill="x")

        tk.Button(
            btn_row,
            text="[STAY FOCUSED]",
            command=self._on_cancel,
            font=("Segoe UI Bold", 10),
            bg=THEME["green"],
            fg=THEME["crust"],
            relief="flat",
            padx=16,
            pady=7,
            cursor="hand2",
        ).pack(side="left")

        tk.Button(
            btn_row,
            text="[CONFIRM ABORT]",
            command=self._on_submit,
            font=("Segoe UI Bold", 10),
            bg=THEME["surface1"],
            fg=THEME["red"],
            relief="flat",
            padx=16,
            pady=7,
            cursor="hand2",
        ).pack(side="right")

        self.protocol("WM_DELETE_WINDOW", self._on_cancel)
        self.update_idletasks()
        self.entry.focus_set()

    def _on_submit(self) -> None:
        typed = self.entry.get().replace("-", "").replace(" ", "").strip().upper()
        if typed == self.token:
            self.passed = True
            self.destroy()
            return
        # Regenerate fresh token on mismatch to defeat partial brute-force / scripts
        self.token = generate_friction_token(16)
        grouped = " - ".join(self.token[i : i + 4] for i in range(0, 16, 4))
        self.canvas.itemconfig(self._token_text_id, text=grouped)
        self.lbl_error.config(text="[ERR] Challenge mismatch. New code generated; enter all 16 characters.")
        self.entry.delete(0, "end")

    def _on_cancel(self) -> None:
        self.passed = False
        self.destroy()


class FocusFlowGUI:
    def __init__(self, root: tk.Tk) -> None:
        self.root = root
        self.root.title(f"FocusFlow v{__version__} — Deep Work & Distraction Shield")
        self.root.geometry("1060x780")
        self.root.minsize(960, 700)
        self.root.configure(bg=THEME["base"])

        # Core engines & unified state directory
        self.state_dir = get_state_dir()
        os.makedirs(self.state_dir, exist_ok=True)
        self.hosts_manager = HostsManager(state_dir=self.state_dir)
        self.session_store = SessionStore(state_dir=self.state_dir)

        # Runtime state
        self.is_active = False
        self.hosts_engaged = False
        self.current_mode = "pomodoro"
        self.current_profile = "study"
        self.pomo_state = "WORK"
        self.pomo_cycle = 1
        self.session_blocked_count = 0
        self.active_session_id = ""
        self.active_session_start_utc: datetime.datetime | None = None
        self.active_planned_minutes = 25
        self.phase_timer = MonotonicPhaseTimer()
        self._supervisor_lock = threading.Lock()
        self._proc_supervisor = ProcessSupervisor([])
        self._anticheat_guard = AntiCheatGuard("standard")
        self._active_sinkhole_domains: list[str] = []
        self._active_session_meta: SessionMeta | None = None

        # Startup orphan crash/reboot recovery
        self.startup_recovery = self.hosts_manager.recover_if_orphaned(
            on_orphan_session=lambda sid: self.session_store.mark_orphaned_session(sid),
            flush_async=True,
        )

        self.load_all_state()
        self.setup_styles()
        self.build_ui()
        self.on_profile_change(initial=True)
        self.refresh_analytics_ui()

        if self.startup_recovery.recovered:
            self.lbl_status.config(
                text=f"[RECOVERED] Restored clean hosts from prior interrupted session ({self.startup_recovery.reason})",
                fg=THEME["yellow"],
            )

        self.worker_running = True
        self.clock_thread = threading.Thread(target=self._clock_worker, name="FocusFlowClock", daemon=True)
        self.clock_thread.start()

        self.supervisor_thread = threading.Thread(target=self._supervisor_worker, name="FocusFlowSupervisor", daemon=True)
        self.supervisor_thread.start()

        self.root.protocol("WM_DELETE_WINDOW", self.on_close)

    # ---------------- STATE & CONFIG SYNC ----------------
    def load_all_state(self) -> None:
        self.config_store = load_config(state_dir=self.state_dir)
        self.gui_prefs: dict[str, object] = {
            "mode": "pomodoro",
            "profile": "study",
            "pomo_work": 25,
            "pomo_break": 5,
            "standard_duration": 60,
            "flex_duration": 90,
            "strict_mode": True,
            "quotes": list(DEFAULT_QUOTES),
        }

        prefs_path = get_gui_settings_path(self.state_dir)
        if os.path.exists(prefs_path):
            try:
                with open(prefs_path, "r", encoding="utf-8-sig") as f:
                    saved = json.load(f)
                if isinstance(saved, dict):
                    for key in ("mode", "profile", "strict_mode"):
                        if key in saved:
                            self.gui_prefs[key] = saved[key]
                    for int_key, default_val in (
                        ("pomo_work", 25),
                        ("pomo_break", 5),
                        ("standard_duration", 60),
                        ("flex_duration", 90),
                    ):
                        if int_key in saved and saved[int_key] is not None and not isinstance(saved[int_key], bool):
                            try:
                                self.gui_prefs[int_key] = max(1, min(720, int(saved[int_key])))
                            except (TypeError, ValueError):
                                self.gui_prefs[int_key] = default_val
                    if isinstance(saved.get("quotes"), list) and saved["quotes"]:
                        valid_quotes = [str(q) for q in saved["quotes"] if q is not None and str(q).strip()]
                        if valid_quotes:
                            self.gui_prefs["quotes"] = valid_quotes
            except Exception:
                pass

        self.current_mode = str(self.gui_prefs.get("mode", "pomodoro"))
        if self.current_mode not in ("pomodoro", "standard", "flex"):
            self.current_mode = "pomodoro"
        self.current_profile = str(self.gui_prefs.get("profile", "study")).strip().lower()
        valid_profiles = set(self.config_store.profiles.keys()) | {"study", "dev", "hardcore"}
        if self.current_profile not in valid_profiles:
            self.current_profile = "study"

    def save_gui_prefs(self) -> None:
        prefs_path = get_gui_settings_path(self.state_dir)
        try:
            with open(prefs_path, "w", encoding="utf-8") as f:
                json.dump(self.gui_prefs, f, indent=2, ensure_ascii=False)
        except Exception:
            pass

    def is_strict_active(self) -> bool:
        if self.current_profile == "hardcore":
            return True
        return bool(self.gui_prefs.get("strict_mode", True))

    def current_strictness_level(self) -> str:
        if self.current_profile == "hardcore":
            return "hardcore"
        if bool(self.gui_prefs.get("strict_mode", True)):
            return "standard"
        return "relaxed"

    # ---------------- STYLES & MAIN LAYOUT ----------------
    def setup_styles(self) -> None:
        style = ttk.Style()
        style.theme_use("clam")

        style.configure("TFrame", background=THEME["base"])
        style.configure("Card.TFrame", background=THEME["surface0"], relief="flat")
        style.configure("Surface.TFrame", background=THEME["mantle"], relief="flat")

        style.configure("TNotebook", background=THEME["base"], borderwidth=0)
        style.configure(
            "TNotebook.Tab",
            background=THEME["mantle"],
            foreground=THEME["subtext"],
            padding=[18, 10],
            font=("Segoe UI Semibold", 10),
            borderwidth=0,
        )
        style.map(
            "TNotebook.Tab",
            background=[("selected", THEME["surface0"])],
            foreground=[("selected", THEME["green"])],
        )

        style.configure(
            "Treeview",
            background=THEME["surface0"],
            foreground=THEME["text"],
            fieldbackground=THEME["surface0"],
            borderwidth=0,
            font=("Segoe UI", 10),
            rowheight=28,
        )
        style.configure(
            "Treeview.Heading",
            background=THEME["surface1"],
            foreground=THEME["text"],
            font=("Segoe UI Bold", 10),
            relief="flat",
        )
        style.map(
            "Treeview",
            background=[("selected", THEME["blue"])],
            foreground=[("selected", THEME["crust"])],
        )

        style.configure(
            "TSpinbox",
            background=THEME["surface0"],
            foreground=THEME["text"],
            fieldbackground=THEME["surface0"],
            arrowcolor=THEME["text"],
        )

        style.configure(
            "TCombobox",
            background=THEME["surface0"],
            foreground=THEME["text"],
            fieldbackground=THEME["mantle"],
            arrowcolor=THEME["text"],
        )
        style.map(
            "TCombobox",
            fieldbackground=[("readonly", THEME["mantle"]), ("disabled", THEME["surface0"])],
            foreground=[("readonly", THEME["text"]), ("disabled", THEME["subtext"])],
        )

    def build_ui(self) -> None:
        header = tk.Frame(self.root, bg=THEME["mantle"], height=68)
        header.pack(fill="x", side="top")
        header.pack_propagate(False)

        title_box = tk.Frame(header, bg=THEME["mantle"])
        title_box.pack(side="left", padx=24, pady=14)

        tk.Label(
            title_box,
            text="FOCUSFLOW",
            font=("Segoe UI Black", 18),
            fg=THEME["green"],
            bg=THEME["mantle"],
        ).pack(side="left")
        tk.Label(
            title_box,
            text=f"  // v{__version__} NATIVE SHIELD",
            font=("Consolas", 10, "bold"),
            fg=THEME["subtext"],
            bg=THEME["mantle"],
        ).pack(side="left", pady=4)

        badge_box = tk.Frame(header, bg=THEME["mantle"])
        badge_box.pack(side="right", padx=24, pady=14)

        elevated = is_elevated()
        if not elevated:
            self.btn_elevate = tk.Button(
                badge_box,
                text="[ELEVATE ADMIN]",
                command=self.action_elevate_admin,
                font=("Segoe UI Bold", 9),
                bg=THEME["peach"],
                fg=THEME["crust"],
                activebackground=THEME["yellow"],
                activeforeground=THEME["crust"],
                relief="flat",
                padx=12,
                pady=4,
                cursor="hand2",
            )
            self.btn_elevate.pack(side="right", padx=(6, 0))

        shield_text = shield_mode_label(elevated)
        shield_color = THEME["teal"] if elevated else THEME["yellow"]
        self.lbl_shield_badge = tk.Label(
            badge_box,
            text=shield_text,
            font=("Consolas", 9, "bold"),
            bg=THEME["surface0"],
            fg=shield_color,
            padx=12,
            pady=5,
        )
        self.lbl_shield_badge.pack(side="right", padx=6)

        self.lbl_streak_badge = tk.Label(
            badge_box,
            text="STREAK: 0 DAYS",
            font=("Consolas", 9, "bold"),
            bg=THEME["surface0"],
            fg=THEME["peach"],
            padx=12,
            pady=5,
        )
        self.lbl_streak_badge.pack(side="right", padx=6)

        self.notebook = ttk.Notebook(self.root)
        self.notebook.pack(fill="both", expand=True, padx=20, pady=(14, 18))

        self.tab_timer = ttk.Frame(self.notebook)
        self.tab_blocks = ttk.Frame(self.notebook)
        self.tab_stats = ttk.Frame(self.notebook)
        self.tab_quotes = ttk.Frame(self.notebook)
        self.tab_settings = ttk.Frame(self.notebook)

        self.notebook.add(self.tab_timer, text="  [01] FOCUS HUD  ")
        self.notebook.add(self.tab_blocks, text="  [02] BLOCKLISTS  ")
        self.notebook.add(self.tab_stats, text="  [03] ANALYTICS  ")
        self.notebook.add(self.tab_quotes, text="  [04] DISCIPLINE  ")
        self.notebook.add(self.tab_settings, text="  [05] SYSTEM  ")

        self.build_tab_timer()
        self.build_tab_blocks()
        self.build_tab_stats()
        self.build_tab_quotes()
        self.build_tab_settings()

        self.notebook.bind("<<NotebookTabChanged>>", self._on_tab_changed)

    def _on_tab_changed(self, _event: object) -> None:
        selected = self.notebook.select()
        if selected == str(self.tab_stats):
            self.refresh_analytics_ui()
        elif selected == str(self.tab_blocks):
            self.refresh_lists()
        elif selected == str(self.tab_timer) and not self.is_active:
            self.config_store = load_config(state_dir=self.state_dir)
            valid_profiles = set(self.config_store.profiles.keys()) | {"study", "dev", "hardcore"}
            if self.current_profile not in valid_profiles:
                self.profile_var.set("study")
                self.on_profile_change()
            else:
                self.refresh_profile_selector()
                self.refresh_lists()

    # ---------------- TAB 1: FOCUS HUD ----------------
    def build_tab_timer(self) -> None:
        f = self.tab_timer

        left_card = tk.Frame(f, bg=THEME["surface0"], padx=30, pady=24)
        left_card.pack(side="left", fill="both", expand=True, padx=(0, 10), pady=10)

        self.lbl_pomo_badge = tk.Label(
            left_card,
            text="POMODORO // STUDY",
            font=("Consolas", 11, "bold"),
            bg=THEME["surface1"],
            fg=THEME["green"],
            padx=14,
            pady=4,
        )
        self.lbl_pomo_badge.pack(anchor="center", pady=(0, 10))

        self.lbl_timer = tk.Label(
            left_card,
            text="25:00",
            font=("Consolas", 68, "bold"),
            fg=THEME["green"],
            bg=THEME["surface0"],
        )
        self.lbl_timer.pack(anchor="center", pady=10)

        self.canvas_progress = tk.Canvas(
            left_card,
            height=12,
            bg=THEME["surface1"],
            highlightthickness=0,
        )
        self.canvas_progress.pack(fill="x", padx=36, pady=(0, 20))
        self.canvas_progress.bind("<Configure>", lambda _e: self.draw_progress_bar(self._current_ratio()))

        self.lbl_status = tk.Label(
            left_card,
            text="[STANDBY] Ready to engage focus session",
            font=("Segoe UI Semibold", 10),
            fg=THEME["subtext"],
            bg=THEME["surface0"],
        )
        self.lbl_status.pack(anchor="center", pady=(0, 24))

        btn_box = tk.Frame(left_card, bg=THEME["surface0"])
        btn_box.pack(anchor="center", pady=6)

        self.btn_toggle = tk.Button(
            btn_box,
            text="[START FOCUS]",
            command=self.toggle_session,
            font=("Segoe UI Black", 11),
            bg=THEME["green"],
            fg=THEME["crust"],
            activebackground=THEME["teal"],
            activeforeground=THEME["crust"],
            relief="flat",
            padx=22,
            pady=11,
            cursor="hand2",
        )
        self.btn_toggle.pack(side="left", padx=6)

        self.btn_break = tk.Button(
            btn_box,
            text="[TAKE BREAK]",
            command=self.give_break,
            font=("Segoe UI Bold", 10),
            bg=THEME["surface1"],
            fg=THEME["yellow"],
            activebackground=THEME["surface2"],
            relief="flat",
            padx=16,
            pady=11,
            state="disabled",
            cursor="hand2",
        )
        self.btn_break.pack(side="left", padx=6)

        self.btn_emergency = tk.Button(
            btn_box,
            text="[EMERGENCY UNLOCK]",
            command=self.prompt_emergency,
            font=("Segoe UI Bold", 10),
            bg=THEME["surface1"],
            fg=THEME["red"],
            activebackground=THEME["red"],
            activeforeground=THEME["crust"],
            relief="flat",
            padx=14,
            pady=11,
            state="disabled",
            cursor="hand2",
        )
        self.btn_emergency.pack(side="left", padx=6)

        # Right Column
        right_panel = tk.Frame(f, bg=THEME["base"], width=330)
        right_panel.pack(side="right", fill="y", padx=(10, 0), pady=10)
        right_panel.pack_propagate(False)

        mode_card = tk.Frame(right_panel, bg=THEME["surface0"], padx=20, pady=14)
        mode_card.pack(fill="x", pady=(0, 12))

        tk.Label(
            mode_card,
            text="SESSION MODE",
            font=("Consolas", 10, "bold"),
            fg=THEME["mauve"],
            bg=THEME["surface0"],
        ).pack(anchor="w", pady=(0, 8))

        self.mode_var = tk.StringVar(value=self.current_mode)
        self.mode_radios: list[tk.Radiobutton] = []
        for val, label in (
            ("pomodoro", "Pomodoro Interval"),
            ("standard", "Fixed Deep Work Block"),
            ("flex", "Extended Open Sprint"),
        ):
            rb = tk.Radiobutton(
                mode_card,
                text=label,
                value=val,
                variable=self.mode_var,
                command=self.on_mode_change,
                font=("Segoe UI Semibold", 10),
                fg=THEME["text"],
                bg=THEME["surface0"],
                selectcolor=THEME["mantle"],
                activebackground=THEME["surface0"],
                activeforeground=THEME["green"],
            )
            rb.pack(anchor="w", pady=2)
            self.mode_radios.append(rb)

        profile_card = tk.Frame(right_panel, bg=THEME["surface0"], padx=20, pady=14)
        profile_card.pack(fill="x", pady=(0, 12))

        tk.Label(
            profile_card,
            text="ACTIVE PROFILE",
            font=("Consolas", 10, "bold"),
            fg=THEME["sapphire"],
            bg=THEME["surface0"],
        ).pack(anchor="w", pady=(0, 8))

        self.profile_var = tk.StringVar(value=self.current_profile)
        self.profile_radios: list[tk.Radiobutton] = []
        self.profile_radios_frame = tk.Frame(profile_card, bg=THEME["surface0"])
        self.profile_radios_frame.pack(fill="x")
        self.profile_combo_frame = tk.Frame(profile_card, bg=THEME["surface0"])
        self.profile_combo: ttk.Combobox | None = None
        self.refresh_profile_selector()

        stats_mini = tk.Frame(right_panel, bg=THEME["surface0"], padx=20, pady=14)
        stats_mini.pack(fill="x")

        tk.Label(
            stats_mini,
            text="LIVE TELEMETRY",
            font=("Consolas", 10, "bold"),
            fg=THEME["peach"],
            bg=THEME["surface0"],
        ).pack(anchor="w", pady=(0, 8))

        self.lbl_mini_profile_targets = tk.Label(
            stats_mini,
            text="Active Rules: 0 domains / 0 procs",
            font=("Segoe UI", 9),
            fg=THEME["subtext"],
            bg=THEME["surface0"],
        )
        self.lbl_mini_profile_targets.pack(anchor="w", pady=2)

        self.lbl_mini_goal = tk.Label(
            stats_mini,
            text="Today: 0 / 120 min [0%]",
            font=("Segoe UI Semibold", 9),
            fg=THEME["teal"],
            bg=THEME["surface0"],
        )
        self.lbl_mini_goal.pack(anchor="w", pady=2)

        self.lbl_mini_blocked = tk.Label(
            stats_mini,
            text="Session Blocks: 0",
            font=("Segoe UI Semibold", 10),
            fg=THEME["text"],
            bg=THEME["surface0"],
        )
        self.lbl_mini_blocked.pack(anchor="w", pady=2)

        self.lbl_mini_cycles = tk.Label(
            stats_mini,
            text="Completed Sprints: 0",
            font=("Segoe UI Semibold", 10),
            fg=THEME["text"],
            bg=THEME["surface0"],
        )
        self.lbl_mini_cycles.pack(anchor="w", pady=2)

    def _format_profile_radio_label(self, key: str) -> str:
        builtin_labels = {
            "study": "STUDY  [25/5m - Full Academic]",
            "dev": "DEV    [50/10m - Docs Open]",
            "hardcore": "AEGIS  [Hardcore Anti-Cheat]",
        }
        if key in builtin_labels:
            return builtin_labels[key]
        prof = self.config_store.profiles.get(key)
        w = prof.default_work_minutes if prof else 25
        b = prof.default_break_minutes if prof else 5
        short = key.upper()[:8].ljust(6)
        return f"{short} [{w}/{b}m - Custom]"

    def refresh_profile_selector(self) -> None:
        if not hasattr(self, "profile_radios_frame"):
            return
        for child in self.profile_radios_frame.winfo_children():
            child.destroy()
        for child in self.profile_combo_frame.winfo_children():
            child.destroy()
        self.profile_radios.clear()
        self.profile_combo = None

        all_names = self.config_store.sorted_profile_names()
        radio_names = all_names if len(all_names) <= 5 else [n for n in ("study", "dev", "hardcore") if n in all_names]
        rb_state = "disabled" if self.is_active else "normal"

        for p_val in radio_names:
            rb = tk.Radiobutton(
                self.profile_radios_frame,
                text=self._format_profile_radio_label(p_val),
                value=p_val,
                variable=self.profile_var,
                command=self.on_profile_change,
                font=("Consolas", 9, "bold"),
                fg=THEME["text"],
                bg=THEME["surface0"],
                selectcolor=THEME["mantle"],
                activebackground=THEME["surface0"],
                activeforeground=THEME["sapphire"],
                state=rb_state,
            )
            rb.pack(anchor="w", pady=2)
            self.profile_radios.append(rb)

        custom_names = [n for n in all_names if n not in ("study", "dev", "hardcore")]
        if custom_names:
            self.profile_combo_frame.pack(fill="x", pady=(6, 0))
            tk.Label(
                self.profile_combo_frame,
                text="ALL / CUSTOM PROFILES:",
                font=("Consolas", 8, "bold"),
                fg=THEME["subtext"],
                bg=THEME["surface0"],
            ).pack(anchor="w", pady=(0, 2))
            combo = ttk.Combobox(
                self.profile_combo_frame,
                textvariable=self.profile_var,
                values=all_names,
                state="disabled" if self.is_active else "readonly",
                font=("Consolas", 9),
            )
            combo.pack(fill="x")
            combo.bind("<<ComboboxSelected>>", lambda _e: self.on_profile_change())
            self.profile_combo = combo
        else:
            self.profile_combo_frame.pack_forget()

    def _current_ratio(self) -> float:
        if self.is_active:
            return self.phase_timer.progress_ratio()
        return 1.0

    def draw_progress_bar(self, ratio: float) -> None:
        self.canvas_progress.delete("all")
        w = self.canvas_progress.winfo_width()
        if w < 10:
            w = 500
        h = 12
        clamped = max(0.0, min(1.0, ratio))
        self.canvas_progress.create_rectangle(0, 0, w, h, fill=THEME["surface1"], width=0)
        fill_color = THEME["green"] if self.pomo_state == "WORK" else THEME["blue"]
        self.canvas_progress.create_rectangle(0, 0, int(w * clamped), h, fill=fill_color, width=0)

    def on_profile_change(self, initial: bool = False) -> None:
        if self.is_active:
            self.profile_var.set(self.current_profile)
            return

        self.config_store = load_config(state_dir=self.state_dir)
        valid_profiles = set(self.config_store.profiles.keys()) | {"study", "dev", "hardcore"}
        selected = self.profile_var.get().strip().lower()
        if selected not in valid_profiles:
            selected = "study"
            self.profile_var.set(selected)
        self.current_profile = selected
        self.gui_prefs["profile"] = selected
        self.refresh_profile_selector()

        prof = load_or_default(selected, state_dir=self.state_dir)
        self.active_profile_obj = prof

        if not initial:
            self.gui_prefs["pomo_work"] = prof.default_work_minutes
            self.gui_prefs["pomo_break"] = prof.default_break_minutes
            if selected == "hardcore":
                self.gui_prefs["strict_mode"] = True
                if hasattr(self, "strict_var"):
                    self.strict_var.set(True)
            if hasattr(self, "spin_p_work"):
                self.spin_p_work.set(prof.default_work_minutes)
            if hasattr(self, "spin_p_break"):
                self.spin_p_break.set(prof.default_break_minutes)
            self.save_gui_prefs()

        self.lbl_mini_profile_targets.config(
            text=f"Active Rules: {len(prof.blocked_domains)} domains / {len(prof.blocked_processes)} procs"
        )
        self.refresh_lists()
        self.on_mode_change()

    def on_mode_change(self) -> None:
        if self.is_active:
            self.mode_var.set(self.current_mode)
            return

        m = self.mode_var.get()
        self.current_mode = m
        self.gui_prefs["mode"] = m
        self.save_gui_prefs()

        prof_tag = self.current_profile.upper()
        if m == "pomodoro":
            mins = int(self.gui_prefs.get("pomo_work", 25))
            self.lbl_timer.config(text=f"{mins:02d}:00", fg=THEME["green"])
            self.lbl_pomo_badge.config(text=f"POMODORO // {prof_tag}", fg=THEME["green"])
        elif m == "standard":
            mins = int(self.gui_prefs.get("standard_duration", 60))
            self.lbl_timer.config(text=f"{mins:02d}:00", fg=THEME["sapphire"])
            self.lbl_pomo_badge.config(text=f"DEEP WORK // {prof_tag}", fg=THEME["sapphire"])
        else:
            mins = int(self.gui_prefs.get("flex_duration", 90))
            self.lbl_timer.config(text=f"{mins:02d}:00", fg=THEME["peach"])
            self.lbl_pomo_badge.config(text=f"EXTENDED // {prof_tag}", fg=THEME["peach"])
        self.draw_progress_bar(1.0)

    # ---------------- TAB 2: BLOCKLISTS ----------------
    def build_tab_blocks(self) -> None:
        f = self.tab_blocks

        paned = tk.PanedWindow(f, orient="horizontal", bg=THEME["base"], sashwidth=8)
        paned.pack(fill="both", expand=True, padx=10, pady=10)

        # Domains Card
        card_sites = tk.Frame(paned, bg=THEME["surface0"], padx=16, pady=16)
        paned.add(card_sites, minsize=420)

        tk.Label(
            card_sites,
            text="BLOCKED DOMAINS (DUAL-STACK IPv4 + IPv6)",
            font=("Consolas", 10, "bold"),
            fg=THEME["blue"],
            bg=THEME["surface0"],
        ).pack(anchor="w", pady=(0, 10))

        input_site_box = tk.Frame(card_sites, bg=THEME["surface0"])
        input_site_box.pack(fill="x", pady=(0, 10))
        self.ent_site = tk.Entry(
            input_site_box,
            bg=THEME["mantle"],
            fg=THEME["text"],
            font=("Consolas", 10),
            insertbackground=THEME["text"],
            relief="flat",
            highlightthickness=1,
            highlightbackground=THEME["surface1"],
        )
        self.ent_site.pack(side="left", fill="x", expand=True, ipady=6, padx=(0, 8))
        self.ent_site.bind("<Return>", lambda _e: self.add_site())

        tk.Button(
            input_site_box,
            text="[+ ADD]",
            command=self.add_site,
            font=("Segoe UI Bold", 9),
            bg=THEME["blue"],
            fg=THEME["crust"],
            relief="flat",
            padx=12,
            pady=5,
            cursor="hand2",
        ).pack(side="right")

        self.tree_sites = ttk.Treeview(card_sites, columns=("domain", "source"), show="headings", selectmode="browse")
        self.tree_sites.heading("domain", text="Domain")
        self.tree_sites.heading("source", text="Scope")
        self.tree_sites.column("domain", anchor="w", width=260)
        self.tree_sites.column("source", anchor="center", width=90)
        self.tree_sites.pack(fill="both", expand=True, pady=(0, 10))

        sites_btn_bar = tk.Frame(card_sites, bg=THEME["surface0"])
        sites_btn_bar.pack(fill="x")
        tk.Button(
            sites_btn_bar,
            text="[REMOVE CUSTOM]",
            command=self.remove_site,
            font=("Segoe UI Semibold", 9),
            bg=THEME["surface1"],
            fg=THEME["red"],
            relief="flat",
            padx=10,
            pady=4,
            cursor="hand2",
        ).pack(side="left")
        tk.Button(
            sites_btn_bar,
            text="[CLEAR CUSTOM]",
            command=self.reset_custom_sites,
            font=("Segoe UI Semibold", 9),
            bg=THEME["surface1"],
            fg=THEME["subtext"],
            relief="flat",
            padx=10,
            pady=4,
            cursor="hand2",
        ).pack(side="right")

        # Processes Card
        card_apps = tk.Frame(paned, bg=THEME["surface0"], padx=16, pady=16)
        paned.add(card_apps, minsize=420)

        tk.Label(
            card_apps,
            text="BLOCKED PROCESSES (WIN32 TOOLHELP32 GUARD)",
            font=("Consolas", 10, "bold"),
            fg=THEME["mauve"],
            bg=THEME["surface0"],
        ).pack(anchor="w", pady=(0, 10))

        input_app_box = tk.Frame(card_apps, bg=THEME["surface0"])
        input_app_box.pack(fill="x", pady=(0, 10))
        self.ent_app = tk.Entry(
            input_app_box,
            bg=THEME["mantle"],
            fg=THEME["text"],
            font=("Consolas", 10),
            insertbackground=THEME["text"],
            relief="flat",
            highlightthickness=1,
            highlightbackground=THEME["surface1"],
        )
        self.ent_app.pack(side="left", fill="x", expand=True, ipady=6, padx=(0, 8))
        self.ent_app.bind("<Return>", lambda _e: self.add_app())

        tk.Button(
            input_app_box,
            text="[+ ADD]",
            command=self.add_app,
            font=("Segoe UI Bold", 9),
            bg=THEME["mauve"],
            fg=THEME["crust"],
            relief="flat",
            padx=12,
            pady=5,
            cursor="hand2",
        ).pack(side="right")

        self.tree_apps = ttk.Treeview(card_apps, columns=("app", "source"), show="headings", selectmode="browse")
        self.tree_apps.heading("app", text="Executable")
        self.tree_apps.heading("source", text="Scope")
        self.tree_apps.column("app", anchor="w", width=260)
        self.tree_apps.column("source", anchor="center", width=90)
        self.tree_apps.pack(fill="both", expand=True, pady=(0, 10))

        apps_btn_bar = tk.Frame(card_apps, bg=THEME["surface0"])
        apps_btn_bar.pack(fill="x")
        tk.Button(
            apps_btn_bar,
            text="[REMOVE CUSTOM]",
            command=self.remove_app,
            font=("Segoe UI Semibold", 9),
            bg=THEME["surface1"],
            fg=THEME["red"],
            relief="flat",
            padx=10,
            pady=4,
            cursor="hand2",
        ).pack(side="left")
        tk.Button(
            apps_btn_bar,
            text="[CLEAR CUSTOM]",
            command=self.reset_custom_apps,
            font=("Segoe UI Semibold", 9),
            bg=THEME["surface1"],
            fg=THEME["subtext"],
            relief="flat",
            padx=10,
            pady=4,
            cursor="hand2",
        ).pack(side="right")

    def refresh_lists(self) -> None:
        self.config_store = load_config(state_dir=self.state_dir)
        prof = self.config_store.profiles.get(self.current_profile)
        prof_domains = set(prof.blocked_domains if prof else [])
        custom_domains = set(self.config_store.custom_domains)

        for item in self.tree_sites.get_children():
            self.tree_sites.delete(item)
        for d in sorted(prof_domains | custom_domains):
            scope = "CUSTOM" if d in custom_domains else f"PROFILE:{self.current_profile.upper()}"
            self.tree_sites.insert("", "end", values=(d, scope))

        prof_procs = set(prof.blocked_processes if prof else [])
        custom_procs = set(self.config_store.custom_processes)
        for item in self.tree_apps.get_children():
            self.tree_apps.delete(item)
        for p in sorted(prof_procs | custom_procs):
            scope = "CUSTOM" if p in custom_procs else f"PROFILE:{self.current_profile.upper()}"
            self.tree_apps.insert("", "end", values=(p, scope))

        merged = load_or_default(self.current_profile, state_dir=self.state_dir)
        self.active_profile_obj = merged
        self.lbl_mini_profile_targets.config(
            text=f"Active Rules: {len(merged.blocked_domains)} domains / {len(merged.blocked_processes)} procs"
        )

    def _sync_active_session_rules(self) -> None:
        if not self.is_active:
            return
        merged = load_or_default(self.current_profile, state_dir=self.state_dir)
        self.active_profile_obj = merged
        all_domains = [*merged.blocked_domains, *DOH_BYPASS_DOMAINS]
        with self._supervisor_lock:
            self._proc_supervisor = ProcessSupervisor(merged.blocked_processes)
            self._active_sinkhole_domains = all_domains
            meta = self._active_session_meta
        if self.hosts_engaged and (self.pomo_state == "WORK" or self.current_strictness_level() == "hardcore"):
            try:
                self.hosts_manager.apply_blocklist(all_domains, meta=meta)
                flush_dns_async()
            except OSError:
                pass

    def add_site(self) -> None:
        raw = self.ent_site.get()
        if not raw.strip():
            return
        try:
            clean = sanitize_domain(raw)
        except DomainValidationError as exc:
            messagebox.showerror("Invalid Domain", f"Rejected domain input:\n{exc}")
            return

        cfg = load_config(state_dir=self.state_dir)
        if clean not in cfg.custom_domains:
            cfg.custom_domains.append(clean)
            save_config(cfg, state_dir=self.state_dir)
        self.ent_site.delete(0, "end")
        self.refresh_lists()
        self._sync_active_session_rules()

    def remove_site(self) -> None:
        if self.is_active and self.is_strict_active():
            messagebox.showwarning("Shield Locked", "Cannot remove blocked domains during an active strict sprint.")
            return
        sel = self.tree_sites.selection()
        if not sel:
            return
        val, scope = self.tree_sites.item(sel[0], "values")
        if scope != "CUSTOM":
            messagebox.showinfo("Profile Rule", f"'{val}' belongs to profile '{self.current_profile}'. Switch profiles to change preset rules.")
            return
        cfg = load_config(state_dir=self.state_dir)
        if val in cfg.custom_domains:
            cfg.custom_domains.remove(val)
            save_config(cfg, state_dir=self.state_dir)
            self.refresh_lists()
            self._sync_active_session_rules()

    def reset_custom_sites(self) -> None:
        if self.is_active and self.is_strict_active():
            messagebox.showwarning("Shield Locked", "Cannot clear custom domains during an active strict sprint.")
            return
        cfg = load_config(state_dir=self.state_dir)
        cfg.custom_domains = []
        save_config(cfg, state_dir=self.state_dir)
        self.refresh_lists()
        self._sync_active_session_rules()

    def add_app(self) -> None:
        raw = self.ent_app.get()
        if not raw.strip():
            return
        try:
            clean_proc = normalize_process_name(raw)
        except ProcessValidationError as exc:
            messagebox.showerror("Protected or Invalid Process", f"Cannot add process:\n{exc}")
            return

        cfg = load_config(state_dir=self.state_dir)
        if clean_proc not in cfg.custom_processes:
            cfg.custom_processes.append(clean_proc)
            save_config(cfg, state_dir=self.state_dir)
        self.ent_app.delete(0, "end")
        self.refresh_lists()
        self._sync_active_session_rules()

    def remove_app(self) -> None:
        if self.is_active and self.is_strict_active():
            messagebox.showwarning("Shield Locked", "Cannot remove blocked processes during an active strict sprint.")
            return
        sel = self.tree_apps.selection()
        if not sel:
            return
        val, scope = self.tree_apps.item(sel[0], "values")
        if scope != "CUSTOM":
            messagebox.showinfo("Profile Rule", f"'{val}' belongs to profile '{self.current_profile}'.")
            return
        cfg = load_config(state_dir=self.state_dir)
        if val in cfg.custom_processes:
            cfg.custom_processes.remove(val)
            save_config(cfg, state_dir=self.state_dir)
            self.refresh_lists()
            self._sync_active_session_rules()

    def reset_custom_apps(self) -> None:
        if self.is_active and self.is_strict_active():
            messagebox.showwarning("Shield Locked", "Cannot clear custom processes during an active strict sprint.")
            return
        cfg = load_config(state_dir=self.state_dir)
        cfg.custom_processes = []
        save_config(cfg, state_dir=self.state_dir)
        self.refresh_lists()
        self._sync_active_session_rules()

    # ---------------- TAB 3: ANALYTICS & EXPORTS ----------------
    def build_tab_stats(self) -> None:
        f = self.tab_stats

        top_kpis = tk.Frame(f, bg=THEME["base"])
        top_kpis.pack(fill="x", pady=10)

        self.kpi_labels: dict[str, tk.Label] = {}
        for key, title, color in (
            ("hours", "DEEP WORK TOTAL", THEME["green"]),
            ("streak", "ACTIVE STREAK", THEME["peach"]),
            ("rate", "COMPLETION RATE", THEME["sapphire"]),
            ("blocked", "DISTRACTIONS KILLED", THEME["red"]),
        ):
            kpi_card = tk.Frame(top_kpis, bg=THEME["surface0"], padx=18, pady=14)
            kpi_card.pack(side="left", fill="x", expand=True, padx=5)
            tk.Label(
                kpi_card,
                text=title,
                font=("Consolas", 9, "bold"),
                fg=THEME["subtext"],
                bg=THEME["surface0"],
            ).pack(anchor="w")
            val_lbl = tk.Label(
                kpi_card,
                text="0",
                font=("Segoe UI Black", 17),
                fg=color,
                bg=THEME["surface0"],
            )
            val_lbl.pack(anchor="w", pady=(4, 0))
            self.kpi_labels[key] = val_lbl

        chart_card = tk.Frame(f, bg=THEME["surface0"], padx=20, pady=18)
        chart_card.pack(fill="both", expand=True, pady=8)

        tk.Label(
            chart_card,
            text="LAST 7 DAYS FOCUS PERFORMANCE (MINUTES)",
            font=("Consolas", 10, "bold"),
            fg=THEME["text"],
            bg=THEME["surface0"],
        ).pack(anchor="w", pady=(0, 12))

        self.chart_canvas = tk.Canvas(chart_card, bg=THEME["mantle"], highlightthickness=0)
        self.chart_canvas.pack(fill="both", expand=True)
        self.chart_canvas.bind("<Configure>", lambda _e: self.draw_stats_chart())

        btn_bar = tk.Frame(f, bg=THEME["base"])
        btn_bar.pack(fill="x", pady=8)

        tk.Button(
            btn_bar,
            text="[EXPORT PNG]",
            command=self.export_report_png,
            font=("Segoe UI Bold", 9),
            bg=THEME["surface0"],
            fg=THEME["green"],
            relief="flat",
            padx=14,
            pady=7,
            cursor="hand2",
        ).pack(side="right", padx=(6, 0))

        tk.Button(
            btn_bar,
            text="[EXPORT CSV]",
            command=lambda: self.export_telemetry_dialog("csv"),
            font=("Segoe UI Bold", 9),
            bg=THEME["surface0"],
            fg=THEME["sapphire"],
            relief="flat",
            padx=14,
            pady=7,
            cursor="hand2",
        ).pack(side="right", padx=6)

        tk.Button(
            btn_bar,
            text="[EXPORT JSON]",
            command=lambda: self.export_telemetry_dialog("json"),
            font=("Segoe UI Bold", 9),
            bg=THEME["surface0"],
            fg=THEME["mauve"],
            relief="flat",
            padx=14,
            pady=7,
            cursor="hand2",
        ).pack(side="right", padx=6)

    def refresh_analytics_ui(self) -> None:
        summary = self.session_store.compute_analytics(days=7)
        self._latest_summary = summary

        streak_unit = "DAY" if summary.active_streak_days == 1 else "DAYS"
        self.lbl_streak_badge.config(text=f"STREAK: {summary.active_streak_days} {streak_unit}")
        self.lbl_mini_cycles.config(text=f"Completed Sprints: {summary.completed_sprints}")
        if hasattr(self, "lbl_mini_goal"):
            self.lbl_mini_goal.config(
                text=f"Today: {summary.today_focus_minutes} / {summary.daily_goal_minutes} min [{summary.daily_goal_progress_pct:.0f}%]"
            )

        if hasattr(self, "kpi_labels"):
            self.kpi_labels["hours"].config(text=f"{summary.total_hours:.1f}h ({summary.total_minutes}m)")
            self.kpi_labels["streak"].config(text=f"{summary.active_streak_days} Days")
            self.kpi_labels["rate"].config(
                text=f"{summary.completion_rate:.0f}% ({summary.completed_sprints}/{summary.attempted_sprints})"
            )
            live_blocks = self.session_blocked_count if self.active_session_id else 0
            self.kpi_labels["blocked"].config(text=str(summary.total_blocked + live_blocks))

        if hasattr(self, "chart_canvas"):
            self.draw_stats_chart()

    def draw_stats_chart(self) -> None:
        self.chart_canvas.delete("all")
        w = self.chart_canvas.winfo_width()
        h = self.chart_canvas.winfo_height()
        if w < 120 or h < 120:
            return

        summary = getattr(self, "_latest_summary", None) or self.session_store.compute_analytics(days=7)
        daily = summary.daily
        values = [d.minutes for d in daily]

        if not values or max(values) == 0:
            self.chart_canvas.create_text(
                w // 2,
                h // 2,
                text="No completed focus sessions recorded yet. Start a sprint to populate telemetry.",
                fill=THEME["subtext"],
                font=("Segoe UI", 11),
            )
            return

        max_val = max(values)
        padding_x = 64
        base_y = h - 46
        chart_h = h - 96
        bar_count = len(daily)
        avail_w = w - (2 * padding_x)
        bar_slot = avail_w / max(1, bar_count)
        bar_w = min(52, int(bar_slot * 0.56))

        for level in (0.25, 0.5, 0.75, 1.0):
            gy = base_y - (chart_h * level)
            self.chart_canvas.create_line(
                padding_x,
                gy,
                w - padding_x,
                gy,
                fill=THEME["surface0"],
                width=1,
                dash=(3, 3),
            )
            self.chart_canvas.create_text(
                padding_x - 12,
                gy,
                text=f"{int(round(max_val * level))}m",
                fill=THEME["subtext"],
                font=("Consolas", 8),
                anchor="e",
            )

        for idx, item in enumerate(daily):
            slot_center = padding_x + (idx * bar_slot) + (bar_slot / 2)
            x0 = slot_center - (bar_w / 2)
            x1 = slot_center + (bar_w / 2)
            bar_height = (item.minutes / max_val) * chart_h if max_val > 0 else 0
            y0 = base_y - bar_height
            y1 = base_y

            color = THEME["green"] if item.minutes >= 50 else THEME["sapphire"]
            if item.minutes > 0:
                self.chart_canvas.create_rectangle(x0, y0, x1, y1, fill=color, outline="")
            self.chart_canvas.create_text(
                slot_center,
                y0 - 11,
                text=f"{item.minutes}m",
                fill=THEME["text"],
                font=("Consolas", 9, "bold"),
            )
            short_date = item.date[-5:]
            self.chart_canvas.create_text(
                slot_center,
                y1 + 18,
                text=short_date,
                fill=THEME["subtext"],
                font=("Consolas", 8),
            )

    def export_telemetry_dialog(self, fmt: str) -> None:
        ext = f".{fmt}"
        default_name = f"focusflow_sessions_{datetime.date.today().strftime('%Y%m%d')}{ext}"
        out_path = filedialog.asksaveasfilename(
            parent=self.root,
            title=f"Export FocusFlow {fmt.upper()} Report",
            initialdir=self.state_dir,
            initialfile=default_name,
            defaultextension=ext,
            filetypes=[(f"{fmt.upper()} files", f"*{ext}"), ("All files", "*.*")],
        )
        if not out_path:
            return
        try:
            saved = self.session_store.export_to_file(out_path, fmt=fmt, days=7)
            messagebox.showinfo("Export Complete", f"Saved {fmt.upper()} telemetry report to:\n{saved}")
        except Exception as exc:
            messagebox.showerror("Export Failed", f"Could not export report:\n{exc}")

    def export_report_png(self) -> None:
        if not HAS_PIL:
            messagebox.showinfo(
                "Optional Dependency Missing",
                "Visual PNG export requires the optional 'Pillow' package (pip install Pillow).\n"
                "Use [EXPORT JSON] or [EXPORT CSV] for zero-dependency exports.",
            )
            return

        default_name = f"FocusFlow_Report_{datetime.date.today().strftime('%Y%m%d')}.png"
        export_path = filedialog.asksaveasfilename(
            parent=self.root,
            title="Export FocusFlow PNG Report",
            initialdir=self.state_dir,
            initialfile=default_name,
            defaultextension=".png",
            filetypes=[("PNG Image", "*.png")],
        )
        if not export_path:
            return

        try:
            summary = self.session_store.compute_analytics(days=7)
            img = Image.new("RGB", (920, 520), color="#1e1e2e")
            draw = ImageDraw.Draw(img)

            draw.rectangle([(0, 0), (920, 72)], fill="#181825")
            draw.text((36, 24), f"FOCUSFLOW v{__version__} // DEEP WORK TELEMETRY REPORT", fill="#a6e3a1")
            draw.text(
                (36, 94),
                f"Total Focus: {summary.total_hours:.1f} hrs ({summary.total_minutes} min)  |  "
                f"Streak: {summary.active_streak_days} days  |  "
                f"Completion: {summary.completion_rate:.1f}% ({summary.completed_sprints}/{summary.attempted_sprints})",
                fill="#cdd6f4",
            )

            max_val = max([d.minutes for d in summary.daily] + [1])
            base_y = 450
            chart_h = 280
            for idx, d in enumerate(summary.daily):
                x0 = 60 + idx * 115
                x1 = x0 + 68
                bh = int((d.minutes / max_val) * chart_h)
                y0 = base_y - bh
                draw.rectangle([(x0, base_y - chart_h), (x1, base_y)], fill="#313244")
                if bh > 0:
                    draw.rectangle([(x0, y0), (x1, base_y)], fill="#a6e3a1")
                draw.text((x0 + 10, base_y + 12), d.date[-5:], fill="#a6adc8")
                draw.text((x0 + 12, max(130, y0 - 18)), f"{d.minutes}m", fill="#cdd6f4")

            img.save(export_path)
            messagebox.showinfo("Export Complete", f"PNG report saved:\n{export_path}")
        except Exception as exc:
            messagebox.showerror("Export Failed", f"Could not save PNG report:\n{exc}")

    # ---------------- TAB 4: DISCIPLINE QUOTES ----------------
    def build_tab_quotes(self) -> None:
        f = self.tab_quotes

        card = tk.Frame(f, bg=THEME["surface0"], padx=30, pady=30)
        card.pack(fill="both", expand=True, padx=20, pady=20)

        tk.Label(
            card,
            text="DISCIPLINE MAXIMS",
            font=("Consolas", 11, "bold"),
            fg=THEME["peach"],
            bg=THEME["surface0"],
        ).pack(anchor="w")

        quotes = list(self.gui_prefs.get("quotes") or DEFAULT_QUOTES)
        self.quote_box = tk.Label(
            card,
            text=f'"{random.choice(quotes)}"',
            font=("Segoe UI", 15, "italic"),
            fg=THEME["text"],
            bg=THEME["mantle"],
            wraplength=780,
            justify="center",
            padx=30,
            pady=40,
        )
        self.quote_box.pack(fill="x", pady=24)

        btn_box = tk.Frame(card, bg=THEME["surface0"])
        btn_box.pack(anchor="w")

        tk.Button(
            btn_box,
            text="[NEXT MAXIM]",
            command=self.next_quote,
            font=("Segoe UI Bold", 10),
            bg=THEME["surface1"],
            fg=THEME["text"],
            relief="flat",
            padx=16,
            pady=8,
            cursor="hand2",
        ).pack(side="left", padx=(0, 10))

        tk.Button(
            btn_box,
            text="[+ ADD CUSTOM]",
            command=self.add_custom_quote,
            font=("Segoe UI Bold", 10),
            bg=THEME["surface1"],
            fg=THEME["mauve"],
            relief="flat",
            padx=16,
            pady=8,
            cursor="hand2",
        ).pack(side="left")

    def next_quote(self) -> None:
        quotes = list(self.gui_prefs.get("quotes") or DEFAULT_QUOTES)
        self.quote_box.config(text=f'"{random.choice(quotes)}"')

    def add_custom_quote(self) -> None:
        new_q = simpledialog.askstring("Add Discipline Maxim", "Enter a custom focus principle:", parent=self.root)
        if new_q and new_q.strip():
            cleaned = new_q.strip()
            quotes = list(self.gui_prefs.get("quotes") or DEFAULT_QUOTES)
            quotes.append(cleaned)
            self.gui_prefs["quotes"] = quotes
            self.save_gui_prefs()
            self.quote_box.config(text=f'"{cleaned}"')

    # ---------------- TAB 5: SYSTEM SETTINGS ----------------
    def build_tab_settings(self) -> None:
        f = self.tab_settings

        card = tk.Frame(f, bg=THEME["surface0"], padx=24, pady=20)
        card.pack(fill="both", expand=True, padx=20, pady=20)

        tk.Label(
            card,
            text="SYSTEM & TIMER CONFIGURATION",
            font=("Consolas", 11, "bold"),
            fg=THEME["green"],
            bg=THEME["surface0"],
        ).pack(anchor="w", pady=(0, 18))

        grid_f = tk.Frame(card, bg=THEME["surface0"])
        grid_f.pack(fill="x", pady=4)

        tk.Label(
            grid_f,
            text="Pomodoro Work Duration (min):",
            font=("Segoe UI", 10),
            fg=THEME["text"],
            bg=THEME["surface0"],
        ).grid(row=0, column=0, sticky="w", pady=8)
        self.spin_p_work = ttk.Spinbox(grid_f, from_=5, to=180, width=10)
        self.spin_p_work.set(int(self.gui_prefs.get("pomo_work", 25)))
        self.spin_p_work.grid(row=0, column=1, sticky="w", padx=16)

        tk.Label(
            grid_f,
            text="Pomodoro Break Duration (min):",
            font=("Segoe UI", 10),
            fg=THEME["text"],
            bg=THEME["surface0"],
        ).grid(row=1, column=0, sticky="w", pady=8)
        self.spin_p_break = ttk.Spinbox(grid_f, from_=1, to=60, width=10)
        self.spin_p_break.set(int(self.gui_prefs.get("pomo_break", 5)))
        self.spin_p_break.grid(row=1, column=1, sticky="w", padx=16)

        tk.Label(
            grid_f,
            text="Fixed Block Duration (min):",
            font=("Segoe UI", 10),
            fg=THEME["text"],
            bg=THEME["surface0"],
        ).grid(row=2, column=0, sticky="w", pady=8)
        self.spin_std = ttk.Spinbox(grid_f, from_=10, to=360, width=10)
        self.spin_std.set(int(self.gui_prefs.get("standard_duration", 60)))
        self.spin_std.grid(row=2, column=1, sticky="w", padx=16)

        tk.Label(
            grid_f,
            text="Daily Focus Goal (min):",
            font=("Segoe UI", 10),
            fg=THEME["text"],
            bg=THEME["surface0"],
        ).grid(row=3, column=0, sticky="w", pady=8)
        self.spin_daily_goal = ttk.Spinbox(grid_f, from_=5, to=1440, width=10)
        self.spin_daily_goal.set(int(self.config_store.daily_goal_minutes))
        self.spin_daily_goal.grid(row=3, column=1, sticky="w", padx=16)

        self.strict_var = tk.BooleanVar(value=bool(self.gui_prefs.get("strict_mode", True)))
        tk.Checkbutton(
            card,
            text="Enable Strict Friction Challenge on early abort & window close",
            variable=self.strict_var,
            font=("Segoe UI Semibold", 10),
            fg=THEME["text"],
            bg=THEME["surface0"],
            selectcolor=THEME["mantle"],
            activebackground=THEME["surface0"],
            activeforeground=THEME["green"],
        ).pack(anchor="w", pady=(14, 6))

        tk.Label(
            card,
            text=f"Unified State Directory: {self.state_dir}",
            font=("Consolas", 9),
            fg=THEME["subtext"],
            bg=THEME["surface0"],
        ).pack(anchor="w", pady=(4, 14))

        action_row = tk.Frame(card, bg=THEME["surface0"])
        action_row.pack(anchor="w", pady=6)

        tk.Button(
            action_row,
            text="[SAVE SETTINGS]",
            command=self.save_tab_settings,
            font=("Segoe UI Bold", 10),
            bg=THEME["green"],
            fg=THEME["crust"],
            relief="flat",
            padx=18,
            pady=8,
            cursor="hand2",
        ).pack(side="left", padx=(0, 10))

        tk.Button(
            action_row,
            text="[FLUSH DNS CACHE]",
            command=self.action_flush_dns,
            font=("Segoe UI Bold", 10),
            bg=THEME["surface1"],
            fg=THEME["sapphire"],
            relief="flat",
            padx=16,
            pady=8,
            cursor="hand2",
        ).pack(side="left")

    def action_elevate_admin(self) -> None:
        if is_elevated():
            return
        if self.is_active:
            messagebox.showwarning("Session Active", "Stop the current session before elevating privileges.")
            return
        if elevate_self(script_path=os.path.abspath(__file__)):
            self.worker_running = False
            self.root.destroy()
        else:
            messagebox.showwarning("Elevation Cancelled", "Administrator UAC prompt was cancelled or declined.")

    def action_flush_dns(self) -> None:
        flush_dns_async()
        messagebox.showinfo("DNS Cache Purge", "Asynchronous DNS resolver cache purge triggered.")

    def save_tab_settings(self) -> None:
        if self.is_active and self.is_strict_active() and not self.strict_var.get():
            self.strict_var.set(True)
            messagebox.showwarning("Shield Locked", "Strict mode cannot be disabled during an active focus sprint.")
            return
        try:
            p_work = max(1, int(self.spin_p_work.get()))
            p_break = max(1, int(self.spin_p_break.get()))
            std_dur = max(1, int(self.spin_std.get()))
            daily_goal = normalize_daily_goal_minutes(int(self.spin_daily_goal.get()))
            self.gui_prefs["pomo_work"] = p_work
            self.gui_prefs["pomo_break"] = p_break
            self.gui_prefs["standard_duration"] = std_dur
            self.gui_prefs["strict_mode"] = True if self.current_profile == "hardcore" else bool(self.strict_var.get())
            self.save_gui_prefs()

            cfg = load_config(state_dir=self.state_dir)
            cfg.daily_goal_minutes = daily_goal
            save_config(cfg, state_dir=self.state_dir)
            self.config_store = cfg
            self.spin_daily_goal.set(daily_goal)

            if not self.is_active:
                self.on_mode_change()
            self.refresh_analytics_ui()
            messagebox.showinfo("Saved", "Configuration updated.")
        except Exception as exc:
            messagebox.showerror("Invalid Input", f"Could not save settings:\n{exc}")

    # ---------------- SESSION LIFECYCLE & ENGINE INTEGRATION ----------------
    def _set_controls_locked_for_session(self, active: bool) -> None:
        rb_state = "disabled" if active else "normal"
        for rb in (*self.mode_radios, *self.profile_radios):
            rb.config(state=rb_state)
        if getattr(self, "profile_combo", None) is not None:
            self.profile_combo.config(state="disabled" if active else "readonly")

    def _verify_friction_challenge(self, reason: str = "STRICT SESSION UNLOCK") -> bool:
        dlg = FrictionChallengeDialog(self.root, reason_label=reason)
        self.root.wait_window(dlg)
        return dlg.passed

    def toggle_session(self) -> None:
        if not self.is_active:
            self.start_session()
        else:
            self.stop_session(verified=False)

    def start_session(self) -> None:
        prof = load_or_default(self.current_profile, state_dir=self.state_dir)
        self.active_profile_obj = prof

        m = self.mode_var.get()
        if m == "pomodoro":
            mins = int(self.gui_prefs.get("pomo_work", prof.default_work_minutes))
            self.pomo_state = "WORK"
            self.lbl_pomo_badge.config(
                text=f"POMODORO CYCLE {self.pomo_cycle} // {prof.name.upper()}",
                fg=THEME["green"],
            )
        elif m == "standard":
            mins = int(self.gui_prefs.get("standard_duration", 60))
            self.pomo_state = "WORK"
            self.lbl_pomo_badge.config(text=f"DEEP WORK // {prof.name.upper()}", fg=THEME["sapphire"])
        else:
            mins = int(self.gui_prefs.get("flex_duration", 90))
            self.pomo_state = "WORK"
            self.lbl_pomo_badge.config(text=f"EXTENDED // {prof.name.upper()}", fg=THEME["peach"])

        self.active_planned_minutes = mins
        self.session_blocked_count = 0
        self.lbl_mini_blocked.config(text="Session Blocks: 0")

        now_utc = datetime.datetime.now(datetime.timezone.utc)
        self.active_session_start_utc = now_utc
        self.active_session_id = generate_session_id()
        self.session_store.start_session(
            session_id=self.active_session_id,
            profile=prof.name,
            planned_minutes=mins,
            start_time=now_utc,
        )

        strict_level = self.current_strictness_level()
        with self._supervisor_lock:
            self._proc_supervisor = ProcessSupervisor(prof.blocked_processes)
            self._anticheat_guard = AntiCheatGuard(strict_level)

        self.phase_timer.start(mins * 60, phase="WORK")
        self.is_active = True
        self._set_controls_locked_for_session(True)

        self._engage_work_shield(mins)

        self.btn_toggle.config(text="[STOP SESSION]", bg=THEME["red"], fg=THEME["crust"])
        break_state = "disabled" if self.current_profile == "hardcore" else "normal"
        self.btn_break.config(text="[TAKE BREAK]", fg=THEME["yellow"], state=break_state)
        self.btn_emergency.config(state="normal")

    def _engage_work_shield(self, duration_minutes: int) -> None:
        prof = self.active_profile_obj
        all_domains = [*prof.blocked_domains, *DOH_BYPASS_DOMAINS]
        now_utc = datetime.datetime.now(datetime.timezone.utc)
        meta = SessionMeta(
            session_id=self.active_session_id,
            pid=os.getpid(),
            started_at=now_utc,
            expected_end_at=now_utc + datetime.timedelta(minutes=max(1, duration_minutes)),
            profile=prof.name,
        )
        with self._supervisor_lock:
            self._active_sinkhole_domains = all_domains
            self._active_session_meta = meta

        self.hosts_engaged = False
        try:
            if self.hosts_manager.apply_blocklist(all_domains, meta=meta):
                self.hosts_engaged = True
                flush_dns_async()
        except OSError:
            self.hosts_engaged = False

        if self.hosts_engaged:
            self.lbl_status.config(
                text="[ACTIVE] FULL SHIELD [HOSTS + PROC] — Dual-stack sinkhole & supervisor engaged",
                fg=THEME["green"],
            )
        else:
            self.lbl_status.config(
                text="[ACTIVE] PROCESS-ONLY [UNPRIVILEGED] — Process guard active (elevate for hosts sinkhole)",
                fg=THEME["yellow"],
            )

    def _release_work_shield_async(self) -> None:
        was_engaged = self.hosts_engaged
        self.hosts_engaged = False
        with self._supervisor_lock:
            self._active_sinkhole_domains = []
            self._active_session_meta = None

        if not was_engaged:
            return

        def _worker() -> None:
            try:
                self.hosts_manager.rollback()
            except Exception:
                pass
            flush_dns_async()

        threading.Thread(target=_worker, name="FocusFlowHostsRollback", daemon=True).start()

    def _handle_external_stop(self) -> None:
        if not self.is_active:
            return
        self.active_session_id = ""
        self.active_session_start_utc = None
        self.session_blocked_count = 0
        self.hosts_engaged = False
        with self._supervisor_lock:
            self._active_sinkhole_domains = []
            self._active_session_meta = None
        self.phase_timer.stop()
        self.is_active = False
        self.pomo_state = "WORK"
        self._set_controls_locked_for_session(False)
        self.btn_toggle.config(text="[START FOCUS]", bg=THEME["green"], fg=THEME["crust"])
        self.btn_break.config(text="[TAKE BREAK]", fg=THEME["yellow"], state="disabled")
        self.btn_emergency.config(state="disabled")
        self.lbl_mini_blocked.config(text="Session Blocks: 0")
        self.lbl_status.config(text="[STANDBY] Session stopped externally — unrestricted system access", fg=THEME["yellow"])
        self.on_mode_change()
        self.refresh_analytics_ui()

    def stop_session(self, verified: bool = False, completed: bool = False) -> bool:
        if not self.is_active:
            return True

        requires_challenge = (self.pomo_state == "WORK" and self.is_strict_active()) or (
            self.pomo_state == "BREAK" and self.current_strictness_level() == "hardcore"
        )
        if not completed and not verified and requires_challenge:
            if not self._verify_friction_challenge("CONFIRM EARLY SESSION ABORT"):
                return False

        if completed and self.active_session_id and not self.session_store.is_session_running(self.active_session_id):
            self._handle_external_stop()
            return True

        elapsed_min = max(0, self.phase_timer.elapsed_seconds() // 60)
        if completed:
            recorded_min = self.active_planned_minutes
        else:
            recorded_min = elapsed_min

        if self.active_session_id:
            self.session_store.finalize_session(
                session_id=self.active_session_id,
                profile=self.current_profile,
                duration_minutes=recorded_min,
                completed=completed,
                blocked_count=self.session_blocked_count,
                status="completed" if completed else "aborted",
                start_time=self.active_session_start_utc,
            )
            self.active_session_id = ""
            self.active_session_start_utc = None
            self.session_blocked_count = 0

        self.phase_timer.stop()
        self.is_active = False
        self.pomo_state = "WORK"
        self._release_work_shield_async()
        self._set_controls_locked_for_session(False)

        self.btn_toggle.config(text="[START FOCUS]", bg=THEME["green"], fg=THEME["crust"])
        self.btn_break.config(text="[TAKE BREAK]", fg=THEME["yellow"], state="disabled")
        self.btn_emergency.config(state="disabled")
        self.lbl_status.config(text="[STANDBY] Shield disengaged — unrestricted system access", fg=THEME["subtext"])
        self.on_mode_change()
        self.refresh_analytics_ui()
        return True

    def prompt_emergency(self) -> None:
        if not self.is_active:
            return
        if self._verify_friction_challenge("EMERGENCY SHIELD OVERRIDE"):
            self.stop_session(verified=True, completed=False)

    def give_break(self) -> None:
        if not self.is_active:
            return
        if self.pomo_state == "WORK":
            if self.is_strict_active() and not self.phase_timer.awaiting_transition:
                if not self._verify_friction_challenge("EARLY BREAK REQUEST"):
                    return
            if self.active_session_id:
                elapsed_min = max(0, self.phase_timer.elapsed_seconds() // 60)
                self.session_store.finalize_session(
                    session_id=self.active_session_id,
                    profile=self.current_profile,
                    duration_minutes=elapsed_min,
                    completed=False,
                    blocked_count=self.session_blocked_count,
                    status="aborted",
                    start_time=self.active_session_start_utc,
                )
                self.active_session_id = ""
                self.active_session_start_utc = None
                self.session_blocked_count = 0
                self.refresh_analytics_ui()
            self._enter_break_phase()
        else:
            self._enter_next_work_phase()

    def _enter_break_phase(self) -> None:
        self.pomo_state = "BREAK"
        mins = int(self.gui_prefs.get("pomo_break", 5))
        continuous_lockdown = self.current_strictness_level() == "hardcore"
        if continuous_lockdown:
            now_utc = datetime.datetime.now(datetime.timezone.utc)
            with self._supervisor_lock:
                domains = list(self._active_sinkhole_domains)
                meta = self._active_session_meta
                if meta is not None:
                    meta = SessionMeta(
                        session_id=meta.session_id,
                        pid=os.getpid(),
                        started_at=meta.started_at,
                        expected_end_at=now_utc + datetime.timedelta(minutes=max(1, mins)),
                        profile=meta.profile,
                    )
                    self._active_session_meta = meta
            if self.hosts_engaged and domains and meta is not None:
                try:
                    self.hosts_manager.apply_blocklist(domains, meta=meta)
                except OSError:
                    pass
            self.lbl_status.config(
                text="[BREAK] Hardcore lockdown continuous — distraction shield remains engaged",
                fg=THEME["yellow"],
            )
        else:
            self._release_work_shield_async()
            self.lbl_status.config(text="[BREAK] Hosts unlocked — step away from the screen", fg=THEME["blue"])
        self.phase_timer.start(mins * 60, phase="BREAK")
        self.lbl_pomo_badge.config(text=f"RECOVERY BREAK ({mins} MIN)", fg=THEME["blue"])
        self.lbl_timer.config(fg=THEME["blue"])
        self.btn_break.config(text="[RESUME WORK]", fg=THEME["green"], state="normal")

    def _enter_next_work_phase(self) -> None:
        self.pomo_state = "WORK"
        self.pomo_cycle += 1
        prof = load_or_default(self.current_profile, state_dir=self.state_dir)
        self.active_profile_obj = prof
        mins = int(self.gui_prefs.get("pomo_work", prof.default_work_minutes))
        self.active_planned_minutes = mins
        self.session_blocked_count = 0
        self.lbl_mini_blocked.config(text="Session Blocks: 0")

        now_utc = datetime.datetime.now(datetime.timezone.utc)
        self.active_session_start_utc = now_utc
        self.active_session_id = generate_session_id()
        self.session_store.start_session(
            session_id=self.active_session_id,
            profile=prof.name,
            planned_minutes=mins,
            start_time=now_utc,
        )

        strict_level = self.current_strictness_level()
        with self._supervisor_lock:
            self._proc_supervisor = ProcessSupervisor(prof.blocked_processes)
            self._anticheat_guard = AntiCheatGuard(strict_level)

        self.phase_timer.start(mins * 60, phase="WORK")
        self._engage_work_shield(mins)
        self.lbl_pomo_badge.config(
            text=f"POMODORO CYCLE {self.pomo_cycle} // {prof.name.upper()}",
            fg=THEME["green"],
        )
        self.lbl_timer.config(fg=THEME["green"])
        break_state = "disabled" if self.current_profile == "hardcore" else "normal"
        self.btn_break.config(text="[TAKE BREAK]", fg=THEME["yellow"], state=break_state)

    def _clock_worker(self) -> None:
        while self.worker_running:
            time.sleep(0.25)
            if not self.is_active or self.phase_timer.awaiting_transition:
                continue

            if self.phase_timer.poll_expired():
                self.root.after(0, self._on_phase_expired)
                continue

            rem = self.phase_timer.remaining_seconds()
            mins = rem // 60
            secs = rem % 60
            time_str = f"{mins:02d}:{secs:02d}"
            ratio = self.phase_timer.progress_ratio()
            self.root.after(0, lambda t=time_str, r=ratio: self._update_clock_ui(t, r))

    def _update_clock_ui(self, time_str: str, ratio: float) -> None:
        if not self.is_active:
            return
        self.lbl_timer.config(text=time_str)
        self.draw_progress_bar(ratio)

    def _on_phase_expired(self) -> None:
        if not self.is_active:
            return

        if self.current_mode == "pomodoro":
            if self.pomo_state == "WORK":
                if self.active_session_id and not self.session_store.is_session_running(self.active_session_id):
                    self._handle_external_stop()
                    return
                # Record completed sprint & release hosts (unless hardcore) BEFORE modal; start break timer AFTER modal closes!
                if self.active_session_id:
                    self.session_store.finalize_session(
                        session_id=self.active_session_id,
                        profile=self.current_profile,
                        duration_minutes=self.active_planned_minutes,
                        completed=True,
                        blocked_count=self.session_blocked_count,
                        status="completed",
                        start_time=self.active_session_start_utc,
                    )
                    self.active_session_id = ""
                    self.active_session_start_utc = None
                    self.session_blocked_count = 0
                if self.current_strictness_level() != "hardcore":
                    self._release_work_shield_async()
                self.lbl_timer.config(text="00:00")
                self.draw_progress_bar(0.0)
                self.refresh_analytics_ui()
                messagebox.showinfo(
                    "Work Sprint Complete",
                    "Focus sprint recorded. Click OK to begin your recovery break.",
                    parent=self.root,
                )
                if self.is_active:
                    self._enter_break_phase()
            else:
                self.lbl_timer.config(text="00:00")
                self.draw_progress_bar(0.0)
                messagebox.showinfo(
                    "Break Finished",
                    "Recovery break complete. Click OK to start the next focus cycle.",
                    parent=self.root,
                )
                if self.is_active:
                    self._enter_next_work_phase()
        else:
            self.stop_session(verified=True, completed=True)
            messagebox.showinfo(
                "Session Complete",
                "Deep work session completed and recorded.",
                parent=self.root,
            )

    def _supervisor_worker(self) -> None:
        tick = 0
        while self.worker_running:
            time.sleep(2.0)
            continuous_break = self.pomo_state == "BREAK" and self.current_strictness_level() == "hardcore"
            if not self.is_active or (self.pomo_state != "WORK" and not continuous_break):
                continue

            sid = self.active_session_id
            if self.pomo_state == "WORK" and sid and not self.session_store.is_session_running(sid):
                self.root.after(0, self._handle_external_stop)
                continue
            if continuous_break and self.hosts_engaged:
                try:
                    if self.hosts_manager.load_journal() is None:
                        self.root.after(0, self._handle_external_stop)
                        continue
                except Exception:
                    pass

            tick += 1
            with self._supervisor_lock:
                sup = self._proc_supervisor
                ac = self._anticheat_guard
                domains = list(self._active_sinkhole_domains)
                meta = self._active_session_meta
                strict_level = self.current_strictness_level()

            hits = 0
            try:
                killed = sup.scan_and_terminate()
                ac_killed = ac.enforce()
                hits += len(killed) + len(ac_killed)
            except Exception:
                pass

            # Periodic mid-session hosts tamper watchdog in standard & hardcore modes
            if self.hosts_engaged and domains and strict_level in ("standard", "hardcore") and tick % 3 == 0:
                try:
                    if self.hosts_manager.verify_or_reapply(domains, meta=meta):
                        hits += 1
                        flush_dns_async()
                except Exception:
                    pass

            if hits > 0 and self.pomo_state == "WORK":
                self.session_blocked_count += hits
                self.root.after(0, self._update_blocked_counter)

    def _update_blocked_counter(self) -> None:
        self.lbl_mini_blocked.config(text=f"Session Blocks: {self.session_blocked_count}")

    def on_close(self) -> None:
        if self.is_active:
            requires_challenge = (self.pomo_state == "WORK" and self.is_strict_active()) or (
                self.pomo_state == "BREAK" and self.current_strictness_level() == "hardcore"
            )
            if requires_challenge:
                if not self._verify_friction_challenge("WINDOW CLOSE DURING STRICT SPRINT"):
                    return
            else:
                if not messagebox.askyesno(
                    "Confirm Exit",
                    "An active session is in progress. Abort session and exit FocusFlow?",
                    parent=self.root,
                ):
                    return

            if self.active_session_id:
                elapsed_min = max(0, self.phase_timer.elapsed_seconds() // 60)
                self.session_store.finalize_session(
                    session_id=self.active_session_id,
                    profile=self.current_profile,
                    duration_minutes=elapsed_min,
                    completed=False,
                    blocked_count=self.session_blocked_count,
                    status="aborted",
                    start_time=self.active_session_start_utc,
                )
                self.active_session_id = ""
                self.session_blocked_count = 0
            if self.hosts_engaged:
                try:
                    self.hosts_manager.rollback()
                except Exception:
                    pass
                flush_dns_async()

        self.worker_running = False
        self.root.destroy()


def main() -> None:
    if "--elevate" in sys.argv and not is_elevated():
        if elevate_self(script_path=os.path.abspath(__file__)):
            return
    configure_windows_dpi()
    root = tk.Tk()
    FocusFlowGUI(root)
    root.mainloop()


if __name__ == "__main__":
    main()
