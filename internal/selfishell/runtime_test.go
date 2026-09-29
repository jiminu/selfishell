package selfishell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(k, "")
	}
	paths, err := UserPaths()
	if err != nil || paths.Config != home+"/.config/selfishell" || paths.State != home+"/.local/state/selfishell" || paths.Cache != home+"/.cache/selfishell" || paths.Data != home+"/.local/share/selfishell" || paths.Resources != paths.State+"/resources" {
		t.Fatalf("paths=%+v, err=%v", paths, err)
	}
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(k, home+"/custom dir")
	}
	paths, err = UserPaths()
	if err != nil || paths.Config != home+"/custom dir/selfishell" || paths.State != paths.Config || paths.Data != paths.Config || paths.Cache != paths.Config {
		t.Fatalf("override paths=%+v, err=%v", paths, err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("path discovery wrote files")
	}

	// A symlink followed by .. must be resolved by the filesystem, not cleaned
	// lexically: alias/.. selects physical/, not the lexical parent of alias.
	if err := os.MkdirAll(home+"/physical/child", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home+"/physical/child", home+"/alias"); err != nil {
		t.Fatal(err)
	}
	prefix := home + "/alias/.."
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(k, prefix+"/custom")
	}
	paths, err = UserPaths()
	expected := Paths{Config: prefix + "/custom/selfishell", State: prefix + "/custom/selfishell", Resources: prefix + "/custom/selfishell/resources", Cache: prefix + "/custom/selfishell", Data: prefix + "/custom/selfishell"}
	if err != nil || paths != expected {
		t.Fatalf("XDG path meaning changed: got %+v want %+v (err=%v)", paths, expected, err)
	}
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(k, "")
	}
	t.Setenv("HOME", prefix)
	paths, err = UserPaths()
	expected = Paths{Config: prefix + "/.config/selfishell", State: prefix + "/.local/state/selfishell", Resources: prefix + "/.local/state/selfishell/resources", Cache: prefix + "/.cache/selfishell", Data: prefix + "/.local/share/selfishell"}
	if err != nil || paths != expected {
		t.Fatalf("HOME path meaning changed: got %+v want %+v (err=%v)", paths, expected, err)
	}
	children, err := os.ReadDir(home + "/physical")
	if err != nil || len(children) != 1 {
		t.Fatalf("path discovery created targets: %v %v", children, err)
	}
	t.Setenv("HOME", "")
	if _, err := UserPaths(); err == nil {
		t.Fatal("empty HOME accepted")
	}
}

func TestPlatform(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", root+"/os-release")
	t.Setenv("SELFISHELL_TEST_PROC_VERSION_FILE", root+"/proc-version")
	for _, tc := range []struct{ system, arch, id, proc, platform, wantArch string }{
		{"Darwin", "arm64", "", "", "macos", "arm64"},
		{"Linux", "x86_64", "ubuntu", "Linux", "ubuntu", "amd64"},
		{"Linux", "aarch64", "\"ubuntu\"", "Microsoft WSL2", "ubuntu-wsl", "arm64"},
		{"Linux", "amd64", "'ubuntu'", "linux", "ubuntu", "amd64"},
		{"Linux", "riscv64", "fedora", "linux", "unsupported-linux", "riscv64"},
		{"Linux", "amd64", "debian", "WSL", "unsupported-wsl", "amd64"},
		{"FreeBSD", "amd64", "", "", "unsupported", "amd64"},
	} {
		t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", tc.system)
		t.Setenv("SELFISHELL_TEST_MACHINE_ARCH", tc.arch)
		if err := os.WriteFile(root+"/os-release", []byte("NAME=ignored\nID="+tc.id+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(root+"/proc-version", []byte(tc.proc), 0600); err != nil {
			t.Fatal(err)
		}
		p := DetectPlatform()
		if p.Name != tc.platform || p.Arch != tc.wantArch {
			t.Fatalf("got %+v, want %s/%s", p, tc.platform, tc.wantArch)
		}
	}
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	if err := os.Remove(root + "/os-release"); err != nil {
		t.Fatal(err)
	}
	if DetectPlatform().Name != "unsupported-linux" {
		t.Fatal("missing distro was supported")
	}
}

// A real child executable verifies argv, streams, environment, cwd and signals.
func TestProcessChild(t *testing.T) {
	if os.Getenv("SELFISHELL_GO_PROCESS_CHILD") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	switch args[0] {
	case "echo":
		input, _ := io.ReadAll(os.Stdin)
		dir, _ := os.Getwd()
		fmt.Fprintf(os.Stdout, "%q\n%s\n%s\n%s", args[1:], input, dir, os.Getenv("HTTPS_PROXY"))
		fmt.Fprint(os.Stderr, "child error\n")
		os.Exit(23)
	case "signal":
		syscall.Kill(os.Getpid(), syscall.SIGTERM)
		time.Sleep(time.Second)
	case "wait":
		fmt.Fprintln(os.Stdout, "ready")
		time.Sleep(time.Hour)
	case "graceful":
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM)
		fmt.Fprintln(os.Stdout, "ready")
		<-stop
		fmt.Fprintln(os.Stdout, "cleaned up")
	case "ignore":
		signal.Ignore(syscall.SIGTERM)
		fmt.Fprintln(os.Stdout, "ready")
		time.Sleep(time.Hour)
	}
	os.Exit(0)
}

func TestProcess(t *testing.T) {
	t.Setenv("SELFISHELL_GO_PROCESS_CHILD", "1")
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8443")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var out, stderr bytes.Buffer
	p := Process{In: strings.NewReader("stdin\x00bytes"), Out: &out, Err: &stderr, Dir: dir}
	args := []string{"-test.run=^TestProcessChild$", "--", "echo", "a b", "$(touch SHOULD_NOT_EXIST)", "", "a'\"b"}
	code, err := p.Run(context.Background(), executable, args...)
	want := fmt.Sprintf("%q\nstdin\x00bytes\n%s\nhttp://proxy.example:8443", args[3:], dir)
	if err != nil || code != 23 || out.String() != want || stderr.String() != "child error\n" {
		t.Fatalf("code=%d err=%v out=%q stderr=%q", code, err, out.String(), stderr.String())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("shell argument was executed")
	}
	code, err = p.Run(context.Background(), executable, "-test.run=^TestProcessChild$", "--", "signal")
	if err != nil || code != 143 {
		t.Fatalf("signal code=%d err=%v", code, err)
	}
	code, err = p.Run(context.Background(), dir+"/missing")
	if code != 127 || err == nil {
		t.Fatalf("missing code=%d err=%v", code, err)
	}
	if err := os.WriteFile(dir+"/not-executable", nil, 0600); err != nil {
		t.Fatal(err)
	}
	code, err = p.Run(context.Background(), dir+"/not-executable")
	if code != 126 || err == nil {
		t.Fatalf("permission code=%d err=%v", code, err)
	}
}

func TestProcessCancellation(t *testing.T) {
	for _, mode := range []string{"wait", "graceful", "ignore"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("SELFISHELL_GO_PROCESS_CHILD", "1")
			executable, _ := os.Executable()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer read.Close()
			defer write.Close()
			done := make(chan error, 1)
			go func() {
				code, err := (Process{Out: write, Err: io.Discard}).Run(ctx, executable, "-test.run=^TestProcessChild$", "--", mode)
				if code != 130 || !errors.Is(err, context.Canceled) {
					done <- fmt.Errorf("code=%d err=%v", code, err)
				} else {
					done <- nil
				}
			}()
			if err := read.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatal(err)
			}
			data := make([]byte, 6)
			if _, err := io.ReadFull(read, data); err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancelled command still running")
			}
			write.Close()
			tail, err := io.ReadAll(read)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "graceful" && string(tail) != "cleaned up\n" {
				t.Fatalf("child was not allowed to clean up: %q", tail)
			}
			if mode == "ignore" && time.Since(started) < 900*time.Millisecond {
				t.Fatal("child was killed before the shutdown grace period")
			}
		})
	}
}

func TestProcessDisablesGitTerminalPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	for _, explicit := range []bool{false, true} {
		var out bytes.Buffer
		p := Process{Out: &out}
		if explicit {
			p.Env = []string{"HOME=" + os.Getenv("HOME"), "GIT_TERMINAL_PROMPT=1"}
		}
		code, err := p.Run(context.Background(), "/bin/sh", "-c", `printf '%s' "$GIT_TERMINAL_PROMPT"`)
		if err != nil || code != 0 || out.String() != "0" {
			t.Fatalf("explicit=%t: code=%d err=%v prompt=%q", explicit, code, err, out.String())
		}
	}
}

func TestTerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Fatal("/dev/null is a character device but not a terminal")
	}
	if IsTerminal(&bytes.Buffer{}) {
		t.Fatal("buffer is not a terminal")
	}
}

func TestCurl(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	file := root + "/file with spaces"
	if err := os.WriteFile(file, []byte("download\x00bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	p := Process{Out: &out, Err: &stderr}
	code, err := p.Curl(context.Background(), "transfer", (&url.URL{Scheme: "file", Path: file}).String())
	if code != 0 || err != nil || out.String() != "download\x00bytes" {
		t.Fatalf("code=%d err=%v out=%q stderr=%q", code, err, out.String(), stderr.String())
	}
	t.Setenv("PATH", root) // Invalid policy must be rejected before looking for curl.
	for _, value := range []string{"0", "-1", "x", " 1"} {
		t.Setenv("SELFISHELL_CURL_CONNECT_TIMEOUT", value)
		code, err = p.Curl(context.Background(), "metadata", "https://example.invalid")
		if code != 2 || err == nil || !strings.Contains(err.Error(), "positive integers") {
			t.Fatalf("policy %q: code=%d err=%v", value, code, err)
		}
	}
}

func TestCurlUsesProxy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requested := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested <- r.URL.String()
		fmt.Fprint(w, "proxy payload")
	}))
	defer proxy.Close()
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	var out bytes.Buffer
	code, err := (Process{Out: &out, Err: io.Discard}).Curl(context.Background(), "metadata", "http://selfishell.invalid/payload")
	if code != 0 || err != nil || out.String() != "proxy payload" {
		t.Fatalf("proxy code=%d err=%v out=%q", code, err, out.String())
	}
	select {
	case got := <-requested:
		if got != "http://selfishell.invalid/payload" {
			t.Fatal(got)
		}
	default:
		t.Fatal("proxy received no request")
	}
}

func TestGitTransferSpeedEnvironment(t *testing.T) {
	for _, tc := range []struct{ name, curlLimit, curlTime, gitLimit, gitTime, want string }{
		{"defaults", "", "", "", "", "1024 30\n"},
		{"configured", "256", "120", "", "", "256 120\n"},
		{"caller override", "256", "120", "5", "", "5 120\n"},
		{"invalid curl policy", "0", "120", "", "", " \n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("TMPDIR", t.TempDir())
			t.Setenv("PATH", root)
			t.Setenv("SELFISHELL_CURL_LOW_SPEED_LIMIT", tc.curlLimit)
			t.Setenv("SELFISHELL_CURL_LOW_SPEED_TIME", tc.curlTime)
			t.Setenv("GIT_HTTP_LOW_SPEED_LIMIT", tc.gitLimit)
			t.Setenv("GIT_HTTP_LOW_SPEED_TIME", tc.gitTime)
			for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "MISE_DATA_DIR", "MISE_CACHE_DIR", "MISE_STATE_DIR", "MISE_CONFIG_DIR"} {
				t.Setenv(key, root+"/private/"+key)
			}
			if err := os.WriteFile(root+"/git", []byte("#!/bin/sh\nprintf '%s %s\\n' \"$GIT_HTTP_LOW_SPEED_LIMIT\" \"$GIT_HTTP_LOW_SPEED_TIME\" >\"$HOME/git-env\"\nexit 9\n"), 0700); err != nil {
				t.Fatal(err)
			}
			op := &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
			dep := Dependency{Kind: "git", Name: "fixture", Source: "file:///private/fixture"}
			if err := op.stageGit(context.Background(), dep, root+"/stage"); err == nil {
				t.Fatal("fake git failure ignored")
			}
			data, err := os.ReadFile(root + "/git-env")
			if err != nil || string(data) != tc.want {
				t.Fatalf("git child environment %q want %q: %v", data, tc.want, err)
			}
		})
	}
}

func TestProcessSkipsRelativePATHEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	safe := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "git"), []byte("#!/bin/sh\nprintf unsafe\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(safe, "git"), []byte("#!/bin/sh\nprintf safe\n"), 0755); err != nil {
		t.Fatal(err)
	}
	workdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(workdir, project)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", rel+":"+safe)
	var out bytes.Buffer
	p := Process{Dir: project, Out: &out, Env: []string{"HOME=" + home, "PATH=" + rel + ":" + safe}}
	if code, err := p.Run(context.Background(), "git"); err != nil || code != 0 || out.String() != "safe" {
		t.Fatalf("PATH lookup: code=%d err=%v output=%q", code, err, out.String())
	}
	out.Reset()
	if code, err := (Process{Dir: project, Out: &out}).Run(context.Background(), "git"); err != nil || code != 0 || out.String() != "safe" {
		t.Fatalf("inherited PATH lookup: code=%d err=%v output=%q", code, err, out.String())
	}
	out.Reset()
	if code, err := p.Run(context.Background(), "./git"); err != nil || code != 0 || out.String() != "unsafe" {
		t.Fatalf("explicit relative command: code=%d err=%v output=%q", code, err, out.String())
	}
}

func TestProcessInheritedEnvironmentUpdatesPWDForDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_HTTP_LOW_SPEED_LIMIT", "256")
	t.Setenv("GIT_HTTP_LOW_SPEED_TIME", "120")
	dir := t.TempDir()
	var out bytes.Buffer
	code, err := (Process{Dir: dir, Out: &out}).Run(context.Background(), "/usr/bin/env")
	if err != nil || code != 0 {
		t.Fatalf("env: code=%d err=%v", code, err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if value, ok := strings.CutPrefix(line, "PWD="); ok {
			if value != dir {
				t.Fatalf("child PWD=%q, want %q", value, dir)
			}
			out.Reset()
			code, err = (Process{Dir: dir, Out: &out, Env: []string{"HOME=" + home, "PWD=/preserved"}}).Run(context.Background(), "/usr/bin/env")
			if err != nil || code != 0 || !strings.Contains(out.String(), "PWD=/preserved\n") {
				t.Fatalf("explicit child PWD: code=%d err=%v output=%q", code, err, out.String())
			}
			return
		}
	}
	t.Fatal("child PWD missing")
}

func TestProbeParallelBoundsConcurrencyAndRunsEveryIndexOnce(t *testing.T) {
	const n = 3*probeLimit + 1
	var active, peak atomic.Int32
	seen := make([]atomic.Int32, n)
	// Each probe waits for a full batch, so a sequential runner fails here.
	deadline := time.Now().Add(5 * time.Second)
	probeParallel(n, func(i int) {
		now := active.Add(1)
		defer active.Add(-1)
		for current := peak.Load(); now > current; current = peak.Load() {
			if peak.CompareAndSwap(current, now) {
				break
			}
		}
		for peak.Load() < probeLimit && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		seen[i].Add(1)
	})
	if got := peak.Load(); got != probeLimit {
		t.Fatalf("peak concurrency %d, want %d", got, probeLimit)
	}
	for i := range seen {
		if got := seen[i].Load(); got != 1 {
			t.Fatalf("probe %d ran %d times", i, got)
		}
	}
}

func TestGitChildIgnoresInheritedRepositorySelectors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(home+"/git", []byte("#!/bin/sh\nprintf '%s|%s|%s|%s' \"$GIT_DIR\" \"$GIT_WORK_TREE\" \"$GIT_INDEX_FILE\" \"$GIT_HTTP_LOW_SPEED_LIMIT\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	p := Process{Out: &out, Env: []string{"HOME=" + home, "PATH=" + home, "GIT_DIR=/foreign", "GIT_WORK_TREE=/foreign", "GIT_INDEX_FILE=/foreign/index", "GIT_HTTP_LOW_SPEED_LIMIT=12"}}
	if code, err := p.Run(context.Background(), "git"); err != nil || code != 0 || out.String() != "|||12" {
		t.Fatalf("Git child environment: code=%d err=%v output=%q", code, err, out.String())
	}
}

func TestIndirectGitChildIgnoresInheritedRepositorySelectors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(home+"/nvim", []byte("#!/bin/sh\nprintf '%s|%s|%s' \"$GIT_DIR\" \"$GIT_WORK_TREE\" \"$GIT_INDEX_FILE\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	p := Process{Out: &out, Env: []string{"HOME=" + home, "PATH=" + home, "GIT_DIR=/foreign", "GIT_WORK_TREE=/foreign", "GIT_INDEX_FILE=/foreign/index"}}
	if code, err := p.Run(context.Background(), "nvim"); err != nil || code != 0 || out.String() != "||" {
		t.Fatalf("indirect child environment: code=%d err=%v output=%q", code, err, out.String())
	}
}

func TestGitSpeedEnvironmentReachesIndirectToolChild(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("PATH", root)
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "MISE_DATA_DIR", "MISE_CACHE_DIR", "MISE_STATE_DIR", "MISE_CONFIG_DIR"} {
		t.Setenv(key, root+"/private/"+key)
	}
	t.Setenv("SELFISHELL_CURL_LOW_SPEED_LIMIT", "256")
	t.Setenv("SELFISHELL_CURL_LOW_SPEED_TIME", "120")
	t.Setenv("GIT_HTTP_LOW_SPEED_LIMIT", "5")
	t.Setenv("GIT_HTTP_LOW_SPEED_TIME", "")
	if err := os.WriteFile(root+"/nvim", []byte("#!/bin/sh\nprintf '%s %s\\n' \"$GIT_HTTP_LOW_SPEED_LIMIT\" \"$GIT_HTTP_LOW_SPEED_TIME\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := (Process{Out: &out, Err: io.Discard}).Run(context.Background(), "nvim")
	if err != nil || code != 0 || out.String() != "5 120\n" {
		t.Fatalf("indirect child: code=%d err=%v env=%q", code, err, out.String())
	}
}
