package selfishell

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Selfishell owns the bounded loader block, while the enclosing .zshrc is a user file.
func TestUbuntuCreatedLoaderUninstallContract(t *testing.T) {
	root, home := testRelease(t), t.TempDir()
	isolateHome(t, home)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	release := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(release, []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", release)
	t.Setenv("SHELL", "/bin/zsh")
	if code, _, stderr := nativeCLI(t, root, "install", "--skip-packages", "--yes"); code != 0 {
		t.Fatalf("configuration-only install: %s", stderr)
	}
	loader, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := blockContent("user-zshrc", "")
	if err != nil || !bytes.Equal(loader, want) {
		t.Fatalf("new loader bytes: %q, error: %v", loader, err)
	}
	if code, _, stderr := nativeCLI(t, root, "uninstall", "--restore", "--yes"); code != 0 {
		t.Fatalf("uninstall: %s", stderr)
	}
	assertCreatedLoaderCleared(t, home)
}

func assertCreatedLoaderCleared(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, ".zshrc")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("user-owned loader file not retained as regular file: %v, %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != 0 {
		t.Fatalf("uninstall left loader bytes: %q, error: %v", data, err)
	}
}
