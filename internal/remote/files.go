package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxRead = 512 * 1024
const maxWrite = 512 * 1024

type FileOps struct{ Roots []string }

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// resolve restricts a path to a configured root after following existing symlinks.
// This is not a race-proof OS sandbox: do not grant access to hostile local users.
func (f FileOps) resolve(path string, create bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if create {
		dir, err := filepath.EvalSymlinks(filepath.Dir(p))
		if err != nil {
			return "", err
		}
		p = filepath.Join(dir, filepath.Base(p))
		if _, err := os.Lstat(p); err == nil {
			p, err = filepath.EvalSymlinks(p)
			if err != nil {
				return "", err
			}
		}
	} else {
		p, err = filepath.EvalSymlinks(p)
		if err != nil {
			return "", err
		}
	}
	for _, root := range f.Roots {
		if within(root, p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("access denied: %q outside configured roots", path)
}
func checksum(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func (f FileOps) Read(path string) (map[string]any, error) {
	p, e := f.resolve(path, false)
	if e != nil {
		return nil, e
	}
	st, e := os.Stat(p)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if st.Size() > maxRead {
		return nil, errors.New("file larger than 512 KiB")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return nil, e
	}
	if !utf8.Valid(b) {
		return nil, errors.New("binary/non-UTF8 file unsupported")
	}
	return map[string]any{"path": p, "content": string(b), "sha256": checksum(b)}, nil
}
func (f FileOps) Write(path, content, expected string, create bool) (map[string]any, error) {
	if len(content) > maxWrite {
		return nil, errors.New("content exceeds 512 KiB")
	}
	p, e := f.resolve(path, true)
	if e != nil {
		return nil, e
	}
	old, e := os.ReadFile(p)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	exists := e == nil
	if create && exists {
		return nil, errors.New("file already exists")
	}
	if !create && !exists {
		return nil, errors.New("file does not exist; use create=true")
	}
	if exists && (expected == "" || checksum(old) != expected) {
		return nil, errors.New("sha256 mismatch (or missing): re-read file before writing")
	}
	mode := fs.FileMode(0600)
	if exists {
		st, e := os.Stat(p)
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			return nil, errors.New("not a regular file")
		}
		mode = st.Mode().Perm()
	}
	tmp, e := os.CreateTemp(filepath.Dir(p), ".chatgpt-remote-*")
	if e != nil {
		return nil, e
	}
	defer os.Remove(tmp.Name())
	if e = tmp.Chmod(mode); e != nil {
		tmp.Close()
		return nil, e
	}
	if _, e = tmp.Write([]byte(content)); e != nil {
		tmp.Close()
		return nil, e
	}
	if e = tmp.Sync(); e != nil {
		tmp.Close()
		return nil, e
	}
	if e = tmp.Close(); e != nil {
		return nil, e
	}
	if e = os.Rename(tmp.Name(), p); e != nil {
		return nil, e
	}
	return map[string]any{"path": p, "bytes": len(content), "sha256": checksum([]byte(content))}, nil
}
func (f FileOps) List(path string, limit int) (map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	p, e := f.resolve(path, false)
	if e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(p)
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, ent := range entries {
		if len(out) >= limit {
			break
		}
		t := "file"
		if ent.IsDir() {
			t = "directory"
		}
		if ent.Type()&os.ModeSymlink != 0 {
			t = "symlink"
		}
		out = append(out, map[string]any{"name": ent.Name(), "type": t})
	}
	return map[string]any{"path": p, "entries": out, "total": len(entries), "truncated": len(entries) > len(out)}, nil
}
func (f FileOps) Search(path, needle string, limit int) (map[string]any, error) {
	if needle == "" {
		return nil, errors.New("query required")
	}
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	p, e := f.resolve(path, false)
	if e != nil {
		return nil, e
	}
	matches := []map[string]any{}
	visited := 0
	stop := errors.New("limit reached")
	e = filepath.WalkDir(p, func(item string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		visited++
		if visited > 3000 {
			return stop
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxRead {
			return nil
		}
		b, err := os.ReadFile(item)
		if err != nil || !utf8.Valid(b) {
			return nil
		}
		for n, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, needle) {
				if len(line) > 300 {
					line = line[:300]
				}
				matches = append(matches, map[string]any{"path": item, "line": n + 1, "text": line})
				if len(matches) >= limit {
					return stop
				}
			}
		}
		return nil
	})
	if e != nil && !errors.Is(e, stop) {
		return nil, e
	}
	return map[string]any{"matches": matches, "files_scanned": visited, "truncated": errors.Is(e, stop)}, nil
}
