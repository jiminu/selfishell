package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"

	"github.com/jiminu/selfishell/internal/pty"
)

type capture struct {
	Status               int
	Stdout, Stderr, Home []byte
}

type snapshotEntry struct {
	Path   []byte `json:"path"`
	Type   string `json:"type"`
	Mode   uint32 `json:"mode"`
	Bytes  []byte `json:"bytes,omitempty"`
	Target []byte `json:"target,omitempty"`
}

func snapshot(root string) ([]byte, error) {
	var entries []snapshotEntry
	var visit func(string) error
	visit = func(path string) error {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		mode := info.Mode()
		e := snapshotEntry{Path: []byte(rel), Mode: uint32(mode.Perm())}
		if mode&os.ModeSetuid != 0 {
			e.Mode |= 04000
		}
		if mode&os.ModeSetgid != 0 {
			e.Mode |= 02000
		}
		if mode&os.ModeSticky != 0 {
			e.Mode |= 01000
		}
		switch {
		case mode.IsRegular():
			e.Type = "regular"
			e.Bytes, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		case mode.IsDir():
			e.Type = "directory"
		case mode&os.ModeSymlink != 0:
			e.Type = "symlink"
			var target string
			target, err = os.Readlink(path)
			e.Target = []byte(target)
			if err != nil {
				return err
			}
		case mode&os.ModeNamedPipe != 0:
			e.Type = "fifo"
		case mode&os.ModeSocket != 0:
			e.Type = "socket"
		case mode&os.ModeDevice != 0:
			if mode&os.ModeCharDevice != 0 {
				e.Type = "character-device"
			} else {
				e.Type = "block-device"
			}
		default:
			e.Type = mode.Type().String()
		}
		entries = append(entries, e)
		if mode.IsDir() {
			children, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
			for _, child := range children {
				if err = visit(filepath.Join(path, child.Name())); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	return json.Marshal(entries)
}
func makeFIFO(path string) error { return syscall.Mkfifo(path, 0600) }

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
func baseEnv(home, tmp string) []string {
	return []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local/share"), "XDG_STATE_HOME=" + filepath.Join(home, ".local/state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"), "SHELL=/bin/zsh", "TMPDIR=" + tmp, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C", "TZ=UTC", "SELFISHELL_TEST_APPLICATIONS_DIR=" + filepath.Join(home, "system-applications")}
}
func withEnv(base []string, extra ...string) []string {
	return append(append([]string{}, base...), extra...)
}
func resolveCommand(name string, env []string) (string, error) {
	if strings.ContainsRune(name, filepath.Separator) {
		return name, nil
	}
	path := "/usr/bin:/bin:/usr/sbin:/sbin"
	for _, item := range env {
		if strings.HasPrefix(item, "PATH=") {
			path = strings.TrimPrefix(item, "PATH=")
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("command %q absent from controlled PATH", name)
}

func runCommand(home string, argv []string, input []byte, extraEnv []string, timeout time.Duration) (capture, error) {
	return runCommandIn(home, home, argv, input, extraEnv, timeout)
}

func runCommandIn(home, dir string, argv []string, input []byte, extraEnv []string, timeout time.Duration) (capture, error) {
	if len(argv) == 0 {
		return capture{}, errors.New("empty command")
	}
	tmp, err := os.MkdirTemp("", "selfishell-command-")
	if err != nil {
		return capture{}, err
	}
	defer os.RemoveAll(tmp)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	env := withEnv(baseEnv(home, tmp), extraEnv...)
	program, err := resolveCommand(argv[0], env)
	if err != nil {
		return capture{}, err
	}
	cmd := exec.CommandContext(ctx, program, argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 500 * time.Millisecond
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	err = cmd.Run()
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	result := capture{Stdout: out.Bytes(), Stderr: errout.Bytes()}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.Status = exit.ExitCode()
			return result, nil
		}
		return result, err
	}
	return result, nil
}

var testCLIOnce sync.Once
var testCLIPath string
var testCLIErr error
var testCLIDir string
var testGoCache string

func TestMain(m *testing.M) {
	// Local runs own a temporary cache. CI can explicitly reuse its compiler
	// cache; that caller-owned directory must survive this test process.
	testGoCache = os.Getenv("SELFISHELL_TEST_GO_CACHE")
	privateCache := testGoCache == ""
	var err error
	if privateCache {
		testGoCache, err = os.MkdirTemp("", "selfishell-test-go-cache-")
	} else if !filepath.IsAbs(testGoCache) {
		err = fmt.Errorf("SELFISHELL_TEST_GO_CACHE must be absolute")
	} else {
		err = os.MkdirAll(testGoCache, 0700)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// In-process release builds inherit this environment. Parallel tests cannot
	// use t.Setenv, so they share one private home instead of the developer's.
	processHome, err := os.MkdirTemp("", "selfishell-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for key, value := range privateHomeEnv(processHome) {
		os.Setenv(key, value)
	}
	code := m.Run()
	os.RemoveAll(processHome)
	if testCLIDir != "" {
		os.RemoveAll(testCLIDir)
	}
	if nativeDir != "" {
		os.RemoveAll(nativeDir)
	}
	extraNative.Range(func(_, value any) bool {
		holder := value.(*nativeAssetFixture)
		if holder.dir != "" {
			os.RemoveAll(holder.dir)
		}
		return true
	})
	if privateCache {
		os.RemoveAll(testGoCache)
	}
	os.Exit(code)
}

func testCLI(t *testing.T) (string, error) {
	t.Helper()
	if override, err := testutil.CLIOverride(); override != "" || err != nil {
		return override, err
	}
	testCLIOnce.Do(func() {
		dir, err := os.MkdirTemp("", "selfishell-test-cli-")
		if err != nil {
			testCLIErr = err
			return
		}
		testCLIPath = filepath.Join(dir, "selfishell")
		testCLIDir = dir
		if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0700); err != nil {
			testCLIErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin/go"), "build", "-o", testCLIPath, "./cmd/selfishell")
		cmd.Dir = repoRoot()
		cmd.Env = withEnv(baseEnv(dir, filepath.Join(dir, "tmp")), "GOTOOLCHAIN=local", "GOCACHE="+testGoCache, "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.WaitDelay = 500 * time.Millisecond
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		out, err := cmd.CombinedOutput()
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		if err != nil {
			testCLIErr = fmt.Errorf("build test CLI: %w: %s", err, out)
		}
	})
	return testCLIPath, testCLIErr
}

func captureCommand(home, executable string, args []string, extraEnv []string) (capture, error) {
	cmd := append([]string{executable}, args...)
	result, err := runCommand(home, cmd, nil, extraEnv, 15*time.Second)
	if err != nil {
		return result, err
	}
	result.Home, err = snapshot(home)
	return result, err
}

func copyFile(from, to string) error {
	b, e := os.ReadFile(from)
	if e != nil {
		return e
	}
	info, e := os.Stat(from)
	if e != nil {
		return e
	}
	return testutil.WriteFile(to, b, info.Mode().Perm())
}
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, e := filepath.Rel(from, path)
		if e != nil {
			return e
		}
		target := filepath.Join(to, rel)
		info, e := os.Lstat(path)
		if e != nil {
			return e
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, e := os.Readlink(path)
			if e != nil {
				return e
			}
			return os.Symlink(link, target)
		}
		if d.Type().IsRegular() {
			return copyFile(path, target)
		}
		return nil
	})
}
func requireStatus(t *testing.T, name string, got capture, want int) {
	t.Helper()
	if got.Status != want {
		t.Fatalf("%s: status %d want %d; stderr=%s", name, got.Status, want, got.Stderr)
	}
}

// requireStdout also reports status and stderr, which tell a lost PTY capture
// from a command that failed before writing its report.
func requireStdout(t *testing.T, got capture, s string) {
	t.Helper()
	if !bytes.Contains(got.Stdout, []byte(s)) {
		t.Fatalf("missing %q in stdout; status=%d stdout=%q stderr=%q", s, got.Status, got.Stdout, got.Stderr)
	}
}
func fixtureTools(t *testing.T, root string) string {
	t.Helper()
	tools := filepath.Join(root, "tools")
	if err := os.MkdirAll(tools, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range strings.Fields("bash env cat chmod cp mv rm mkdir ln readlink dirname basename find sed awk grep cut sort head tail tr cksum cmp dd uname touch mktemp rmdir wc date tar gzip") {
		path, err := resolveCommand(name, baseEnv(root, os.TempDir()))
		if err != nil {
			t.Fatalf("required fixture tool %s: %v", name, err)
		}
		if err = os.Symlink(path, filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
	}
	checksumTools := 0
	for _, name := range []string{"shasum", "sha256sum"} {
		path, err := resolveCommand(name, baseEnv(root, os.TempDir()))
		if err != nil {
			continue
		}
		if err := os.Symlink(path, filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
		checksumTools++
	}
	if checksumTools == 0 {
		t.Fatal("required fixture checksum tool: need shasum or sha256sum")
	}
	return tools
}

func capturePTY(home, executable string, args []string, extraEnv []string) (capture, error) {
	return capturePTYStreams(home, executable, args, extraEnv, false)
}
func capturePTYOutput(home, executable string, args []string, extraEnv []string) (capture, error) {
	return capturePTYStreams(home, executable, args, extraEnv, true)
}
func capturePTYStreams(home, executable string, args []string, extraEnv []string, stdoutPTY bool) (capture, error) {
	tmp, err := os.MkdirTemp("", "selfishell-pty-")
	if err != nil {
		return capture{}, err
	}
	defer os.RemoveAll(tmp)
	master, slave, err := pty.Open()
	if err != nil {
		return capture{}, err
	}
	defer master.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = home
	cmd.Env = withEnv(baseEnv(home, tmp), extraEnv...)
	cmd.Stdin = bytes.NewReader(nil)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 500 * time.Millisecond
	var out bytes.Buffer
	if stdoutPTY {
		cmd.Stdout = slave
		cmd.Stderr = &out
	} else {
		cmd.Stdout = &out
		cmd.Stderr = slave
	}
	var stderr bytes.Buffer
	readDone := make(chan struct{})
	go func() { io.Copy(&stderr, master); close(readDone) }()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	err = cmd.Run()
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	slave.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		master.Close()
		select {
		case <-readDone:
		case <-time.After(time.Second):
			return capture{}, fmt.Errorf("PTY reader did not exit after close")
		}
	}
	if ctx.Err() != nil {
		return capture{}, ctx.Err()
	}
	result := capture{Stdout: out.Bytes(), Stderr: stderr.Bytes()}
	if stdoutPTY {
		result.Stdout, result.Stderr = stderr.Bytes(), out.Bytes()
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.Status = exit.ExitCode()
		} else {
			return result, err
		}
	}
	result.Home, err = snapshot(home)
	return result, err
}

// copyCLIFixture uses only the current checkout and an already-built Go CLI.
func copyCLIFixture(t *testing.T, root, executable string) {
	t.Helper()
	mustFS(t, os.MkdirAll(filepath.Join(root, "bin"), 0700))
	mustFS(t, copyFile(executable, filepath.Join(root, "bin/selfishell")))
	mustFS(t, copyTree(filepath.Join(repoRoot(), "config"), filepath.Join(root, "config")))
	for _, name := range []string{"packages.conf", "dependencies.conf"} {
		mustFS(t, copyFile(filepath.Join(repoRoot(), name), filepath.Join(root, name)))
	}
	mustFS(t, testutil.WriteFile(filepath.Join(root, "VERSION"), []byte("0.0.0-test\n"), 0644))
}
