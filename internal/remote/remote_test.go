package remote

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileAccessAndSha(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	f := FileOps{[]string{root}}
	target := filepath.Join(root, "demo.txt")
	if _, e := f.Write(target, "one", "", true); e != nil {
		t.Fatal(e)
	}
	r, e := f.Read(target)
	if e != nil {
		t.Fatal(e)
	}
	if r["content"] != "one" {
		t.Fatal(r)
	}
	if _, e := f.Write(target, "two", "wrong", false); e == nil {
		t.Fatal("write with wrong sha succeeded")
	}
	if _, e := f.Write(target, "two", r["sha256"].(string), false); e != nil {
		t.Fatal(e)
	}
	if _, e := f.Read(filepath.Join(outside, "no.txt")); e == nil {
		t.Fatal("outside access succeeded")
	}
	if _, e := f.Write(filepath.Join(outside, "escape.txt"), "x", "", true); e == nil {
		t.Fatal("outside write succeeded")
	}
	link := filepath.Join(root, "escape")
	if e := os.Symlink(outside, link); e != nil {
		t.Skip("symlink not supported")
	}
	if _, e := f.Write(filepath.Join(link, "escape.txt"), "x", "", true); e == nil {
		t.Fatal("symlink escape succeeded")
	}
}
func TestMCP(t *testing.T) {
	root := t.TempDir()
	s := Server{Config: Config{Address: DefaultAddr, Roots: []string{root}}, Token: strings.Repeat("a", 64)}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	call := func(method string, auth string, params any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		r, e := http.NewRequest("POST", srv.URL+"/mcp", bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Content-Type", "application/json")
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		res, e := srv.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var response map[string]any
		_ = json.NewDecoder(res.Body).Decode(&response)
		return res.StatusCode, response
	}
	if code, _ := call("initialize", "", nil); code != 401 {
		t.Fatalf("expected 401 got %d", code)
	}
	if code, result := call("initialize", s.Token, map[string]any{"protocolVersion": "2025-11-25"}); code != 200 || result["result"] == nil {
		t.Fatalf("initialization failed: %d %v", code, result)
	}
	if code, result := call("tools/list", s.Token, nil); code != 200 || result["result"] == nil {
		t.Fatalf("tools/list failed: %d %v", code, result)
	}
	if code, result := call("tools/call", s.Token, map[string]any{"name": "get_status", "arguments": map[string]any{}}); code != 200 || result["result"] == nil {
		t.Fatalf("get_status failed %v", result)
	}
	if code, result := call("tools/call", s.Token, map[string]any{"name": "run_command", "arguments": map[string]any{"directory": root, "program": "git"}}); code != 200 || !result["result"].(map[string]any)["isError"].(bool) {
		t.Fatalf("disabled command was allowed %v", result)
	}
}
func TestNoAuthLoopbackAndOrigin(t *testing.T) {
	s := Server{Config: Config{Address: "0.0.0.0:8765", Roots: []string{"/tmp"}}, NoAuthLocal: true}
	if s.Validate() == nil {
		t.Fatal("non-loopback accepted")
	}
	s.Config.Address = DefaultAddr
	if e := s.Validate(); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "http://localhost/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://attacker.invalid")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("expected origin block, got %d", w.Code)
	}
}
