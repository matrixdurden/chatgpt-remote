package remote

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultAddr = "127.0.0.1:8765"

type Config struct {
	Address         string   `json:"address"`
	Roots           []string `json:"roots"`
	EnableCommands  bool     `json:"enable_commands"`
	AllowedCommands []string `json:"allowed_commands"`
}

func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "chatgpt-remote"), nil
}
func ConfigPath() (string, error) { d, e := ConfigDir(); return filepath.Join(d, "config.json"), e }
func TokenPath() (string, error)  { d, e := ConfigDir(); return filepath.Join(d, "token"), e }

func Setup(root string) (string, error) {
	p, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	p, e = filepath.EvalSymlinks(p)
	if e != nil {
		return "", fmt.Errorf("project root: %w", e)
	}
	st, e := os.Stat(p)
	if e != nil || !st.IsDir() {
		return "", errors.New("project root must be an existing directory")
	}
	dir, e := ConfigDir()
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	if e = os.Chmod(dir, 0700); e != nil {
		return "", e
	}
	path, _ := ConfigPath()
	if _, e = os.Stat(path); e == nil {
		return "", fmt.Errorf("%s already exists; edit roots in that file manually", path)
	}
	cfg := Config{Address: DefaultAddr, Roots: []string{p}, EnableCommands: false, AllowedCommands: []string{"git", "go"}}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	b = append(b, '\n')
	if e = os.WriteFile(path, b, 0600); e != nil {
		return "", e
	}
	tokpath, _ := TokenPath()
	if _, e = os.Stat(tokpath); errors.Is(e, os.ErrNotExist) {
		if _, e = RotateToken(); e != nil {
			return "", e
		}
	}
	return path, nil
}
func RotateToken() (string, error) {
	d, e := ConfigDir()
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(d, 0700); e != nil {
		return "", e
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		return "", e
	}
	token := hex.EncodeToString(raw)
	p, _ := TokenPath()
	f, e := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if e != nil {
		return "", e
	}
	_, e = f.WriteString(token + "\n")
	closeErr := f.Close()
	if e != nil {
		return "", e
	}
	return token, closeErr
}
func Load() (Config, string, error) {
	var c Config
	p, e := ConfigPath()
	if e != nil {
		return c, "", e
	}
	raw, e := os.ReadFile(p)
	if e != nil {
		return c, "", fmt.Errorf("cannot read config; run setup: %w", e)
	}
	if e = json.Unmarshal(raw, &c); e != nil {
		return c, "", e
	}
	if c.Address == "" {
		c.Address = DefaultAddr
	}
	if len(c.Roots) == 0 {
		return c, "", errors.New("at least one root is required")
	}
	for i, p := range c.Roots {
		p, e = filepath.Abs(p)
		if e != nil {
			return c, "", e
		}
		p, e = filepath.EvalSymlinks(p)
		if e != nil {
			return c, "", fmt.Errorf("root %q: %w", c.Roots[i], e)
		}
		st, e := os.Stat(p)
		if e != nil || !st.IsDir() {
			return c, "", fmt.Errorf("invalid root %s", p)
		}
		c.Roots[i] = p
	}
	tokenPath, _ := TokenPath()
	b, e := os.ReadFile(tokenPath)
	if e != nil {
		return c, "", fmt.Errorf("token missing; run token rotate: %w", e)
	}
	tok := strings.TrimSpace(string(b))
	if len(tok) < 48 {
		return c, "", errors.New("invalid token")
	}
	return c, tok, nil
}
