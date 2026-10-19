# ChatGPT Remote — Kurulum ve bağlantı

Linux (Ubuntu/Debian) veya Windows 11 içindeki **WSL2 Ubuntu** terminalini aç.

## 1. Kur

```bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/install.sh | bash
```

Programı ve yerel MCP servisini kurar; mevcut ayarları korur. `~/projects` varsa yalnızca bu klasörü erişime açar. Yoksa `~/chatgpt-remote-workspace` oluşturur. `sudo`, Docker veya Go kurmak gerekmez.

## 2. Bağlan

```bash
chatgpt-remote
```

Komut tanınmıyorsa `~/.local/bin/chatgpt-remote` kullan veya terminali yeniden aç. İlk çalıştırmada resmî `cloudflared` indirilebilir (SHA-256 ile doğrulanır). Ekranda **HTTPS MCP URL** ve **eşleştirme kodu** görürsün.

1. [ChatGPT Plugins](https://chatgpt.com/plugins) → **Add custom MCP server**.
2. HTTPS MCP URL'yi **`/mcp` dâhil** gir. Kimlik doğrulama varsa **OAuth / otomatik kayıt (DCR)** seç.
3. Plugin'i bağlamayı onayla. Açılan sayfada terminaldeki eşleştirme kodunu gir.
4. Desteklenen ChatGPT sohbetinden `get_status` çağrısını dene.

Bağlantı açıkken terminali kapatma; `Ctrl+C` bağlantıyı keser. Bu bir Cloudflare **Quick Tunnel**: ücretsiz ve hesap gerektirmeyen geçici geliştirme tünelidir. Yeniden açıldığında URL değişir; ChatGPT bağlantısını yeniden kurman gerekebilir. Gerçek ChatGPT üzerinden uçtan uca doğrulama henüz yapılmadı, hesabının özel MCP izinleri de gerekiyor.

**Güvenlik:** Tünel tek başına güvenli değildir. MCP yetkilendirmesi için OAuth 2.1/PKCE, kullanıcının onayladığı eşleştirme kodu ve token kontrolü vardır. Bu yeni uygulama bağımsız güvenlik denetiminden geçmedi. Hassas/üretim sunucularında kullanma. Terminal komutları varsayılan olarak kapalıdır. Açarsan çalıştırılan programlar proje kökü dışına da erişebilir; izin verilen komut listesi bir sandbox değildir.

## 3. Tamamen kaldır

```bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/uninstall.sh | bash
```

Kaldırma öncesinde açık tüneli `Ctrl+C` ile kapat. Program, servis, ayar, token, tünel yardımcı programı ve loglar silinir; kişisel proje dosyaları korunur. ChatGPT'de oluşturduğun plugin bağlantısını ayrıca kaldırmalısın.

Sorun giderme: `~/.local/bin/chatgpt-remote doctor` ve `~/.local/bin/chatgpt-remote status`.
