package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// SetEnv replaces t.Setenv for an injected environment such as
// process.Environment: it sets key in *env only, starting from the process
// environment when *env is nil, so the test can run in parallel. A subtest
// copies env first (env := env); SetEnv never shares entries.
func SetEnv[E ~[]string](env *E, key, value string) {
	entries := []string(*env)
	if entries == nil {
		entries = os.Environ()
	}
	out := make(E, 0, len(entries)+1)
	for _, entry := range entries {
		if name, _, _ := strings.Cut(entry, "="); name != key {
			out = append(out, entry)
		}
	}
	*env = append(out, key+"="+value)
}

// WriteTestFile creates path's parent directories and writes value through
// WriteFile, failing the test on error.
func WriteTestFile(t testing.TB, path, value string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
}

// ReadTestFile returns path's contents, failing the test on error.
func ReadTestFile(t testing.TB, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// GitCommand runs git in dir without system or global configuration and
// returns its trimmed output, failing the test on error.
func GitCommand(t testing.TB, dir string, args ...string) string {
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
