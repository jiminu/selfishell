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
	"sync"
	"syscall"
	"time"
)

// Process passes files through unchanged, so an interactive child keeps its
// terminal. It never constructs shell source from arguments.
type Process struct {
	In            io.Reader
	Out, Err      io.Writer
	Dir           string
	Env           []string
	repoScopedGit bool
	progress      *operationProgress
	foreground    bool // Commands which can prompt retain their terminal streams.
}

// outputTail bounds hidden tool output while retaining failure diagnostics.
type outputTail struct{ data []byte }

func (t *outputTail) Write(p []byte) (int, error) {
	n := len(p)
	if n >= 8192 {
		t.data = append(t.data[:0], p[n-8192:]...)
		return n, nil
	}
	if excess := len(t.data) + n - 8192; excess > 0 {
		copy(t.data, t.data[excess:])
		t.data = t.data[:len(t.data)-excess]
	}
	t.data = append(t.data, p...)
	return n, nil
}

// Run cancels and reaps its direct child. WaitDelay bounds inherited pipe waits.
// It does not create a new process group, which would break foreground TTY reads.
func (p Process) Run(ctx context.Context, name string, args ...string) (int, error) {
	return p.run(ctx, false, name, args...)
}

// runCLI lets the continuation cancel and reap its own package child before exiting.
func (p Process) runCLI(ctx context.Context, name string, args ...string) (int, error) {
	return p.run(ctx, true, name, args...)
}

func (p Process) run(ctx context.Context, forwardCancel bool, name string, args ...string) (int, error) {
	inheritedEnv := p.Env == nil
	// Approved dependencies are public. Apply this to indirect Git children too,
	// so authentication failures cannot wait behind captured progress output.
	p = withEnvironment(p, map[string]string{"GIT_TERMINAL_PROMPT": "0"})
	// Propagate transfer limits to tools that start Git themselves,
	// including Neovim plugin sync.
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
			p = withEnvironment(p, set)
		}
	}
	if inheritedEnv && p.Dir != "" {
		if pwd, err := filepath.Abs(p.Dir); err == nil {
			p = withEnvironment(p, map[string]string{"PWD": pwd}) // exec.Command updates PWD only while Env is nil.
		}
	}
	{
		// Installer children can start Git indirectly. Repository selection
		// belongs to the operation, never to the invoking shell's workspace.
		blocked := map[string]bool{
			"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true,
			"GIT_COMMON_DIR": true, "GIT_OBJECT_DIRECTORY": true,
			"GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_NAMESPACE": true,
			"GIT_PREFIX": true, "GIT_CEILING_DIRECTORIES": true,
			"GIT_GRAFT_FILE": true, "GIT_REPLACE_REF_BASE": true,
		}
		clean := make([]string, 0, len(p.environment()))
		for _, item := range p.environment() {
			key, _, _ := strings.Cut(item, "=")
			if !blocked[key] || (p.repoScopedGit && (key == "GIT_DIR" || key == "GIT_WORK_TREE")) {
				clean = append(clean, item)
			}
		}
		p.Env = clean
	}
	if !strings.ContainsRune(name, os.PathSeparator) {
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
	// Give tools a bounded opportunity to release locks and clean temporary
	// files. CommandContext escalates to Kill if they outlive WaitDelay.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	var captured *outputTail
	if p.progress != nil && p.progress.compact {
		if p.foreground || forwardCancel {
			p.progress.pause()
		} else if p.Out == p.progress.out && p.Err == p.progress.stderr {
			if p.progress.stop == nil && p.progress.label != "" {
				p.progress.stage(p.progress.label)
			}
			captured = &outputTail{}
			cmd.Stdout, cmd.Stderr = captured, captured
		}
	}
	if forwardCancel {
		// Outlast the continuation's own one-second child shutdown deadline.
		cmd.WaitDelay = 5 * time.Second
	}
	err := cmd.Run()
	if err != nil && captured != nil {
		p.progress.pause()
		_, _ = p.progress.stderr.Write(captured.data)
	}
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

// probeLimit bounds concurrent read-only probes such as Git dirty checks.
const probeLimit = 8

// probeParallel runs probe(0..n-1), at most probeLimit at once. Probes store
// results by index so callers still report in declaration order, and their
// children must not share the caller's stdin.
func probeParallel(n int, probe func(int)) {
	slots := make(chan struct{}, probeLimit)
	var wg sync.WaitGroup
	for i := range n {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			probe(i)
		})
	}
	wg.Wait()
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
		if !filepath.IsAbs(dir) {
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
	// Same bounded retry as install.sh: timeouts and HTTP 408/429/5xx only, and
	// --retry-max-time also caps a server's Retry-After delay.
	policy := []string{"-fsSL", "--connect-timeout", connect, "--speed-limit", limit, "--speed-time", duration, "--retry", "3", "--retry-max-time", "60"}
	switch mode {
	case "metadata":
		policy = append(policy, "--max-time", maximum)
	case "transfer":
	default:
		return 2, fmt.Errorf("Unknown Selfishell curl mode: %s", mode)
	}
	return p.Run(ctx, "curl", append(policy, args...)...)
}
