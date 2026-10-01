package selfishell

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func failureResource(t *testing.T, root, name string) Resource {
	t.Helper()
	all, err := ManagedResources(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range all {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("missing resource %s", name)
	return Resource{}
}

func TestConfigurationFailureRetryPreservesOperationScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"install", []string{"install", "--yes"}, "selfishell install"},
		{"install-config-only", []string{"install", "--yes", "--skip-packages"}, "selfishell install --skip-packages"},
		{"update", []string{"update", "--tools-only", "--yes"}, "selfishell update --tools-only"},
		{"update-config-only", []string{"update", "--tools-only", "--skip-packages"}, "selfishell update --tools-only --skip-packages"},
		{"after-cli-update", []string{"update", "--continue-after-cli-update", "--skip-packages"}, "selfishell update --tools-only --skip-packages"},
		{"install-dry-run", []string{"install", "--skip-packages", "--dry-run"}, ""},
		{"update-dry-run", []string{"update", "--tools-only", "--skip-packages", "--dry-run"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, paths := compactDiagnosticFixture(t, "ubuntu", false)
			blockWrite(t, paths.Resources+"/vimrc.state", []byte("invalid\n"))
			code, _, stderr := blockRun(t, root, "", tc.args...)
			if code != 1 || !strings.Contains(stderr, "vimrc.state") {
				t.Fatalf("expected configuration failure: %d %q", code, stderr)
			}
			if tc.want == "" {
				if strings.Contains(stderr, "retry with:") {
					t.Fatalf("dry run suggested a mutating retry: %q", stderr)
				}
			} else if strings.Count(stderr, "retry with:") != 1 || !strings.Contains(stderr, "retry with: "+tc.want+"\n") {
				t.Fatalf("missing scoped retry %q: %q", tc.want, stderr)
			}
		})
	}
}

func TestManagedMalformedStateStopsInstallWithoutMutation(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "vimrc")
	state := paths.Resources + "/vimrc.state"
	original := blockRead(t, r.Target)
	blockWrite(t, state, []byte("2\nfile\nbogus\n"+r.Target+"\n-\n-\n123:4\n"))
	code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(stderr, state) {
		t.Fatalf("%d %q", code, stderr)
	}
	blockEqual(t, r.Target, original)
	if _, err := os.Lstat(paths.State + "/backups"); !os.IsNotExist(err) {
		t.Fatalf("backup created: %v", err)
	}
}

func TestManagedMalformedLateStateStopsUninstallBeforeRemoval(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	loader := home + "/.zshrc"
	before := blockRead(t, loader)
	earlier := blockRead(t, paths.Config+"/zsh/zshrc")
	state := paths.Resources + "/user-zshrc.state"
	blockWrite(t, state, []byte("2\n"))
	code, _, stderr := blockRun(t, root, "", "uninstall", "--yes")
	if code != 1 || !strings.Contains(stderr, state) {
		t.Fatalf("%d %q", code, stderr)
	}
	blockEqual(t, loader, before)
	blockEqual(t, paths.Config+"/zsh/zshrc", earlier)
	blockEqual(t, paths.State+"/configured", []byte("1\n"))
}

func TestManagedEditedLoaderMarkerSurvivesBothOperations(t *testing.T) {
	root, home, _ := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	target := home + "/.zshrc"
	changed := bytes.Replace(blockRead(t, target), []byte("# <<< Selfishell initialize <<<"), []byte("# <<< Selfishell initialize changed <<<"), 1)
	blockWrite(t, target, changed)
	for _, args := range [][]string{{"install", "--skip-packages", "--yes"}, {"uninstall", "--yes"}} {
		if code, _, _ := blockRun(t, root, "", args...); code != 1 {
			t.Fatalf("%v: %d", args, code)
		}
		blockEqual(t, target, changed)
	}
}

func TestManagedInstallWithoutTerminalRequiresYes(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	code, _, stderr := blockRun(t, root, "", "install", "--skip-packages")
	if code != 2 || !strings.Contains(stderr, "--yes") {
		t.Fatalf("%d %q", code, stderr)
	}
	if _, err := os.Lstat(paths.Config); !os.IsNotExist(err) {
		t.Fatalf("config created: %v", err)
	}
}

func TestManagedSameContentReplacedSymlinkIsPreserved(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "vimrc")
	statePath := paths.Resources + "/vimrc.state"
	stateBytes := blockRead(t, statePath)
	personal := home + "/personal-vimrc"
	if err := os.Rename(r.Target, personal); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(personal, r.Target); err != nil {
		t.Fatal(err)
	}
	personalBytes := blockRead(t, personal)
	for _, args := range [][]string{{"status"}, {"install", "--skip-packages", "--yes"}, {"uninstall", "--yes"}} {
		code, _, _ := blockRun(t, root, "", args...)
		if code == 0 {
			t.Fatalf("%v accepted replaced symlink", args)
		}
		if dest, err := os.Readlink(r.Target); err != nil || dest != personal {
			t.Fatalf("link: %q %v", dest, err)
		}
		blockEqual(t, personal, personalBytes)
		blockEqual(t, statePath, stateBytes)
	}
	state := blockState(t, paths, r.Name)
	m := managed{c: CLI{Root: root}, paths: paths}
	if err := m.installFile(r, true); err == nil {
		t.Fatal("preflight accepted replaced symlink")
	}
	if err := m.preflightUninstall(ResourceState{Resource: r, State: state}, false); err == nil {
		t.Fatal("uninstall preflight accepted replaced symlink")
	}
}

func TestManagedUninstallDryRunPreviewsBackupWithoutMutation(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	target := filepath.Join(home, ".config/starship.toml")
	blockWrite(t, target, []byte("personal starship\n"))
	blockOK(t, root, "install", "--skip-packages", "--yes")
	state := blockState(t, paths, "user-starship")
	loader := blockRead(t, home+"/.zshrc")
	stateBytes := blockRead(t, paths.Resources+"/user-starship.state")
	output := blockOK(t, root, "uninstall", "--restore", "--dry-run")
	if !strings.Contains(output, "Would restore: "+state.Backup+" -> "+target) {
		t.Fatal(output)
	}
	if dest, err := os.Readlink(target); err != nil || dest != paths.Config+"/starship.toml" {
		t.Fatalf("link: %q %v", dest, err)
	}
	blockEqual(t, home+"/.zshrc", loader)
	blockEqual(t, paths.Resources+"/user-starship.state", stateBytes)
}

func TestManagedInterruptedBlockUninstallExplainsRetry(t *testing.T) {
	for _, tc := range []struct {
		name, vimrc string
		diagnostics []string
	}{
		{"absent", "user vimrc\n", nil},
		{"malformed", "\" >>> Selfishell vimrc >>>\nuser vimrc\n", []string{"Cannot manage the Selfishell user-vimrc block", "Preserving the file."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home, paths := blockHome(t, "macos")
			blockOK(t, root, "install", "--skip-packages", "--yes")
			state := blockState(t, paths, "user-vimrc")
			state.Status = "pending"
			if err := WriteState(paths.Resources+"/user-vimrc.state", state); err != nil {
				t.Fatal(err)
			}
			blockWrite(t, home+"/.vimrc", []byte(tc.vimrc))
			earlier := blockRead(t, paths.Config+"/zsh/zshrc")
			loader := blockRead(t, home+"/.zshrc")
			code, _, stderr := blockRun(t, root, "", "uninstall", "--yes")
			for _, want := range append(tc.diagnostics, "An interrupted install left this unfinished; run 'selfishell install', then uninstall again.") {
				if code != 1 || !strings.Contains(stderr, want) {
					t.Fatalf("%d %q, want %q", code, stderr, want)
				}
			}
			blockEqual(t, paths.Config+"/zsh/zshrc", earlier)
			blockEqual(t, home+"/.vimrc", []byte(tc.vimrc))
			blockEqual(t, home+"/.zshrc", loader)
			if blockState(t, paths, "user-vimrc").Status != "pending" {
				t.Fatal("pending state changed")
			}
			blockEqual(t, paths.State+"/configured", []byte("1\n"))
		})
	}
}

func TestManagedPendingLoaderAndFileRecoverExactBytes(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	loader := failureResource(t, root, "user-zshrc")
	file := failureResource(t, root, "zsh-common")
	user := []byte("original zshrc\n")
	blockWrite(t, loader.Target, user)
	content, err := blockContent(loader.Name, paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := checksumBytes(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteState(paths.Resources+"/user-zshrc.state", State{"block", "pending", loader.Target, "selfishell-zsh-loader-v1", "-", sum}); err != nil {
		t.Fatal(err)
	}
	blockWrite(t, file.Target, []byte("preexisting managed path"))
	backup := file.Target + ".backup.interrupted"
	source := blockRead(t, file.Source)
	sum, err = checksumBytes(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteState(paths.Resources+"/zsh-common.state", State{"file", "pending", file.Target, "-", backup, sum}); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	got := blockRead(t, home+"/.zshrc")
	if !bytes.Contains(got, user) || !bytes.Contains(got, content) {
		t.Fatalf("loader recovery %q", got)
	}
	if info, err := os.Lstat(loader.Target); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("loader type: %v %v", info, err)
	}
	if blockState(t, paths, loader.Name).Status != "active" {
		t.Fatal("loader still pending")
	}
	blockEqual(t, backup, []byte("preexisting managed path"))
	blockEqual(t, file.Target, source)
	if blockState(t, paths, file.Name).Status != "active" {
		t.Fatal("file still pending")
	}
}

func TestManagedMiseGlobalExistingTypesAndDirectoryPreflight(t *testing.T) {
	for _, kind := range []string{"file", "dangling-link", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root, home, paths := blockHome(t, "macos")
			target := home + "/.config/mise/config.toml"
			other := home + "/other"
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "file":
				blockWrite(t, target, []byte("user file\n"))
			case "dangling-link":
				if err := os.Symlink(other, target); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			code, _, _ := blockRun(t, root, "", "install", "--skip-packages", "--yes")
			if kind == "directory" {
				if code != 1 {
					t.Fatalf("directory accepted: %d", code)
				}
				for _, path := range []string{paths.Config, paths.State, home + "/.config/mise/conf.d/selfishell.toml"} {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("preflight wrote %s: %v", path, err)
					}
				}
			} else if code != 0 {
				t.Fatalf("%s install %d", kind, code)
			}
			after, err := os.Lstat(target)
			if err != nil || before.Mode().Type() != after.Mode().Type() {
				t.Fatalf("type changed: %v %v", after, err)
			}
			if kind == "dangling-link" {
				if a, _ := os.Readlink(target); a != other {
					t.Fatalf("link changed: %q", a)
				}
			}
			if kind == "file" {
				blockEqual(t, target, []byte("user file\n"))
			}
			if kind != "directory" {
				if _, err := os.Lstat(paths.Resources + "/mise-global.state"); !os.IsNotExist(err) {
					t.Fatalf("create-once acquired state: %v", err)
				}
			}
		})
	}
}

func TestManagedCacheOnlyInvalidatedByGeneratorChange(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	copyRoot := home + "/release"
	if err := os.Mkdir(copyRoot, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"packages.conf", "dependencies.conf"} {
		blockWrite(t, copyRoot+"/"+name, blockRead(t, root+"/"+name))
	}
	for _, path := range []string{"config/shared", "config/macos"} {
		if err := copyTree(root+"/"+path, copyRoot+"/"+path); err != nil {
			t.Fatal(err)
		}
	}
	blockOK(t, copyRoot, "install", "--skip-packages", "--yes")
	cache := []string{"zoxide-init.zsh", "fzf-init.zsh", "starship-init.zsh"}
	for _, name := range cache {
		blockWrite(t, paths.Cache+"/"+name, []byte("cached "+name))
	}
	check := func(present bool) {
		for _, name := range cache {
			path := paths.Cache + "/" + name
			if present {
				blockEqual(t, path, []byte("cached "+name))
			} else if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatalf("stale %s: %v", name, err)
			}
		}
	}
	blockOK(t, copyRoot, "install", "--skip-packages", "--yes")
	check(true)
	f, err := os.OpenFile(copyRoot+"/config/shared/vimrc", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\nset noshowmode\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	blockOK(t, copyRoot, "install", "--skip-packages", "--yes")
	check(true)
	f, err = os.OpenFile(copyRoot+"/config/shared/zsh/interactive.zsh", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\n# changed generator\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	blockOK(t, copyRoot, "install", "--skip-packages", "--yes", "--dry-run")
	check(true)
	blockOK(t, copyRoot, "install", "--skip-packages", "--yes")
	check(false)
}

func copyTree(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		to := filepath.Join(target, rel)
		if info.IsDir() {
			return os.MkdirAll(to, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, to)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(to, data, info.Mode().Perm())
	})
}

func TestManagedUninstallGhosttyOrderAtRemovalFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permission failure requires an unprivileged process")
	}
	root, _, paths := blockHome(t, "macos")
	blockWrite(t, paths.State+"/ghostty", []byte("1\n"))
	blockOK(t, root, "install", "--skip-packages", "--yes")
	parent := paths.Config + "/ghostty"
	if err := os.Chmod(parent, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0700) })
	code, out, _ := blockRun(t, root, "", "uninstall", "--yes")
	if code != 1 || strings.Contains(out, "Selfishell configuration uninstalled.") {
		t.Fatalf("uninstall: %d %q", code, out)
	}
	if _, err := os.Lstat(paths.Resources + "/user-ghostty.state"); !os.IsNotExist(err) {
		t.Fatalf("user block state remains: %v", err)
	}
	if bytes.Contains(blockRead(t, filepath.Dir(paths.Config)+"/ghostty/config.ghostty"), []byte("# >>> Selfishell ghostty >>>")) {
		t.Fatal("user block remains")
	}
	if _, err := os.Lstat(paths.Resources + "/ghostty-config.state"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Config + "/ghostty/config.ghostty"); err != nil {
		t.Fatal(err)
	}
	blockEqual(t, paths.State+"/configured", []byte("1\n"))
}

func TestManagedConflictSkipOverwriteAndOriginalRestore(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	r := failureResource(t, root, "vimrc")
	original := []byte("original before install\n")
	blockWrite(t, r.Target, original)
	blockOK(t, root, "install", "--skip-packages", "--yes")
	statePath := paths.Resources + "/vimrc.state"
	state := blockState(t, paths, r.Name)
	if state.Backup == "-" {
		t.Fatal("missing original backup")
	}
	blockEqual(t, state.Backup, original)
	modified := []byte("user edited managed vimrc\n")
	blockWrite(t, r.Target, modified)
	beforeState := blockRead(t, statePath)
	code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(stderr, "Managed file was modified; preserving it") {
		t.Fatalf("yes: %d %q", code, stderr)
	}
	blockEqual(t, r.Target, modified)
	blockEqual(t, statePath, beforeState)
	if _, err := os.Lstat(paths.State + "/backups"); !os.IsNotExist(err) {
		t.Fatalf("conflict backup made on refusal: %v", err)
	}
	// Interactive skip preserves bytes and state, and a later resource proceeds.
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	if err := os.Remove(paths.Config + "/starship.toml"); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := blockRun(t, root, "y\nn\n", "install", "--skip-packages")
	if code != 0 || !strings.Contains(out, "Skipped modified managed file: "+r.Target) {
		t.Fatalf("skip: %d %q %q", code, out, stderr)
	}
	blockEqual(t, r.Target, modified)
	blockEqual(t, statePath, beforeState)
	blockEqual(t, paths.Config+"/starship.toml", blockRead(t, root+"/config/shared/starship.toml"))
	if _, err := os.Lstat(paths.State + "/backups"); !os.IsNotExist(err) {
		t.Fatalf("skip backup: %v", err)
	}
	code, out, stderr = blockRun(t, root, "y\ny\n", "install", "--skip-packages")
	if code != 0 || !strings.Contains(out, "Updated managed file: "+r.Target) || strings.Contains(out, "Installed managed file: "+r.Target) {
		t.Fatalf("overwrite: %d %q %q", code, out, stderr)
	}
	blockEqual(t, r.Target, blockRead(t, r.Source))
	if got := blockState(t, paths, r.Name).Backup; got != state.Backup {
		t.Fatalf("original backup changed: %q != %q", got, state.Backup)
	}
	entries, err := os.ReadDir(paths.State + "/backups")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("conflict backups: %v", entries)
	}
	blockEqual(t, paths.State+"/backups/"+entries[0].Name(), modified)
	blockOK(t, root, "uninstall", "--restore", "--yes")
	blockEqual(t, r.Target, original)
	_ = home
}

func TestManagedReplacedLinkPreflightPreventsEarlierFileRepair(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "user-starship")
	statePath := paths.Resources + "/user-starship.state"
	beforeState := blockRead(t, statePath)
	if err := os.Remove(r.Target); err != nil {
		t.Fatal(err)
	}
	blockWrite(t, r.Target, []byte("user replaced link\n"))
	earlier := paths.Config + "/zsh/runtime.zsh"
	if err := os.Remove(earlier); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(stderr, "Managed link was replaced; preserving it") {
		t.Fatalf("%d %q", code, stderr)
	}
	blockEqual(t, r.Target, []byte("user replaced link\n"))
	blockEqual(t, statePath, beforeState)
	if _, err := os.Lstat(earlier); !os.IsNotExist(err) {
		t.Fatalf("earlier repaired: %v", err)
	}
	_ = home
}

func TestManagedLinkCreateFailureRollbackAndRetry(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	r := failureResource(t, root, "user-starship")
	original := []byte("preexisting user starship config\n")
	blockWrite(t, r.Target, original)
	var out bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out}, paths: paths, createLink: func(string, string) error { return errors.New("injected link failure") }}
	if err := m.installLink(r, false); err == nil {
		t.Fatal("link failure reported success")
	}
	blockEqual(t, r.Target, original)
	if _, err := os.Lstat(paths.Resources + "/user-starship.state"); !os.IsNotExist(err) {
		t.Fatalf("rolled back state remains: %v", err)
	}
	if strings.Contains(out.String(), "Linked:") {
		t.Fatal("false success")
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	if dest, err := os.Readlink(r.Target); err != nil || dest != r.Source {
		t.Fatalf("retry link: %q %v", dest, err)
	}
	_ = home
}

func TestManagedMissingActiveLinkFailureStaysPendingAndRetries(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "user-starship")
	if err := os.Remove(r.Target); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out}, paths: paths, createLink: func(string, string) error { return errors.New("injected link failure") }}
	if err := m.installLink(r, false); err == nil {
		t.Fatal("link failure reported success")
	}
	if _, err := os.Lstat(r.Target); !os.IsNotExist(err) {
		t.Fatalf("partial target: %v", err)
	}
	if blockState(t, paths, r.Name).Status != "pending" {
		t.Fatal("failure claimed active")
	}
	if strings.Contains(out.String(), "Linked:") {
		t.Fatal("false success")
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	if dest, err := os.Readlink(r.Target); err != nil || dest != r.Source {
		t.Fatalf("retry link: %q %v", dest, err)
	}
	if blockState(t, paths, r.Name).Status != "active" {
		t.Fatal("retry left pending")
	}
}

func TestManagedSourceCheckoutCanDisappearAfterInstall(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	copyRoot := home + "/release"
	if err := os.Mkdir(copyRoot, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"packages.conf", "dependencies.conf"} {
		blockWrite(t, copyRoot+"/"+name, blockRead(t, root+"/"+name))
	}
	for _, dir := range []string{"config/shared", "config/macos"} {
		if err := copyTree(root+"/"+dir, copyRoot+"/"+dir); err != nil {
			t.Fatal(err)
		}
	}
	blockOK(t, copyRoot, "install", "--skip-packages", "--yes")
	if err := os.RemoveAll(copyRoot); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zsh/common.zsh", "zsh/update-notice.zsh", "vim/vimrc"} {
		if _, err := os.Stat(paths.Config + "/" + name); err != nil {
			t.Fatal(err)
		}
	}
	loader := home + "/.zshrc"
	if info, err := os.Lstat(loader); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("loader: %v %v", info, err)
	}
	if _, err := os.Stat("/bin/zsh"); err == nil {
		blockWrite(t, home+"/.zprofile", []byte("set -e\nsource \"$HOME/.zshrc\"\nSELFISHELL_MISE_SHIMS_TEST=loaded\n"))
		if output, err := runNativeZprofile(home, "/usr/bin:/bin", 5*time.Second); err != nil || strings.TrimSpace(string(output)) != "loaded" {
			t.Fatalf("source after removal: %v %q", err, output)
		}
	}
}

func TestManagedRuntimeRespectsCallerMiseGlobalConfig(t *testing.T) {
	root, home, _ := blockHome(t, "macos")
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("zsh unavailable")
	}
	fake := home + "/tools/mise"
	blockWrite(t, fake, []byte("#!/bin/sh\nif [ \"$1\" = activate ]; then printf ':\\n'; fi\n"))
	if err := os.Chmod(fake, 0700); err != nil {
		t.Fatal(err)
	}
	runtime := root + "/config/shared/zsh/runtime.zsh"
	for _, tc := range []struct{ name, setup, check, want string }{
		{"set", `MISE_GLOBAL_CONFIG_FILE="$HOME/personal-mise.toml"`, `SELFISHELL_MISE_SHIMS_TEST="$MISE_GLOBAL_CONFIG_FILE"`, home + "/personal-mise.toml"},
		{"unset", `unset MISE_GLOBAL_CONFIG_FILE`, `[[ -z "${MISE_GLOBAL_CONFIG_FILE+x}" ]] && SELFISHELL_MISE_SHIMS_TEST=unset`, "unset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := "set -e\n" + tc.setup + "\n_selfishell_command_path() { command -v \"$1\"; }\nsource \"" + runtime + "\"\n" + tc.check + "\n"
			blockWrite(t, home+"/.zprofile", []byte(profile))
			out, err := runNativeZprofile(home, home+"/tools:/usr/bin:/bin", 5*time.Second)
			if err != nil || strings.TrimSpace(string(out)) != tc.want {
				t.Fatalf("%s: %v %q", tc.name, err, out)
			}
		})
	}
}

func TestManagedDryRunConflictPreservesBytesAndPreviewsDecision(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "vimrc")
	statePath := paths.Resources + "/vimrc.state"
	modified := []byte("user_modified_data\n")
	blockWrite(t, r.Target, modified)
	state := blockRead(t, statePath)
	code, out, stderr := blockRun(t, root, "", "update", "--tools-only", "--skip-packages", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(out, "Conflict: modified managed file: "+r.Target) || !strings.Contains(out, "Would require an overwrite or skip decision.") {
		t.Fatalf("%d %q %q", code, out, stderr)
	}
	blockEqual(t, r.Target, modified)
	blockEqual(t, statePath, state)
	if _, err := os.Lstat(paths.State + "/backups"); !os.IsNotExist(err) {
		t.Fatalf("backup created: %v", err)
	}
}

func TestManagedAtomicFileFailureKeepsOriginalAndPendingRetry(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	r := failureResource(t, root, "vimrc")
	original := []byte("original existing file\n")
	blockWrite(t, r.Target, original)
	var out bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out}, paths: paths, atomicWrite: func(string, []byte, os.FileMode) error { return errors.New("injected copy/chmod/write failure") }}
	if err := m.installFile(r, false); err == nil {
		t.Fatal("write failure reported success")
	}
	if strings.Contains(out.String(), "Installed managed file") {
		t.Fatal("false success")
	}
	if blockState(t, paths, r.Name).Status != "pending" {
		t.Fatal("failed write marked active")
	}
	state := blockState(t, paths, r.Name)
	blockEqual(t, state.Backup, original)
	if _, err := os.Lstat(r.Target); !os.IsNotExist(err) {
		t.Fatalf("partial target: %v", err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockEqual(t, r.Target, blockRead(t, r.Source))
	if got := blockState(t, paths, r.Name).Backup; got != state.Backup {
		t.Fatalf("backup identity changed: %q", got)
	}
	_ = home
}

func TestManagedUpdateWriteFailureRetriesWithFreshProcess(t *testing.T) {
	for _, backup := range []bool{false, true} {
		for _, outcome := range []string{"write-failed", "write-completed", "user-edited", "overwrite-failed"} {
			name := fmt.Sprintf("backup=%t/%s", backup, outcome)
			t.Run(name, func(t *testing.T) {
				changed := outcome == "user-edited"
				root, _, paths := blockHome(t, "macos")
				r := failureResource(t, root, "vimrc")
				original := []byte("personal original\n")
				if backup {
					blockWrite(t, r.Target, original)
				}
				blockOK(t, root, "install", "--skip-packages", "--yes")
				before := blockRead(t, r.Target)
				saved := blockState(t, paths, r.Name).Backup
				// Use a separate source; testRelease may be shared by other tests.
				r.Source = filepath.Join(t.TempDir(), "new-vimrc")
				updated := append(bytes.Clone(before), []byte("\" new release\n")...)
				blockWrite(t, r.Source, updated)
				if outcome == "overwrite-failed" {
					before = []byte("personal edit before overwrite\n")
					blockWrite(t, r.Target, before)
				}
				var out bytes.Buffer
				m := managed{c: CLI{Root: root, Out: &out}, paths: paths, yes: true, actions: map[string]string{r.Name: "overwrite"},
					atomicWrite: func(path string, data []byte, mode os.FileMode) error {
						if outcome == "write-completed" {
							if err := writeAtomic(path, data, mode); err != nil {
								return err
							}
						}
						return errors.New("injected interruption before active state commit")
					}}
				if err := m.installFile(r, false); err == nil {
					t.Fatal("expected update failure")
				}
				want := before
				if outcome == "write-completed" {
					want = updated
				}
				blockEqual(t, r.Target, want)
				if outcome == "overwrite-failed" {
					entries, err := os.ReadDir(paths.State + "/backups")
					if err != nil || len(entries) != 1 {
						t.Fatalf("conflict backup: %v %v", entries, err)
					}
					blockEqual(t, paths.State+"/backups/"+entries[0].Name(), before)
				}
				if changed {
					blockWrite(t, r.Target, []byte("personal edit after failure\n"))
				}
				retry := managed{c: CLI{Root: root, Out: &out}, paths: paths, yes: true}
				for _, preflight := range []bool{true, false} {
					err := retry.installFile(r, preflight)
					if changed {
						if err == nil || !strings.Contains(err.Error(), "modified") {
							t.Fatalf("edit was not protected: %v", err)
						}
						blockEqual(t, r.Target, []byte("personal edit after failure\n"))
					} else if err != nil {
						t.Fatalf("retry preflight=%t: %v", preflight, err)
					}
				}
				if !changed {
					blockEqual(t, r.Target, updated)
					if state := blockState(t, paths, r.Name); state.Status != "active" || state.Backup != saved {
						t.Fatalf("retry state: %+v", state)
					}
				}
				if backup {
					blockEqual(t, saved, original)
				}
			})
		}
	}
}

func TestRestoreStateRemovalFailureCanRetryUninstall(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, _, paths := blockHome(t, "macos")
			r := failureResource(t, root, "user-nvim")
			if kind == "file" {
				r = failureResource(t, root, "vimrc")
				blockWrite(t, r.Target, []byte("personal original\n"))
			} else if kind == "directory" {
				blockWrite(t, r.Target+"/init.lua", []byte("personal original\n"))
			} else {
				if err := os.MkdirAll(filepath.Dir(r.Target), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("personal-nvim", r.Target); err != nil {
					t.Fatal(err)
				}
			}
			blockOK(t, root, "install", "--skip-packages", "--yes")
			state := blockState(t, paths, r.Name)
			var out bytes.Buffer
			m := managed{c: CLI{Root: root, Out: &out}, paths: paths,
				removeState: func(string) error { return errors.New("injected state removal failure") }}
			if err := m.removeResource(ResourceState{Resource: r, State: state}, true); err == nil {
				t.Fatal("expected state removal failure")
			}
			before, err := os.Lstat(r.Target)
			if err != nil {
				t.Fatal(err)
			}
			blockOK(t, root, "uninstall", "--restore", "--dry-run")
			blockOK(t, root, "uninstall", "--restore", "--yes")
			after, err := os.Lstat(r.Target)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("restored user path changed: %v", err)
			}
			if _, err := os.Lstat(paths.Resources + "/" + r.Name + ".state"); !os.IsNotExist(err) {
				t.Fatalf("state remains: %v", err)
			}
			blockOK(t, root, "install", "--skip-packages", "--yes")
			blockOK(t, root, "uninstall", "--restore", "--yes")
		})
	}
}

func TestInterruptedRestorePreflightAndRetry(t *testing.T) {
	for _, situation := range []string{"before-move", "occupied", "both-missing", "after-move-user-edit"} {
		t.Run(situation, func(t *testing.T) {
			root, _, paths := blockHome(t, "macos")
			r := failureResource(t, root, "user-starship")
			original := []byte("personal config\n")
			blockWrite(t, r.Target, original)
			blockOK(t, root, "install", "--skip-packages", "--yes")
			state := blockState(t, paths, r.Name)
			if err := os.Remove(r.Target); err != nil {
				t.Fatal(err)
			}
			state.Status = "restoring"
			statePath := paths.Resources + "/" + r.Name + ".state"
			if err := WriteState(statePath, state); err != nil {
				t.Fatal(err)
			}
			switch situation {
			case "occupied":
				blockWrite(t, r.Target, []byte("late occupant\n"))
			case "both-missing":
				if err := os.Remove(state.Backup); err != nil {
					t.Fatal(err)
				}
			case "after-move-user-edit":
				if err := moveBackupNoReplace(state.Backup, r.Target); err != nil {
					t.Fatal(err)
				}
				blockWrite(t, r.Target, []byte("personal edit after restore\n"))
			}
			before := blockRead(t, statePath)
			// Install must not adopt or back up a restore that has not finished.
			code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes")
			if code != 1 || !strings.Contains(stderr, "selfishell uninstall --restore") {
				t.Fatalf("install did not explain recovery: %d %q", code, stderr)
			}
			blockEqual(t, statePath, before)
			if situation == "occupied" || situation == "both-missing" {
				// A later restore conflict must prevent removal of every resource.
				intact := paths.Config + "/zsh/common.zsh"
				managedBytes := blockRead(t, intact)
				code, _, stderr = blockRun(t, root, "", "uninstall", "--restore", "--yes")
				if code != 1 {
					t.Fatalf("unsafe restore accepted: %q", stderr)
				}
				blockEqual(t, intact, managedBytes)
				blockEqual(t, statePath, before)
				if situation == "occupied" {
					blockEqual(t, r.Target, []byte("late occupant\n"))
					blockEqual(t, state.Backup, original)
				}
				return
			}
			if situation == "before-move" {
				code, _, _ = blockRun(t, root, "", "uninstall", "--yes")
				if code != 1 {
					t.Fatal("pending restore silently abandoned")
				}
			}
			blockOK(t, root, "uninstall", "--restore", "--dry-run")
			blockEqual(t, statePath, before)
			if situation == "before-move" {
				blockEqual(t, state.Backup, original)
				if _, err := os.Lstat(r.Target); !os.IsNotExist(err) {
					t.Fatalf("dry-run restored target: %v", err)
				}
			}
			blockOK(t, root, "uninstall", "--restore", "--yes")
			want := original
			if situation == "after-move-user-edit" {
				want = []byte("personal edit after restore\n")
			}
			blockEqual(t, r.Target, want)
			if _, err := os.Lstat(statePath); !os.IsNotExist(err) {
				t.Fatalf("restore record remains: %v", err)
			}
		})
	}
}

func TestUninstallReportsRetainedOriginalBackup(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	r := failureResource(t, root, "user-starship")
	original := []byte("personal starship config\n")
	blockWrite(t, r.Target, original)
	blockOK(t, root, "install", "--skip-packages", "--yes")
	backup := blockState(t, paths, r.Name).Backup
	out := blockOK(t, root, "uninstall", "--yes")
	if !strings.Contains(out, backup) {
		t.Fatalf("backup location not reported: %s", out)
	}
	blockEqual(t, backup, original)
}

func TestManagedBlockWriteFailureKeepsUserBytesPendingAndRetries(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "absent-target"
		if existing {
			name = "existing-target"
		}
		t.Run(name, func(t *testing.T) {
			root, home, paths := blockHome(t, "macos")
			r := failureResource(t, root, "user-zshrc")
			original := []byte("original zshrc\n")
			if existing {
				blockWrite(t, r.Target, original)
			}
			var out bytes.Buffer
			m := managed{c: CLI{Root: root, Out: &out}, paths: paths, atomicWrite: func(string, []byte, os.FileMode) error { return errors.New("injected atomic write failure") }}
			if err := m.installBlock(r, false); err == nil {
				t.Fatal("block write failure reported success")
			}
			if existing {
				blockEqual(t, r.Target, original)
			} else if _, err := os.Lstat(r.Target); !os.IsNotExist(err) {
				t.Fatalf("partial target: %v", err)
			}
			if blockState(t, paths, r.Name).Status != "pending" {
				t.Fatal("failed block marked active")
			}
			if strings.Contains(out.String(), "Added Selfishell block") {
				t.Fatal("false success")
			}
			blockOK(t, root, "install", "--skip-packages", "--yes")
			if !bytes.Contains(blockRead(t, home+"/.zshrc"), []byte("# >>> Selfishell initialize >>>")) {
				t.Fatal("retry did not add block")
			}
		})
	}
}

func TestManagedBlockRemovalFailureKeepsStateForRetry(t *testing.T) {
	for _, point := range []string{"write", "state-removal"} {
		t.Run(point, func(t *testing.T) {
			root, _, paths := blockHome(t, "macos")
			blockOK(t, root, "install", "--skip-packages", "--yes")
			r := failureResource(t, root, "user-zshrc")
			before := blockRead(t, r.Target)
			injected := errors.New("injected " + point + " failure")
			m := managed{c: CLI{Root: root}, paths: paths}
			if point == "write" {
				m.atomicWrite = func(string, []byte, os.FileMode) error { return injected }
			} else {
				m.removeState = func(string) error { return injected }
			}
			if err := m.removeResource(ResourceState{Resource: r, State: blockState(t, paths, r.Name)}, false); err == nil {
				t.Fatal("removal reported success")
			}
			if point == "write" {
				blockEqual(t, r.Target, before)
			}
			if blockState(t, paths, r.Name).Status != "pending" {
				t.Fatal("failed removal cleared state")
			}
			// Repair through install first, then normal uninstall.
			blockOK(t, root, "install", "--skip-packages", "--yes")
			blockOK(t, root, "uninstall", "--yes")
			if bytes.Contains(blockRead(t, r.Target), []byte("# >>> Selfishell initialize >>>")) {
				t.Fatal("retry left block")
			}
		})
	}
}

func TestManagedFinalMarkerWriteFailureHasNoSuccess(t *testing.T) {
	for _, marker := range []string{"configured", "ghostty"} {
		t.Run(marker, func(t *testing.T) {
			root, _, paths := blockHome(t, "macos")
			blockOK(t, root, "install", "--skip-packages", "--yes")
			other := "ghostty"
			if marker == "ghostty" {
				other = "configured"
			}
			otherBytes := blockRead(t, paths.State+"/"+other)
			if err := os.Remove(paths.State + "/" + marker); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(paths.State+"/"+marker, 0700); err != nil {
				t.Fatal(err)
			}
			code, out, _ := blockRun(t, root, "", "install", "--skip-packages", "--yes")
			if code != 1 || strings.Contains(out, "Selfishell configuration installed.") {
				t.Fatalf("%s failure: %d %q", marker, code, out)
			}
			if info, err := os.Stat(paths.State + "/" + marker); err != nil || !info.IsDir() {
				t.Fatalf("marker changed: %v %v", info, err)
			}
			blockEqual(t, paths.State+"/"+other, otherBytes)
			if matches, err := filepath.Glob(paths.State + "/" + marker + ".tmp.*"); err != nil || len(matches) != 0 {
				t.Fatalf("temp remnants: %v %v", matches, err)
			}
			if err := os.Remove(paths.State + "/" + marker); err != nil {
				t.Fatal(err)
			}
			out = blockOK(t, root, "install", "--skip-packages", "--yes")
			if !strings.Contains(out, "Selfishell configuration installed.") {
				t.Fatalf("retry missing success: %q", out)
			}
			for name, want := range map[string][]byte{"configured": []byte("1\n"), "ghostty": []byte("1\n")} {
				path := paths.State + "/" + name
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("retry marker %s: %v %v", name, info, err)
				}
				blockEqual(t, path, want)
			}
		})
	}
}
