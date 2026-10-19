package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type limitedBuffer struct {
	b   bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if l.b.Len() < l.max {
		take := l.max - l.b.Len()
		if take > len(p) {
			take = len(p)
		}
		_, _ = l.b.Write(p[:take])
	}
	return n, nil
}
func RunCommand(ctx context.Context, cfg Config, dir, name string, args []string, seconds int) (map[string]any, error) {
	if !cfg.EnableCommands {
		return nil, errors.New("command execution disabled in config")
	}
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return nil, errors.New("use an allowed executable name, not a path")
	}
	ok := false
	for _, v := range cfg.AllowedCommands {
		if v == name {
			ok = true
			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("command %q not in allowed_commands", name)
	}
	f := FileOps{cfg.Roots}
	p, e := f.resolve(dir, false)
	if e != nil {
		return nil, e
	}
	if seconds <= 0 || seconds > 30 {
		seconds = 15
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	path, e := exec.LookPath(name)
	if e != nil {
		return nil, e
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = p
	// No shell expansion. Tools run with the current OS user privileges.
	out := &limitedBuffer{max: 65536}
	errout := &limitedBuffer{max: 65536}
	cmd.Stdout = out
	cmd.Stderr = errout
	e = cmd.Run()
	exit := 0
	if e != nil {
		var ee *exec.ExitError
		if errors.As(e, &ee) {
			exit = ee.ExitCode()
		} else if ctx.Err() != nil {
			exit = -1
		} else {
			return nil, e
		}
	}
	return map[string]any{"program": filepath.Base(path), "args": args, "directory": p, "exit_code": exit, "stdout": out.b.String(), "stderr": errout.b.String(), "output_truncated": out.b.Len() >= out.max || errout.b.Len() >= errout.max, "timed_out": ctx.Err() != nil}, nil
}
