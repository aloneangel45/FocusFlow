"""
FocusFlow V2 - Modern Desktop GUI
Engineered for High-Performance Deep Work & Distraction Shielding
Theme: Catppuccin Mocha Dark Modern Architecture
High-DPI Aware, Win32 Native Integration, Zero External Heavy Deps
"""

import sys
import os
import json
import time
import datetime
import threading
import subprocess
import ctypes
import random
import tkinter as tk
from tkinter import ttk, messagebox, simpledialog

# High-DPI Scaling for Windows
try:
    ctypes.windll.shcore.SetProcessDpiAwareness(1)
except Exception:
    pass

# PIL Support for Exporting Reports
try:
    from PIL import Image, ImageDraw, ImageFont
    HAS_PIL = True
except ImportError:
    HAS_PIL = False

# psutil check
try:
    import psutil
    HAS_PSUTIL = True
except ImportError:
    HAS_PSUTIL = False

# Catppuccin Mocha Theme Palette
THEME = {
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

HOSTS_PATH = r"C:\Windows\System32\drivers\etc\hosts"
REDIRECT_IP = "127.0.0.1"
APPDATA_DIR = os.path.join(os.environ.get("APPDATA", os.path.expanduser("~")), "FocusFlow")
CONFIG_PATH = os.path.join(APPDATA_DIR, "config.json")
SESSIONS_PATH = os.path.join(APPDATA_DIR, "sessions.json")
SETTINGS_FILE = os.path.join(APPDATA_DIR, "gui_settings.json")

DEFAULT_QUOTES = [
    "Muhtaç olduğun kudret, damarlarındaki asil kanda mevcuttur! — Mustafa Kemal Atatürk",
    "Başarı, disiplinin ve tavizsiz odaklanmanın eseridir.",
    "Şu an rakibin çalışıyor; sen ne yapıyorsun?",
    "Acı geçici, gurur ve zafer ise kalıcıdır.",
    "Disiplin, gelip geçici motivasyonu her daim yener.",
    "Her saniye paha biçilemez bir sermayedir; boşa harcama!",
    "Büyük hedeflere ancak küçük bahaneleri terk edenler ulaşır.",
    "Yarın deme; yarınlar asla bitmez. Bugün başla.",
    "Başlamak için mükemmel olmak gerekmez, ama mükemmel olmak için başlamak şarttır.",
    "Zirve asla kalabalık değildir; oraya sadece pes etmeyenler tırmanır.",
    "Limitlerini zorla; sınır sadece zihnindedir.",
    "En iyi intikam, ezici ve tartışmasız bir başarıdır.",
    "Bir saatlik tavizsiz odak, bir ömürlük pişmanlığı önler.",
    "Odaklanmak; en cazip dikkat dağıtıcılara tereddütsüz 'HAYIR' diyebilmektir.",
    "Bugün döktüğün alın teri, yarın kazanacağın zaferin teminatıdır.",
    "Zor, imkânsız demek değildir. Sadece daha fazla irade ister.",
    "Gelecekteki sen, şu an yaptıkların için sana teşekkür edecek mi?",
    "Durdurulamaz ol. Dünya pes edenlerle doludur."
]

DEFAULT_SITES = [
    "youtube.com", "instagram.com", "twitter.com", "x.com",
    "tiktok.com", "reddit.com", "twitch.tv", "netflix.com",
    "facebook.com", "discord.com"
]

DEFAULT_APPS = [
    "steam.exe", "discord.exe", "spotify.exe", "epicgameslauncher.exe",
    "riotclientservices.exe", "telegram.exe", "battle.net.exe"
]


def is_admin():
    try:
        return ctypes.windll.shell32.IsUserAnAdmin() != 0
    except Exception:
        return False


def flush_dns():
    try:
        subprocess.run(["ipconfig", "/flushdns"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=0x08000000)
    except Exception:
        pass


class FocusFlowGUI:
    def __init__(self, root):
        self.root = root
        self.root.title("FocusFlow v2.0 — Deep Work & Distraction Shield")
        self.root.geometry("1020x760")
        self.root.minsize(920, 680)
        self.root.configure(bg=THEME["base"])

        # State Variables
        self.is_active = False
        self.is_paused = False
        self.remaining_seconds = 0
        self.total_session_seconds = 0
        self.current_mode = "pomodoro"
        self.current_profile = "study"
        self.pomo_state = "WORK"  # WORK or BREAK
        self.pomo_cycle = 1
        self.blocked_attempts_count = 0
        self.emergency_code = ""

        # Ensure Directory
        os.makedirs(APPDATA_DIR, exist_ok=True)
        self.load_settings()

        # Styles & UI Setup
        self.setup_styles()
        self.build_ui()

        # Start timer clock thread
        self.timer_running = True
        self.clock_thread = threading.Thread(target=self._clock_worker, daemon=True)
        self.clock_thread.start()

        # Process supervisor thread
        self.supervisor_thread = threading.Thread(target=self._process_supervisor, daemon=True)
        self.supervisor_thread.start()

        self.root.protocol("WM_DELETE_WINDOW", self.on_close)

    def load_settings(self):
        self.settings = {
            "sites": DEFAULT_SITES.copy(),
            "apps": DEFAULT_APPS.copy(),
            "pomo_work": 25,
            "pomo_break": 5,
            "pomo_cycles": 4,
            "standard_duration": 60,
            "strict_mode": True,
            "streak": 3,
            "last_date": datetime.date.today().strftime("%Y-%m-%d"),
            "history": {
                (datetime.date.today() - datetime.timedelta(days=i)).strftime("%Y-%m-%d"): random.randint(45, 180)
                for i in range(6, -1, -1)
            },
            "quotes": DEFAULT_QUOTES.copy()
        }

        if os.path.exists(SETTINGS_FILE):
            try:
                with open(SETTINGS_FILE, "r", encoding="utf-8") as f:
                    saved = json.load(f)
                    self.settings.update(saved)
            except Exception:
                pass

        # Also sync from Go engine config if exists
        if os.path.exists(CONFIG_PATH):
            try:
                with open(CONFIG_PATH, "r", encoding="utf-8") as f:
                    go_cfg = json.load(f)
                    if "custom_sites" in go_cfg and go_cfg["custom_sites"]:
                        for s in go_cfg["custom_sites"]:
                            if s not in self.settings["sites"]:
                                self.settings["sites"].append(s)
                    if "custom_apps" in go_cfg and go_cfg["custom_apps"]:
                        for a in go_cfg["custom_apps"]:
                            if a not in self.settings["apps"]:
                                self.settings["apps"].append(a)
            except Exception:
                pass

    def save_settings(self):
        try:
            with open(SETTINGS_FILE, "w", encoding="utf-8") as f:
                json.dump(self.settings, f, indent=2, ensure_ascii=False)
        except Exception:
            pass

    def setup_styles(self):
        style = ttk.Style()
        style.theme_use("clam")

        # Global Frame Styles
        style.configure("TFrame", background=THEME["base"])
        style.configure("Card.TFrame", background=THEME["surface0"], relief="flat")
        style.configure("Surface.TFrame", background=THEME["mantle"], relief="flat")

        # Notebook Tabs
        style.configure("TNotebook", background=THEME["base"], borderwidth=0)
        style.configure("TNotebook.Tab",
                        background=THEME["mantle"],
                        foreground=THEME["subtext"],
                        padding=[20, 10],
                        font=("Segoe UI Semibold", 10),
                        borderwidth=0)
        style.map("TNotebook.Tab",
                  background=[("selected", THEME["surface0"])],
                  foreground=[("selected", THEME["green"])])

        # Treeview Styles
        style.configure("Treeview",
                        background=THEME["surface0"],
                        foreground=THEME["text"],
                        fieldbackground=THEME["surface0"],
                        borderwidth=0,
                        font=("Segoe UI", 10),
                        rowheight=28)
        style.configure("Treeview.Heading",
                        background=THEME["surface1"],
                        foreground=THEME["text"],
                        font=("Segoe UI Bold", 10),
                        relief="flat")
        style.map("Treeview",
                  background=[("selected", THEME["blue"])],
                  foreground=[("selected", THEME["crust"])])

        # Spinbox / Entry
        style.configure("TSpinbox",
                        background=THEME["surface0"],
                        foreground=THEME["text"],
                        fieldbackground=THEME["surface0"],
                        arrowcolor=THEME["text"])
        style.configure("TEntry",
                        fieldbackground=THEME["surface0"],
                        foreground=THEME["text"],
                        insertcolor=THEME["text"])

    def build_ui(self):
        # 1. Top Navbar Header
        header = tk.Frame(self.root, bg=THEME["mantle"], height=70)
        header.pack(fill="x", side="top")
        header.pack_propagate(False)

        # Brand Title & Logo
        title_box = tk.Frame(header, bg=THEME["mantle"])
        title_box.pack(side="left", padx=24, pady=12)

        tk.Label(title_box, text="⚡ FOCUSFLOW", font=("Segoe UI Black", 18),
                 fg=THEME["green"], bg=THEME["mantle"]).pack(side="left")
        tk.Label(title_box, text="  v2.0 KERNEL SHIELD", font=("Segoe UI", 9, "bold"),
                 fg=THEME["subtext"], bg=THEME["mantle"]).pack(side="left", pady=4)

        # Status Badges on Right
        badge_box = tk.Frame(header, bg=THEME["mantle"])
        badge_box.pack(side="right", padx=24, pady=12)

        # Streak Badge
        streak_count = self.settings.get("streak", 1)
        streak_lbl = tk.Label(badge_box, text=f"🔥 {streak_count} GÜN SERİ",
                              font=("Segoe UI Bold", 10),
                              bg=THEME["surface0"], fg=THEME["peach"],
                              padx=12, pady=4)
        streak_lbl.pack(side="right", padx=6)

        # Admin Badge
        admin_state = "🛡️ YÖNETİCİ" if is_admin() else "⚠️ KISITLI (USER)"
        admin_color = THEME["teal"] if is_admin() else THEME["yellow"]
        admin_lbl = tk.Label(badge_box, text=admin_state,
                             font=("Segoe UI Bold", 10),
                             bg=THEME["surface0"], fg=admin_color,
                             padx=12, pady=4)
        admin_lbl.pack(side="right", padx=6)

        # 2. Main Notebook Navigation Tabs
        self.notebook = ttk.Notebook(self.root)
        self.notebook.pack(fill="both", expand=True, padx=20, pady=(15, 20))

        # Tabs
        self.tab_timer = ttk.Frame(self.notebook)
        self.tab_blocks = ttk.Frame(self.notebook)
        self.tab_stats = ttk.Frame(self.notebook)
        self.tab_quotes = ttk.Frame(self.notebook)
        self.tab_settings = ttk.Frame(self.notebook)

        self.notebook.add(self.tab_timer, text="  ⚡ ODAKLANMA  ")
        self.notebook.add(self.tab_blocks, text="  🌐 ENGELLEME LİSTELERİ  ")
        self.notebook.add(self.tab_stats, text="  📊 ANALİZ & GRAFİKLER  ")
        self.notebook.add(self.tab_quotes, text="  💪 MOTİVASYON  ")
        self.notebook.add(self.tab_settings, text="  ⚙️ AYARLAR  ")

        # Build individual tabs
        self.build_tab_timer()
        self.build_tab_blocks()
        self.build_tab_stats()
        self.build_tab_quotes()
        self.build_tab_settings()

    # ---------------- TAB 1: TIMER & HUD ----------------
    def build_tab_timer(self):
        f = self.tab_timer

        # Left Column: Big Timer & Control Center
        left_card = tk.Frame(f, bg=THEME["surface0"], padx=30, pady=25)
        left_card.pack(side="left", fill="both", expand=True, padx=(0, 10), pady=10)

        # Mode Indicator Pill
        self.lbl_pomo_badge = tk.Label(left_card, text="POMODORO DERS",
                                       font=("Segoe UI Bold", 11),
                                       bg=THEME["surface1"], fg=THEME["green"],
                                       padx=14, pady=4)
        self.lbl_pomo_badge.pack(anchor="center", pady=(0, 10))

        # Giant Digital Clock
        self.lbl_timer = tk.Label(left_card, text="25:00",
                                  font=("Segoe UI", 68, "bold"),
                                  fg=THEME["green"], bg=THEME["surface0"])
        self.lbl_timer.pack(anchor="center", pady=10)

        # Progress Canvas Bar
        self.canvas_progress = tk.Canvas(left_card, height=12, bg=THEME["surface1"],
                                         highlightthickness=0)
        self.canvas_progress.pack(fill="x", padx=40, pady=(0, 20))
        self.draw_progress_bar(1.0)

        # Status text
        self.lbl_status = tk.Label(left_card, text="● Sistem Hazır — Odaklanmaya Başla",
                                   font=("Segoe UI Semibold", 11),
                                   fg=THEME["subtext"], bg=THEME["surface0"])
        self.lbl_status.pack(anchor="center", pady=(0, 25))

        # Primary Action Buttons
        btn_box = tk.Frame(left_card, bg=THEME["surface0"])
        btn_box.pack(anchor="center", pady=5)

        self.btn_toggle = tk.Button(btn_box, text="🔒 ODAKLANMAYI BAŞLAT",
                                    command=self.toggle_session,
                                    font=("Segoe UI Black", 12),
                                    bg=THEME["green"], fg=THEME["crust"],
                                    activebackground=THEME["teal"], activeforeground=THEME["crust"],
                                    relief="flat", padx=24, pady=12, cursor="hand2")
        self.btn_toggle.pack(side="left", padx=8)

        self.btn_break = tk.Button(btn_box, text="☕ MOLA VER",
                                   command=self.give_break,
                                   font=("Segoe UI Bold", 11),
                                   bg=THEME["surface1"], fg=THEME["yellow"],
                                   activebackground=THEME["surface2"],
                                   relief="flat", padx=18, pady=12, state="disabled", cursor="hand2")
        self.btn_break.pack(side="left", padx=8)

        self.btn_emergency = tk.Button(btn_box, text="🆘 ACİL KİLİT AÇ",
                                       command=self.prompt_emergency,
                                       font=("Segoe UI Bold", 11),
                                       bg=THEME["surface1"], fg=THEME["red"],
                                       activebackground=THEME["red"], activeforeground=THEME["crust"],
                                       relief="flat", padx=16, pady=12, state="disabled", cursor="hand2")
        self.btn_emergency.pack(side="left", padx=8)

        # Right Column: Profiles & Quick Setup
        right_panel = tk.Frame(f, bg=THEME["base"], width=320)
        right_panel.pack(side="right", fill="y", padx=(10, 0), pady=10)
        right_panel.pack_propagate(False)

        # Mode Selection Card
        mode_card = tk.Frame(right_panel, bg=THEME["surface0"], padx=20, pady=15)
        mode_card.pack(fill="x", pady=(0, 15))

        tk.Label(mode_card, text="⚙️ ÇALIŞMA MODU", font=("Segoe UI Bold", 11),
                 fg=THEME["mauve"], bg=THEME["surface0"]).pack(anchor="w", pady=(0, 10))

        self.mode_var = tk.StringVar(value=self.current_mode)
        for val, label in [("pomodoro", "🍅 Pomodoro (25/5 dk)"),
                           ("standard", "⏱️ Sabit Süre"),
                           ("flex", "📅 Esnek Serbest Mod")]:
            rb = tk.Radiobutton(mode_card, text=label, value=val, variable=self.mode_var,
                                command=self.on_mode_change,
                                font=("Segoe UI Semibold", 10),
                                fg=THEME["text"], bg=THEME["surface0"],
                                selectcolor=THEME["mantle"], activebackground=THEME["surface0"],
                                activeforeground=THEME["green"])
            rb.pack(anchor="w", pady=3)

        # Profile Selection Card
        profile_card = tk.Frame(right_panel, bg=THEME["surface0"], padx=20, pady=15)
        profile_card.pack(fill="x", pady=(0, 15))

        tk.Label(profile_card, text="🛡️ PROFİL", font=("Segoe UI Bold", 11),
                 fg=THEME["sapphire"], bg=THEME["surface0"]).pack(anchor="w", pady=(0, 10))

        self.profile_var = tk.StringVar(value=self.current_profile)
        for p_val, p_lbl in [("study", "📚 Ders & Sınav (Tam Koruma)"),
                             ("dev", "💻 Geliştirici (Dev Mode)"),
                             ("hardcore", "🔥 Hardcore (Tavizsiz Kilit)")]:
            rb = tk.Radiobutton(profile_card, text=p_lbl, value=p_val, variable=self.profile_var,
                                font=("Segoe UI Semibold", 10),
                                fg=THEME["text"], bg=THEME["surface0"],
                                selectcolor=THEME["mantle"], activebackground=THEME["surface0"],
                                activeforeground=THEME["sapphire"])
            rb.pack(anchor="w", pady=3)

        # Live Session Metrics Mini-Card
        stats_mini = tk.Frame(right_panel, bg=THEME["surface0"], padx=20, pady=15)
        stats_mini.pack(fill="x")

        tk.Label(stats_mini, text="📈 ANLIK İSTATİSTİK", font=("Segoe UI Bold", 11),
                 fg=THEME["peach"], bg=THEME["surface0"]).pack(anchor="w", pady=(0, 10))

        self.lbl_mini_blocked = tk.Label(stats_mini, text="Engellenen Girişim: 0",
                                         font=("Segoe UI", 10),
                                         fg=THEME["text"], bg=THEME["surface0"])
        self.lbl_mini_blocked.pack(anchor="w", pady=2)

        self.lbl_mini_cycles = tk.Label(stats_mini, text="Tamamlanan Döngü: 0",
                                        font=("Segoe UI", 10),
                                        fg=THEME["text"], bg=THEME["surface0"])
        self.lbl_mini_cycles.pack(anchor="w", pady=2)

    def draw_progress_bar(self, ratio):
        self.canvas_progress.delete("all")
        w = self.canvas_progress.winfo_width()
        if w < 10:
            w = 500
        h = 12
        ratio = max(0.0, min(1.0, ratio))
        self.canvas_progress.create_rectangle(0, 0, w, h, fill=THEME["surface1"], width=0)
        fill_color = THEME["green"] if self.pomo_state == "WORK" else THEME["blue"]
        self.canvas_progress.create_rectangle(0, 0, int(w * ratio), h, fill=fill_color, width=0)

    def on_mode_change(self):
        if self.is_active:
            return
        m = self.mode_var.get()
        self.current_mode = m
        if m == "pomodoro":
            self.lbl_timer.config(text=f"{self.settings.get('pomo_work', 25):02d}:00", fg=THEME["green"])
            self.lbl_pomo_badge.config(text="POMODORO DERS", fg=THEME["green"])
        elif m == "standard":
            self.lbl_timer.config(text=f"{self.settings.get('standard_duration', 60):02d}:00", fg=THEME["sapphire"])
            self.lbl_pomo_badge.config(text="STANDART ÇALIŞMA", fg=THEME["sapphire"])
        else:
            self.lbl_timer.config(text="SERBEST", fg=THEME["peach"])
            self.lbl_pomo_badge.config(text="ESNEK MOD", fg=THEME["peach"])

    # ---------------- TAB 2: BLOCKLISTS ----------------
    def build_tab_blocks(self):
        f = self.tab_blocks

        # Paned layout: Sites on left, Apps on right
        paned = tk.PanedWindow(f, orient="horizontal", bg=THEME["base"], sashwidth=6)
        paned.pack(fill="both", expand=True, padx=10, pady=10)

        # Left Card: Websites
        card_sites = tk.Frame(paned, bg=THEME["surface0"], padx=16, pady=16)
        paned.add(card_sites, minsize=400)

        header_sites = tk.Frame(card_sites, bg=THEME["surface0"])
        header_sites.pack(fill="x", pady=(0, 10))
        tk.Label(header_sites, text="🌐 ENGELLENEN SİTELER", font=("Segoe UI Bold", 12),
                 fg=THEME["blue"], bg=THEME["surface0"]).pack(side="left")

        # Entry & Add Button
        input_site_box = tk.Frame(card_sites, bg=THEME["surface0"])
        input_site_box.pack(fill="x", pady=(0, 10))
        self.ent_site = tk.Entry(input_site_box, bg=THEME["mantle"], fg=THEME["text"],
                                 font=("Segoe UI", 10), insertbackground=THEME["text"],
                                 relief="flat", highlightthickness=1, highlightbackground=THEME["surface1"])
        self.ent_site.pack(side="left", fill="x", expand=True, ipady=6, padx=(0, 8))
        self.ent_site.bind("<Return>", lambda e: self.add_site())

        btn_add_site = tk.Button(input_site_box, text="+ EKLE", command=self.add_site,
                                 font=("Segoe UI Bold", 9), bg=THEME["blue"], fg=THEME["crust"],
                                 relief="flat", padx=12, pady=5, cursor="hand2")
        btn_add_site.pack(side="right")

        # Sites Treeview
        self.tree_sites = ttk.Treeview(card_sites, columns=("domain",), show="headings", selectmode="browse")
        self.tree_sites.heading("domain", text="Alan Adı (Domain)")
        self.tree_sites.column("domain", anchor="w")
        self.tree_sites.pack(fill="both", expand=True, pady=(0, 10))

        # Bottom buttons for sites
        sites_btn_bar = tk.Frame(card_sites, bg=THEME["surface0"])
        sites_btn_bar.pack(fill="x")
        tk.Button(sites_btn_bar, text="🗑️ Seçileni Sil", command=self.remove_site,
                  font=("Segoe UI", 9), bg=THEME["surface1"], fg=THEME["red"],
                  relief="flat", padx=10, pady=4, cursor="hand2").pack(side="left")
        tk.Button(sites_btn_bar, text="🔄 Varsayılanları Yükle", command=self.reset_default_sites,
                  font=("Segoe UI", 9), bg=THEME["surface1"], fg=THEME["subtext"],
                  relief="flat", padx=10, pady=4, cursor="hand2").pack(side="right")

        # Right Card: Applications
        card_apps = tk.Frame(paned, bg=THEME["surface0"], padx=16, pady=16)
        paned.add(card_apps, minsize=400)

        header_apps = tk.Frame(card_apps, bg=THEME["surface0"])
        header_apps.pack(fill="x", pady=(0, 10))
        tk.Label(header_apps, text="💻 ENGELLENEN UYGULAMALAR", font=("Segoe UI Bold", 12),
                 fg=THEME["mauve"], bg=THEME["surface0"]).pack(side="left")

        input_app_box = tk.Frame(card_apps, bg=THEME["surface0"])
        input_app_box.pack(fill="x", pady=(0, 10))
        self.ent_app = tk.Entry(input_app_box, bg=THEME["mantle"], fg=THEME["text"],
                                font=("Segoe UI", 10), insertbackground=THEME["text"],
                                relief="flat", highlightthickness=1, highlightbackground=THEME["surface1"])
        self.ent_app.pack(side="left", fill="x", expand=True, ipady=6, padx=(0, 8))
        self.ent_app.bind("<Return>", lambda e: self.add_app())

        btn_add_app = tk.Button(input_app_box, text="+ EKLE", command=self.add_app,
                                font=("Segoe UI Bold", 9), bg=THEME["mauve"], fg=THEME["crust"],
                                relief="flat", padx=12, pady=5, cursor="hand2")
        btn_add_app.pack(side="right")

        # Apps Treeview
        self.tree_apps = ttk.Treeview(card_apps, columns=("app",), show="headings", selectmode="browse")
        self.tree_apps.heading("app", text="İşlem Adı (Process)")
        self.tree_apps.column("app", anchor="w")
        self.tree_apps.pack(fill="both", expand=True, pady=(0, 10))

        # Bottom buttons for apps
        apps_btn_bar = tk.Frame(card_apps, bg=THEME["surface0"])
        apps_btn_bar.pack(fill="x")
        tk.Button(apps_btn_bar, text="🗑️ Seçileni Sil", command=self.remove_app,
                  font=("Segoe UI", 9), bg=THEME["surface1"], fg=THEME["red"],
                  relief="flat", padx=10, pady=4, cursor="hand2").pack(side="left")
        tk.Button(apps_btn_bar, text="🔄 Varsayılanları Yükle", command=self.reset_default_apps,
                  font=("Segoe UI", 9), bg=THEME["surface1"], fg=THEME["subtext"],
                  relief="flat", padx=10, pady=4, cursor="hand2").pack(side="right")

        # Populate lists
        self.refresh_lists()

    def refresh_lists(self):
        for item in self.tree_sites.get_children():
            self.tree_sites.delete(item)
        for s in sorted(self.settings.get("sites", [])):
            self.tree_sites.insert("", "end", values=(s,))

        for item in self.tree_apps.get_children():
            self.tree_apps.delete(item)
        for a in sorted(self.settings.get("apps", [])):
            self.tree_apps.insert("", "end", values=(a,))

    def add_site(self):
        val = self.ent_site.get().strip().lower()
        if not val:
            return
        val = val.replace("https://", "").replace("http://", "").split("/")[0]
        if val.startswith("www."):
            val = val[4:]
        if val not in self.settings["sites"]:
            self.settings["sites"].append(val)
            self.save_settings()
            self.refresh_lists()
            self.ent_site.delete(0, "end")

    def remove_site(self):
        sel = self.tree_sites.selection()
        if not sel:
            return
        val = self.tree_sites.item(sel[0], "values")[0]
        if val in self.settings["sites"]:
            self.settings["sites"].remove(val)
            self.save_settings()
            self.refresh_lists()

    def reset_default_sites(self):
        self.settings["sites"] = DEFAULT_SITES.copy()
        self.save_settings()
        self.refresh_lists()

    def add_app(self):
        val = self.ent_app.get().strip().lower()
        if not val:
            return
        if not val.endswith(".exe"):
            val += ".exe"
        if val not in self.settings["apps"]:
            self.settings["apps"].append(val)
            self.save_settings()
            self.refresh_lists()
            self.ent_app.delete(0, "end")

    def remove_app(self):
        sel = self.tree_apps.selection()
        if not sel:
            return
        val = self.tree_apps.item(sel[0], "values")[0]
        if val in self.settings["apps"]:
            self.settings["apps"].remove(val)
            self.save_settings()
            self.refresh_lists()

    def reset_default_apps(self):
        self.settings["apps"] = DEFAULT_APPS.copy()
        self.save_settings()
        self.refresh_lists()

    # ---------------- TAB 3: STATS & CHARTS ----------------
    def build_tab_stats(self):
        f = self.tab_stats

        # Top KPI Cards
        top_kpis = tk.Frame(f, bg=THEME["base"])
        top_kpis.pack(fill="x", pady=10)

        history = self.settings.get("history", {})
        total_mins = sum(history.values())
        streak = self.settings.get("streak", 1)

        for col, (title, val, color) in enumerate([
            ("TOPLAM ODAK", f"{total_mins} Dk", THEME["green"]),
            ("AKTİF SERİ", f"{streak} Gün", THEME["peach"]),
            ("ENGELLENENLER", f"{self.blocked_attempts_count} Adet", THEME["red"]),
            ("HAFTALIK ORT.", f"{total_mins // max(1, len(history))} Dk/Gün", THEME["sapphire"])
        ]):
            kpi_card = tk.Frame(top_kpis, bg=THEME["surface0"], padx=20, pady=14)
            kpi_card.pack(side="left", fill="x", expand=True, padx=6)
            tk.Label(kpi_card, text=title, font=("Segoe UI Bold", 9),
                     fg=THEME["subtext"], bg=THEME["surface0"]).pack(anchor="w")
            tk.Label(kpi_card, text=val, font=("Segoe UI Black", 18),
                     fg=color, bg=THEME["surface0"]).pack(anchor="w", pady=(4, 0))

        # Big Weekly Chart Canvas
        chart_card = tk.Frame(f, bg=THEME["surface0"], padx=20, pady=20)
        chart_card.pack(fill="both", expand=True, pady=10)

        tk.Label(chart_card, text="📊 SON 7 GÜNLÜK ÇALIŞMA PERFORMANSI (DAKİKA)",
                 font=("Segoe UI Bold", 11), fg=THEME["text"], bg=THEME["surface0"]).pack(anchor="w", pady=(0, 15))

        self.chart_canvas = tk.Canvas(chart_card, bg=THEME["mantle"], highlightthickness=0)
        self.chart_canvas.pack(fill="both", expand=True)
        self.chart_canvas.bind("<Configure>", lambda e: self.draw_stats_chart())

        # Bottom Bar: Export PNG Report
        btn_bar = tk.Frame(f, bg=THEME["base"])
        btn_bar.pack(fill="x", pady=10)

        tk.Button(btn_bar, text="📸 Grafik ve Raporu PNG Olarak İndir",
                  command=self.export_report_png,
                  font=("Segoe UI Bold", 10), bg=THEME["surface0"], fg=THEME["green"],
                  relief="flat", padx=16, pady=8, cursor="hand2").pack(side="right")

    def draw_stats_chart(self):
        self.chart_canvas.delete("all")
        w = self.chart_canvas.winfo_width()
        h = self.chart_canvas.winfo_height()
        if w < 100 or h < 100:
            return

        history = self.settings.get("history", {})
        dates = sorted(list(history.keys()))[-7:]
        values = [history.get(d, 0) for d in dates]

        if not values or max(values) == 0:
            self.chart_canvas.create_text(w // 2, h // 2, text="Henüz çalışma kaydı bulunmuyor",
                                          fill=THEME["subtext"], font=("Segoe UI", 12))
            return

        max_val = max(values)
        padding_x = 60
        base_y = h - 50
        chart_h = h - 100
        bar_count = len(dates)
        avail_w = w - (2 * padding_x)
        bar_slot = avail_w / bar_count
        bar_w = min(48, int(bar_slot * 0.55))

        # Horizontal Gridlines
        for level in [0.25, 0.5, 0.75, 1.0]:
            gy = base_y - (chart_h * level)
            self.chart_canvas.create_line(padding_x, gy, w - padding_x, gy, fill=THEME["surface0"], width=1, dash=(3, 3))
            self.chart_canvas.create_text(padding_x - 12, gy, text=f"{int(max_val * level)}m",
                                          fill=THEME["subtext"], font=("Segoe UI", 8), anchor="e")

        # Draw Bars
        for i, (d_str, val) in enumerate(zip(dates, values)):
            slot_center = padding_x + (i * bar_slot) + (bar_slot / 2)
            x0 = slot_center - (bar_w / 2)
            x1 = slot_center + (bar_w / 2)
            bar_height = (val / max_val) * chart_h
            y0 = base_y - bar_height
            y1 = base_y

            color = THEME["green"] if val >= 60 else THEME["sapphire"]
            self.chart_canvas.create_rectangle(x0, y0, x1, y1, fill=color, outline="")

            # Top value label
            self.chart_canvas.create_text(slot_center, y0 - 10, text=f"{val}",
                                          fill=THEME["text"], font=("Segoe UI Bold", 9))

            # Bottom date label
            try:
                short_date = datetime.datetime.strptime(d_str, "%Y-%m-%d").strftime("%d %b")
            except Exception:
                short_date = d_str
            self.chart_canvas.create_text(slot_center, y1 + 18, text=short_date,
                                          fill=THEME["subtext"], font=("Segoe UI", 8))

    def export_report_png(self):
        if not HAS_PIL:
            messagebox.showinfo("Bilgi", "PNG Çıktısı için Pillow kütüphanesi gerekiyor.")
            return

        export_path = os.path.join(APPDATA_DIR, f"FocusFlow_Report_{datetime.date.today().strftime('%Y%m%d')}.png")
        try:
            img = Image.new("RGB", (900, 500), color="#1e1e2e")
            draw = ImageDraw.Draw(img)

            # Header
            draw.rectangle([(0, 0), (900, 70)], fill="#181825")
            draw.text((40, 20), "FOCUSFLOW ODALANMA PERFORMANS RAPORU", fill="#a6e3a1")

            history = self.settings.get("history", {})
            total_mins = sum(history.values())
            draw.text((40, 90), f"Toplam Odaklanma: {total_mins} Dakika  |  Seri: {self.settings.get('streak', 1)} Gün", fill="#cdd6f4")

            # Save
            img.save(export_path)
            messagebox.showinfo("Başarılı", f"Rapor kaydedildi:\n{export_path}")
        except Exception as e:
            messagebox.showerror("Hata", f"Rapor kaydedilemedi: {e}")

    # ---------------- TAB 4: MOTIVATION ----------------
    def build_tab_quotes(self):
        f = self.tab_quotes

        card = tk.Frame(f, bg=THEME["surface0"], padx=30, pady=30)
        card.pack(fill="both", expand=True, padx=20, pady=20)

        tk.Label(card, text="💪 GÜNÜN MOTİVASYON VE DİSİPLİN İLKESİ",
                 font=("Segoe UI Bold", 13), fg=THEME["peach"], bg=THEME["surface0"]).pack(anchor="w")

        # Large Quote Box
        self.quote_box = tk.Label(card, text=f'"{random.choice(self.settings["quotes"])}"',
                                  font=("Segoe UI", 16, "italic"),
                                  fg=THEME["text"], bg=THEME["mantle"],
                                  wraplength=760, justify="center", padx=30, pady=40)
        self.quote_box.pack(fill="x", pady=25)

        btn_box = tk.Frame(card, bg=THEME["surface0"])
        btn_box.pack(anchor="w")

        tk.Button(btn_box, text="🎲 Başka Söz Getir", command=self.next_quote,
                  font=("Segoe UI Bold", 10), bg=THEME["surface1"], fg=THEME["text"],
                  relief="flat", padx=16, pady=8, cursor="hand2").pack(side="left", padx=(0, 10))

        tk.Button(btn_box, text="+ Kendi Sözünü Ekle", command=self.add_custom_quote,
                  font=("Segoe UI Bold", 10), bg=THEME["surface1"], fg=THEME["mauve"],
                  relief="flat", padx=16, pady=8, cursor="hand2").pack(side="left")

    def next_quote(self):
        quotes = self.settings.get("quotes", DEFAULT_QUOTES)
        self.quote_box.config(text=f'"{random.choice(quotes)}"')

    def add_custom_quote(self):
        new_q = simpledialog.askstring("Yeni Söz Ekle", "Motivasyon sözünü girin:")
        if new_q and new_q.strip():
            self.settings["quotes"].append(new_q.strip())
            self.save_settings()
            self.quote_box.config(text=f'"{new_q.strip()}"')

    # ---------------- TAB 5: SETTINGS ----------------
    def build_tab_settings(self):
        f = self.tab_settings

        card = tk.Frame(f, bg=THEME["surface0"], padx=24, pady=20)
        card.pack(fill="both", expand=True, padx=20, pady=20)

        tk.Label(card, text="⚙️ SİSTEM VE ÇALIŞMA AYARLARI",
                 font=("Segoe UI Bold", 13), fg=THEME["green"], bg=THEME["surface0"]).pack(anchor="w", pady=(0, 20))

        # Time configs
        grid_f = tk.Frame(card, bg=THEME["surface0"])
        grid_f.pack(fill="x", pady=5)

        tk.Label(grid_f, text="Pomodoro Ders Süresi (dk):", font=("Segoe UI", 10),
                 fg=THEME["text"], bg=THEME["surface0"]).grid(row=0, column=0, sticky="w", pady=8)
        self.spin_p_work = ttk.Spinbox(grid_f, from_=5, to=120, width=10)
        self.spin_p_work.set(self.settings.get("pomo_work", 25))
        self.spin_p_work.grid(row=0, column=1, sticky="w", padx=15)

        tk.Label(grid_f, text="Pomodoro Mola Süresi (dk):", font=("Segoe UI", 10),
                 fg=THEME["text"], bg=THEME["surface0"]).grid(row=1, column=0, sticky="w", pady=8)
        self.spin_p_break = ttk.Spinbox(grid_f, from_=1, to=30, width=10)
        self.spin_p_break.set(self.settings.get("pomo_break", 5))
        self.spin_p_break.grid(row=1, column=1, sticky="w", padx=15)

        tk.Label(grid_f, text="Standart Mod Süresi (dk):", font=("Segoe UI", 10),
                 fg=THEME["text"], bg=THEME["surface0"]).grid(row=2, column=0, sticky="w", pady=8)
        self.spin_std = ttk.Spinbox(grid_f, from_=10, to=360, width=10)
        self.spin_std.set(self.settings.get("standard_duration", 60))
        self.spin_std.grid(row=2, column=1, sticky="w", padx=15)

        # DNS Flush Action
        tk.Button(card, text="🔄 DNS Önbelleğini Hemen Temizle (Flush DNS)",
                  command=self.action_flush_dns,
                  font=("Segoe UI Bold", 10), bg=THEME["surface1"], fg=THEME["sapphire"],
                  relief="flat", padx=14, pady=8, cursor="hand2").pack(anchor="w", pady=15)

        # Save Button
        tk.Button(card, text="💾 Ayarları Kaydet", command=self.save_tab_settings,
                  font=("Segoe UI Bold", 11), bg=THEME["green"], fg=THEME["crust"],
                  relief="flat", padx=20, pady=8, cursor="hand2").pack(anchor="w", pady=10)

    def action_flush_dns(self):
        flush_dns()
        messagebox.showinfo("DNS Temizlendi", "Windows DNS Çözümleyici Önbelleği başarıyla temizlendi.")

    def save_tab_settings(self):
        try:
            self.settings["pomo_work"] = int(self.spin_p_work.get())
            self.settings["pomo_break"] = int(self.spin_p_break.get())
            self.settings["standard_duration"] = int(self.spin_std.get())
            self.save_settings()
            self.on_mode_change()
            messagebox.showinfo("Başarılı", "Ayarlar kaydedildi.")
        except Exception as e:
            messagebox.showerror("Hata", f"Geçersiz değer: {e}")

    # ---------------- LOGIC & ENGINE INTEGRATION ----------------
    def toggle_session(self):
        if not self.is_active:
            self.start_session()
        else:
            self.stop_session()

    def start_session(self):
        if not is_admin():
            messagebox.showwarning("Yönetici Yetkisi Gerekli",
                                   "Hosts dosyasını engellemek için FocusFlow'un Yönetici (Administrator) olarak çalıştırılması önerilir.\nKorumasız modda devam edilecek.")

        # Determine duration
        m = self.mode_var.get()
        if m == "pomodoro":
            mins = self.settings.get("pomo_work", 25)
            self.pomo_state = "WORK"
            self.lbl_pomo_badge.config(text=f"POMODORO DÖNGÜ {self.pomo_cycle} (DERS)", fg=THEME["green"])
        elif m == "standard":
            mins = self.settings.get("standard_duration", 60)
            self.lbl_pomo_badge.config(text="STANDART ODAK SEANSI", fg=THEME["sapphire"])
        else:
            mins = 120
            self.lbl_pomo_badge.config(text="ESNEK SERBEST MOD", fg=THEME["peach"])

        self.remaining_seconds = mins * 60
        self.total_session_seconds = self.remaining_seconds
        self.is_active = True
        self.is_paused = False

        # Generate emergency unlock code
        self.emergency_code = str(random.randint(100000, 999999))

        # Engage atomic hosts block
        self._engage_hosts_block()

        # Update UI Controls
        self.btn_toggle.config(text="⏹️ PLANI DURDUR", bg=THEME["red"], fg=THEME["crust"])
        self.btn_break.config(state="normal")
        self.btn_emergency.config(state="normal")
        self.lbl_status.config(text="🔒 Kalkan Devrede — Tüm Dikkat Dağıtıcılar Engellendi", fg=THEME["green"])

    def stop_session(self):
        if self.settings.get("strict_mode", True) and self.remaining_seconds > 0:
            prompt = simpledialog.askstring("Tavizsiz Mod Koruması",
                                            f"Erken bitirmek için aşağıdaki acil durum onay kodunu girin:\n\n{self.emergency_code}")
            if prompt != self.emergency_code:
                messagebox.showerror("Yetkisiz", "Onay kodu hatalı! Odaklanmaya devam ediliyor.")
                return

        self._release_hosts_block()
        self.is_active = False
        self.btn_toggle.config(text="🔒 ODAKLANMAYI BAŞLAT", bg=THEME["green"], fg=THEME["crust"])
        self.btn_break.config(state="disabled")
        self.btn_emergency.config(state="disabled")
        self.lbl_status.config(text="● Kalkan Kaldırıldı — Sistem Serbest", fg=THEME["subtext"])
        self.on_mode_change()

    def give_break(self):
        if not self.is_active:
            return
        if self.pomo_state == "WORK":
            self.pomo_state = "BREAK"
            mins = self.settings.get("pomo_break", 5)
            self.remaining_seconds = mins * 60
            self.total_session_seconds = self.remaining_seconds
            self._release_hosts_block()
            self.lbl_pomo_badge.config(text=f"MOLA VAKTİ ({mins} DK)", fg=THEME["blue"])
            self.lbl_status.config(text="☕ Mola zamanı — Rahatla ve nefes al", fg=THEME["blue"])
            self.btn_break.config(text="▶️ DERSE DÖN", fg=THEME["green"])
        else:
            self.pomo_state = "WORK"
            self.pomo_cycle += 1
            mins = self.settings.get("pomo_work", 25)
            self.remaining_seconds = mins * 60
            self.total_session_seconds = self.remaining_seconds
            self._engage_hosts_block()
            self.lbl_pomo_badge.config(text=f"POMODORO DÖNGÜ {self.pomo_cycle} (DERS)", fg=THEME["green"])
            self.lbl_status.config(text="🔒 Kalkan Devrede — Masaya dönüldü", fg=THEME["green"])
            self.btn_break.config(text="☕ MOLA VER", fg=THEME["yellow"])

    def prompt_emergency(self):
        prompt = simpledialog.askstring("Acil Durum Kilidi",
                                        f"Kalkanı hemen kaldırmak için aşağıdaki acil durum kodunu girin:\n\n{self.emergency_code}")
        if prompt == self.emergency_code:
            self.stop_session()
        else:
            messagebox.showerror("Hata", "Kod eşleşmedi!")

    def _engage_hosts_block(self):
        sites = self.settings.get("sites", [])
        if not sites or not is_admin():
            return
        try:
            with open(HOSTS_PATH, "r", encoding="utf-8", errors="ignore") as f:
                content = f.read()

            block_marker_start = "# --- FOCUSFLOW BLOCK START ---"
            block_marker_end = "# --- FOCUSFLOW BLOCK END ---"

            if block_marker_start in content:
                # Remove existing block first
                p1 = content.find(block_marker_start)
                p2 = content.find(block_marker_end) + len(block_marker_end)
                content = content[:p1] + content[p2:]

            new_block = [block_marker_start]
            for s in sites:
                new_block.append(f"{REDIRECT_IP} {s}")
                new_block.append(f"{REDIRECT_IP} www.{s}")
            new_block.append(block_marker_end)

            with open(HOSTS_PATH, "w", encoding="utf-8") as f:
                f.write(content.strip() + "\n\n" + "\n".join(new_block) + "\n")

            flush_dns()
        except Exception:
            pass

    def _release_hosts_block(self):
        if not is_admin():
            return
        try:
            if not os.path.exists(HOSTS_PATH):
                return
            with open(HOSTS_PATH, "r", encoding="utf-8", errors="ignore") as f:
                content = f.read()

            block_marker_start = "# --- FOCUSFLOW BLOCK START ---"
            block_marker_end = "# --- FOCUSFLOW BLOCK END ---"

            if block_marker_start in content and block_marker_end in content:
                p1 = content.find(block_marker_start)
                p2 = content.find(block_marker_end) + len(block_marker_end)
                cleaned = content[:p1].rstrip() + "\n" + content[p2:].lstrip()
                with open(HOSTS_PATH, "w", encoding="utf-8") as f:
                    f.write(cleaned)

            flush_dns()
        except Exception:
            pass

    def _clock_worker(self):
        while self.timer_running:
            time.sleep(1)
            if self.is_active and not self.is_paused:
                if self.remaining_seconds > 0:
                    self.remaining_seconds -= 1
                    mins = self.remaining_seconds // 60
                    secs = self.remaining_seconds % 60
                    time_str = f"{mins:02d}:{secs:02d}"

                    ratio = self.remaining_seconds / max(1, self.total_session_seconds)

                    # Update GUI safely
                    self.root.after(0, lambda t=time_str, r=ratio: self._update_clock_ui(t, r))
                else:
                    self.root.after(0, self._on_session_timer_expired)

    def _update_clock_ui(self, time_str, ratio):
        self.lbl_timer.config(text=time_str)
        self.draw_progress_bar(ratio)

    def _on_session_timer_expired(self):
        if self.current_mode == "pomodoro":
            if self.pomo_state == "WORK":
                self.give_break()
                messagebox.showinfo("Tebrikler!", "Ders seansı tamamlandı! Mola vakti.")
            else:
                self.give_break()
                messagebox.showinfo("Mola Bitti", "Mola bitti! Yeni odak seansı başlıyor.")
        else:
            self.stop_session()
            messagebox.showinfo("Seans Tamamlandı", "Hedeflenen çalışma süresi başarıyla bitti!")

    def _process_supervisor(self):
        while self.timer_running:
            time.sleep(2)
            if self.is_active and self.pomo_state == "WORK" and HAS_PSUTIL:
                blocked_apps = [a.lower() for a in self.settings.get("apps", [])]
                try:
                    for proc in psutil.process_iter(["pid", "name"]):
                        p_name = proc.info["name"]
                        if p_name and p_name.lower() in blocked_apps:
                            try:
                                proc.kill()
                                self.blocked_attempts_count += 1
                                self.root.after(0, self._update_blocked_counter)
                            except Exception:
                                pass
                except Exception:
                    pass

    def _update_blocked_counter(self):
        self.lbl_mini_blocked.config(text=f"Engellenen Girişim: {self.blocked_attempts_count}")

    def on_close(self):
        if self.is_active:
            if not messagebox.askyesno("Çıkış Onayı", "Odaklanma oturumu hala aktif! Çıkmak istiyor musunuz?"):
                return
            self._release_hosts_block()
        self.timer_running = False
        self.root.destroy()


def main():
    root = tk.Tk()
    app = FocusFlowGUI(root)
    root.mainloop()


if __name__ == "__main__":
    main()

