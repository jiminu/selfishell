package selfishell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Process passes files through unchanged, so an interactive child keeps its
// terminal. It never constructs shell source from arguments.
type Process struct {
	In       io.Reader
	Out, Err io.Writer
	Dir      string
	Env      []string
}

// Run cancels and reaps its direct child. WaitDelay bounds inherited pipe waits.
// It does not create a new process group, which would break foreground TTY reads.
func (p Process) Run(ctx context.Context, name string, args ...string) (int, error) {
	// The Bash CLI exports these before running tools, including tools that
	// start Git themselves (for example Neovim's plugin sync).
	env := p.environment()
	limit, duration := envValue(env, "SELFISHELL_CURL_LOW_SPEED_LIMIT"), envValue(env, "SELFISHELL_CURL_LOW_SPEED_TIME")
	if limit == "" {
		limit = "1024"
	}
	if duration == "" {
		duration = "30"
	}
	valid := func(value string) bool {
		return value[0] >= '1' && value[0] <= '9' && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) < 0
	}
	if valid(limit) && valid(duration) {
		set := map[string]string{}
		if envValue(env, "GIT_HTTP_LOW_SPEED_LIMIT") == "" {
			set["GIT_HTTP_LOW_SPEED_LIMIT"] = limit
		}
		if envValue(env, "GIT_HTTP_LOW_SPEED_TIME") == "" {
			set["GIT_HTTP_LOW_SPEED_TIME"] = duration
		}
		if len(set) != 0 {
			if p.Env == nil && p.Dir != "" {
				if pwd, err := filepath.Abs(p.Dir); err == nil {
					set["PWD"] = pwd // exec.Command updates PWD only while Env is nil.
				}
			}
			p = withEnvironment(p, set)
		}
	}
	if p.Env != nil && !strings.ContainsRune(name, os.PathSeparator) {
		path, err := p.lookPath(name)
		if err != nil {
			return 127, err
		}
		name = path
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr, cmd.Dir = p.In, p.Out, p.Err, p.Dir
	if p.Env != nil {
		cmd.Env = p.Env
	}
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if ctx.Err() != nil {
		return 130, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return exit.ExitCode(), nil
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, exec.ErrNotFound) {
		return 127, err
	}
	return 126, err
}

// lookPath uses the child's PATH, never the caller's PATH when Env is explicit.
func (p Process) lookPath(name string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		return name, nil
	}
	path := os.Getenv("PATH")
	if p.Env != nil {
		path = ""
		for _, entry := range p.Env {
			if strings.HasPrefix(entry, "PATH=") {
				path = strings.TrimPrefix(entry, "PATH=")
			}
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: %s", exec.ErrNotFound, name)
}

// Curl retains the existing transport, proxy environment and local-file behavior.
// Callers select and verify a temporary destination before activating a download.
func (p Process) Curl(ctx context.Context, mode string, args ...string) (int, error) {
	connect := envDefault("SELFISHELL_CURL_CONNECT_TIMEOUT", "10")
	limit := envDefault("SELFISHELL_CURL_LOW_SPEED_LIMIT", "1024")
	duration := envDefault("SELFISHELL_CURL_LOW_SPEED_TIME", "30")
	maximum := envDefault("SELFISHELL_CURL_METADATA_MAX_TIME", "15")
	for _, value := range []string{connect, limit, duration, maximum} {
		if value == "0" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 2, fmt.Errorf("Selfishell curl timeout and speed settings must be positive integers.")
		}
	}
	policy := []string{"-fsSL", "--connect-timeout", connect, "--speed-limit", limit, "--speed-time", duration}
	switch mode {
	case "metadata":
		policy = append(policy, "--max-time", maximum)
	case "transfer":
	default:
		return 2, fmt.Errorf("Unknown Selfishell curl mode: %s", mode)
	}
	return p.Run(ctx, "curl", append(policy, args...)...)
}
