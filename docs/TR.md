# ChatGPT Remote — Türkçe Hızlı Başlangıç

**Durum:** v0.1.0; çalışan yerel MCP prototipi. ChatGPT hesabında MCP plugin oluşturma/yazma yetkisi bulunması garanti değildir. Sunucu, kendi makinenizde kurulur; kullanıcının onayı olmadan ChatGPT hesabınıza bağlantı ekleyemez.

## Kurulum (Ubuntu / WSL)

1. Repoyu GitHub'a yükledikten sonra `git clone https://github.com/matrixdurden/chatgpt-remote.git` ile indirin.
2. `cd chatgpt-remote` klasörüne geçin.
3. `./scripts/install.sh --root "$HOME/projects"` komutunu çalıştırın. Klasörün önceden bulunması gerekir.
4. `~/.local/bin/chatgpt-remote serve` komutuyla yerel sunucuyu başlatın.
5. İkinci terminalde `~/.local/bin/chatgpt-remote doctor` ile MCP protokolünü doğrulayın.

Kaynak koddan kurulum için Go 1.23+ gerekir. Önceden derlenmiş Linux kurulum paketinde Go kurulması gerekmez. `--service` seçeneği yalnızca systemd kullanıcı servisi desteklenen ortamlarda çalışır.

## ChatGPT bağlantısı

1. [ChatGPT Plugins](https://chatgpt.com/plugins) sayfasını açın. `Add custom MCP server` görünmüyorsa plan veya hesap kısıtı olabilir.
2. Destekleniyorsa OpenAI'nin [Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels) bileşenini ayrıca yetkilendirip kurun.
3. Önceden başlattığınız `serve` işlemini durdurun; ardından `chatgpt-remote serve --tunnel-local` ile başlatın. **Bu seçenekte yalnızca yerel 127.0.0.1 adresine gelen istekler için token gerekmiyor. Kendi denetiminizde, erişimi sınırlandırılmış bir tünel dışında kullanmayın.**
4. Tüneli `http://127.0.0.1:8765/mcp` adresine yönlendirin.
5. ChatGPT Plugins → `Add custom MCP server` → `Tunnel` ile tüneli seçin; plugin'i yükleyip, ChatGPT'de `@` ile kullanın.

**Dikkat:** Sunucunun 8765 portunu internetten erişilebilir yapmayın. `--tunnel-local` ile herkese açık tünel oluşturmayın. Bağlantının gerçekten çalışıp çalışmadığını ChatGPT hesabınızda test etmek gerekir.

## Deneme komutları

ChatGPT plugin'i kurulduğunda şu işlemleri isteyin:

- `chatgpt-remote get_status aracını çalıştır.`
- `İzin verilen projelerimden birinin klasörünü list_files ile göster.`
- `Test klasöründe deneme.txt oluştur ve sonra read_file ile oku.`

Başlangıçta terminal komutlarını çalıştırma **kapalıdır**. `~/.config/chatgpt-remote/config.json` içindeki `enable_commands` yalnızca riskleri anladıktan sonra etkinleştirilebilir; komutlar sunucunun işletim sistemi kullanıcısının haklarıyla çalışır. Klasör sınırları terminal alt işlemleri için bir güvenlik sandbox'ı değildir.
