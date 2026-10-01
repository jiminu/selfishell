package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestCurlOperation(t *testing.T) {
	root := t.TempDir()
	home, tmp := filepath.Join(root, "home"), filepath.Join(root, "tmp")
	for _, dir := range []string{home, tmp} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nout=/dev/stdout\nwhile [ \"$#\" -gt 0 ]; do [ \"$1\" = -o ] && out=\"$2\"; shift; done\nprintf 'metadata-payload' >\"$out\"\nexit \"${CURL_STATUS:-0}\"\n"
	if err := os.WriteFile(filepath.Join(root, "curl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bin := filepath.Join(root, "selfishell-dev")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	build.Env = []string{"GOCACHE=" + testutil.GoCache(t), "HOME=" + home, "PATH=" + os.Getenv("PATH"), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "MISE_DATA_DIR=" + filepath.Join(home, "mise-data"), "TMPDIR=" + tmp}
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	run := func(status string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.CommandContext(ctx, bin, append([]string{"curl"}, args...)...)
		cmd.Env = []string{"HOME=" + home, "PATH=" + root + ":/usr/bin:/bin", "TMPDIR=" + tmp, "CURL_STATUS=" + status}
		output, err := cmd.CombinedOutput()
		if _, ok := err.(*exec.ExitError); err != nil && !ok {
			t.Fatalf("run %v: %v: %s", args, err, output)
		}
		return string(output), cmd.ProcessState.ExitCode()
	}
	for _, args := range [][]string{{"metadata"}, {"unknown", "https://example.invalid"}} {
		if out, status := run("0", args...); status != 2 {
			t.Fatalf("accepted %v: status=%d output=%q", args, status, out)
		}
	}
	if out, status := run("0", "metadata", "https://example.invalid/metadata"); status != 0 || out != "metadata-payload" {
		t.Fatalf("metadata status=%d output=%q", status, out)
	}
	// Stdout responses are staged in TMPDIR so a retry cannot append to a partial one.
	if staged, _ := os.ReadDir(tmp); len(staged) != 0 {
		t.Fatalf("staged response left behind: %v", staged)
	}
	if out, status := run("22", "metadata", "-H", "Authorization: Bearer secret-123", "https://example.invalid"); status != 22 || strings.Contains(out, "secret-123") {
		t.Fatalf("failure status=%d output=%q", status, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
		t.Fatalf("curl operation made provision paths: %v", err)
	}
}
