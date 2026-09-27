package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func nativeConsumerFixture(t *testing.T, versions ...string) *bootstrapFixture {
	t.Helper()
	f := newBootstrapFixture(t, versions...)
	if runtime.GOOS == "darwin" {
		f.env = append(f.env, "SELFISHELL_TEST_SYSTEM_NAME=Darwin", "XDG_CONFIG_HOME="+f.home+"/xdg-config", "XDG_STATE_HOME="+f.home+"/xdg-state", "XDG_CACHE_HOME="+f.home+"/xdg-cache", "XDG_DATA_HOME="+f.home+"/xdg-data")
	}
	return f
}

func countBlock(t *testing.T, path, marker string) {
	t.Helper()
	info, err := os.Lstat(path)
	mustFS(t, err)
	if !info.Mode().IsRegular() {
		t.Fatalf("user file is not regular: %s %v", path, info.Mode())
	}
	if got := bytes.Count(readBytes(t, path), []byte(marker)); got != 1 {
		t.Fatalf("%s: block count %d", path, got)
	}
}

func assertCleanResources(t *testing.T, f *bootstrapFixture) []byte {
	t.Helper()
	got := f.cliRun(t, "status")
	if got.Status != 0 && got.Status != 1 {
		t.Fatalf("status: %d %s", got.Status, got.Stderr)
	}
	output := append(append([]byte{}, got.Stdout...), got.Stderr...)
	for _, marker := range []string{"[CHANGED]", "[MALFORMED]", "[PENDING]"} {
		if bytes.Contains(output, []byte(marker)) {
			t.Fatalf("status contains %s: %s", marker, output)
		}
	}
	return got.Stdout
}

func backupPaths(t *testing.T, roots ...string) []string {
	t.Helper()
	var paths []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.Contains(d.Name(), ".backup.") {
				paths = append(paths, path)
			}
			return nil
		})
		mustFS(t, err)
	}
	return paths
}

func TestNativeMacInstalledArchiveConsumer(t *testing.T) {
	if os.Getenv("SELFISHELL_NATIVE_MACOS_E2E") != "1" {
		t.Skip("requires SELFISHELL_NATIVE_MACOS_E2E=1")
	}
	if runtime.GOOS != "darwin" {
		t.Fatal("requires native macOS")
	}
	t.Logf("exact archive consumer executing %s/%s", runtime.GOOS, runtime.GOARCH)
	const initial, next = "0.0.0-macos.1", "0.0.0-macos.2"
	t.Run("primary", func(t *testing.T) {
		f := nativeConsumerFixture(t, initial, next)
		config, state := f.home+"/xdg-config", f.home+"/xdg-state"
		mustFS(t, os.MkdirAll(config, 0700))
		originalZsh := []byte("export SELFISHELL_E2E_MARKER=1\r\nalias ll=\"ls -la\"")
		originalVim := []byte("set nocompatible\r\nset background=dark")
		originalStarship := []byte("format = \"user starship config\"\n")
		zshrc, vimrc, starship := f.home+"/.zshrc", f.home+"/.vimrc", config+"/starship.toml"
		mustFS(t, os.WriteFile(zshrc, originalZsh, 0640))
		mustFS(t, os.WriteFile(vimrc, originalVim, 0600))
		mustFS(t, os.WriteFile(starship, originalStarship, 0600))
		requireOK(t, f.run(t, "--version", initial, "--setup", "--yes", "--skip-packages"))
		requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+initial)
		countBlock(t, zshrc, "# >>> Selfishell initialize >>>")
		countBlock(t, vimrc, "\" >>> Selfishell vimrc >>>")
		for _, item := range []struct {
			path   string
			suffix []byte
		}{{zshrc, originalZsh}, {vimrc, originalVim}} {
			if !bytes.HasSuffix(readBytes(t, item.path), item.suffix) {
				t.Fatalf("user suffix changed: %s", item.path)
			}
		}
		info, err := os.Lstat(zshrc)
		mustFS(t, err)
		if info.Mode().Perm() != 0640 {
			t.Fatalf("zshrc mode %v", info.Mode())
		}
		if info, err := os.Stat(config + "/selfishell"); err != nil || !info.IsDir() {
			t.Fatalf("XDG config missing: %v", err)
		}
		if info, err := os.Stat(state + "/selfishell"); err != nil || !info.IsDir() {
			t.Fatalf("XDG state missing: %v", err)
		}
		requireAbsent(t, f.home+"/.zshenv")
		err = filepath.WalkDir(f.home, func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				target, e := os.Readlink(path)
				if e != nil {
					return e
				}
				if strings.HasPrefix(target, repoRoot()) {
					return fmt.Errorf("source link: %s -> %s", path, target)
				}
			}
			return nil
		})
		mustFS(t, err)
		backups := backupPaths(t, config, state)
		starshipBackups, err := filepath.Glob(starship + ".backup.*")
		mustFS(t, err)
		if len(starshipBackups) != 1 || !bytes.Equal(readBytes(t, starshipBackups[0]), originalStarship) {
			t.Fatalf("starship backup: %v", starshipBackups)
		}
		requireOK(t, f.cliRun(t, "install", "--skip-packages", "--yes"))
		countBlock(t, zshrc, "# >>> Selfishell initialize >>>")
		countBlock(t, vimrc, "\" >>> Selfishell vimrc >>>")
		if after := backupPaths(t, config, state); !equalStrings(backups, after) {
			t.Fatalf("reinstall created backup: %v -> %v", backups, after)
		}
		status := assertCleanResources(t, f)
		requireContains(t, status, "[INFO] Selfishell configuration is installed.")
		requireContains(t, readBytes(t, state+"/selfishell/configured"), "1\n")
		f.env = append(f.env, "SELFISHELL_RELEASE_ROOT=file:///network-must-not-be-used")
		requireOK(t, f.cliRun(t, "update", "--tools-only", "--skip-packages", "--yes"))
		countBlock(t, zshrc, "# >>> Selfishell initialize >>>")
		countBlock(t, vimrc, "\" >>> Selfishell vimrc >>>")
		if !bytes.HasSuffix(readBytes(t, zshrc), originalZsh) || !bytes.HasSuffix(readBytes(t, vimrc), originalVim) {
			t.Fatal("update changed user suffix")
		}
		assertCleanResources(t, f)
		requireOK(t, f.cliRun(t, "uninstall", "--restore", "--yes"))
		if !bytes.Equal(readBytes(t, zshrc), originalZsh) || !bytes.Equal(readBytes(t, vimrc), originalVim) || !bytes.Equal(readBytes(t, starship), originalStarship) {
			t.Fatal("restore changed original bytes")
		}
		info, err = os.Lstat(zshrc)
		mustFS(t, err)
		if info.Mode().Perm() != 0640 {
			t.Fatalf("restored mode %v", info.Mode())
		}
		requireAbsent(t, config+"/selfishell/zsh/zshrc")
	})
	t.Run("ghostty", func(t *testing.T) {
		f := nativeConsumerFixture(t, initial)
		requireOK(t, f.run(t, "--version", initial, "--setup", "--yes", "--skip-packages"))
		if got := string(readBytes(t, f.home+"/xdg-state/selfishell/ghostty")); got != "1\n" {
			t.Fatalf("ghostty choice %q", got)
		}
		if info, err := os.Stat(f.home + "/xdg-config/selfishell/ghostty/config.ghostty"); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("managed ghostty: %v", err)
		}
		entry := f.home + "/xdg-config/ghostty/config.ghostty"
		countBlock(t, entry, "# >>> Selfishell ghostty >>>")
	})
	t.Run("purge", func(t *testing.T) {
		f := nativeConsumerFixture(t, initial)
		requireOK(t, f.run(t, "--version", initial, "--setup", "--yes", "--skip-packages"))
		requireLink(t, f.cli, filepath.Join(f.share, "current/bin/selfishell"))
		requireOK(t, f.cliRun(t, "uninstall", "--restore", "--purge", "--yes"))
		for _, path := range []string{f.cli, f.prefix + "/bin/sfs", f.share, f.home + "/xdg-cache/selfishell", f.home + "/xdg-state/selfishell"} {
			requireAbsent(t, path)
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func fullUbuntuCommand(t *testing.T, f *bootstrapFixture, dir string, timeout time.Duration, args ...string) capture {
	t.Helper()
	got, err := runCommandIn(f.home, dir, args, nil, f.env, timeout)
	mustFS(t, err)
	return got
}

func TestNativeUbuntuInstalledArchiveConsumer(t *testing.T) {
	if os.Getenv("SELFISHELL_UBUNTU_FULL_E2E") != "1" {
		t.Skip("requires SELFISHELL_UBUNTU_FULL_E2E=1")
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Fatal("requires a disposable root Ubuntu container")
	}
	t.Logf("exact archive consumer executing %s/%s", runtime.GOOS, runtime.GOARCH)
	osRelease := readBytes(t, "/etc/os-release")
	if !bytes.Contains(osRelease, []byte("ID=ubuntu\n")) || !bytes.Contains(osRelease, []byte("VERSION_ID=\"24.04\"")) {
		t.Fatal("requires Ubuntu 24.04")
	}
	if _, err := os.Stat("/usr/bin/sudo"); err == nil {
		t.Fatal("disposable image unexpectedly contains sudo")
	}
	const initial, next = "0.0.0-container.1", "0.0.0-container.2"
	f := nativeConsumerFixture(t, initial, next)
	for i, entry := range f.env {
		if strings.HasPrefix(entry, "MISE_OFFLINE=") {
			f.env = append(f.env[:i], f.env[i+1:]...)
			break
		}
	}
	f.prefix = f.home + "/.local"
	f.share = f.prefix + "/share/selfishell"
	f.cli = f.prefix + "/bin/selfishell"
	// The opt-in has real Apt and developer tools, but all home, XDG and mise paths remain private.
	f.env = append(f.env,
		"SELFISHELL_TEST_SYSTEM_NAME=Linux", "SELFISHELL_TEST_OS_RELEASE_FILE=/etc/os-release",
		"XDG_CONFIG_HOME="+f.home+"/.config", "XDG_DATA_HOME="+f.home+"/.local/share", "XDG_STATE_HOME="+f.home+"/.local/state", "XDG_CACHE_HOME="+f.home+"/.cache",
		"MISE_DATA_DIR="+f.home+"/.local/share/mise", "MISE_STATE_DIR="+f.home+"/.local/state/mise", "MISE_CACHE_DIR="+f.home+"/.cache/mise", "MISE_CONFIG_DIR="+f.home+"/.config/mise", "DEBIAN_FRONTEND=noninteractive",
		"PATH="+f.prefix+"/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "GITHUB_TOKEN"} {
		if value, ok := os.LookupEnv(key); ok {
			f.env = append(f.env, key+"="+value)
		}
	}
	if path, err := resolveCommand("go", f.env); err == nil {
		t.Fatalf("installed CLI PATH exposes Go: %s", path)
	}
	if path, err := resolveCommand("sudo", f.env); err == nil {
		t.Fatalf("disposable container PATH exposes sudo: %s", path)
	}
	project := f.home + "/project"
	mustFS(t, os.MkdirAll(project, 0700))
	mustFS(t, os.WriteFile(project+"/mise.toml", []byte("[tools]\nnode = \"0.0.0\"\nneovim = \"0.0.0\"\nstarship = \"0.0.0\"\n"), 0600))
	f.env = append(f.env, "MISE_TRUSTED_CONFIG_PATHS="+f.home)
	requireOK(t, fullUbuntuCommand(t, f, f.home, 45*time.Second, "/bin/bash", repoRoot()+"/install.sh", "--prefix", f.prefix, "--version", initial))
	requireOK(t, fullUbuntuCommand(t, f, project, 35*time.Minute, f.cli, "install", "--yes"))
	requireOK(t, fullUbuntuCommand(t, f, project, 45*time.Second, f.cli, "status"))
	requireOK(t, fullUbuntuCommand(t, f, project, 45*time.Second, f.cli, "doctor"))
	deps := strings.Split(string(readBytes(t, filepath.Join(f.share, "current/dependencies.conf"))), "\n")
	heads := map[string]bool{}
	directories := map[string]os.FileInfo{}
	for _, line := range deps {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "zsh-plugin" {
			continue
		}
		target := f.home + "/.local/share/zinit/plugins/" + strings.ReplaceAll(fields[1], "/", "---")
		head := fullUbuntuCommand(t, f, f.home, 15*time.Second, "git", "-C", target, "rev-parse", "HEAD")
		requireOK(t, head)
		if strings.TrimSpace(string(head.Stdout)) != fields[2] || heads[fields[2]] {
			t.Fatalf("plugin HEAD mismatch or repeated revision: %s %s", fields[1], head.Stdout)
		}
		heads[fields[2]] = true
		meta := target + "/._zinit"
		if info, err := os.Stat(meta); err != nil || !info.IsDir() {
			t.Fatalf("plugin metadata %s: %v", target, err)
		}
		if got := string(readBytes(t, meta+"/.gitignore")); got != "*\n" {
			t.Fatalf("plugin ignore %q", got)
		}
		for _, name := range []string{"ver", "teleid", "light-mode"} {
			info, err := os.Stat(meta + "/" + name)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("plugin metadata %s: %v", name, err)
			}
		}
		status := fullUbuntuCommand(t, f, f.home, 15*time.Second, "git", "-C", target, "status", "--porcelain")
		requireOK(t, status)
		if len(status.Stdout) != 0 {
			t.Fatalf("dirty plugin %s: %s", target, status.Stdout)
		}
		info, err := os.Stat(target)
		mustFS(t, err)
		directories[target] = info
	}
	if len(directories) == 0 {
		t.Fatal("no declared Zinit plugins checked")
	}
	requireOK(t, fullUbuntuCommand(t, f, project, 35*time.Minute, f.cli, "install", "--yes"))
	for path, before := range directories {
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("plugin checkout replaced: %s %v", path, err)
		}
	}
	requireOK(t, fullUbuntuCommand(t, f, project, 45*time.Second, f.cli, "status"))
	f.env = append(f.env, "MISE_OFFLINE=1")
	zsh := fullUbuntuCommand(t, f, f.home, 45*time.Second, "zsh", "-d", "-i", "-c", `for tool in starship fzf zoxide; do [[ "${commands[$tool]}" == "$MISE_DATA_DIR/installs/"* ]] || exit 1; done; (( $+functions[prompt_starship_precmd] && $+functions[fzf-file-widget] && $+functions[__zoxide_z] ))`)
	requireOK(t, zsh)
	vim := fullUbuntuCommand(t, f, f.home, 45*time.Second, "vim", "--not-a-term", "-c", "if !&number || !&relativenumber | cquit 1 | endif", "-c", "q")
	requireOK(t, vim)
	where := fullUbuntuCommand(t, f, f.home, 30*time.Second, f.prefix+"/bin/mise", "-C", f.home, "where", "starship")
	requireOK(t, where)
	starship := strings.TrimSpace(string(where.Stdout))
	moved := f.home + "/starship-temporarily-uninstalled"
	mustFS(t, os.Rename(starship, moved))
	t.Cleanup(func() {
		if _, err := os.Stat(moved); err == nil {
			_ = os.Rename(moved, starship)
		}
	})
	f.env = append(f.env, "PATH="+f.home+"/.local/share/mise/shims:"+f.prefix+"/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	doctor := fullUbuntuCommand(t, f, project, 45*time.Second, f.cli, "doctor")
	if doctor.Status == 0 || !bytes.Contains(append(doctor.Stdout, doctor.Stderr...), []byte("Tool: starship is missing (mise)")) {
		t.Fatalf("orphan starship accepted: %d %s %s", doctor.Status, doctor.Stdout, doctor.Stderr)
	}
	mustFS(t, os.Rename(moved, starship))
	requireOK(t, fullUbuntuCommand(t, f, project, 45*time.Second, f.cli, "doctor"))
	requireOK(t, fullUbuntuCommand(t, f, project, 45*time.Second, f.cli, "update", "--cli-only", "--version", next, "--yes"))
	requireLink(t, filepath.Join(f.share, "current"), "releases/"+next)
	requireLink(t, filepath.Join(f.share, "previous"), "releases/"+initial)
	requireContains(t, fullUbuntuCommand(t, f, project, 30*time.Second, f.cli, "version").Stdout, "selfishell "+next)
	f.env = append(f.env, "SELFISHELL_RELEASE_ROOT=file:///network-must-not-be-used")
	requireOK(t, fullUbuntuCommand(t, f, project, 45*time.Second, f.cli, "rollback", "--yes"))
	requireLink(t, filepath.Join(f.share, "current"), "releases/"+initial)
	requireContains(t, fullUbuntuCommand(t, f, project, 30*time.Second, f.cli, "version").Stdout, "selfishell "+initial)
	requireOK(t, fullUbuntuCommand(t, f, project, 45*time.Second, f.cli, "uninstall", "--restore", "--purge", "--yes"))
	for _, path := range []string{f.cli, f.prefix + "/bin/sfs", f.share} {
		requireAbsent(t, path)
	}
	info, err := os.Lstat(f.home + "/.zshrc")
	mustFS(t, err)
	if !info.Mode().IsRegular() || len(readBytes(t, f.home+"/.zshrc")) != 0 {
		t.Fatal("created loader was not retained empty")
	}
}
