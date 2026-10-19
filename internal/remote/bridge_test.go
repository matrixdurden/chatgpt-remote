package remote

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func requestBridge(b *Bridge, method, uri, body, contentType, bearer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, uri, strings.NewReader(body))
	r.Host = "example.trycloudflare.com"
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w
}
func TestBridgeOAuthEndToEnd(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-secret" {
			http.Error(w, "bad token", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
	}))
	defer backend.Close()
	b := NewBridge(backend.URL+"/mcp", "local-secret", "0123456789abcdef12345678")
	b.Activate("https://example.trycloudflare.com")
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server"} {
		res := requestBridge(b, "GET", "https://example.trycloudflare.com"+path, "", "", "")
		if res.Code != 200 {
			t.Fatalf("metadata %s: %d", path, res.Code)
		}
	}
	unauthorized := requestBridge(b, "POST", "https://example.trycloudflare.com/mcp", `{"jsonrpc":"2.0"}`, "application/json", "")
	if unauthorized.Code != 401 || !strings.Contains(unauthorized.Header().Get("WWW-Authenticate"), "resource_metadata") {
		t.Fatalf("missing OAuth challenge %d", unauthorized.Code)
	}
	badClient := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/register", `{"redirect_uris":["https://evil.example/callback"]}`, "application/json", "")
	if badClient.Code != 400 {
		t.Fatalf("accepted hostile client: %d", badClient.Code)
	}
	callback := "https://chatgpt.com/connector_platform_oauth_redirect"
	registration := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/register", `{"redirect_uris":["`+callback+`"],"token_endpoint_auth_method":"none"}`, "application/json", "")
	if registration.Code != 201 {
		t.Fatalf("registration failed %d %s", registration.Code, registration.Body.String())
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if e := json.Unmarshal(registration.Body.Bytes(), &reg); e != nil {
		t.Fatal(e)
	}
	verifier := "test-verifier-test-verifier-test-verifier-test-verifier"
	dig := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(dig[:])
	params := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {callback}, "state": {"test-state"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {"https://example.trycloudflare.com/mcp"}}
	login := requestBridge(b, "GET", "https://example.trycloudflare.com/oauth/authorize?"+params.Encode(), "", "", "")
	if login.Code != 200 {
		t.Fatalf("login failed %d %s", login.Code, login.Body.String())
	}
	nonce := regexp.MustCompile(`name="nonce" value="([a-f0-9]+)"`).FindStringSubmatch(login.Body.String())
	if len(nonce) < 2 {
		t.Fatal("missing nonce")
	}
	bad := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/authorize", url.Values{"nonce": {nonce[1]}, "code": {"wrong"}}.Encode(), "application/x-www-form-urlencoded", "")
	if bad.Code != 403 {
		t.Fatalf("accepted wrong pairing code: %d", bad.Code)
	}
	// A browser approval after six minutes must still be accepted.
	b.mu.Lock()
	b.Pending[nonce[1]].Created = time.Now().Add(-6 * time.Minute)
	b.mu.Unlock()
	grant := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/authorize", url.Values{"nonce": {nonce[1]}, "code": {"0123456789abcdef12345678"}}.Encode(), "application/x-www-form-urlencoded", "")
	if grant.Code != 303 {
		t.Fatalf("consent failed: %d %s", grant.Code, grant.Body.String())
	}
	red, e := url.Parse(grant.Header().Get("Location"))
	if e != nil {
		t.Fatal(e)
	}
	if red.Host != "chatgpt.com" || red.Query().Get("state") != "test-state" || red.Query().Get("iss") != "https://example.trycloudflare.com" {
		t.Fatalf("invalid redirect %s", red)
	}
	code := red.Query().Get("code")
	tokenReq := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "client_id": {reg.ClientID}, "redirect_uri": {callback}, "resource": {"https://example.trycloudflare.com/mcp"}}
	wrong := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {"bad"}, "client_id": {reg.ClientID}, "redirect_uri": {callback}}
	failed := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/token", wrong.Encode(), "application/x-www-form-urlencoded", "")
	if failed.Code != 400 {
		t.Fatalf("accepted wrong PKCE: %d", failed.Code)
	}
	issued := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/token", tokenReq.Encode(), "application/x-www-form-urlencoded", "")
	if issued.Code != 200 {
		t.Fatalf("token issue failed %d %s", issued.Code, issued.Body.String())
	}
	var tokens struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if e := json.Unmarshal(issued.Body.Bytes(), &tokens); e != nil {
		t.Fatal(e)
	}
	ok := requestBridge(b, "POST", "https://example.trycloudflare.com/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "application/json", tokens.Access)
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), "tools") {
		t.Fatalf("proxy: %d %s", ok.Code, ok.Body.String())
	}
	repeated := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/token", tokenReq.Encode(), "application/x-www-form-urlencoded", "")
	if repeated.Code != 400 {
		t.Fatalf("reused authorization code: %d", repeated.Code)
	}
	newToken := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens.Refresh}, "client_id": {reg.ClientID}}.Encode(), "application/x-www-form-urlencoded", "")
	if newToken.Code != 200 {
		t.Fatalf("refresh: %d %s", newToken.Code, newToken.Body.String())
	}
}
func TestBridgeRejectsHostMismatch(t *testing.T) {
	b := NewBridge("http://127.0.0.1:8765/mcp", "token", "code")
	b.Activate("https://test.trycloudflare.com")
	r := httptest.NewRequest("GET", "https://bad.example/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("unexpected status %d", w.Code)
	}
}

func TestBridgeExpiredPairingHasRecoveryInstructions(t *testing.T) {
	b := NewBridge("http://127.0.0.1:8765/mcp", "local-secret", "0123456789abcdef12345678")
	b.Activate("https://example.trycloudflare.com")
	// A stale or invalid request is a recoverable browser error, not raw JSON.
	for _, nonce := range []string{"unknown", "expired"} {
		if nonce == "expired" {
			b.mu.Lock()
			b.Pending[nonce] = &pendingAuth{Created: time.Now().Add(-31 * time.Minute)}
			b.mu.Unlock()
		}
		res := requestBridge(b, "POST", "https://example.trycloudflare.com/oauth/authorize", url.Values{"nonce": {nonce}, "code": {"0123456789abcdef12345678"}}.Encode(), "application/x-www-form-urlencoded", "")
		if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "expired_request") || !strings.Contains(res.Body.String(), "return to ChatGPT") {
			t.Fatalf("unexpected expired approval result: %d %s", res.Code, res.Body.String())
		}
	}
}

func TestPairingTicketSurvivesPendingEvictionAndRestart(t *testing.T) {
	orig := NewBridge("http://127.0.0.1:8765/mcp", "stable-private-token", "my-pairing-code")
	orig.Activate("https://example.trycloudflare.com")
	entry := pendingAuth{ClientID: "client1", Redirect: "https://chatgpt.com/connector_platform_oauth_redirect", State: "state", Challenge: "abcdef", Resource: orig.resource(), Scope: "mcp:tools", Created: time.Now()}
	ticket, err := orig.sealPairing(entry)
	if err != nil {
		t.Fatal(err)
	}
	// Another bridge process, but the same local secret and public hostname.
	fresh := NewBridge("http://127.0.0.1:8765/mcp", "stable-private-token", "my-pairing-code")
	fresh.Activate("https://example.trycloudflare.com")
	form := url.Values{"nonce": {ticket}, "code": {"my-pairing-code"}}
	r := requestBridge(fresh, "POST", "https://example.trycloudflare.com/oauth/authorize", form.Encode(), "application/x-www-form-urlencoded", "")
	if r.Code != http.StatusSeeOther {
		t.Fatalf("fresh process should restore signed ticket: %d %s", r.Code, r.Body.String())
	}
	// Same approval is never accepted twice.
	repeated := requestBridge(fresh, "POST", "https://example.trycloudflare.com/oauth/authorize", form.Encode(), "application/x-www-form-urlencoded", "")
	if repeated.Code != http.StatusBadRequest {
		t.Fatalf("reused ticket must fail: %d", repeated.Code)
	}
}

func TestPairingTicketRejectsTamperingWrongOriginAndExpiry(t *testing.T) {
	bridge := NewBridge("http://127.0.0.1:8765/mcp", "stable-private-token", "pairing")
	bridge.Activate("https://test.trycloudflare.com")
	entry := pendingAuth{ClientID: "client1", Redirect: "https://chatgpt.com/connector_platform_oauth_redirect", State: "state", Challenge: "abcdef", Resource: bridge.resource(), Scope: "mcp:tools", Created: time.Now()}
	ticket, err := bridge.sealPairing(entry)
	if err != nil {
		t.Fatal(err)
	}
	flipped := []byte(ticket)
	if flipped[len(flipped)-1] == '0' {
		flipped[len(flipped)-1] = '1'
	} else {
		flipped[len(flipped)-1] = '0'
	}
	if _, err = bridge.unsealPairing(string(flipped)); err == nil {
		t.Fatal("tampered ticket accepted")
	}
	other := NewBridge("http://127.0.0.1:8765/mcp", "stable-private-token", "pairing")
	other.Activate("https://other.trycloudflare.com")
	if _, err = other.unsealPairing(ticket); err == nil {
		t.Fatal("ticket valid on other origin")
	}
	otherToken := NewBridge("http://127.0.0.1:8765/mcp", "different-private-token", "pairing")
	otherToken.Activate("https://test.trycloudflare.com")
	if _, err = otherToken.unsealPairing(ticket); err == nil {
		t.Fatal("ticket valid with wrong token")
	}
	entry.Created = time.Now().Add(-31 * time.Minute)
	oldTicket, err := bridge.sealPairing(entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.unsealPairing(oldTicket); err == nil {
		t.Fatal("expired ticket accepted")
	}
}
