package neovim

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

// setEnv, writeTestFile, readTestFile and gitCommand copy the
// internal/selfishell test helpers.
func setEnv(e *process.Environment, key, value string) {
	*e = e.With(map[string]string{key: value})
}

func writeTestFile(t *testing.T, path, value string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func gitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	// Detached auto-maintenance after a fixture commit would race path snapshots.
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "maintenance.auto=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}
