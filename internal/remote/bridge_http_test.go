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

// Verify DCR, authorization, form POST, PKCE, token and MCP proxy
// through real local HTTP connections rather than direct handler calls.
func TestOAuthRealHTTPRoundTrip(t *testing.T) {
  backend:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    if r.URL.Path!="/mcp" || r.Header.Get("Authorization")!="Bearer local-secret" {http.Error(w,"unauthorized",401);return}
    w.Header().Set("Content-Type","application/json")
    _,_=io.WriteString(w, "{\\"jsonrpc\\":\\"2.0\\",\\"id\\":1,\\"result\\":{\\"tools\\":[{\\"name\\":\\"get_status\\"}]}}")
  }))
  defer backend.Close()
  b:=NewBridge(backend.URL+"/mcp","local-secret","pairing-123")
  base:="https://oauth-check.trycloudflare.com"
  b.Activate(base)
  httpSrv:=httptest.NewServer(b);defer httpSrv.Close()
  cli:=&http.Client{Timeout:5*time.Second,CheckRedirect:func(_ *http.Request,_ []*http.Request)error{return http.ErrUseLastResponse}}
  call:=func(method,path,body,ctype,bearer string)(int,string,http.Header){
    t.Helper()
    req,e:=http.NewRequest(method,httpSrv.URL+path,strings.NewReader(body));if e!=nil{t.Fatal(e)}
    req.Host="oauth-check.trycloudflare.com"
    if ctype!=""{req.Header.Set("Content-Type",ctype)}
    if bearer!=""{req.Header.Set("Authorization","Bearer "+bearer)}
    res,e:=cli.Do(req);if e!=nil{t.Fatal(e)};defer res.Body.Close()
    raw,_:=io.ReadAll(io.LimitReader(res.Body,128*1024))
    return res.StatusCode,string(raw),res.Header
  }
  for _,path:=range []string{"/.well-known/oauth-protected-resource","/.well-known/oauth-authorization-server"}{
    status,_,_:=call("GET",path,"","","");if status!=200{t.Fatalf("discovery: %d",status)}
  }
  callback:="https://chatgpt.com/connector_platform_oauth_redirect"
  status,body,_:=call("POST","/oauth/register",fmt.Sprintf("{\\"redirect_uris\\":[%q],\\"token_endpoint_auth_method\\":\\"none\\"}",callback),"application/json","")
  if status!=201{t.Fatalf("registration: %d %s",status,body)}
  var reg struct{ClientID string `json:"client_id"`}
  if e:=json.Unmarshal([]byte(body),&reg);e!=nil{t.Fatal(e)}
  verifier:="valid-pkce-verifier-valid-pkce-verifier-valid-pkce-verifier"
  hash:=sha256.Sum256([]byte(verifier))
  challenge:=base64.RawURLEncoding.EncodeToString(hash[:])
  query:=url.Values{"response_type":{"code"},"client_id":{reg.ClientID},"redirect_uri":{callback},"state":{"state-123"},"code_challenge":{challenge},"code_challenge_method":{"S256"},"resource":{base+"/mcp"}}
  status,body,_=call("GET","/oauth/authorize?"+query.Encode(),"","","")
  if status!=200{t.Fatalf("authorize: %d %s",status,body)}
  match:=regexp.MustCompile("name=\\"nonce\\" value=\\"([a-f0-9]+)\\"").FindStringSubmatch(body)
  if len(match)!=2{t.Fatal("nonce missing")}
  form:=url.Values{"nonce":{match[1]},"code":{"pairing-123"}}
  status,_,h:=call("POST","/oauth/authorize",form.Encode(),"application/x-www-form-urlencoded","")
  if status!=303{t.Fatalf("approval: %d",status)}
  red,e:=url.Parse(h.Get("Location"));if e!=nil{t.Fatal(e)}
  if red.Host!="chatgpt.com" || red.Query().Get("state")!="state-123"{t.Fatalf("callback: %s",red)}
  token:=url.Values{"grant_type":{"authorization_code"},"code":{red.Query().Get("code")},"code_verifier":{verifier},"client_id":{reg.ClientID},"redirect_uri":{callback},"resource":{base+"/mcp"}}
  status,body,_=call("POST","/oauth/token",token.Encode(),"application/x-www-form-urlencoded","")
  if status!=200{t.Fatalf("token: %d %s",status,body)}
  var tok struct{Access string `json:"access_token"`}
  if e:=json.Unmarshal([]byte(body),&tok);e!=nil{t.Fatal(e)}
  if tok.Access==""{t.Fatal("missing token")}
  status,body,_=call("POST","/mcp","{\\"jsonrpc\\":\\"2.0\\",\\"id\\":1,\\"method\\":\\"tools/list\\"}","application/json",tok.Access)
  if status!=200 || !strings.Contains(body,"get_status"){t.Fatalf("MCP: %d %s",status,body)}
  status,body,_=call("POST","/oauth/authorize",form.Encode(),"application/x-www-form-urlencoded","")
  if status!=400 || !strings.Contains(body,"Close this old page"){t.Fatalf("stale form: %d %s",status,body)}
}