# ⚡ FocusFlow V2 (Türkçe Kılavuz)

<p align="center">
  <strong>Rust ile geliştirilmiş, Kernel seviyesinde derin odaklanma motoru, atomik hosts engelleyicisi ve ultra hafif Win32 proses denetleyicisi.</strong>
</p>

---

## 🚀 FocusFlow V2 ile Neler Değişti?

Eski Python versiyonu (V1) harika bir prototipti; ancak V2 ile birlikte sistem tamamen **Rust** ile baştan kodlandı:

1. **Sıfır İnternet Bozulma Riski (Atomic Rollback):**
   * Eski sistemde elektrik kesilse veya program kapansa hosts dosyası bozulabiliyordu.
   * V2'de **SHA-256 hash kontrollü yedekleme** ve atomik dosya takası kullanılır. Program çöktüğü veya kapandığı an orijinal hosts dosyası %100 temiz şekilde geri yüklenir.
2. **DNS-over-HTTPS (DoH) Koruması:**
   * Modern tarayıcılar (Chrome, Brave, Edge) DNS-over-HTTPS ile hosts dosyasını delebiliyordu.
   * V2, yerel DNS önbelleğini (`ipconfig /flushdns`) otomatik temizler ve DoH kaçış noktalarını kilitler.
3. **<4MB Bellek & %0 CPU:**
   * Python runtime'ı ve ağır `psutil` döngüleri yerine doğrudan Windows Win32 API (`CreateToolhelp32Snapshot`) kullanılır. Sistem boştayken hissedilmez bile.
4. **Hazır Profiller:**
   * `focusflow start --profile dev` (Yazılımcı modu: GitHub, StackOverflow, Docs açık; sosyal medya kapalı)
   * `focusflow start --profile study` (YKS/KPSS/Sınav modu: Oyunlar, Discord, TikTok, YouTube kilitli)
   * `focusflow start --profile hardcore` (Aegis kilidi: Görev Yöneticisi engellemesi açık)

---

## 💻 Kullanım Kılavuzu

> **Not:** Windows hosts dosyasını yönetebilmek için terminali **Yönetici Olarak (Administrator)** çalıştırın.

```bash
# 50 dakikalık yazılımcı seansı başlat
focusflow start --profile dev --minutes 50

# 25 dakikalık YKS / KPSS çalışma seansı başlat
focusflow start --profile study --minutes 25

# Korumayı anında kaldır ve hosts dosyasını sıfırla
focusflow stop

# Durumu kontrol et
focusflow status

# Geçmiş çalışma istatistiklerini gör
focusflow stats
```

---

## 📄 Lisans

Bu proje [MIT Lisansı](LICENSE) ile tamamen açık kaynaklıdır.  
Geliştirici: [aloneangel45](https://github.com/aloneangel45)
