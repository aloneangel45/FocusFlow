# FocusFlow v2.1 (Turkce Kullanim ve Mimari Kilavuzu)

<p align="center">
  <strong>Cokmeye Karsi Korumali Derin Odaklanma Motoru, Cift Yiginli (IPv4/IPv6) Atomik Hosts Engelleyicisi ve Catppuccin Mocha Masaustu HUD.</strong>
</p>

---

## FocusFlow v2.1 Mimarisi ve Yenilikler

FocusFlow v2.1, **Go (yerel CLI motoru)** ve **Python standart kutuphanesi (`focusflow_core` + `FocusFlowGUI.py`, sifir zorunlu harici bagimlilik)** uzerinde tamamen birlestirilmis bir sistem mimarisi sunar:

1. **Ortak Durum ve Yapilandirma Katmani:**
   * Hem Go CLI (`focusflow`) hem de Python masaustu arayuzu (`FocusFlowGUI.py`) ayni durum dizinini (`FOCUSFLOW_HOME` -> Windows'ta `%APPDATA%\FocusFlow`, Linux/macOS'ta `~/.config/focusflow`), ayni `config.json`, `sessions.json` ve `hosts.focusflow.journal.json` semalarini kullanir.
2. **SHA-256 Write-Ahead Journal ve Otomatik Kurtarma (`RecoverIfOrphaned`):**
   * `hosts` dosyasina dokunmadan once mevcut FocusFlow bloklari temizlenerek saf temel kopyanin SHA-256 ozeti alinir (`hosts.focusflow.bak`) ve `hosts.focusflow.journal.json` diske yazilir.
   * Elektrik kesintisi, sistem cokmesi veya yeniden baslatma sonrasinda CLI ya da GUI acildiginda yetim oturum otomatik tespit edilir; SHA-256 dogrulamasi (veya deterministik isaretci temizligi) ile `hosts` dosyasi Windows ACL izinleri (`ReplaceFileW`) korunarak geri yuklenir.
3. **Cift Yiginli IPv4 + IPv6 Sinkhole ve Gelismis Anti-DoH:**
   * Yalnizca `0.0.0.0` degil, `::` (IPv6) kayitlari ve yaygin DNS-over-HTTPS kacis sunuculari (`cloudflare-dns.com`, `dns.google`, `dns.quad9.net`, `dns.adguard-dns.com`, `dns.nextdns.io`, `doh.cleanbrowsing.org`, `doh.dns.sb`, `freedns.controld.com`) birlikte engellenir.
4. **Sifir Harici Bagimlilikli Win32 / POSIX Proses Denetleyicisi:**
   * Windows'ta `psutil` gerektirmeden dogrudan `ctypes` uzerinden Win32 `CreateToolhelp32Snapshot` (`Process32FirstW` / `Process32NextW`) kullanilir. Kritik sistem surecleri (`explorer.exe`, `svchost.exe`, `lsass.exe`, `csrss.exe`, `python.exe`, `focusflow.exe`, `wt.exe`) koruma listesiyle kesin olarak engellenir.
5. **Gercek Telemetri ve Yuksek Surtunmeli Siki Mod (Friction Challenge):**
   * Tum istatistikler, aktif gun serisi ve tamamlanma oranlari dogrudan `sessions.json` verilerinden hesaplanir. Siki mod aktifken kopyala-yapistir kapali 16 karakterlik dogrulama kodu olmadan oturum veya pencere erken kapatilamaz.

---

## Hizli Baslangic

### 1. Masaustu HUD (GUI)
Windows uzerinde `Launch_FocusFlow_GUI.bat` dosyasini calistirin veya terminalden baslatin:
```bash
python FocusFlowGUI.py
```
Yonetici (Administrator) yetkisiyle baslatmak icin:
```bash
python FocusFlowGUI.py --elevate
```

### 2. Komut Satiri Araci (Go CLI)
Kaynak koddan derleme:
```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=2.1.0" -o focusflow.exe ./cmd/focusflow
```

Temel komutlar:
```bash
# 50 dakikalik yazilimci seansi (Yonetici yetkisi isteyerek)
focusflow start --profile dev --minutes 50 --elevate

# 25 dakikalik akademik/sinav calisma seansi
focusflow start --profile study --minutes 25

# Hardcore Aegis kilidi (Anti-Cheat aktif)
focusflow start --profile hardcore --strict hardcore

# Kalkan, yetki ve journal durumunu incele
focusflow status
focusflow status --json

# Ozel site ve uygulama kurallarini yonet
focusflow list --json
focusflow add site reddit.com
focusflow add app steam.exe

# Gercek calisma istatistikleri ve 7 gunluk ASCII grafik
focusflow stats --days 7
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
