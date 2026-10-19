# ChatGPT Remote — Türkçe Kurulum Rehberi

## Destek

- ✅ Linux (Ubuntu, Debian ve benzeri): x86-64 / ARM64
- ✅ Windows 11 üzerinde WSL2 Ubuntu/Debian: aynı Linux kurulumu
- ❌ PowerShell/CMD üzerinde native Windows: henüz yok

WSL, Windows dosyalarını /mnt/c üzerinden görebilir. Ancak yalnızca açıkça izin verilen kök dizinlere erişilir. Bu, Windows masaüstü/GUI kontrolü anlamına gelmez.

## Tek komutla kurulum

WSL Ubuntu veya doğrudan Linux terminalinde:

~~~bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/install.sh | bash
~~~

Kendi proje dizinini paylaşmak için:

~~~bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/install.sh | bash -s -- --root "$HOME/projects"
~~~

Varsayılan olarak ~/projects varsa onu kullanır, yoksa ~/chatgpt-remote-workspace oluşturur. Sudo, git veya Go yüklemeniz gerekmez. GitHub Release varsa doğrulanan hazır binary indirilir; yoksa kaynak kod indirilerek geçici Go araç zinciriyle derlenir. Geçici dosyalar kaldırılır.

Linux systemd --user açıksa servis oradan başlatılır. WSL/systemd yoksa arka planda başlatılır; WSL kapatıldığında süreç durur. Dosya kökleri sınırlıdır; terminal komutları güvenlik için varsayılan olarak devre dışıdır.

## Yerel test

~~~bash
~/.local/bin/chatgpt-remote doctor
~/.local/bin/chatgpt-remote status
~~~

MCP endpoint: http://127.0.0.1:8765/mcp. Yerel test, ChatGPT bağlantısı ve hesap yetkisini doğrulamaz.

## Tek komutla kaldırma (ayar ve token dâhil)

~~~bash
bash "$HOME/.local/share/chatgpt-remote/uninstall.sh"
~~~

Alternatif:

~~~bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/uninstall.sh | bash
~~~

Servis, binary, ayarlar, token, PID ve loglar silinir. Kurulumun oluşturduğu özel workspace boşsa silinir; içinde kullanıcı dosyası varsa korunur. Mevcut projeler asla silinmez. Harici MCP tüneli ve ChatGPT plugin bağlantısını ayrıca kaldırmanız gerekir.

## ChatGPT bağlantısı

ChatGPT Plugins sayfasında Add custom MCP server seçeneğini ve hesap izinlerini doğrulayın: https://chatgpt.com/plugins. Tam yetkili özel MCP araçları Plus planlarında garanti değildir. Desteklenen bağlantı üzerinden ayrıca yetkili Secure MCP Tunnel veya OAuth proxy gerekir. Başka kaynaklardan HTTP ile herkese açık, kimlik doğrulamasız erişim vermeyin.

Resmî kaynak: https://developers.openai.com/api/docs/guides/secure-mcp-tunnels

Bu proje OpenAI'ın resmî ürünü değildir.
