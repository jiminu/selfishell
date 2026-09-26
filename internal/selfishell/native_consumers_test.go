package selfishell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nativeCLI(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run(args)
	return code, out.String(), stderr.String()
}

func TestNativeMacConfigurationConsumer(t *testing.T) {
	if os.Getenv("SELFISHELL_NATIVE_MACOS_E2E") != "1" {
		t.Skip("requires SELFISHELL_NATIVE_MACOS_E2E=1")
	}
	if DetectPlatform().Name != "macos" {
		t.Fatal("macOS E2E must run on macOS")
	}
	root, home := testRelease(t), t.TempDir()
	isolateHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", home+"/xdg-config")
	t.Setenv("XDG_STATE_HOME", home+"/xdg-state")
	t.Setenv("XDG_CACHE_HOME", home+"/xdg-cache")
	t.Setenv("XDG_DATA_HOME", home+"/xdg-data")
	t.Setenv("SHELL", "/bin/zsh")
	originalZsh := []byte("export SELFISHELL_E2E_MARKER=1\r\nalias ll=\"ls -la\"")
	originalVim := []byte("set nocompatible\r\nset background=dark")
	if err := os.WriteFile(home+"/.zshrc", originalZsh, 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home+"/.vimrc", originalVim, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home+"/xdg-config", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home+"/xdg-config/starship.toml", []byte("format = \"user starship config\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if code, _, stderr := nativeCLI(t, root, "install", "--skip-packages", "--yes"); code != 0 {
			t.Fatalf("install %d: %s", code, stderr)
		}
		zsh, _ := os.ReadFile(home + "/.zshrc")
		vim, _ := os.ReadFile(home + "/.vimrc")
		if !bytes.HasSuffix(zsh, originalZsh) || !bytes.HasSuffix(vim, originalVim) {
			t.Fatal("user bytes changed")
		}
		if strings.Count(string(zsh), "# >>> Selfishell initialize >>>") != 1 || strings.Count(string(vim), "\" >>> Selfishell vimrc >>>") != 1 {
			t.Fatal("duplicate user block")
		}
		info, _ := os.Stat(home + "/.zshrc")
		if info.Mode().Perm() != 0640 {
			t.Fatal("zshrc mode changed", info.Mode())
		}
		if _, err := os.Lstat(home + "/.zshenv"); !os.IsNotExist(err) {
			t.Fatal("macOS zshenv managed")
		}
	}
	paths, _ := UserPaths()
	if data, err := os.ReadFile(paths.State + "/ghostty"); err != nil || string(data) != "1\n" {
		t.Fatalf("Ghostty choice %q %v", data, err)
	}
	if _, err := os.Stat(home + "/xdg-config/ghostty/config.ghostty"); err != nil {
		t.Fatal("Ghostty entrypoint missing", err)
	}
	backups, err := filepath.Glob(home + "/xdg-config/starship.toml.backup.*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("Starship backups: %v %v", backups, err)
	}
	if code, _, stderr := nativeCLI(t, root, "uninstall", "--restore", "--yes"); code != 0 {
		t.Fatalf("uninstall: %s", stderr)
	}
	if zsh, _ := os.ReadFile(home + "/.zshrc"); !bytes.Equal(zsh, originalZsh) {
		t.Fatalf("zsh restore: %q", zsh)
	}
	if vim, _ := os.ReadFile(home + "/.vimrc"); !bytes.Equal(vim, originalVim) {
		t.Fatalf("vim restore: %q", vim)
	}
	if starship, _ := os.ReadFile(home + "/xdg-config/starship.toml"); string(starship) != "format = \"user starship config\"\n" {
		t.Fatalf("starship restore: %q", starship)
	}
}
