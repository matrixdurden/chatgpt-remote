package remote

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Bridge authenticates ChatGPT using OAuth 2.1 PKCE and only then forwards
// MCP messages to the existing loopback-only, bearer-authenticated daemon.
// Tokens and pairing approvals exist in memory and vanish upon disconnect.
type Bridge struct {
	mu         sync.Mutex
	Base       string
	LocalURL   string
	LocalToken string
	PairCode   string
	Clients    map[string][]string
	Pending    map[string]*pendingAuth
	Redeemed   map[string]time.Time
	Codes      map[string]*authCode
	Tokens     map[string]*issuedToken
	Refresh    map[string]*issuedToken
	Client     *http.Client
	ready      chan struct{}
}

const pairingRequestTTL = 30 * time.Minute

type pendingAuth struct {
	ClientID, Redirect, State, Challenge, Resource, Scope string
	Created                                               time.Time
	Attempts                                              int
}
type authCode struct {
	Auth    pendingAuth
	Expires time.Time
}
type issuedToken struct {
	ClientID, Resource, Scope string
	Expires                   time.Time
}

func randomHex(bytesCount int) string {
	b := make([]byte, bytesCount)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func NewBridge(localURL, token, pairCode string) *Bridge {
	return &Bridge{LocalURL: localURL, LocalToken: token, PairCode: pairCode, Clients: map[string][]string{}, Pending: map[string]*pendingAuth{}, Redeemed: map[string]time.Time{}, Codes: map[string]*authCode{}, Tokens: map[string]*issuedToken{}, Refresh: map[string]*issuedToken{}, Client: &http.Client{Timeout: 42 * time.Second}, ready: make(chan struct{})}
}
func (b *Bridge) Activate(publicBase string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Base = publicBase
	close(b.ready)
}
func (b *Bridge) resource() string { return b.Base + "/mcp" }

// A sealed browser ticket prevents a legitimate form from failing only because
// the original in-memory pending request was evicted or lost. Its encryption
// key is derived from the private local daemon token; it is never sent to the
// browser. The public origin is authenticated as AES-GCM additional data.
// Short non-secret correlation ID for comparing the authorize GET and form POST.
func pairingLogID(ticket string) string {
	sum := sha256.Sum256([]byte(ticket))
	return hex.EncodeToString(sum[:4])
}
func (b *Bridge) sealPairing(entry pendingAuth) (string, error) {
	raw, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte("chatgpt-remote/pairing/v1:" + b.LocalToken))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, raw, []byte(b.Base))
	return hex.EncodeToString(sealed), nil
}
func (b *Bridge) unsealPairing(ticket string) (*pendingAuth, error) {
	if len(ticket) < 60 || len(ticket) > 4096 {
		return nil, errors.New("invalid pairing ticket")
	}
	sealed, err := hex.DecodeString(ticket)
	if err != nil {
		return nil, errors.New("invalid pairing ticket")
	}
	key := sha256.Sum256([]byte("chatgpt-remote/pairing/v1:" + b.LocalToken))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("invalid pairing ticket")
	}
	raw, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(b.Base))
	if err != nil {
		return nil, errors.New("invalid pairing ticket")
	}
	var entry pendingAuth
	if err = json.Unmarshal(raw, &entry); err != nil {
		return nil, errors.New("invalid pairing ticket")
	}
	if entry.Created.IsZero() || entry.Created.After(time.Now().Add(time.Minute)) ||
		time.Since(entry.Created) > pairingRequestTTL ||
		entry.Resource != b.resource() || entry.Scope != "mcp:tools" ||
		!validRedirect(entry.Redirect) || entry.ClientID == "" ||
		entry.Challenge == "" || entry.State == "" {
		return nil, errors.New("expired or invalid pairing ticket")
	}
	return &entry, nil
}
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func oauthError(w http.ResponseWriter, status int, code string) {
	respondJSON(w, status, map[string]string{"error": code})
}
func validRedirect(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "chatgpt.com" || u.User != nil || u.Fragment != "" {
		return false
	}
	return u.Path == "/connector_platform_oauth_redirect" || strings.HasPrefix(u.Path, "/connector/oauth/") || strings.HasPrefix(u.Path, "/oauth/")
}
func (b *Bridge) authorizedRedirect(clientID, redirect string) bool {
	if !validRedirect(redirect) {
		return false
	}
	for _, s := range b.Clients[clientID] {
		if s == redirect {
			return true
		}
	}
	return false
}
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	select {
	case <-b.ready:
	default:
		http.Error(w, "tunnel initializing", http.StatusServiceUnavailable)
		return
	}
	base := b.Base
	if base == "" {
		http.Error(w, "tunnel initializing", 503)
		return
	}
	target, _ := url.Parse(base)
	if !strings.EqualFold(r.Host, target.Host) {
		http.Error(w, "unexpected host", 403)
		return
	}
	switch r.URL.Path {
	case "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		respondJSON(w, 200, map[string]any{"resource": b.resource(), "authorization_servers": []string{base}, "scopes_supported": []string{"mcp:tools"}})
	case "/.well-known/oauth-authorization-server", "/.well-known/oauth-authorization-server/mcp":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		respondJSON(w, 200, map[string]any{"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token", "registration_endpoint": base + "/oauth/register", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"}, "code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{"mcp:tools"}, "authorization_response_iss_parameter_supported": true})
	case "/oauth/register":
		b.register(w, r)
	case "/oauth/authorize":
		b.authorize(w, r)
	case "/oauth/token":
		b.token(w, r)
	case "/mcp":
		b.proxy(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (b *Bridge) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
		TokenAuth    string   `json:"token_endpoint_auth_method"`
		GrantTypes   []string `json:"grant_types"`
	}
	if e := json.NewDecoder(r.Body).Decode(&req); e != nil || len(req.RedirectURIs) == 0 || len(req.RedirectURIs) > 8 || (req.TokenAuth != "" && req.TokenAuth != "none") {
		oauthError(w, 400, "invalid_client_metadata")
		return
	}
	for _, v := range req.RedirectURIs {
		if !validRedirect(v) {
			oauthError(w, 400, "invalid_redirect_uri")
			return
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.Clients) >= 50 {
		oauthError(w, 429, "slow_down")
		return
	}
	id := randomHex(20)
	b.Clients[id] = append([]string{}, req.RedirectURIs...)
	respondJSON(w, 201, map[string]any{"client_id": id, "redirect_uris": req.RedirectURIs, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}})
}
func (b *Bridge) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		q := r.URL.Query()
		if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || q.Get("state") == "" {
			oauthError(w, 400, "invalid_request")
			return
		}
		clientID, red := q.Get("client_id"), q.Get("redirect_uri")
		resource := q.Get("resource")
		if resource != "" && resource != b.resource() && resource != b.Base {
			oauthError(w, 400, "invalid_target")
			return
		}
		b.mu.Lock()
		allowed := b.authorizedRedirect(clientID, red)
		b.mu.Unlock()
		if !allowed {
			oauthError(w, 400, "unauthorized_client")
			return
		}
		sc := q.Get("scope")
		if sc != "" && sc != "mcp:tools" {
			oauthError(w, 400, "invalid_scope")
			return
		}
		if resource == "" {
			resource = b.resource()
		}
		entry := pendingAuth{ClientID: clientID, Redirect: red, State: q.Get("state"), Challenge: q.Get("code_challenge"), Resource: resource, Scope: "mcp:tools", Created: time.Now()}
		nonce, sealErr := b.sealPairing(entry)
		if sealErr != nil {
			http.Error(w, "authorization unavailable", http.StatusInternalServerError)
			return
		}
		b.mu.Lock()
		// Keep the pending set bounded. OAuth registration can trigger multiple
		// browser tabs and stale authorizations; expire old sessions on each GET.
		now := time.Now()
		for key, pending := range b.Pending {
			if now.Sub(pending.Created) > pairingRequestTTL {
				delete(b.Pending, key)
			}
		}
		if len(b.Pending) >= 128 {
			b.mu.Unlock()
			oauthError(w, http.StatusTooManyRequests, "slow_down")
			return
		}
		b.Pending[nonce] = &entry
		pendingCount := len(b.Pending)
		b.mu.Unlock()
		log.Printf("oauth: pairing page issued (request=%s active_requests=%d)", pairingLogID(nonce), pendingCount)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		_ = pairingPage.Execute(w, struct{ Nonce string }{nonce})
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if e := r.ParseForm(); e != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	nonce, provided := r.Form.Get("nonce"), r.Form.Get("code")
	log.Printf("oauth: pairing form received (request=%s ticket_bytes=%d)", pairingLogID(nonce), len(nonce))
	b.mu.Lock()
	entry, ok := b.Pending[nonce]
	_, used := b.Redeemed[nonce]
	if !ok && !used {
		// Recover a valid browser session even after a pending map eviction.
		// The server token and public hostname authenticate its contents.
		if restored, err := b.unsealPairing(nonce); err == nil && len(b.Pending) < 128 {
			entry = restored
			b.Pending[nonce] = entry
			ok = true
			log.Printf("oauth: pairing restored from secure browser ticket")
		}
	}
	if !ok || used || time.Since(entry.Created) > pairingRequestTTL {
		log.Printf("oauth: pairing rejected: invalid, consumed or expired request")
		delete(b.Pending, nonce)
		b.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, expiredPairingPage)
		return
	}
	entry.Attempts++
	if entry.Attempts > 5 {
		delete(b.Pending, nonce)
		b.mu.Unlock()
		oauthError(w, 429, "too_many_attempts")
		return
	}
	// Use constant-time equality and consume the request on successful approval.
	if len(provided) != len(b.PairCode) || subtle.ConstantTimeCompare([]byte(provided), []byte(b.PairCode)) != 1 {
		b.mu.Unlock()
		log.Printf("oauth: pairing rejected: incorrect code")
		http.Error(w, "Wrong pairing code. Go back and try again.", 403)
		return
	}
	code := randomHex(32)
	b.Codes[code] = &authCode{Auth: *entry, Expires: time.Now().Add(60 * time.Second)}
	delete(b.Pending, nonce)
	b.Redeemed[nonce] = time.Now()
	// Keep one-time-use markers bounded; full session tickets still expire.
	for key, usedAt := range b.Redeemed {
		if time.Since(usedAt) > pairingRequestTTL {
			delete(b.Redeemed, key)
		}
	}
	b.mu.Unlock()
	log.Printf("oauth: pairing approved; redirecting to ChatGPT")
	dest, _ := url.Parse(entry.Redirect)
	query := dest.Query()
	query.Set("code", code)
	query.Set("state", entry.State)
	query.Set("iss", b.Base)
	dest.RawQuery = query.Encode()
	http.Redirect(w, r, dest.String(), http.StatusSeeOther)
}

// Browsers previously showed only {"error":"expired_request"}, leaving no
// indication of how to recover. An OAuth request is never silently recreated:
// the user must explicitly restart authorization from ChatGPT.
const expiredPairingPage = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>ChatGPT Remote — Session expired</title><style>body{font:16px system-ui,sans-serif;max-width:420px;margin:12vh auto;padding:24px;color:#171717}p{color:#555;line-height:1.5}</style><h2>Pairing session expired</h2><p>The authorization session could not be found in the current tunnel or has expired (expired_request). Close this old page and return to ChatGPT. Remove the old connection, use the URL currently shown in your terminal, and start a fresh authorization. Do not reuse this tab.</p><p>Keep the original chatgpt-remote terminal running. Inspect its OAuth diagnostic lines to see whether a new pairing page was issued. If you restarted it, the Cloudflare URL changed and you must update the MCP connection URL in ChatGPT.</p></html>`

var pairingPage = template.Must(template.New("authorize").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>ChatGPT Remote — Authorize</title><style>body{font:16px system-ui,sans-serif;max-width:420px;margin:12vh auto;padding:24px;color:#171717}input,button{box-sizing:border-box;width:100%;padding:13px;margin-top:13px;font:inherit;border-radius:8px}input{border:1px solid #aaa}button{border:0;background:#202020;color:white;cursor:pointer}p{color:#555;line-height:1.5}</style><h2>Connect ChatGPT Remote</h2><p>Only continue if you started this connection. Enter the pairing code shown in your Linux terminal to allow ChatGPT to access your permitted project files.</p><form method="post" action="/oauth/authorize"><input type="hidden" name="nonce" value="{{.Nonce}}"><input type="text" name="code" autocomplete="off" spellcheck="false" placeholder="Pairing code" required autofocus><button type="submit">Authorize connection</button></form><p>Do not share this code with others.</p></html>`))

func (b *Bridge) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if e := r.ParseForm(); e != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	form := r.PostForm
	b.mu.Lock()
	defer b.mu.Unlock()
	switch form.Get("grant_type") {
	case "authorization_code":
		code := form.Get("code")
		item, ok := b.Codes[code]
		if !ok || time.Now().After(item.Expires) {
			delete(b.Codes, code)
			oauthError(w, 400, "invalid_grant")
			return
		}
		a := item.Auth
		if form.Get("client_id") != a.ClientID || form.Get("redirect_uri") != a.Redirect || (form.Get("resource") != "" && form.Get("resource") != a.Resource) {
			oauthError(w, 400, "invalid_grant")
			return
		}
		h := sha256.Sum256([]byte(form.Get("code_verifier")))
		expected := base64.RawURLEncoding.EncodeToString(h[:])
		if len(expected) != len(a.Challenge) || subtle.ConstantTimeCompare([]byte(expected), []byte(a.Challenge)) != 1 {
			oauthError(w, 400, "invalid_grant")
			return
		}
		delete(b.Codes, code)
		b.issue(w, a.ClientID, a.Resource, a.Scope)
	case "refresh_token":
		key := form.Get("refresh_token")
		tok, ok := b.Refresh[key]
		if !ok || time.Now().After(tok.Expires) || tok.ClientID != form.Get("client_id") || (form.Get("resource") != "" && form.Get("resource") != tok.Resource) {
			oauthError(w, 400, "invalid_grant")
			return
		}
		delete(b.Refresh, key)
		b.issue(w, tok.ClientID, tok.Resource, tok.Scope)
	default:
		oauthError(w, 400, "unsupported_grant_type")
	}
}

// caller holds b.mu
func (b *Bridge) issue(w http.ResponseWriter, clientID, resource, scope string) {
	a := randomHex(32)
	f := randomHex(32)
	b.Tokens[a] = &issuedToken{ClientID: clientID, Resource: resource, Scope: scope, Expires: time.Now().Add(time.Hour)}
	b.Refresh[f] = &issuedToken{ClientID: clientID, Resource: resource, Scope: scope, Expires: time.Now().Add(7 * 24 * time.Hour)}
	respondJSON(w, 200, map[string]any{"access_token": a, "refresh_token": f, "token_type": "Bearer", "expires_in": 3600, "scope": scope})
}
func (b *Bridge) proxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	auth := r.Header.Get("Authorization")
	var token string
	if strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimPrefix(auth, "Bearer ")
	}
	b.mu.Lock()
	p, ok := b.Tokens[token]
	valid := ok && time.Now().Before(p.Expires) && (p.Resource == b.resource() || p.Resource == b.Base) && p.Scope == "mcp:tools"
	b.mu.Unlock()
	if !valid {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource", scope="mcp:tools"`, b.Base))
		http.Error(w, "OAuth required", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	payload, e := io.ReadAll(r.Body)
	if e != nil {
		http.Error(w, "request too large", 413)
		return
	}
	req, e := http.NewRequestWithContext(r.Context(), "POST", b.LocalURL, bytes.NewReader(payload))
	if e != nil {
		http.Error(w, "internal error", 500)
		return
	}
	req.Header.Set("Authorization", "Bearer "+b.LocalToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, e := b.Client.Do(req)
	if e != nil {
		http.Error(w, "local daemon unavailable", 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 2*1024*1024))
}

// Bare `chatgpt-remote` starts an ephemeral, authorized Cloudflare Quick Tunnel.
func RunQuickTunnel() error {
	c, token, e := Load()
	if e != nil {
		return e
	}
	local := "http://" + c.Address + "/mcp"
	// Confirm the installed daemon is really handling authenticated MCP requests.
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req, _ := http.NewRequest("POST", local, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{Timeout: 4 * time.Second}
	resp, e := cli.Do(req)
	if e != nil {
		return fmt.Errorf("local MCP server is not running: %w", e)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("local MCP server returned HTTP %d", resp.StatusCode)
	}
	binary, e := cloudflaredBinary()
	if e != nil {
		return e
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	code := randomHex(12)
	bridge := NewBridge(local, token, code)
	httpServer := &http.Server{Handler: bridge, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 40 * time.Second, WriteTimeout: 45 * time.Second}
	go func() { _ = httpServer.Serve(l) }()
	defer httpServer.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	args := []string{"tunnel", "--no-autoupdate", "--url", "http://" + l.Addr().String()}
	tunnel := exec.CommandContext(ctx, binary, args...)
	stderr, e := tunnel.StderrPipe()
	if e != nil {
		return e
	}
	tunnel.Stdout = io.Discard
	if e = tunnel.Start(); e != nil {
		return e
	}
	defer func() {
		if tunnel.Process != nil {
			_ = tunnel.Process.Kill()
			_ = tunnel.Wait()
		}
	}()
	urls := make(chan string, 1)
	ended := make(chan error, 1)
	re := regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			u := re.FindString(line)
			if u != "" {
				select {
				case urls <- u:
				default:
				}
			}
		}
		ended <- errors.New("Cloudflare tunnel ended before a URL was received")
	}()
	fmt.Printf("ChatGPT Remote v0.3.2 (process %d)\n", os.Getpid())
	fmt.Println("Starting Cloudflare secure MCP connection...")
	select {
	case endpoint := <-urls:
		bridge.Activate(endpoint)
		fmt.Println("\nChatGPT Remote connected to Cloudflare.")
		fmt.Println("\nMCP URL:", endpoint+"/mcp")
		fmt.Println("Pairing code:", code)
		fmt.Println("\nIn ChatGPT: Add custom MCP server → paste the MCP URL → choose OAuth / automatic client registration.")
		fmt.Println("When prompted in the browser, enter the pairing code above.")
		fmt.Println("\nKeep this terminal open. Ctrl+C disconnects. The URL changes on restart.")
	case e := <-ended:
		return e
	case <-time.After(45 * time.Second):
		return errors.New("Cloudflare tunnel URL not received within 45 seconds")
	case <-ctx.Done():
		return nil
	}
	select {
	case <-ctx.Done():
		return nil
	case e := <-ended:
		return e
	}
}
func cloudflaredBinary() (string, error) {
	if path, e := exec.LookPath("cloudflared"); e == nil {
		return path, nil
	}
	d, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	binDir := filepath.Join(d, ".local", "share", "chatgpt-remote", "bin")
	dest := filepath.Join(binDir, "cloudflared")
	if info, e := os.Stat(dest); e == nil && info.Mode().IsRegular() && info.Mode().Perm()&0100 != 0 {
		return dest, nil
	}
	arch := runtime.GOARCH
	if runtime.GOOS != "linux" || (arch != "amd64" && arch != "arm64") {
		return "", errors.New("Cloudflare tunnel supports Linux/WSL on amd64 or arm64")
	}
	fmt.Println("Downloading Cloudflare tunnel client (first run only)...")
	c := &http.Client{Timeout: 45 * time.Second}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/cloudflare/cloudflared/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "chatgpt-remote")
	response, e := c.Do(req)
	if e != nil {
		return "", e
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("Cloudflare release lookup: HTTP %d", response.StatusCode)
	}
	var doc struct {
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if e = json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&doc); e != nil {
		return "", e
	}
	name := "cloudflared-linux-" + arch
	for _, asset := range doc.Assets {
		if asset.Name != name {
			continue
		}
		if !strings.HasPrefix(asset.Digest, "sha256:") || len(asset.Digest) != 71 || asset.Size < 1000000 || asset.Size > 100000000 {
			return "", errors.New("Cloudflare binary is missing verified digest or expected size")
		}
		u, e := url.Parse(asset.URL)
		if e != nil || u.Scheme != "https" || u.Host != "github.com" {
			return "", errors.New("unexpected Cloudflare release URL")
		}
		down, e := c.Get(asset.URL)
		if e != nil {
			return "", e
		}
		defer down.Body.Close()
		if down.StatusCode != 200 {
			return "", fmt.Errorf("Cloudflare binary download: HTTP %d", down.StatusCode)
		}
		if e = os.MkdirAll(binDir, 0700); e != nil {
			return "", e
		}
		temp, e := os.CreateTemp(binDir, ".cloudflared-")
		if e != nil {
			return "", e
		}
		defer os.Remove(temp.Name())
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(temp, h), io.LimitReader(down.Body, asset.Size+1))
		if e != nil {
			_ = temp.Close()
			return "", e
		}
		if n != asset.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), strings.TrimPrefix(asset.Digest, "sha256:")) {
			_ = temp.Close()
			return "", errors.New("cloudflared checksum verification failed")
		}
		if e = temp.Chmod(0700); e != nil {
			_ = temp.Close()
			return "", e
		}
		if e = temp.Close(); e != nil {
			return "", e
		}
		if e = os.Rename(temp.Name(), dest); e != nil {
			return "", e
		}
		return dest, nil
	}
	return "", errors.New("no verified cloudflared release found for this architecture")
}
