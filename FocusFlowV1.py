import tkinter as tk
from tkinter import messagebox, ttk, simpledialog
import psutil
import os
import sys
import datetime
import time
import threading
import json
import random
import string
import hashlib
import http.server
import socketserver
import winreg
import logging
import subprocess
import webbrowser
import pyautogui
import ctypes
import urllib.parse 
from threading import Lock
from queue import Queue

# --- KÜTÜPHANE KONTROLÜ (Resim Çıktısı İçin) ---
try:
    from PIL import Image, ImageDraw, ImageFont
    HAS_PIL = True
except ImportError:
    HAS_PIL = False 

# --- AYARLAR ---
pyautogui.FAILSAFE = False

# --- LOGLAMA ---
logging.basicConfig(filename='focusflow_log.txt', level=logging.ERROR, 
                    format='%(asctime)s - %(levelname)s - %(message)s')

# --- SABİTLER ---
HOSTS_PATH = r"C:\Windows\System32\drivers\etc\hosts"
REDIRECT_IP = "127.0.0.1"
SETTINGS_FILE = "focusflow_settings.json"
APP_NAME = "FocusFlowService"
MOTIVATION_URL = "http://127.0.0.1:8080" 

# --- MODERN RENK PALETİ ---
COLOR_BG = "#1e1e2e"
COLOR_CARD = "#313244"
COLOR_TEXT = "#cdd6f4"
COLOR_PRIMARY = "#a6e3a1"
COLOR_SECONDARY = "#89b4fa"
COLOR_WARNING = "#f9e2af"
COLOR_DANGER = "#f38ba8"
COLOR_ACCENT = "#cba6f7"
COLOR_SUCCESS = COLOR_PRIMARY
COLOR_LIGHT = "#ffffff"

DEFAULT_QUOTES = [
    "Muhtaç olduğun kudret, damarlarındaki asil kanda mevcuttur!",
    "Başarı, disiplinin eseridir.",
    "Şu an rakibin çalışıyor, sen ne yapıyorsun?",
    "Acı geçici, gurur kalıcıdır.",
    "Disiplin, motivasyonu her zaman yener.",
    "Her dakika değerlidir, boşa harcama!",
    "Hedefine odaklan, engellerden güçlenerek geç!",
    "Yarın deme, yarınlar bitmez.",
    "Başlamak için mükemmel olmak zorunda değilsin, ama mükemmel olmak için başlamak zorundasın.",
    "Hayallerin, mazeretlerinden büyük olmalı.",
    "Zor, imkansız demek değildir.",
    "Terlemeden kazanılan zaferin tadı olmaz.",
    "Bugün yaptığın fedakarlıklar, yarınki gülüşlerin olacak.",
    "Vazgeçmek, korkakların işidir.",
    "Zirve, kalabalık değildir.",
    "Kendine bir söz ver ve onu tut.",
    "Yorgunluk sadece zihindedir, bedenin daha fazlasını yapabilir.",
    "Şikayet etme, daha çok çalış.",
    "En iyi intikam, devasa bir başarıdır.",
    "Karanlık olmadan yıldızları göremezsin.",
    "Limitlerini zorla, çünkü limit yok.",
    "Sadece inanmak yetmez, ter dökmelisin.",
    "Uykun geliyorsa yüzünü yıka, hayallerin uyumuyor.",
    "Bir saatlik çalışma, bir ömürlük pişmanlığı önler.",
    "Kaybetmekten korkma, denememekten kork.",
    "Odaklanmak, hayır diyebilmektir.",
    "Senin potansiyelin, tahmin ettiğinden çok daha fazla.",
    "Bugün ektiğini, yarın biçeceksin.",
    "Zaman en değerli sermayendir, iflas etme.",
    "Rahatlık bölgesi, hayallerin öldüğü yerdir.",
    "Bir gün değil, birinci gün.",
    "Büyük hayaller, büyük bedeller ister.",
    "Sızlanma, çalış. Hayat sızlananları beklemez.",
    "Gelecekteki sen, şu an yaptıkların için sana teşekkür edecek mi?",
    "Durdurulamaz ol.",
    "Başarı tesadüf değildir.",
    "Eğer kolay olsaydı, herkes yapardı.",
    "Seni öldürmeyen şey, başarıya götürür.",
    "Kendi hikayenin kahramanı ol.",
    "Bugün çalışmazsan, yarın başkası senin yerine kazanır.",
    "Disiplin özgürlüktür.",
    "Motivasyon başlar, alışkanlık devam ettirir.",
    "Düşmek sorun değil, kalkmamak sorundur.",
    "Yolun sonunu değil, attığın adımı düşün.",
    "Odaklan, üret, başar.",
    "Telefonu bırak, kalemine sarıl.",
    "Beynin bir kas, onu çalıştır.",
    "Zaman akıyor, sen durma.",
    "Mazeretler sadece kaybedenler içindir.",
    "Asla pes etme. Asla.",
    "Kendini kandırma, o masaya otur.",
    "Başarı, sessizce çalışmaktır.",
    "Gürültüyü görmezden gel, hedefe kilitlen.",
    "Yetenek çalışmazsa, çalışma yeteneği yener.",
    "Bugünün acısı, yarının gücüdür.",
    "Hedefin güneş olsun, ıskalarsan yıldızlara ulaşırsın.",
    "Çalışmak, en asil eylemdir.",
    "Geleceğini, şu an inşa ediyorsun.",
    "Zihnin efendisi ol, kölesi değil.",
    "Bahaneler seni bir yere götürmez.",
    "Şampiyonlar salonda değil, zihinde doğar.",
    "Sabır ve ter, başarının formülüdür.",
    "Yol yokuşsa, zirveye yaklaşıyorsun demektir.",
    "Kendi şansını kendin yarat.",
    "Durma, nefes al ve devam et.",
    "Başarmak zorundasın, başka seçeneğin yok.",
    "Hayat, cesurları sever.",
    "Odaklanmış bir zihin, en güçlü silahtır.",
    "Sadece yap.",
    "Bekleme, zaman asla 'tam doğru' olmayacak.",
    "Küçük adımlar, büyük mesafeler kat ettirir.",
    "Kendine inan, dünya inansa ne yazar?",
    "Çalış, kazan, tekrarla.",
    "Boş zaman yoktur, boşa geçen zaman vardır.",
    "Disiplin, anlık hazzı reddetmektir.",
    "İrade, kas gibidir; kullandıkça güçlenir.",
    "Zorluklar, karakterini ortaya çıkarır.",
    "Daha iyisini yapabilirsin.",
    "Bugün kendine bir iyilik yap ve çalış.",
    "Hayallerine giden yol, konfor alanından geçmez.",
    "Başarı detaylarda gizlidir.",
    "Sıradan olma, efsane ol.",
    "Yapabileceğine inanırsan, yolun yarısını gitmişsindir.",
    "Erteleme, hayatını çalmasına izin verme.",
    "En büyük rakibin, aynadaki kişi.",
    "Zafer, vazgeçmeyenlerindir.",
    "Korkularının üzerine git.",
    "Büyük düşün, büyük yaşa.",
    "Her saniye yeni bir fırsattır.",
    "Zihinsel dayanıklılık, fiziksel güçten üstündür.",
    "Çalışmadan dua etmek, biletsiz piyango beklemektir.",
    "O masadan kalkma.",
    "Bildirimleri kapat, beynini aç.",
    "Yarı yolda bırakma.",
    "Kendine saygı duy ve çalış.",
    "Başarı bedel ister, ödemeye hazır mısın?",
    "Hayat kısa, izin bırak.",
    "Odaklan. Sadece odaklan.",
    "Yapabilirsin. Yapacaksın."
]

# --- HTML ŞABLONU ---
def get_html_content(violation_count, quotes):
    quote = random.choice(quotes) if quotes else "Odaklan ve başar!"
    return f"""
    <html><head><title>ODAKLAN!</title>
    <style>
    body{{ background: linear-gradient(135deg, #1e1e2e 0%, #313244 100%); color: #fff; text-align: center; font-family: 'Segoe UI', sans-serif; margin-top: 10%; padding: 20px; }}
    h1{{ font-size: 80px; text-shadow: 3px 3px 6px rgba(0,0,0,0.3); color: #f38ba8; }} 
    .quote{{ background: rgba(255,255,255,0.1); padding: 30px; border-radius: 15px; font-size: 28px; margin: 40px auto; max-width: 700px; border: 1px solid #89b4fa; }}
    </style></head><body>
    <div style="font-size:100px">⛔</div>
    <h1>YASAKLI BÖLGE!</h1>
    <div class="quote">"{quote}"</div>
    <div style="font-size:20px; margin-top:30px; color: #a6e3a1;">İhlal Sayısı: {violation_count}</div>
    </body></html>
    """

# --- GLOBAL DEĞİŞKENLER ---
violation_counter = 0
violation_lock = Lock()
server_port = 8080

def increment_violation():
    global violation_counter
    with violation_lock:
        violation_counter += 1
        return violation_counter

def get_active_window_info():
    try:
        hwnd = ctypes.windll.user32.GetForegroundWindow()
        length = ctypes.windll.user32.GetWindowTextLengthW(hwnd)
        buff = ctypes.create_unicode_buffer(length + 1)
        ctypes.windll.user32.GetWindowTextW(hwnd, buff, length + 1)
        title = buff.value.lower()
        pid = ctypes.c_ulong()
        ctypes.windll.user32.GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
        process_name = psutil.Process(pid.value).name().lower()
        return title, process_name
    except: return "", ""

# --- AKILLI MASAÜSTÜ BULUCU (OneDrive Çözümü) ---
def get_desktop_path():
    user_home = os.path.expanduser("~")
    possible_paths = [
        os.path.join(user_home, "Desktop"),
        os.path.join(user_home, "OneDrive", "Desktop"),
        os.path.join(user_home, "OneDrive", "Masaüstü"),
        os.path.join(user_home, "Masaüstü")
    ]
    for path in possible_paths:
        if os.path.exists(path):
            return path
    return user_home

# --- SUNUCU ---
class MotivationHandler(http.server.BaseHTTPRequestHandler):
    quotes = DEFAULT_QUOTES
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(get_html_content(violation_counter, MotivationHandler.quotes).encode('utf-8'))
    def log_message(self, format, *args): return

def start_server():
    global server_port
    while True:
        try: socketserver.TCPServer(("", server_port), MotivationHandler).serve_forever(); break
        except: server_port += 1

# --- HOSTS ---
class HostsManager:
    _lock = Lock()
    @staticmethod
    def clean_hosts():
        with HostsManager._lock:
            try:
                if not os.access(HOSTS_PATH, os.W_OK): return
                with open(HOSTS_PATH, 'r') as f: lines = f.readlines()
                new_lines = [l for l in lines if "# --- FOCUSFLOW" not in l and REDIRECT_IP not in l]
                with open(HOSTS_PATH, 'w') as f: f.writelines(new_lines)
                try: subprocess.run(["ipconfig", "/flushdns"], shell=True, stdout=subprocess.DEVNULL)
                except: pass
            except: pass
    @staticmethod
    def write_blocks(sites):
        # Sadece domain olanları (nokta içerenleri) hosts'a yaz
        domain_sites = [s for s in sites if "." in s]
        HostsManager.clean_hosts()
        if not domain_sites: return
        with HostsManager._lock:
            try:
                with open(HOSTS_PATH, 'a') as f:
                    f.write("\n# --- FOCUSFLOW BLOCK START ---\n")
                    for s in domain_sites: f.write(f"{REDIRECT_IP} {s}\n{REDIRECT_IP} www.{s}\n")
                    f.write("# --- FOCUSFLOW BLOCK END ---\n")
                try: subprocess.run(["ipconfig", "/flushdns"], shell=True, stdout=subprocess.DEVNULL)
                except: pass
            except: pass

# --- GRAFİK ÇİZİCİ ---
class SimpleGraph(tk.Canvas):
    def __init__(self, parent, data_dict, width=550, height=180, bg=COLOR_CARD):
        super().__init__(parent, width=width, height=height, bg=bg, highlightthickness=0)
        self.data = data_dict
        self.draw_graph()

    def draw_graph(self):
        self.delete("all")
        if not self.data: 
            self.create_text(275, 90, text="Henüz Veri Yok", fill=COLOR_TEXT, font=("Segoe UI", 12))
            return
        
        dates = sorted(list(self.data.keys()))[-7:] # Son 7 gün
        values = [self.data[d] for d in dates]
        if not values: return
        
        max_val = max(values) if max(values) > 0 else 1
        bar_width = 40
        spacing = 30
        start_x = 40
        base_y = 150
        
        self.create_text(275, 15, text="Haftalık Çalışma Süresi (Dakika)", fill=COLOR_TEXT, font=("Segoe UI", 10, "bold"))
        
        for i, (date_str, val) in enumerate(zip(dates, values)):
            x0 = start_x + i * (bar_width + spacing)
            bar_height = (val / max_val) * 100
            y0 = base_y - bar_height
            x1 = x0 + bar_width
            y1 = base_y
            
            color = COLOR_PRIMARY if val >= 60 else COLOR_SECONDARY
            self.create_rectangle(x0, y0, x1, y1, fill=color, outline="")
            self.create_text((x0+x1)/2, y0-10, text=str(val), fill=COLOR_TEXT, font=("Segoe UI", 9))
            
            try: d_lbl = datetime.datetime.strptime(date_str, "%Y-%m-%d").strftime("%d/%m")
            except: d_lbl = date_str
            self.create_text((x0+x1)/2, y1+15, text=d_lbl, fill=COLOR_TEXT, font=("Segoe UI", 8))

# --- ANA UYGULAMA ---
class FocusFlow:
    def __init__(self, root):
        self.root = root
        self.root.title("FocusFlow")
        self.root.geometry("900x900")
        self.root.configure(bg=COLOR_BG)
        
        self.setup_styles()
        self.settings = self.load_settings()
        self.is_locked = False
        self.lock_hash = ""
        self.pomodoro_state = self.settings.get("pomodoro_state", "WORK")
        self.current_task_name = "" 
        
        # --- MOTİVASYON SÖZLERİNİ GÜNCELLEME DEDEKTİFİ ---
        # Eğer kayıtlı sözler azsa (eski sürümse) yenileriyle değiştir
        saved_quotes = self.settings.get("motivation_quotes", [])
        if len(saved_quotes) < 50: 
            self.motivation_quotes = DEFAULT_QUOTES.copy()
            self.settings["motivation_quotes"] = self.motivation_quotes
            self.save_settings()
        else:
            self.motivation_quotes = saved_quotes
        
        MotivationHandler.quotes = self.motivation_quotes
        # ------------------------------------------------
        
        self.gui_queue = Queue()
        self.calculate_streak()
        HostsManager.clean_hosts()
        self.add_to_startup()
        self.setup_ui()

        threading.Thread(target=self.daemon_loop, daemon=True).start()
        threading.Thread(target=start_server, daemon=True).start()
        
        self.process_gui_queue()
        self.update_stats_display()
        root.protocol("WM_DELETE_WINDOW", self.on_close)

    def setup_styles(self):
        style = ttk.Style()
        style.theme_use('clam')
        style.configure('TFrame', background=COLOR_BG)
        style.configure('TLabel', background=COLOR_BG, foreground=COLOR_TEXT, font=('Segoe UI', 10))
        style.configure('Card.TFrame', background=COLOR_CARD, relief='flat')
        style.configure('TNotebook', background=COLOR_BG, borderwidth=0)
        style.configure('TNotebook.Tab', background=COLOR_CARD, foreground=COLOR_TEXT, padding=[15, 8], font=('Segoe UI', 10, 'bold'), borderwidth=0)
        style.map('TNotebook.Tab', background=[('selected', COLOR_PRIMARY)], foreground=[('selected', '#000000')])
        style.configure('TLabelframe', background=COLOR_BG, foreground=COLOR_ACCENT, borderwidth=1, relief='solid')
        style.configure('TLabelframe.Label', background=COLOR_BG, foreground=COLOR_ACCENT, font=('Segoe UI', 11, 'bold'))
        style.configure('TRadiobutton', background=COLOR_BG, foreground=COLOR_TEXT, font=('Segoe UI', 10))
        style.map('TRadiobutton', background=[('active', COLOR_BG)])
        style.configure('TCheckbutton', background=COLOR_BG, foreground=COLOR_TEXT, font=('Segoe UI', 10))
        style.map('TCheckbutton', background=[('active', COLOR_BG)])
        style.configure("Treeview", background=COLOR_CARD, foreground=COLOR_TEXT, fieldbackground=COLOR_CARD, borderwidth=0, font=('Segoe UI', 10))
        style.configure("Treeview.Heading", background=COLOR_SECONDARY, foreground="white", font=('Segoe UI', 10, 'bold'))
        style.map("Treeview", background=[('selected', COLOR_PRIMARY)], foreground=[('selected', 'black')])

    def load_settings(self):
        today = datetime.date.today().strftime("%Y-%m-%d")
        default = {
            "sites": [], "apps": [], "whitelist_sites": [], "phone": "", 
            "mode": "standard", "duration": 60, 
            "pomo_work": 25, "pomo_break": 5, 
            "flex_break": 15, "start_time_str": "09:00", "end_time_str": "20:00", "flex_strict": True,
            "stats": {"total_sessions": 0, "total_minutes": 0, "total_violations": 0},
            "history": {today: 0},
            "session_log": [],
            "streak": 0, "last_session_date": "",
            "filter_mode": "blacklist",
            "is_locked": False, "lock_hash": "", "end_timestamp": 0, "pomodoro_state": "WORK", 
            "motivation_quotes": DEFAULT_QUOTES.copy(), "whatsapp_enabled": True,
            "work_start_timestamp": 0, "flex_end_timestamp": 0, "pomo_cycles": 4
        }
        if os.path.exists(SETTINGS_FILE):
            try:
                with open(SETTINGS_FILE, "r", encoding="utf-8") as f: 
                    saved = json.load(f)
                    default.update(saved)
                    if "session_log" not in default: default["session_log"] = []
                    return default
            except: return default
        return default

    def save_settings(self):
        try:
            with open(SETTINGS_FILE, "w", encoding="utf-8") as f: json.dump(self.settings, f, indent=2, ensure_ascii=False)
        except: pass

    def calculate_streak(self):
        today = datetime.date.today()
        last_date_str = self.settings.get("last_session_date", "")
        if last_date_str:
            try:
                last_date = datetime.datetime.strptime(last_date_str, "%Y-%m-%d").date()
                diff = (today - last_date).days
                if diff > 1: self.settings["streak"] = 0 
            except: self.settings["streak"] = 0
        self.save_settings()

    def add_to_startup(self):
        try:
            key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, r"Software\Microsoft\Windows\CurrentVersion\Run", 0, winreg.KEY_ALL_ACCESS)
            script = os.path.abspath(sys.argv[0])
            python_path = sys.executable.replace("python.exe", "pythonw.exe") if sys.executable.endswith("python.exe") else sys.executable
            winreg.SetValueEx(key, APP_NAME, 0, winreg.REG_SZ, f'"{python_path}" "{script}"')
            winreg.CloseKey(key)
        except: pass

    def create_modern_button(self, parent, text, command, color, width=20, state="normal"):
        return tk.Button(parent, text=text, command=command, bg=color, fg="#ffffff", 
                         font=("Segoe UI", 11, "bold"), relief="flat", 
                         activebackground=color, activeforeground="#eeeeee", 
                         cursor="hand2", width=width, state=state, pady=8)

    # --- ARAYÜZ ---
    def setup_ui(self):
        self.notebook = ttk.Notebook(self.root)
        self.notebook.pack(pady=20, fill='both', expand=True, padx=20)

        self.tab_home = ttk.Frame(self.notebook); self.notebook.add(self.tab_home, text='🏠 Ana Sayfa')
        self.tab_sites = ttk.Frame(self.notebook); self.notebook.add(self.tab_sites, text='🌐 Siteler')
        self.tab_apps = ttk.Frame(self.notebook); self.notebook.add(self.tab_apps, text='💻 Uygulamalar')
        self.tab_motivation = ttk.Frame(self.notebook); self.notebook.add(self.tab_motivation, text='💪 Motivasyon')
        self.tab_stats = ttk.Frame(self.notebook); self.notebook.add(self.tab_stats, text='📊 İstatistikler')
        self.tab_set = ttk.Frame(self.notebook); self.notebook.add(self.tab_set, text='⚙️ Ayarlar')

        self.setup_home()
        self.setup_lists(self.tab_sites, "sites", "Web Sitesi (youtube.com)")
        self.setup_lists(self.tab_apps, "apps", "Uygulama (steam.exe)")
        self.setup_motivation()
        self.setup_stats()
        self.setup_settings()

    def setup_home(self):
        f = self.tab_home
        
        header_frame = tk.Frame(f, bg=COLOR_BG)
        header_frame.pack(fill='x', pady=10)
        tk.Label(header_frame, text="⚡ FocusFlow", font=("Segoe UI", 36, "bold"), fg=COLOR_PRIMARY, bg=COLOR_BG).pack()
        
        streak_val = self.settings.get("streak", 0)
        tk.Label(header_frame, text=f"🔥 {streak_val} Günlük Seri", font=("Segoe UI", 14, "bold"), fg="#ff9f43", bg=COLOR_BG).pack()

        self.mode_var = tk.StringVar(value=self.settings["mode"])
        fr = ttk.LabelFrame(f, text="Çalışma Modu")
        fr.pack(pady=10, fill="x", padx=40)
        ttk.Radiobutton(fr, text="⏱️ Standart Mod", variable=self.mode_var, value="standard", command=self.toggle_mode_ui).pack(side="left", padx=20, pady=10)
        ttk.Radiobutton(fr, text="🍅 Detaylı Zamanlayıcı", variable=self.mode_var, value="pomodoro", command=self.toggle_mode_ui).pack(side="left", padx=20, pady=10)
        ttk.Radiobutton(fr, text="📅 Esnek Mod", variable=self.mode_var, value="flex", command=self.toggle_mode_ui).pack(side="left", padx=20, pady=10)
        
        self.frm_time = ttk.LabelFrame(f, text="Zaman Ayarları")
        self.frm_time.pack(pady=5, fill="x", padx=40)
        
        self.lbl_std = ttk.Label(self.frm_time, text="Süre (dk):"); self.spin_std = ttk.Spinbox(self.frm_time, from_=1, to=600, width=10); self.spin_std.set(self.settings["duration"])
        self.lbl_p_w = ttk.Label(self.frm_time, text="Ders (dk):"); self.spin_p_w = ttk.Spinbox(self.frm_time, from_=1, to=120, width=8); self.spin_p_w.set(self.settings["pomo_work"])
        self.lbl_p_b = ttk.Label(self.frm_time, text="Mola (dk):"); self.spin_p_b = ttk.Spinbox(self.frm_time, from_=1, to=6, width=8); self.spin_p_b.set(self.settings["pomo_break"])
        self.lbl_cycles = ttk.Label(self.frm_time, text="Tekrar Sayısı:"); self.spin_cycles = ttk.Spinbox(self.frm_time, from_=1, to=20, width=8); self.spin_cycles.set(self.settings.get("pomo_cycles", 4))
        self.lbl_flex_start = ttk.Label(self.frm_time, text="Başlangıç (SS:DD):"); self.ent_flex_start = ttk.Entry(self.frm_time, width=8); self.ent_flex_start.insert(0, datetime.datetime.now().strftime("%H:%M"))
        self.lbl_flex_end = ttk.Label(self.frm_time, text="Bitiş (SS:DD):"); self.ent_flex_end = ttk.Entry(self.frm_time, width=8); self.ent_flex_end.insert(0, self.settings.get("end_time_str", "20:00"))
        self.lbl_flex_break = ttk.Label(self.frm_time, text="Mola Süresi (dk):"); self.spin_flex_break = ttk.Spinbox(self.frm_time, from_=5, to=60, width=8); self.spin_flex_break.set(self.settings.get("flex_break", 15))
        self.var_flex_strict = tk.BooleanVar(value=self.settings.get("flex_strict", True))
        self.chk_flex_strict = tk.Checkbutton(self.frm_time, text="⚠️ 40 dk Kuralı Olsun", variable=self.var_flex_strict, bg=COLOR_BG)

        btn_frame = tk.Frame(f, bg=COLOR_BG); btn_frame.pack(pady=10)
        self.btn_start = self.create_modern_button(btn_frame, "🔒 PLANI BAŞLAT", self.start_lock, COLOR_PRIMARY, width=25)
        self.btn_start.pack(pady=5)
        
        self.btn_break = self.create_modern_button(btn_frame, "☕ MOLA VER", self.take_manual_break, COLOR_WARNING, width=25, state="disabled")
        
        self.btn_emerg = tk.Button(btn_frame, text="🆘 ACİL DURUM KODU", command=self.emergency_unlock, bg=COLOR_DANGER, fg="white", font=("Segoe UI", 9), relief="flat", state="disabled"); self.btn_emerg.pack(pady=5)
        self.lbl_status = tk.Label(f, text="✅ Sistem Hazır", font=("Segoe UI", 12), bg=COLOR_BG, fg=COLOR_SECONDARY); self.lbl_status.pack(pady=5)
        self.toggle_mode_ui()

    def toggle_mode_ui(self):
        for w in self.frm_time.winfo_children(): w.grid_forget()
        self.btn_break.pack_forget() 
        if self.mode_var.get()=="standard": 
            self.lbl_std.grid(row=0,column=0,padx=20,pady=20); self.spin_std.grid(row=0,column=1,padx=10)
        elif self.mode_var.get()=="pomodoro":
            self.lbl_p_w.grid(row=0,column=0,padx=15,pady=10); self.spin_p_w.grid(row=0,column=1)
            self.lbl_p_b.grid(row=0,column=2,padx=15); self.spin_p_b.grid(row=0,column=3)
            self.lbl_cycles.grid(row=1,column=0,padx=15,pady=10); self.spin_cycles.grid(row=1,column=1)
        else: 
            self.lbl_flex_start.grid(row=0,column=0,padx=15,pady=10); self.ent_flex_start.grid(row=0,column=1)
            self.lbl_flex_end.grid(row=0,column=2,padx=15); self.ent_flex_end.grid(row=0,column=3)
            self.lbl_flex_break.grid(row=1,column=0,padx=15,pady=10); self.spin_flex_break.grid(row=1,column=1)
            # --- DÜZELTME BURADA: sticky="w" EKLENDİ ---
            self.chk_flex_strict.grid(row=1, column=2, columnspan=2, sticky="w", padx=5)
            # -------------------------------------------
            self.btn_break.pack(pady=5, after=self.btn_start)
        self.settings["mode"] = self.mode_var.get()

    def setup_lists(self, parent, key, title):
        tk.Label(parent, text=f"➕ {title}", font=("Segoe UI", 12, "bold"), bg=COLOR_BG, fg=COLOR_ACCENT).pack(pady=15)
        input_frame = tk.Frame(parent, bg=COLOR_BG); input_frame.pack(pady=5, padx=40, fill="x")
        entry = ttk.Entry(input_frame, font=('Segoe UI', 10)); entry.pack(side=tk.LEFT, fill="x", expand=True, padx=(0, 10))
        lst = tk.Listbox(parent, height=14, font=("Consolas", 10), bg=COLOR_CARD, fg=COLOR_TEXT, selectbackground=COLOR_SECONDARY, borderwidth=0, highlightthickness=0)
        
        # --- GÜNCELLENEN EKLEME MANTIĞI (URL TEMİZLEYİCİ) ---
        def add():
            v = entry.get().strip().lower()
            
            # Sadece 'sites' kısmındaysak ve içinde nokta (.) varsa URL temizliği yap
            if key == "sites" and "." in v:
                if v.startswith("http://") or v.startswith("https://"):
                    try:
                        v = urllib.parse.urlparse(v).netloc
                    except: pass
                if "/" in v:
                    v = v.split("/")[0]

            if v and v not in self.settings[key]: 
                if key=="apps" and not v.endswith(".exe"): v+=".exe"
                self.settings[key].append(v); lst.insert(tk.END, v); self.save_settings(); entry.delete(0, tk.END)
        # ----------------------------------------------------

        def rem():
            try: i = lst.curselection()[0]; v = lst.get(i); self.settings[key].remove(v); lst.delete(i); self.save_settings()
            except: pass
        tk.Button(input_frame, text="Ekle", command=add, bg=COLOR_SUCCESS, fg="white", relief="flat", font=("Segoe UI", 9, "bold"), width=8).pack(side=tk.LEFT, padx=2)
        tk.Button(input_frame, text="Sil", command=rem, bg=COLOR_DANGER, fg="white", relief="flat", font=("Segoe UI", 9, "bold"), width=8).pack(side=tk.LEFT, padx=2)
        lst.pack(pady=10, fill='both', expand=True, padx=40)
        for i in self.settings[key]: lst.insert(tk.END, i)

    def setup_motivation(self):
        f = self.tab_motivation
        tk.Label(f, text="💪 Motivasyon Sözleri", font=("Segoe UI", 16, "bold"), fg=COLOR_ACCENT, bg=COLOR_BG).pack(pady=20)
        l_frame = ttk.Frame(f); l_frame.pack(pady=10, padx=40, fill="both", expand=True)
        l = tk.Listbox(l_frame, height=15, font=("Segoe UI", 10), bg=COLOR_CARD, fg=COLOR_TEXT, borderwidth=0, highlightthickness=0); l.pack(fill="both", expand=True, pady=10)
        for q in self.motivation_quotes: l.insert("end", q)
        bf = tk.Frame(f, bg=COLOR_BG); bf.pack(pady=10)
        def add():
            t = simpledialog.askstring("Ekle", "Söz:")
            if t: self.motivation_quotes.append(t); self.save_settings(); l.insert("end", t)
        def rem():
            try: i=l.curselection()[0]; self.motivation_quotes.pop(i); l.delete(i); self.save_settings()
            except: pass
        self.create_modern_button(bf, "Ekle", add, COLOR_SUCCESS, width=10).pack(side="left", padx=5)
        self.create_modern_button(bf, "Sil", rem, COLOR_DANGER, width=10).pack(side="left", padx=5)

    def setup_stats(self):
        tk.Label(self.tab_stats, text="📊 İstatistikler", font=("Segoe UI", 18, "bold"), fg=COLOR_ACCENT, bg=COLOR_BG).pack(pady=10)
        self.graph_frame = tk.Frame(self.tab_stats, bg=COLOR_BG); self.graph_frame.pack(pady=5)
        self.graph = SimpleGraph(self.graph_frame, self.settings.get("history", {}))
        self.graph.pack()

        tk.Label(self.tab_stats, text="📅 Bugünün Kayıtları", font=("Segoe UI", 12, "bold"), fg=COLOR_TEXT, bg=COLOR_BG).pack(pady=(15,5))
        tree_frame = tk.Frame(self.tab_stats)
        tree_frame.pack(pady=5, padx=20, fill="x", expand=False)
        columns = ("saat", "gorev", "sure")
        self.tree = ttk.Treeview(tree_frame, columns=columns, show="headings", height=6)
        self.tree.heading("saat", text="Saat"); self.tree.column("saat", width=80, anchor="center")
        self.tree.heading("gorev", text="Çalışma Konusu"); self.tree.column("gorev", width=250)
        self.tree.heading("sure", text="Süre"); self.tree.column("sure", width=80, anchor="center")
        scrollbar = ttk.Scrollbar(tree_frame, orient="vertical", command=self.tree.yview)
        self.tree.configure(yscrollcommand=scrollbar.set)
        self.tree.pack(side="left", fill="x", expand=True)
        scrollbar.pack(side="right", fill="y")
        self.create_modern_button(self.tab_stats, "📄 Rapor Al (.png)", self.export_png_report, COLOR_SECONDARY, width=30).pack(pady=15)

    def update_daily_log_display(self):
        for i in self.tree.get_children(): self.tree.delete(i)
        today_str = datetime.date.today().strftime("%Y-%m-%d")
        logs = self.settings.get("session_log", [])
        todays_logs = [log for log in logs if log.get("date") == today_str]
        for log in todays_logs:
            self.tree.insert("", "end", values=(log.get("time"), log.get("task"), f"{log.get('duration')} dk"))

    def export_png_report(self):
        # Eğer Pillow yüklü değilse uyarı ver ve TXT olarak kaydetmeyi teklif et veya otomatik yap
        if not HAS_PIL:
            if messagebox.askyesno("Eksik Kütüphane", "Resim oluşturmak için 'Pillow' kütüphanesi gerekli.\nYine de Metin (.txt) olarak kaydetmek ister misin?"):
                self.export_weekly_report()
            return

        logs = self.settings.get("session_log", [])
        if not logs: messagebox.showinfo("Bilgi", "Henüz kayıt yok."); return

        try:
            # Resim oluşturma (A4 boyutuna yakın veya içerik kadar uzun)
            width, height = 800, 1000
            bg_color = (30, 30, 46) # Koyu tema
            text_color = (205, 214, 244)
            accent_color = (166, 227, 161) # Yeşil
            
            img = Image.new('RGB', (width, height), color=bg_color)
            d = ImageDraw.Draw(img)
            
            # Font ayarla
            try:
                # Windows'ta varsa Arial, yoksa default
                title_font = ImageFont.truetype("arial.ttf", 40)
                header_font = ImageFont.truetype("arial.ttf", 25)
                text_font = ImageFont.truetype("arial.ttf", 18)
            except:
                title_font = ImageFont.load_default()
                header_font = ImageFont.load_default()
                text_font = ImageFont.load_default()

            # Başlık
            d.text((50, 50), "FOCUSFLOW RAPORU", font=title_font, fill=accent_color)
            d.text((50, 100), f"Tarih: {datetime.datetime.now().strftime('%d.%m.%Y %H:%M')}", font=text_font, fill=(150, 150, 150))
            
            y = 150
            today = datetime.date.today()
            logs_by_date = {}
            for log in logs:
                d_str = log.get("date")
                if d_str not in logs_by_date: logs_by_date[d_str] = []
                logs_by_date[d_str].append(log)
            
            sorted_dates = sorted(logs_by_date.keys(), reverse=True)
            
            for date_str in sorted_dates:
                d_obj = datetime.datetime.strptime(date_str, "%Y-%m-%d").date()
                if (today - d_obj).days > 7: continue # Son 7 gün
                
                total_min = sum(l['duration'] for l in logs_by_date[date_str])
                
                # Tarih başlığı arka planı
                d.rectangle([(40, y), (760, y+40)], fill=(49, 50, 68))
                d.text((50, y+5), f"{date_str} - Toplam: {total_min} dk", font=header_font, fill=text_color)
                y += 50
                
                for l in logs_by_date[date_str]:
                    line = f"• [{l['time']}] {l['task']} ({l['duration']} dk)"
                    d.text((70, y), line, font=text_font, fill=text_color)
                    y += 30
                    if y > height - 50: break
                y += 20
                if y > height - 50: break

            # Kaydetme Yeri (OneDrive Fix)
            desktop = get_desktop_path()
            filepath = os.path.join(desktop, f"FocusFlow_Rapor_{datetime.date.today()}.png")
            img.save(filepath)
            
            messagebox.showinfo("Başarılı", f"PNG Rapor masaüstüne kaydedildi:\n{filepath}")
            
        except Exception as e:
            messagebox.showerror("Hata", f"Resim oluşturulamadı:\n{e}")

    def export_weekly_report(self):
        try:
            logs = self.settings.get("session_log", [])
            if not logs: messagebox.showinfo("Bilgi", "Henüz kayıt yok."); return
            
            desktop = get_desktop_path()
            filepath = os.path.join(desktop, "FocusFlow_Rapor.txt")
            
            with open(filepath, "w", encoding="utf-8") as f:
                f.write("=== FOCUSFLOW HAFTALIK ÇALIŞMA RAPORU ===\n\n")
                today = datetime.date.today()
                logs_by_date = {}
                for log in logs:
                    d = log.get("date")
                    if d not in logs_by_date: logs_by_date[d] = []
                    logs_by_date[d].append(log)
                
                sorted_dates = sorted(logs_by_date.keys(), reverse=True)
                for d in sorted_dates:
                    date_obj = datetime.datetime.strptime(d, "%Y-%m-%d").date()
                    if (today - date_obj).days > 7: continue
                    total_min = sum(l['duration'] for l in logs_by_date[d])
                    f.write(f"\nTARİH: {d} (Toplam: {total_min} dk)\n")
                    f.write("-" * 40 + "\n")
                    for l in logs_by_date[d]:
                        f.write(f"[{l['time']}] {l['task']} -> {l['duration']} dakika\n")
            messagebox.showinfo("Başarılı", f"Rapor masaüstüne kaydedildi:\n{filepath}")
        except Exception as e:
            messagebox.showerror("Hata", f"Dosya kaydedilemedi:\n{e}")

    def setup_settings(self):
        f = self.tab_set
        tk.Label(f, text="⚙️ Sistem Ayarları", font=("Segoe UI", 18, "bold"), fg=COLOR_TEXT, bg=COLOR_BG).pack(pady=20)
        filter_frame = ttk.LabelFrame(f, text="🛡️ Engelleme Türü")
        filter_frame.pack(pady=10, padx=40, fill="x")
        self.filter_var = tk.StringVar(value=self.settings.get("filter_mode", "blacklist"))
        ttk.Radiobutton(filter_frame, text="⛔ Kara Liste (Engelle)", variable=self.filter_var, value="blacklist", command=lambda: self.settings.update({"filter_mode":"blacklist"})).pack(side="left", padx=20, pady=10)
        ttk.Radiobutton(filter_frame, text="✅ Beyaz Liste (Sadece İzin Ver)", variable=self.filter_var, value="whitelist", command=lambda: self.settings.update({"filter_mode":"whitelist"})).pack(side="left", padx=20, pady=10)
        
        wa_frame = ttk.LabelFrame(f, text="WhatsApp")
        wa_frame.pack(pady=10, padx=40, fill="x")
        
        self.var_wa_enabled = tk.BooleanVar(value=self.settings.get("whatsapp_enabled", True))
        
        # --- DÜZELTME BURADA ---
        def toggle_wa():
            self.settings["whatsapp_enabled"] = self.var_wa_enabled.get()
            self.save_settings()

        tk.Checkbutton(wa_frame, text="Bildirimleri Kullan", variable=self.var_wa_enabled, bg=COLOR_BG, fg=COLOR_TEXT, selectcolor=COLOR_BG, activebackground=COLOR_BG, command=toggle_wa).pack(pady=10, anchor="w", padx=15)
        
        tk.Label(wa_frame, text="Tel (+90...):", bg=COLOR_BG, fg=COLOR_TEXT).pack(pady=5, anchor="w", padx=15)
        self.ent_phone = ttk.Entry(wa_frame); self.ent_phone.insert(0, self.settings["phone"]); self.ent_phone.pack(pady=5, padx=15, anchor="w", fill="x")
        
        self.create_modern_button(f, "Ayarları Kaydet", lambda:[self.settings.update({"phone":self.ent_phone.get()}), self.save_settings(), messagebox.showinfo("OK","Kaydedildi")], COLOR_SUCCESS, width=20).pack(pady=10)
        self.create_modern_button(f, "İstatistik Sıfırla", lambda: [self.settings.update({"stats":{"total_sessions":0,"total_minutes":0,"total_violations":0}, "history":{}, "session_log":[]}), self.save_settings(), self.update_stats_display()], COLOR_DANGER, width=20).pack()

    # --- MANTIK ---
    def start_lock(self):
        # --- DÜZELTME BURADA: Güvenlik kontrolü ---
        wa = self.settings.get("whatsapp_enabled", True)
        
        # Eğer WA kapalıysa, telefon sormasına gerek yok!
        if wa and (not self.settings["phone"] or len(self.settings["phone"])<10):
            messagebox.showerror("Hata", "WhatsApp açıkken telefon girmek zorunludur!"); return
            
        if not self.settings["sites"] and not self.settings["apps"]: messagebox.showwarning("Uyarı", "Liste boş!"); return
        task = simpledialog.askstring("Hedef Belirle", "Şu an ne çalışacaksın?\n(Örn: Matematik - Türev)")
        if not task: return 
        self.current_task_name = task
        
        # WA kapalıysa soru sorma!
        if wa and not messagebox.askyesno("WhatsApp", "WhatsApp Web açılsın mı?\n\n⚠️ Mesaj otomatik gidecek, 30 sn DOKUNMA!"): return
        
        raw_code = ''.join(random.choices(string.ascii_uppercase + string.digits, k=10))
        self.lock_hash = hashlib.sha256(raw_code.encode()).hexdigest()
        self.is_locked = True
        now = datetime.datetime.now()
        msg = ""
        if self.settings["mode"] == "standard":
            try: mins = int(self.spin_std.get())
            except: mins = 60
            self.settings["end_timestamp"] = (now + datetime.timedelta(minutes=mins)).timestamp()
            msg = f"🚀 *FocusFlow: {self.current_task_name}*\n\n⏱️ Süre: {mins} dk\n🔐 KOD: {raw_code}"
        elif self.settings["mode"] == "pomodoro":
            try:
                self.settings["pomo_work"] = int(self.spin_p_w.get()); self.settings["pomo_break"] = int(self.spin_p_b.get())
                self.settings["pomo_cycles"] = int(self.spin_cycles.get())
                self.current_cycle_count = 0; self.target_cycles = self.settings["pomo_cycles"]
                mins = self.settings["pomo_work"]
                self.settings["end_timestamp"] = (now + datetime.timedelta(minutes=mins)).timestamp()
                self.pomodoro_state = "WORK"
                msg = f"⏱️ *FocusFlow: {self.current_task_name}*\n\n🔄 Hedef: {self.target_cycles} Tur\n📚 Ders: {mins} dk\n🔐 KOD: {raw_code}"
            except: return
        elif self.settings["mode"] == "flex":
            try:
                s_h, s_m = map(int, self.ent_flex_start.get().split(':')); e_h, e_m = map(int, self.ent_flex_end.get().split(':'))
                start_dt = now.replace(hour=s_h, minute=s_m, second=0)
                end_dt = now.replace(hour=e_h, minute=e_m, second=0)
                if end_dt < now and end_dt < start_dt: end_dt += datetime.timedelta(days=1)
                if start_dt < now: start_dt = now
                self.settings["flex_end_timestamp"] = end_dt.timestamp()
                self.settings["work_start_timestamp"] = start_dt.timestamp()
                self.settings["end_timestamp"] = start_dt.timestamp()
                self.settings["flex_break"] = int(self.spin_flex_break.get())
                self.settings["flex_strict"] = self.var_flex_strict.get() 
                self.pomodoro_state = "WORK"
                self.btn_break.pack(pady=10, after=self.btn_start)
                msg = f"📅 *FocusFlow: {self.current_task_name}*\n\n⏰ Bitiş: {self.ent_flex_end.get()}\n🔐 KOD: {raw_code}"
            except: messagebox.showerror("Hata", "Saat formatı SS:DD olmalı!"); self.is_locked=False; return
        self.save_settings(); self.update_ui_state()
        
        # WA kapalıysa direkt kodu göster
        if not wa: messagebox.showinfo("KOD", f"Kod: {raw_code}")
        
        threading.Thread(target=self.bg_start, args=(self.settings["phone"], msg, wa), daemon=True).start()

    def bg_start(self, phone, msg, wa):
        if self.settings.get("filter_mode") != "whitelist" and (self.settings["mode"] != "flex" or datetime.datetime.now().timestamp() >= self.settings["work_start_timestamp"]):
             HostsManager.write_blocks(self.settings["sites"])
        if wa:
            self.gui_queue.put(("status", "WhatsApp... DOKUNMA!", COLOR_WARNING))
            try:
                encoded_msg = urllib.parse.quote(msg)
                url = f"https://web.whatsapp.com/send?phone={phone}&text={encoded_msg}"
                try: webbrowser.get('brave').open(url)
                except: webbrowser.open(url)
                time.sleep(30)
                pyautogui.press('enter'); time.sleep(1); pyautogui.press('enter')
                time.sleep(2); pyautogui.hotkey('ctrl', 'w')
                self.gui_queue.put(("wa_success", None))
            except Exception as e: self.gui_queue.put(("wa_error", str(e)))
        self.gui_queue.put(("status", "🔒 SİSTEM AKTİF", COLOR_PRIMARY))

    def record_session(self, duration_mins):
        if duration_mins < 1: return
        today_str = datetime.date.today().strftime("%Y-%m-%d")
        now_time = datetime.datetime.now().strftime("%H:%M")
        new_entry = { "date": today_str, "time": now_time, "task": self.current_task_name, "duration": duration_mins }
        self.settings["session_log"].append(new_entry)
        self.settings["history"][today_str] = self.settings["history"].get(today_str, 0) + duration_mins
        self.settings["stats"]["total_sessions"] += 1
        self.settings["stats"]["total_minutes"] += duration_mins
        self.save_settings()

    def take_manual_break(self):
        now = datetime.datetime.now()
        work_duration = int((now.timestamp() - self.settings["work_start_timestamp"]) / 60)
        self.pomodoro_state = "BREAK"
        break_min = self.settings["flex_break"]
        self.settings["end_timestamp"] = (now + datetime.timedelta(minutes=break_min)).timestamp()
        self.record_session(work_duration) 
        HostsManager.clean_hosts()
        self.btn_break.config(state="disabled", text=f"☕ Moladasın ({break_min} dk)", bg=COLOR_WARNING)
        msg = f"☕ *Mola Zamanı!*\n\n✅ Tamamlanan: {self.current_task_name} ({work_duration} dk)\n🧘 Mola: {break_min} dk"
        if self.settings.get("whatsapp_enabled", True) and self.settings["phone"]:
            threading.Thread(target=self.send_cycle_message, args=(self.settings["phone"], msg), daemon=True).start()

    def emergency_unlock(self):
        c = simpledialog.askstring("ACİL", "Kod:")
        if c and hashlib.sha256(c.strip().upper().encode()).hexdigest() == self.lock_hash:
            threading.Thread(target=self.bg_unlock, args=("ACİL",), daemon=True).start()
        else: messagebox.showerror("HATA", "Yanlış")

    def bg_unlock(self, reason):
        HostsManager.clean_hosts()
        self.is_locked = False
        self.settings["end_timestamp"] = 0
        if reason != "ACİL":
            today_str = datetime.date.today().strftime("%Y-%m-%d")
            if self.settings.get("last_session_date") != today_str:
                self.settings["streak"] = self.settings.get("streak", 0) + 1
                self.settings["last_session_date"] = today_str
        self.save_settings()
        end_msg = f"🎉 *Oturum Bitti!*\n✅ Durum: {reason}\n⚠️ İhlal: {violation_counter}"
        if self.settings.get("whatsapp_enabled", True) and self.settings["phone"]:
            threading.Thread(target=self.send_cycle_message, args=(self.settings["phone"], end_msg), daemon=True).start()
        self.gui_queue.put(("unlock", reason))

    def send_cycle_message(self, phone, msg):
        try:
            encoded_msg = urllib.parse.quote(msg)
            url = f"https://web.whatsapp.com/send?phone={phone}&text={encoded_msg}"
            try: webbrowser.get('brave').open(url)
            except: webbrowser.open(url)
            time.sleep(30)
            pyautogui.press('enter'); time.sleep(1); pyautogui.press('enter')
            time.sleep(2); pyautogui.hotkey('ctrl', 'w') 
        except: pass

    def handle_pomodoro_cycle(self):
        now = datetime.datetime.now(); msg = ""
        if self.pomodoro_state == "WORK":
            self.pomodoro_state = "BREAK"
            mins = self.settings["pomo_break"]
            self.settings["end_timestamp"] = (now + datetime.timedelta(minutes=mins)).timestamp()
            self.record_session(self.settings["pomo_work"])
            HostsManager.clean_hosts()
            msg = f"☕ *Mola Zamanı!*\n\n{mins} dk dinlen. 🧘‍♂️"
            self.gui_queue.put(("pomodoro_msg", f"☕ Mola: {mins} dk"))
        else:
            self.current_cycle_count += 1
            if self.current_cycle_count >= self.target_cycles:
                threading.Thread(target=self.bg_unlock, args=("HEDEF TAMAMLANDI",), daemon=True).start(); return
            self.pomodoro_state = "WORK"
            mins = self.settings["pomo_work"]
            self.settings["end_timestamp"] = (now + datetime.timedelta(minutes=mins)).timestamp()
            if self.settings.get("filter_mode") != "whitelist": HostsManager.write_blocks(self.settings["sites"])
            msg = f"📚 *Tur {self.current_cycle_count + 1}/{self.target_cycles} Başlıyor!*\n\n{mins} dk odaklan! 🔥"
            self.gui_queue.put(("pomodoro_msg", f"📚 Tur {self.current_cycle_count + 1}: {mins} dk"))
        if self.settings.get("whatsapp_enabled", True) and self.settings["phone"]:
            threading.Thread(target=self.send_cycle_message, args=(self.settings["phone"], msg), daemon=True).start()

    def daemon_loop(self):
        while True:
            if self.is_locked:
                now = datetime.datetime.now(); now_ts = now.timestamp()
                
                # --- ANTI-CHEAT ---
                try:
                    for proc in psutil.process_iter(['name']):
                        if proc.info['name'] in ['Taskmgr.exe', 'cmd.exe', 'powershell.exe']:
                            try: proc.kill()
                            except: pass
                except: pass

                if self.settings["mode"] == "flex":
                    if now_ts >= self.settings["flex_end_timestamp"]:
                        threading.Thread(target=self.bg_unlock, args=("GÜN BİTTİ",), daemon=True).start(); time.sleep(5); continue
                    if self.pomodoro_state == "WORK":
                        worked_min = int((now_ts - self.settings["work_start_timestamp"]) / 60)
                        is_strict = self.settings.get("flex_strict", True)
                        if not is_strict or worked_min >= 40: self.gui_queue.put(("enable_break_btn", True))
                        else: self.gui_queue.put(("enable_break_btn", False))
                        if self.settings.get("filter_mode") != "whitelist": HostsManager.write_blocks(self.settings["sites"])
                        self.gui_queue.put(("status", f"📚 Çalışma: {worked_min} dk", COLOR_DANGER))
                    elif self.pomodoro_state == "BREAK":
                        rem = int(self.settings["end_timestamp"] - now_ts)
                        if rem <= 0:
                            self.pomodoro_state = "WORK"; self.settings["work_start_timestamp"] = now_ts 
                            msg = f"📚 *Mola Bitti!*\n\nHadi tekrar derse! Odaklan. 💪"
                            if self.settings.get("whatsapp_enabled", True): threading.Thread(target=self.send_cycle_message, args=(self.settings["phone"], msg), daemon=True).start()
                            self.gui_queue.put(("reset_break_btn", None))
                        else:
                            m, s = divmod(rem, 60); self.gui_queue.put(("status", f"☕ Mola Kalan: {m:02d}:{s:02d}", COLOR_SUCCESS))
                else: 
                    if now_ts >= self.settings["end_timestamp"]:
                        if self.settings["mode"] == "standard": 
                            self.record_session(self.settings["duration"])
                            threading.Thread(target=self.bg_unlock, args=("SÜRE DOLDU",), daemon=True).start(); time.sleep(5)
                        else: self.handle_pomodoro_cycle()
                        continue
                    rem = int(self.settings["end_timestamp"] - now_ts); m, s = divmod(rem, 60)
                    st = f"⏳ Kalan: {m:02d}:{s:02d}"
                    if self.settings["mode"] == "pomodoro": st = f"{'📚' if self.pomodoro_state=='WORK' else '☕'} {self.pomodoro_state} | Tur: {self.current_cycle_count}/{self.target_cycles} | {st}"
                    self.gui_queue.put(("status", st, COLOR_DANGER if (self.settings["mode"]=="standard" or self.pomodoro_state=="WORK") else COLOR_SUCCESS))

                should_block = False
                if self.settings["mode"] == "standard": should_block = True
                elif self.settings["mode"] == "pomodoro" and self.pomodoro_state == "WORK": should_block = True
                elif self.settings["mode"] == "flex" and self.pomodoro_state == "WORK": should_block = True
                if should_block:
                    title, proc_name = get_active_window_info()
                    browsers = ['brave.exe', 'chrome.exe', 'msedge.exe', 'firefox.exe', 'opera.exe']
                    is_whitelist_mode = (self.settings.get("filter_mode") == "whitelist")
                    found_violation = False
                    if is_whitelist_mode:
                        if proc_name in browsers:
                            is_allowed = False
                            for site in self.settings["sites"]: 
                                k = site.split('.')[0]
                                if k in title or (len(k)<2 and (f" {k} " in title)): is_allowed = True
                            if not is_allowed: found_violation = True
                    else:
                        for site in self.settings["sites"]:
                            # --- YENİ KELİME BAZLI ENGELLEME ---
                            if "." in site: # Domain ise (youtube.com)
                                k = site.split('.')[0]
                                if k in title or (len(k)<2 and (f" {k} " in title)): found_violation = True
                            else: # Kelime ise (shorts, reels)
                                if site in title: # Direkt başlıkta ara
                                    found_violation = True
                                    # Eğer kelime yasaklıysa kapat (hosts ile değil, kill ile)
                                    pyautogui.hotkey('ctrl', 'w') 
                            # -----------------------------------
                    
                    if found_violation and proc_name in browsers:
                        pyautogui.hotkey('ctrl', 'w'); increment_violation(); webbrowser.open(MOTIVATION_URL); time.sleep(1)
                    
                    if not is_whitelist_mode:
                        apps = [a.lower() for a in self.settings["apps"]]
                        for proc in psutil.process_iter(['name']):
                            if proc.info['name'].lower() in apps:
                                try:
                                    proc.kill()
                                    increment_violation()
                                except:
                                    pass
            else: self.gui_queue.put(("status", "✅ Sistem Hazır", COLOR_SUCCESS))
            time.sleep(1)

    def process_gui_queue(self):
        try:
            while not self.gui_queue.empty():
                i = self.gui_queue.get_nowait()
                if i[0] == "status": self.lbl_status.config(text=i[1], fg=i[2])
                elif i[0] == "unlock": self.update_ui_state(); self.graph.draw_graph(); self.update_daily_log_display(); messagebox.showinfo("Bitti", i[1])
                elif i[0] == "wa_success": pass 
                elif i[0] == "wa_error": messagebox.showwarning("Hata", f"WA Hatası: {i[1]}")
                elif i[0] == "pomodoro_msg": pass 
                elif i[0] == "enable_break_btn":
                    if i[1]: self.btn_break.config(state="normal", text="☕ MOLA VER (Aktif)", bg=COLOR_PRIMARY)
                    else: self.btn_break.config(state="disabled", text="☕ MOLA VER (40dk Dolmadı)", bg=COLOR_WARNING)
                elif i[0] == "reset_break_btn":
                    self.btn_break.config(state="disabled", text="☕ MOLA VER (40dk Dolmadı)", bg=COLOR_WARNING)
        except: pass
        self.root.after(200, self.process_gui_queue)

    def update_stats_display(self):
        try:
            self.update_daily_log_display() 
            self.graph.draw_graph() 
        except: pass
        self.root.after(10000, self.update_stats_display)

    def update_ui_state(self):
        st = "disabled" if self.is_locked else "normal"
        self.btn_start.config(state=st)
        self.btn_emerg.config(state="normal" if self.is_locked else "disabled")
        if not self.is_locked: self.btn_break.pack_forget()
        for i in [1,2,3,5]: 
            try: self.notebook.tab(i, state=st)
            except: pass

    def on_close(self):
        if self.is_locked: messagebox.showwarning("Dur", "Çalışma bitmeden kapatılamaz!"); self.root.iconify()
        elif messagebox.askyesno("Çıkış", "Kapatılsın mı?"): HostsManager.clean_hosts(); self.root.destroy(); sys.exit()

if __name__ == "__main__":
    # --- YÖNETİCİ İZNİ KONTROLÜ VE OTO-YÜKSELTME ---
    def is_admin():
        try:
            return ctypes.windll.shell32.IsUserAnAdmin()
        except:
            return False

    if not is_admin():
        ctypes.windll.shell32.ShellExecuteW(None, "runas", sys.executable, " ".join(sys.argv), None, 1)
        sys.exit()
    else:
        root = tk.Tk()
        app = FocusFlow(root)
        root.mainloop()
