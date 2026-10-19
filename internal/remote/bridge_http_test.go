package remote

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// This exercises the full OAuth chain using actual TCP/HTTP requests. It
// simulates a ChatGPT DCR + PKCE client, a reverse-proxy Host header, and the
// private MCP backend, rather than invoking handlers directly.
func TestOAuthRealHTTPRoundTrip(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || r.Header.Get("Authorization") != "Bearer local-secret" {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"get_status"}]}}`)
	}))
	defer backend.Close()
	bridge := NewBridge(backend.URL+"/mcp", "local-secret", "pairing-123")
	base := "https://oauth-check.trycloudflare.com"
	bridge.Activate(base)
	httpSrv := httptest.NewServer(bridge)
	defer httpSrv.Close()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	exchange := func(method, path, content, ctype, bearer string) (int, string, http.Header) {
		t.Helper()
		req, err := http.NewRequest(method, httpSrv.URL+path, strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "oauth-check.trycloudflare.com"
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 128*1024))
		return res.StatusCode, string(raw), res.Header
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server"} {
		status, _, _ := exchange("GET", path, "", "", "")
		if status != 200 {
			t.Fatalf("discovery %s: %d", path, status)
		}
	}
	callback := "https://chatgpt.com/connector_platform_oauth_redirect"
	status, response, _ := exchange("POST", "/oauth/register", fmt.Sprintf(`{"redirect_uris":[%q],"token_endpoint_auth_method":"none"}`, callback), "application/json", "")
	if status != 201 {
		t.Fatalf("DCR: %d %s", status, response)
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal([]byte(response), &reg); err != nil {
		t.Fatal(err)
	}
	verifier := "valid-pkce-verifier-valid-pkce-verifier-valid-pkce-verifier"
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	query := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {callback}, "state": {"state-123"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {base + "/mcp"}}
	status, form, _ := exchange("GET", "/oauth/authorize?"+query.Encode(), "", "", "")
	if status != 200 {
		t.Fatalf("login: %d %s", status, form)
	}
	nonce := regexp.MustCompile(`name="nonce" value="([0-9a-f]+)"`).FindStringSubmatch(form)
	if len(nonce) != 2 {
		t.Fatalf("nonce missing: %s", form)
	}
	approval := url.Values{"nonce": {nonce[1]}, "code": {"pairing-123"}}
	status, _, headers := exchange("POST", "/oauth/authorize", approval.Encode(), "application/x-www-form-urlencoded", "")
	if status != 303 {
		t.Fatalf("approval: %d", status)
	}
	redirect, err := url.Parse(headers.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if redirect.Host != "chatgpt.com" || redirect.Query().Get("state") != "state-123" {
		t.Fatalf("bad redirect: %s", redirect)
	}
	tokenReq := url.Values{"grant_type": {"authorization_code"}, "code": {redirect.Query().Get("code")}, "code_verifier": {verifier}, "client_id": {reg.ClientID}, "redirect_uri": {callback}, "resource": {base + "/mcp"}}
	status, response, _ = exchange("POST", "/oauth/token", tokenReq.Encode(), "application/x-www-form-urlencoded", "")
	if status != 200 {
		t.Fatalf("token: %d %s", status, response)
	}
	var token struct {
		Access string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(response), &token); err != nil {
		t.Fatal(err)
	}
	if token.Access == "" {
		t.Fatal("empty access token")
	}
	status, response, _ = exchange("POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "application/json", token.Access)
	if status != 200 || !strings.Contains(response, `"get_status"`) {
		t.Fatalf("MCP proxy: %d %s", status, response)
	}
	// Approvals are single-use; a stale browser tab must receive helpful HTML.
	status, response, _ = exchange("POST", "/oauth/authorize", approval.Encode(), "application/x-www-form-urlencoded", "")
	if status != 400 || !strings.Contains(response, "expired_request") || !strings.Contains(response, "text/html") && !strings.Contains(response, "Close this old page") {
		t.Fatalf("stale form: %d %s", status, response)
	}
}
