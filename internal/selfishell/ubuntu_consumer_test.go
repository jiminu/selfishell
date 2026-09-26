package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUbuntuFullInstallConsumer(t *testing.T) {
	if os.Getenv("SELFISHELL_UBUNTU_FULL_E2E") != "1" {
		t.Skip("requires SELFISHELL_UBUNTU_FULL_E2E=1")
	}
	if DetectPlatform().Name != "ubuntu" || os.Geteuid() != 0 {
		t.Fatal("requires a disposable root Ubuntu container")
	}
	root, home := testRelease(t), t.TempDir()
	isolateHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	t.Setenv("XDG_DATA_HOME", home+"/.local/share")
	t.Setenv("XDG_STATE_HOME", home+"/.local/state")
	t.Setenv("XDG_CACHE_HOME", home+"/.cache")
	t.Setenv("MISE_DATA_DIR", home+"/.local/share/mise")
	t.Setenv("MISE_STATE_DIR", home+"/.local/state/mise")
	t.Setenv("MISE_CACHE_DIR", home+"/.cache/mise")
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", home+"/.local/bin:"+os.Getenv("PATH"))
	project := home + "/project"
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project+"/mise.toml", []byte("[tools]\nnode = \"0.0.0\"\nneovim = \"0.0.0\"\nstarship = \"0.0.0\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MISE_TRUSTED_CONFIG_PATHS", project)
	working, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(working) })
	if code, _, stderr := nativeCLI(t, root, "install", "--yes"); code != 0 {
		t.Fatalf("full installation: %s", stderr)
	}
	if code, _, stderr := nativeCLI(t, root, "status"); code != 0 {
		t.Fatalf("status: %s", stderr)
	}
	if code, _, stderr := nativeCLI(t, root, "doctor"); code != 0 {
		t.Fatalf("doctor: %s", stderr)
	}
	deps, err := ReadDependencies(root + "/dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	op := &PackageOperation{Process: Process{Env: DeveloperToolEnv(root)}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pluginDirectories := map[string]os.FileInfo{}
	declaredHeads := map[string]bool{}
	for _, dep := range deps {
		if dep.Kind != "zsh-plugin" {
			continue
		}
		target := home + "/.local/share/zinit/plugins/" + strings.ReplaceAll(dep.Name, "/", "---")
		actual, err := op.gitHead(ctx, target)
		if err != nil || actual != dep.Version {
			t.Fatalf("Zinit plugin %s: HEAD %s want %s: %v", dep.Name, actual, dep.Version, err)
		}
		if declaredHeads[dep.Version] {
			t.Fatalf("declared Zinit plugins share a revision: %s", dep.Version)
		}
		declaredHeads[dep.Version] = true
		meta := target + "/._zinit"
		if info, err := os.Stat(meta); err != nil || !info.IsDir() {
			t.Fatalf("Zinit metadata missing for %s: %v", dep.Name, err)
		}
		if data, err := os.ReadFile(meta + "/.gitignore"); err != nil || string(data) != "*\n" {
			t.Fatalf("Zinit metadata ignore missing for %s: %v", dep.Name, err)
		}
		for _, name := range []string{"ver", "teleid", "light-mode"} {
			if info, err := os.Stat(meta + "/" + name); err != nil || !info.Mode().IsRegular() {
				t.Fatalf("Zinit bookkeeping %s missing for %s: %v", name, dep.Name, err)
			}
		}
		if status, err := op.commandOutput(ctx, "git", "-C", target, "status", "--porcelain"); err != nil || status != "" {
			t.Fatalf("Zinit checkout or ignore rules dirty: %s %q %v", dep.Name, status, err)
		}
		pluginDirectories[target], err = os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(pluginDirectories) == 0 {
		t.Fatal("no declared Zinit plugins checked")
	}
	if code, _, stderr := nativeCLI(t, root, "install", "--yes"); code != 0 {
		t.Fatalf("repeat installation: %s", stderr)
	}
	for target, before := range pluginDirectories {
		after, err := os.Stat(target)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("intact Zinit checkout replaced: %s %v", target, err)
		}
	}
	var zshOut bytes.Buffer
	sh := Process{Dir: home, Out: &zshOut, Err: &zshOut, Env: append(DeveloperToolEnv(root), "MISE_OFFLINE=1")}
	code, err := sh.Run(ctx, "zsh", "-d", "-i", "-c", `for tool in starship fzf zoxide; do [[ "${commands[$tool]}" == "$MISE_DATA_DIR/installs/"* ]] || exit 1; done; (( $+functions[prompt_starship_precmd] && $+functions[fzf-file-widget] && $+functions[__zoxide_z] ))`)
	if err != nil || code != 0 {
		t.Fatalf("installed Zsh integration: %d %v\n%s", code, err, zshOut.String())
	}
	if code, err := sh.Run(ctx, "vim", "--not-a-term", "-c", "if !&number || !&relativenumber | cquit 1 | endif", "-c", "q"); err != nil || code != 0 {
		t.Fatalf("installed Vim config: %d %v", code, err)
	}
	mise := home + "/.local/bin/mise"
	var where bytes.Buffer
	whereProcess := sh
	whereProcess.Out, whereProcess.Err = &where, &where
	code, err = whereProcess.Run(ctx, mise, "-C", root+"/config/shared", "where", "starship")
	if err != nil || code != 0 {
		t.Fatalf("mise where starship: %d %v %s", code, err, where.String())
	}
	starship := strings.TrimSpace(where.String())
	moved := home + "/starship-temporarily-uninstalled"
	if err := os.Rename(starship, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := os.Stat(moved); err == nil {
			_ = os.Rename(moved, starship)
		}
	})
	if code, stdout, stderr := nativeCLI(t, root, "doctor"); code == 0 || !strings.Contains(stdout+stderr, "Tool: starship is missing (mise)") {
		t.Fatalf("doctor accepted orphan shim: %d %s %s", code, stdout, stderr)
	}
	if err := os.Rename(moved, starship); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := nativeCLI(t, root, "uninstall", "--restore", "--yes"); code != 0 {
		t.Fatalf("uninstall: %s", stderr)
	}
	assertCreatedLoaderCleared(t, home)
}

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
