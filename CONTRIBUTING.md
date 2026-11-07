# FocusFlow'a Katkıda Bulunma Rehberi

FocusFlow açık kaynaklı bir topluluk projesidir. Hata bildirimleri, yeni özellik önerileri ve katkılarınızı memnuniyetle kabul ediyoruz!

## Geliştirme Ortamı

### Gereksinimler
- **Python 3.10+**
- Git

### Kurulum
```bash
git clone https://github.com/aloneangel45/FocusFlow.git
cd FocusFlow
pip install -r requirements.txt
```

### Testleri Çalıştırma
```bash
pytest tests/
```

### Katkı Süreci
1. Projeyi Fork'layın.
2. Yeni bir dal (branch) açın: `git checkout -b feat/yeni-ozellik`
3. Değişikliklerinizi yapın ve test edin.
4. Conventional Commits formatında commit atın (`feat:`, `fix:`, `docs:`).
5. Pull Request gönderin.
