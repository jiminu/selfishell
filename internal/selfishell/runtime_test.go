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
	"strings"
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
		code, err := (Process{Out: write, Err: io.Discard}).Run(ctx, executable, "-test.run=^TestProcessChild$", "--", "wait")
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
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled command still running")
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
