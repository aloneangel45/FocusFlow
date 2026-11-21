# FocusFlow v2.3 (Turkce Kullanim ve Mimari Kilavuzu)

<p align="center">
  <strong>Cokmeye Karsi Korumali Derin Odaklanma Motoru, Cift Yiginli (IPv4/IPv6) Atomik Hosts Engelleyicisi ve Catppuccin Mocha Masaustu HUD.</strong>
</p>

---

## FocusFlow v2.3 Mimarisi ve Yenilikler

FocusFlow v2.3, **Go (yerel CLI motoru)** ve **Python standart kutuphanesi (`focusflow_core` + `FocusFlowGUI.py`, sifir zorunlu harici bagimlilik)** uzerinde tamamen birlestirilmis bir sistem mimarisi sunar:

1. **Ortak Durum, Ozel Profiller, Otomatik Zamanlama ve Kancalar:**
   * Hem Go CLI (`focusflow`) hem de Python masaustu arayuzu (`FocusFlowGUI.py`) ayni durum dizinini (`FOCUSFLOW_HOME` -> Windows'ta `%APPDATA%\FocusFlow`, Linux/macOS'ta `~/.config/focusflow`), ayni `config.json` (ozel profiller, `daily_goal_minutes`, `schedules`, `hooks`, `notifications_enabled`), `sessions.json` ve `hosts.focusflow.journal.json` semalarini kayipsiz sekilde paylasir.
2. **Otomatik Yinelenen Zamanlayici (`focusflow schedule`):**
   * Hafta ici/hafta sonu veya gece yarisini asan (`22:00`–`02:00`) tekrarlayan odak pencereleri tanimlanir (`schedule add|list|remove|next|run`); arka plan zamanlayicisi pencere basladiginda kalkani otomatik etkinlestirir ve bitiste temizce geri alir.
3. **90 Gunluk Katki Isi Haritasi (`stats --heatmap` + Masaustu HUD):**
   * Gunluk odak hedefine gore olceklenen 13 hafta x 7 gunluk GitHub tarzi ANSI isi haritasi (`--no-color` / `NO_COLOR` ile saf ASCII `[ ] [.] [o] [O] [#]` destegi), aktif/en uzun seri hesabi ve Python HUD icinde Catppuccin Mocha isi haritasi paneli sunar.
4. **Sifir Bagimlilikli Yerel OS Bildirimleri ve Yetki Dusurmeli Yasam Dongusu Kancalari:**
   * Sprint baslangici, mola ve tamamlanma anlarinda engellemesiz yerel bildirimler (Windows Toast, macOS `osascript`, Linux `notify-send`; `--no-notify` ile kapatilabilir) ve `--on-start`/`--on-end` kabuk kancalari (`10s` zaman asimi, POSIX `sudo` altinda `SUDO_UID`/`SUDO_GID` yetki dusurme korumasi) calistirir.
2. **Cok Dongulu Pomodoro Motoru (`--cycles` &amp; `--break`):**
   * Tek komutla ardisik calisma/mola donguleri (`focusflow start --cycles 4 --break 5`) yurutulur; her calisma seansi bittiginde aninda `sessions.json` dosyasina kaydedilir ve mola sirasinda `hosts` kalkani gecici olarak kaldirilir (`--strict hardcore` modunda kesintisiz kilit korunur).
3. **SHA-256 Write-Ahead Journal ve Otomatik Kurtarma (`RecoverIfOrphaned`):**
   * `hosts` dosyasina dokunmadan once mevcut FocusFlow bloklari temizlenerek saf temel kopyanin SHA-256 ozeti alinir (`hosts.focusflow.bak`) ve `hosts.focusflow.journal.json` diske yazilir.
   * Elektrik kesintisi, sistem cokmesi veya yeniden baslatma sonrasinda CLI ya da GUI acildiginda yetim oturum otomatik tespit edilir; SHA-256 dogrulamasi (veya deterministik isaretci temizligi) ile `hosts` dosyasi Windows ACL izinleri (`ReplaceFileW`) korunarak geri yuklenir.
4. **Cift Yiginli IPv4 + IPv6 Sinkhole ve Gelismis Anti-DoH:**
   * Yalnizca `0.0.0.0` degil, `::` (IPv6) kayitlari ve yaygin DNS-over-HTTPS kacis sunuculari (`cloudflare-dns.com`, `dns.google`, `dns.quad9.net`, `dns.adguard-dns.com`, `dns.nextdns.io`, `doh.cleanbrowsing.org`, `doh.dns.sb`, `freedns.controld.com`) birlikte engellenir.
5. **Sifir Harici Bagimlilikli Win32 / POSIX Proses Denetleyicisi:**
   * Windows'ta `psutil` gerektirmeden dogrudan `ctypes` uzerinden Win32 `CreateToolhelp32Snapshot` (`Process32FirstW` / `Process32NextW`) kullanilir. Kritik sistem surecleri (`explorer.exe`, `svchost.exe`, `lsass.exe`, `csrss.exe`, `python.exe`, `focusflow.exe`, `wt.exe`) koruma listesiyle kesin olarak engellenir.
6. **Gercek Telemetri ve Yuksek Surtunmeli Siki Mod (Friction Challenge):**
   * Tum istatistikler, gunluk hedef ilerlemesi (`Today: X / Y min [Z%]`), aktif gun serisi ve tamamlanma oranlari dogrudan `sessions.json` verilerinden hesaplanir. Siki mod aktifken kopyala-yapistir kapali 16 karakterlik dogrulama kodu olmadan oturum veya pencere erken kapatilamaz.

---

## Hizli Baslangic

### 1. Tek Satirlik Kurulum (SHA-256 Dogrulamali)

**Linux ve macOS (`amd64` / `arm64`):**
```bash
curl -fsSL https://raw.githubusercontent.com/aloneangel45/FocusFlow/main/install.sh | sh
```

**Windows PowerShell (`amd64` / `arm64`):**
```powershell
irm https://raw.githubusercontent.com/aloneangel45/FocusFlow/main/install.ps1 | iex
```

### 2. Masaustu HUD (GUI)
Windows uzerinde `Launch_FocusFlow_GUI.bat` dosyasini calistirin veya terminalden baslatin:
```bash
python FocusFlowGUI.py
```
Yonetici (Administrator) yetkisiyle baslatmak icin:
```bash
python FocusFlowGUI.py --elevate
```

### 3. Komut Satiri Araci (Go CLI)
Kaynak koddan derleme:
```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=2.3.0" -o focusflow.exe ./cmd/focusflow
```

Temel komutlar:
```bash
# 50 dakikalik yazilimci seansi (Yonetici yetkisi isteyerek)
focusflow start --profile dev --minutes 50 --elevate

# 4 donguluk Pomodoro seansi (25 dk calisma / 5 dk mola)
focusflow start --profile study --cycles 4 --break 5 --elevate

# Hardcore Aegis kilidi (mola dahil kesintisiz kalkan ve Anti-Cheat)
focusflow start --profile hardcore --cycles 2 --break 5 --strict hardcore

# Ozel isimli profilleri listele, olustur, incele ve sil
focusflow profile list
focusflow profile create yazar --from study --minutes 45 --break 10 --sites medium.com --apps slack.exe --desc "Derin yazarlik modu"
focusflow profile show yazar --json
focusflow profile delete yazar

# Gunluk odak hedefini goruntule veya guncelle (5..1440 dk)
focusflow goal
focusflow goal --minutes 180
focusflow goal --json

# Kalkan, gunluk hedef ilerlemesi ve journal durumunu incele
focusflow status
focusflow status --json

# Ozel site ve uygulama kurallarini yonet
focusflow list --json
focusflow add site reddit.com
focusflow add app steam.exe

# Otomatik tekrarlayan odak pencereleri ve zamanlayici daemon
focusflow schedule add --name "Sabah Dev" --days mon-fri --from 09:00 --to 12:00 --profile dev
focusflow schedule next
focusflow schedule run --check-interval 30s --elevate

# Gercek calisma istatistikleri, 7 gunluk grafik ve 90 gunluk isi haritasi
focusflow stats --days 7
focusflow stats --heatmap
focusflow stats --heatmap --no-color
focusflow export --format csv --out rapor.csv

# Sistem tanilama ve cokme sonrasi kurtarma
focusflow doctor
focusflow recover --elevate

# Aninda geri alma ve DNS onbellek temizligi
focusflow stop
focusflow flush
```

---

## Testler

Tum Go ve Python testleri gecici `FOCUSFLOW_HOME` dizinlerinde izole olarak calisir; gercek `%APPDATA%` verilerinize veya sistem `hosts` dosyaniza dokunmaz:

```bash
go vet ./...
go test -v ./...
python -m pytest -v
```

---

## Lisans

Bu proje [MIT Lisansi](LICENSE) kapsaminda acik kaynaklidir.
Gelistirici: [aloneangel45](https://github.com/aloneangel45)
