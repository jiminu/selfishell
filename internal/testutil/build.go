// Package testutil contains build setup shared only by tests.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// GoCache borrows only an explicitly selected cache; otherwise the test owns it.
func GoCache(t testing.TB) string {
	t.Helper()
	cache := os.Getenv("SELFISHELL_TEST_GO_CACHE")
	if cache == "" {
		return t.TempDir()
	}
	if !filepath.IsAbs(cache) {
		t.Fatal("SELFISHELL_TEST_GO_CACHE must be absolute")
	}
	return cache
}

func CLIOverride() (string, error) {
	path := os.Getenv("SELFISHELL_TEST_CLI")
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("SELFISHELL_TEST_CLI must be absolute")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("invalid SELFISHELL_TEST_CLI %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("SELFISHELL_TEST_CLI is not executable: %s", path)
	}
	return path, nil
}

// CopyCLI keeps each fixture's release-root discovery local to that fixture.
// A symlink to the shared binary would resolve back into the source checkout.
func CopyCLI(t testing.TB, target string) bool {
	t.Helper()
	source, err := CLIOverride()
	if err != nil {
		t.Fatal(err)
	}
	if source == "" {
		return false
	}
	target, err = filepath.Abs(target)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(source) == target {
		return true
	}
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if resolvedSource == filepath.Join(resolvedParent, filepath.Base(target)) {
		t.Fatal("SELFISHELL_TEST_CLI resolves to the copy target; use the target path directly")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0755); err != nil {
		t.Fatal(err)
	}
	return true
}
