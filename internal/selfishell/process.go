package selfishell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
