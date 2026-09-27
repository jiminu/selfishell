package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Copy the real tracked entrypoint so absence tests never touch the checkout's
// shared .build directory, which benchmark tests may use in parallel.
func sourceLauncherFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "source with spaces")
	home := filepath.Join(t.TempDir(), "private home")
	mustFS(t, os.MkdirAll(filepath.Join(root, "bin"), 0700))
	mustFS(t, os.MkdirAll(home, 0700))
	entry := filepath.Join(root, "bin", "selfishell")
	mustFS(t, copyFile(filepath.Join(repoRoot(), "bin", "selfishell"), entry))
	return root, home, entry
}

func TestSourceLauncherRequiresExplicitBuild(t *testing.T) {
	root, home, entry := sourceLauncherFixture(t)
	got, err := runCommandIn(home, home, []string{entry, "version"}, nil, maintenanceMiseEnv(home), 5*time.Second)
	mustFS(t, err)
	if got.Status == 0 || len(got.Stdout) != 0 || !strings.Contains(string(got.Stderr), "bash scripts/build-cli.sh") {
		t.Fatalf("missing binary: status=%d stdout=%q stderr=%q", got.Status, got.Stdout, got.Stderr)
	}
	if _, err := os.Lstat(filepath.Join(root, ".build")); !os.IsNotExist(err) {
		t.Fatalf("launcher built implicitly: %v", err)
	}
}

func TestSourceLauncherForwardsThroughSymlinkFromHostileCWD(t *testing.T) {
	root, home, entry := sourceLauncherFixture(t)
	mustFS(t, os.MkdirAll(filepath.Join(root, ".build"), 0700))
	binary := filepath.Join(root, ".build", "selfishell")
	mustFS(t, os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'arg:%s\\n' \"$1\"\ncat\nprintf 'error\\n' >&2\nexit 17\n"), 0700))
	link := filepath.Join(home, "sfs")
	mustFS(t, os.Symlink(entry, link))
	got, err := runCommandIn(home, home, []string{link, "argument with spaces"}, []byte("input\n"), maintenanceMiseEnv(home), 5*time.Second)
	mustFS(t, err)
	if got.Status != 17 || string(got.Stdout) != "arg:argument with spaces\ninput\n" || string(got.Stderr) != "error\n" {
		t.Fatalf("forwarding: status=%d stdout=%q stderr=%q", got.Status, got.Stdout, got.Stderr)
	}
}
