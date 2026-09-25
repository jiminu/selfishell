package migration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
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
)

type capture struct {
	Status               int
	Stdout, Stderr, Home []byte
}

func compareCapture(want, got capture) error {
	if want.Status != got.Status {
		return fmt.Errorf("status: want %d got %d", want.Status, got.Status)
	}
	if !bytes.Equal(want.Stdout, got.Stdout) {
		return fmt.Errorf("stdout differs")
	}
	if !bytes.Equal(want.Stderr, got.Stderr) {
		return fmt.Errorf("stderr differs")
	}
	if !bytes.Equal(want.Home, got.Home) {
		return fmt.Errorf("home differs")
	}
	return nil
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
	return []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local/share"), "XDG_STATE_HOME=" + filepath.Join(home, ".local/state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"), "SHELL=/bin/zsh", "TMPDIR=" + tmp, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C", "TZ=UTC"}
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
	cmd.Dir = home
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

func exportCommit(repo, commit, dest string) error {
	check, err := runCommand(repo, []string{"git", "-C", repo, "cat-file", "-e", commit + "^{commit}"}, nil, nil, 10*time.Second)
	if err != nil || check.Status != 0 {
		return fmt.Errorf("missing migration reference %s; fetch repository history (CI: fetch-depth: 0): %v %s", commit, err, check.Stderr)
	}
	archive, err := runCommand(repo, []string{"git", "-C", repo, "archive", commit}, nil, nil, 30*time.Second)
	if err != nil || archive.Status != 0 {
		return fmt.Errorf("archive %s: %v %s", commit, err, archive.Stderr)
	}
	if err := os.MkdirAll(dest, 0700); err != nil {
		return err
	}
	extract, err := runCommand(dest, []string{"tar", "-xf", "-"}, archive.Stdout, nil, 30*time.Second)
	if err != nil || extract.Status != 0 {
		return fmt.Errorf("extract %s: %v %s", commit, err, extract.Stderr)
	}
	return nil
}

var candidateOnce sync.Once
var candidatePath string
var candidateErr error
var candidateDir string

func TestMain(m *testing.M) {
	code := m.Run()
	if candidateDir != "" {
		os.RemoveAll(candidateDir)
	}
	os.Exit(code)
}

func candidateCLI(t *testing.T) (string, error) {
	t.Helper()
	if override := os.Getenv("SELFISHELL_TEST_CLI"); override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("SELFISHELL_TEST_CLI must be absolute")
		}
		info, err := os.Stat(override)
		if err != nil {
			return "", fmt.Errorf("invalid SELFISHELL_TEST_CLI %s: %w", override, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return "", fmt.Errorf("SELFISHELL_TEST_CLI is not executable: %s", override)
		}
		return override, nil
	}
	candidateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "selfishell-candidate-")
		if err != nil {
			candidateErr = err
			return
		}
		candidatePath = filepath.Join(dir, "selfishell")
		candidateDir = dir
		if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0700); err != nil {
			candidateErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin/go"), "build", "-o", candidatePath, "./cmd/selfishell")
		cmd.Dir = repoRoot()
		cmd.Env = withEnv(baseEnv(dir, filepath.Join(dir, "tmp")), "GOTOOLCHAIN=local", "GOCACHE="+filepath.Join(dir, "go-cache"), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
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
			candidateErr = fmt.Errorf("build candidate: %w: %s", err, out)
		}
	})
	return candidatePath, candidateErr
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

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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
	return os.WriteFile(to, b, info.Mode().Perm())
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
func requireEqual(t *testing.T, name string, want, got capture) {
	t.Helper()
	if err := compareCapture(want, got); err != nil {
		t.Fatalf("%s: %v\nwant stdout=%q stderr=%q\ngot stdout=%q stderr=%q", name, err, want.Stdout, want.Stderr, got.Stdout, got.Stderr)
	}
}
func fixtureTools(t *testing.T, root string) string {
	t.Helper()
	tools := filepath.Join(root, "tools")
	if err := os.MkdirAll(tools, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range strings.Fields("bash env cat chmod cp mv rm mkdir ln readlink dirname basename find sed awk grep cut sort head tail tr cksum cmp dd uname touch mktemp rmdir wc date tar gzip shasum sha256sum") {
		path, err := resolveCommand(name, baseEnv(root, os.TempDir()))
		if err != nil {
			continue
		}
		if err = os.Symlink(path, filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(filepath.Join(tools, "date"))
	if err := copyFile(filepath.Join(repoRoot(), "tests/fixtures/go_migration/date.bash"), filepath.Join(tools, "date")); err != nil {
		t.Fatal(err)
	}
	os.Chmod(filepath.Join(tools, "date"), 0755)
	return tools
}

func capturePTY(home, executable string, args []string, extraEnv []string) (capture, error) {
	master, slave, err := openPTY()
	if err != nil {
		return capture{}, err
	}
	defer master.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = home
	cmd.Env = withEnv(baseEnv(home, os.TempDir()), extraEnv...)
	cmd.Stdin = bytes.NewReader(nil)
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 500 * time.Millisecond
	var out bytes.Buffer
	cmd.Stdout = &out
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
	slave.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		master.Close()
		<-readDone
	}
	if ctx.Err() != nil {
		return capture{}, ctx.Err()
	}
	result := capture{Stdout: out.Bytes(), Stderr: stderr.Bytes()}
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
