package remote

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

type Server struct {
	Config      Config
	Token       string
	NoAuthLocal bool
}
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func resp(id json.RawMessage, result any) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}
func rpcErr(id json.RawMessage, code int, msg string) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{code, msg}}
}

func Schema(description string, props map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "description": description, "properties": props, "required": required, "additionalProperties": false}
}
func prop(t, d string) map[string]any { return map[string]any{"type": t, "description": d} }
func (s Server) Tools() []map[string]any {
	path := prop("string", "Absolute path within one of the configured roots")
	tools := []map[string]any{
		{"name": "list_files", "description": "List files/directories at an absolute path inside an allowed project root.", "inputSchema": Schema("List directory entries", map[string]any{"path": path, "limit": prop("integer", "Maximum entries, 1-200")}, "path"), "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "read_file", "description": "Read a UTF-8 text file and receive its sha256 for safe edits (up to 512 KiB).", "inputSchema": Schema("Read file", map[string]any{"path": path}, "path"), "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "search_text", "description": "Search exact case-sensitive text under a directory, ignoring .git/node_modules; returns paths and line numbers.", "inputSchema": Schema("Search code", map[string]any{"path": path, "query": prop("string", "Literal search text"), "limit": prop("integer", "Max 50 matches")}, "path", "query"), "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "write_file", "description": "Create a new text file (create=true) or atomically replace an existing file after checking its sha256. For editing, first call read_file. This changes project files.", "inputSchema": Schema("Write full UTF-8 text file", map[string]any{"path": path, "content": prop("string", "Full file contents"), "expected_sha256": prop("string", "Required for edits: previous read_file sha256"), "create": prop("boolean", "True only to create absent file")}, "path", "content"), "annotations": map[string]any{"readOnlyHint": false, "destructiveHint": true}},
		{"name": "run_command", "description": "Run an allowlisted executable with argument array (no shell) in an allowed directory. Disabled by default. Command subprocesses have the OS user's privileges; do not treat allowlist as a security sandbox.", "inputSchema": Schema("Run command", map[string]any{"directory": path, "program": prop("string", "Allowlisted executable name (e.g. git)"), "args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "timeout_seconds": prop("integer", "Maximum 30 seconds")}, "directory", "program"), "annotations": map[string]any{"readOnlyHint": false, "destructiveHint": true}},
		{"name": "get_status", "description": "Return service version, accessible roots, and whether commands are enabled (never returns credentials).", "inputSchema": Schema("Get status", map[string]any{}), "annotations": map[string]any{"readOnlyHint": true}},
	}
	return tools
}
func loopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if origin := r.Header.Get("Origin"); origin != "" {
			http.Error(w, "Origin not accepted", http.StatusForbidden)
			return
		}
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", 405)
			return
		}
		if !s.NoAuthLocal {
			given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if len(given) != len(s.Token) || subtle.ConstantTimeCompare([]byte(given), []byte(s.Token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="chatgpt-remote"`)
				http.Error(w, "unauthorized", 401)
				return
			}
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			http.Error(w, "expected JSON", 415)
			return
		}
		body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024*1024))
		if e != nil {
			http.Error(w, "payload too large", 413)
			return
		}
		var req rpcRequest
		if e = json.Unmarshal(body, &req); e != nil || req.JSONRPC != "2.0" || req.Method == "" {
			send(w, rpcErr(json.RawMessage("null"), -32600, "invalid JSON-RPC request"))
			return
		}
		if len(req.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		switch req.Method {
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(req.Params, &params)
			v := params.ProtocolVersion
			if v != "2025-03-26" && v != "2025-06-18" && v != "2025-11-25" {
				v = "2025-11-25"
			}
			send(w, resp(req.ID, map[string]any{"protocolVersion": v, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "chatgpt-remote", "version": "0.1.0"}, "instructions": "Operate only within explicitly configured roots; confirm writes and terminal operations."}))
		case "ping":
			send(w, resp(req.ID, map[string]any{}))
		case "tools/list":
			send(w, resp(req.ID, map[string]any{"tools": s.Tools()}))
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if e = json.Unmarshal(req.Params, &p); e != nil {
				send(w, rpcErr(req.ID, -32602, "invalid params"))
				return
			}
			data, e := s.call(r, p.Name, p.Arguments)
			if e != nil {
				send(w, resp(req.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": "Error: " + e.Error()}}, "isError": true}))
				return
			}
			b, _ := json.Marshal(data)
			send(w, resp(req.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "isError": false}))
		default:
			send(w, rpcErr(req.ID, -32601, "method not found"))
		}
	})
	return mux
}
func send(w http.ResponseWriter, r *rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(r)
}
func (s Server) call(r *http.Request, name string, raw json.RawMessage) (any, error) {
	var a struct {
		Path      string   `json:"path"`
		Query     string   `json:"query"`
		Limit     int      `json:"limit"`
		Content   string   `json:"content"`
		Expected  string   `json:"expected_sha256"`
		Create    bool     `json:"create"`
		Directory string   `json:"directory"`
		Program   string   `json:"program"`
		Args      []string `json:"args"`
		Timeout   int      `json:"timeout_seconds"`
	}
	if len(raw) > 0 {
		if e := json.Unmarshal(raw, &a); e != nil {
			return nil, e
		}
	}
	f := FileOps{s.Config.Roots}
	switch name {
	case "list_files":
		return f.List(a.Path, a.Limit)
	case "read_file":
		return f.Read(a.Path)
	case "search_text":
		return f.Search(a.Path, a.Query, a.Limit)
	case "write_file":
		return f.Write(a.Path, a.Content, a.Expected, a.Create)
	case "run_command":
		return RunCommand(r.Context(), s.Config, a.Directory, a.Program, a.Args, a.Timeout)
	case "get_status":
		return map[string]any{"version": "0.1.0", "roots": s.Config.Roots, "commands_enabled": s.Config.EnableCommands, "allowed_commands": s.Config.AllowedCommands}, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}
func (s Server) Validate() error {
	if !loopbackAddr(s.Config.Address) {
		return errors.New("server must listen on a loopback IP, e.g. 127.0.0.1:8765")
	}
	if s.NoAuthLocal && !loopbackAddr(s.Config.Address) {
		return errors.New("unauthenticated requires loopback bind")
	}
	if !s.NoAuthLocal && s.Token == "" {
		return errors.New("missing auth token")
	}
	return nil
}
