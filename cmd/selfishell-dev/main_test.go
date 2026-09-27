package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCurlOperation(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(root, "curl")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$CURL_ARGS\"\nprintf '%s\\n' \"$HTTPS_PROXY\" >\"$CURL_PROXY\"\nprintf 'metadata-payload'\nexit \"${CURL_STATUS:-0}\"\n"
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bin := filepath.Join(root, "selfishell-dev")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	build.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "MISE_DATA_DIR=" + filepath.Join(home, "mise-data"), "TMPDIR=" + filepath.Join(root, "tmp")}
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	run := func(status, args string, extra ...string) (string, int) {
		t.Helper()
		cmd := exec.CommandContext(ctx, bin, append([]string{"curl"}, extra...)...)
		cmd.Env = []string{"HOME=" + home, "PATH=" + root + ":" + os.Getenv("PATH"), "GOTOOLCHAIN=local", "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "MISE_DATA_DIR=" + filepath.Join(home, "mise-data"), "TMPDIR=" + filepath.Join(root, "tmp"), "CURL_ARGS=" + filepath.Join(root, "args"), "CURL_PROXY=" + filepath.Join(root, "proxy"), "HTTPS_PROXY=http://proxy.invalid:8080", "CURL_STATUS=" + status, "SELFISHELL_CURL_CONNECT_TIMEOUT=7", "SELFISHELL_CURL_LOW_SPEED_LIMIT=512", "SELFISHELL_CURL_LOW_SPEED_TIME=4", "SELFISHELL_CURL_METADATA_MAX_TIME=9"}
		output, err := cmd.CombinedOutput()
		if err == nil {
			return string(output), 0
		}
		if e, ok := err.(*exec.ExitError); ok {
			return string(output), e.ExitCode()
		}
		t.Fatalf("run %s: %v: %s", args, err, output)
		return "", -1
	}
	out, status := run("0", "metadata", "metadata", "-H", "Authorization: Bearer secret-123", "https://example.invalid/metadata")
	if status != 0 || out != "metadata-payload" {
		t.Fatalf("metadata status=%d output=%q", status, out)
	}
	got, err := os.ReadFile(filepath.Join(root, "args"))
	if err != nil {
		t.Fatal(err)
	}
	want := "-fsSL\n--connect-timeout\n7\n--speed-limit\n512\n--speed-time\n4\n--max-time\n9\n-H\nAuthorization: Bearer secret-123\nhttps://example.invalid/metadata\n"
	if string(got) != want {
		t.Fatalf("metadata argv=%q", got)
	}
	proxy, err := os.ReadFile(filepath.Join(root, "proxy"))
	if err != nil || string(proxy) != "http://proxy.invalid:8080\n" {
		t.Fatalf("proxy=%q err=%v", proxy, err)
	}
	out, status = run("0", "transfer", "transfer", "https://example.invalid/archive", "-o", filepath.Join(root, "archive"))
	if status != 0 || out != "metadata-payload" {
		t.Fatalf("transfer status=%d output=%q", status, out)
	}
	got, _ = os.ReadFile(filepath.Join(root, "args"))
	if strings.Contains(string(got), "--max-time") || !strings.HasSuffix(string(got), "https://example.invalid/archive\n-o\n"+filepath.Join(root, "archive")+"\n") {
		t.Fatalf("transfer argv=%q", got)
	}
	for _, argv := range [][]string{{"curl"}, {"curl", "unknown", "https://example.invalid"}, {"bad", "mode", "url"}} {
		_, status = run("0", "invalid", argv...)
		if status == 0 {
			t.Fatalf("accepted %v", argv)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
		t.Fatalf("invalid operation made provision paths: %v", err)
	}
	out, status = run("22", "failure", "metadata", "-H", "Authorization: Bearer secret-123", "https://example.invalid")
	if status != 22 || strings.Contains(out, "secret-123") {
		t.Fatalf("failure status=%d output=%q", status, out)
	}
	cmdInvalid := exec.CommandContext(ctx, bin, "curl", "metadata", "https://example.invalid")
	cmdInvalid.Env = []string{"HOME=" + home, "PATH=" + root + ":/usr/bin:/bin", "SELFISHELL_CURL_CONNECT_TIMEOUT=0", "TMPDIR=" + filepath.Join(root, "tmp")}
	invalidOut, invalidErr := cmdInvalid.CombinedOutput()
	if exit, ok := invalidErr.(*exec.ExitError); !ok || exit.ExitCode() != 2 || !strings.Contains(string(invalidOut), "positive integers") {
		t.Fatalf("invalid policy: err=%v output=%q", invalidErr, invalidOut)
	}
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 2\n"), 0700); err != nil {
		t.Fatal(err)
	}
	short, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	cmd := exec.CommandContext(short, bin, "curl", "metadata", "https://example.invalid")
	cmd.Env = []string{"HOME=" + home, "PATH=" + root + ":/usr/bin:/bin", "TMPDIR=" + filepath.Join(root, "tmp")}
	if _, err := cmd.CombinedOutput(); err == nil || short.Err() == nil {
		t.Fatalf("cancelled transport: err=%v context=%v", err, short.Err())
	}
}
