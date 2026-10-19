package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/matrixdurden/chatgpt-remote/internal/remote"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func usage() {
	fmt.Println(`chatgpt-remote v0.3.2

Run without arguments to open a secure, temporary Cloudflare MCP tunnel.

Advanced commands:

Commands:
  setup --root /absolute/project/path
  serve [--tunnel-local]       Start local MCP server (loopback only)
  doctor                       Test MCP initialize/tools over localhost
  status                       Show configuration (not token)
  token rotate                 Rotate private local API token

--tunnel-local disables bearer authentication for the LOOPBACK listener only.
Use only with a trusted, access-controlled secure MCP tunnel. Never open this
listener to the Internet. ChatGPT custom MCP access depends on your plan.`)
}
func die(e error) { fmt.Fprintln(os.Stderr, "error:", e); os.Exit(1) }
func main() {
	if len(os.Args) < 2 {
		if err := remote.RunQuickTunnel(); err != nil {
			die(err)
		}
		return
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("chatgpt-remote v0.3.2")
	case "setup":
		fs := flag.NewFlagSet("setup", flag.ExitOnError)
		root := fs.String("root", "", "Existing project directory to share")
		_ = fs.Parse(os.Args[2:])
		if *root == "" {
			die(fmt.Errorf("--root is required"))
		}
		p, e := remote.Setup(*root)
		if e != nil {
			die(e)
		}
		fmt.Println("Saved configuration:", p)
		fmt.Println("Run: chatgpt-remote serve")
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		tunnel := fs.Bool("tunnel-local", false, "Allow no-auth local requests for a trusted secure tunnel")
		_ = fs.Parse(os.Args[2:])
		c, token, e := remote.Load()
		if e != nil {
			die(e)
		}
		s := remote.Server{Config: c, Token: token, NoAuthLocal: *tunnel}
		if e = s.Validate(); e != nil {
			die(e)
		}
		fmt.Printf("MCP endpoint: http://%s/mcp\nAccessible roots: %s\nCommand execution: %t\n", c.Address, strings.Join(c.Roots, ", "), c.EnableCommands)
		if *tunnel {
			fmt.Println("WARNING: loopback endpoint has NO AUTHENTICATION. Allow only a trusted access-controlled tunnel.")
		}
		h := &http.Server{Addr: c.Address, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second}
		die(h.ListenAndServe())
	case "status":
		c, _, e := remote.Load()
		if e != nil {
			die(e)
		}
		fmt.Printf("MCP: http://%s/mcp\nRoots: %s\nCommands enabled: %t\nAllowed programs: %s\n", c.Address, strings.Join(c.Roots, ", "), c.EnableCommands, strings.Join(c.AllowedCommands, ", "))
	case "token":
		if len(os.Args) != 3 || os.Args[2] != "rotate" {
			die(fmt.Errorf("expected token rotate"))
		}
		_, e := remote.RotateToken()
		if e != nil {
			die(e)
		}
		fmt.Println("Token rotated. Restart server to apply.")
	case "doctor":
		c, token, e := remote.Load()
		if e != nil {
			die(e)
		}
		url := "http://" + c.Address + "/mcp"
		client := &http.Client{Timeout: 4 * time.Second}
		for _, m := range []string{"initialize", "tools/list"} {
			payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": m}
			if m == "initialize" {
				payload["params"] = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "chatgpt-remote-doctor", "version": "0.1.0"}}
			}
			b, _ := json.Marshal(payload)
			req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			res, err := client.Do(req)
			if err != nil {
				die(fmt.Errorf("%s: %w", m, err))
			}
			raw, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
			_ = res.Body.Close()
			if res.StatusCode != 200 {
				die(fmt.Errorf("%s: HTTP %d: %s", m, res.StatusCode, raw))
			}
			fmt.Printf("%s OK (%s)\n", m, res.Status)
		}
		fmt.Println("MCP local test passed. External ChatGPT connection not tested.")
	case "config-path":
		p, e := remote.ConfigPath()
		if e != nil {
			die(e)
		}
		fmt.Println(filepath.Clean(p))
	default:
		usage()
	}
}
