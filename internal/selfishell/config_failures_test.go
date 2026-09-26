package selfishell

import (
	"bytes"
	"errors"
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

func TestManagedMalformedStateStopsInstallWithoutMutation(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "vimrc")
	state := paths.Resources + "/vimrc.state"
	original := blockRead(t, r.Target)
	valid := "2\nfile\nactive\n" + r.Target + "\n-\n-\n123:4\n"
	cases := map[string]string{
		"truncated-one": "2\n", "truncated-six": "2\nfile\nactive\n" + r.Target + "\n-\n-\n",
		"empty-kind":       strings.Replace(valid, "file\n", "\n", 1),
		"unknown-kind":     strings.Replace(valid, "file\n", "bogus\n", 1),
		"unknown-status":   strings.Replace(valid, "active\n", "bogus\n", 1),
		"empty-target":     strings.Replace(valid, r.Target, "", 1),
		"version":          strings.Replace(valid, "2\n", "3\n", 1),
		"no-final-newline": strings.TrimSuffix(valid, "\n"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			blockWrite(t, state, []byte(data))
			code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes")
			if code != 1 || !strings.Contains(stderr, state) {
				t.Fatalf("%d %q", code, stderr)
			}
			blockEqual(t, r.Target, original)
			if _, err := os.Lstat(paths.State + "/backups"); !os.IsNotExist(err) {
				t.Fatalf("backup created: %v", err)
			}
		})
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	referent := filepath.Join(home, "state-referent")
	blockWrite(t, referent, []byte(valid))
	if err := os.Symlink(referent, state); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(stderr, state) {
		t.Fatalf("symlink state: %d %q", code, stderr)
	}
	blockEqual(t, r.Target, original)
	blockEqual(t, referent, []byte(valid))
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

func TestManagedFreshLoaderRejectsUntrackedDuplicateAndDirectory(t *testing.T) {
	for _, kind := range []string{"untracked", "duplicate", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root, home, paths := blockHome(t, "macos")
			target := home + "/.zshrc"
			content, err := blockContent("user-zshrc", paths.Config)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "untracked":
				blockWrite(t, target, content)
			case "duplicate":
				blockWrite(t, target, append(append([]byte{}, content...), content...))
			case "directory":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			}
			code, _, _ := blockRun(t, root, "", "install", "--skip-packages", "--yes")
			if code != 1 {
				t.Fatalf("%s accepted: %d", kind, code)
			}
			if kind == "directory" {
				if info, err := os.Stat(target); err != nil || !info.IsDir() {
					t.Fatalf("directory changed: %v %v", info, err)
				}
			} else if kind == "duplicate" {
				blockEqual(t, target, append(append([]byte{}, content...), content...))
			} else {
				blockEqual(t, target, content)
			}
			for _, path := range []string{paths.Config, paths.State} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("created %s: %v", path, err)
				}
			}
		})
	}
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
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	state := blockState(t, paths, "user-vimrc")
	state.Status = "pending"
	if err := WriteState(paths.Resources+"/user-vimrc.state", state); err != nil {
		t.Fatal(err)
	}
	userVimrc := []byte("user vimrc\n")
	blockWrite(t, home+"/.vimrc", userVimrc)
	earlier := blockRead(t, paths.Config+"/zsh/zshrc")
	loader := blockRead(t, home+"/.zshrc")
	code, _, stderr := blockRun(t, root, "", "uninstall", "--yes")
	if code != 1 || !strings.Contains(stderr, "An interrupted install left this unfinished; run 'selfishell install', then uninstall again.") {
		t.Fatalf("%d %q", code, stderr)
	}
	blockEqual(t, paths.Config+"/zsh/zshrc", earlier)
	blockEqual(t, home+"/.vimrc", userVimrc)
	blockEqual(t, home+"/.zshrc", loader)
	if blockState(t, paths, "user-vimrc").Status != "pending" {
		t.Fatal("pending state changed")
	}
	blockEqual(t, paths.State+"/configured", []byte("1\n"))
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

func TestManagedActiveFileUpdateAndMissingTargetReportDistinctVerbs(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed-source", true: "missing-target"}[missing], func(t *testing.T) {
			root, _, paths := blockHome(t, "macos")
			r := failureResource(t, root, "zsh-common")
			if !missing {
				blockWrite(t, r.Target, []byte("old managed bytes"))
			}
			checksum := "123:4"
			if !missing {
				var err error
				checksum, err = checksumBytes([]byte("old managed bytes"))
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteState(paths.Resources+"/zsh-common.state", State{"file", "active", r.Target, "-", "-", checksum}); err != nil {
				t.Fatal(err)
			}
			out := blockOK(t, root, "install", "--skip-packages", "--yes")
			want, reject := "Updated managed file: ", "Installed managed file: "
			if missing {
				want, reject = reject, want
			}
			if !strings.Contains(out, want+r.Target) || strings.Contains(out, reject+r.Target) {
				t.Fatal(out)
			}
			blockEqual(t, r.Target, blockRead(t, r.Source))
		})
	}
}

func TestManagedMiseGlobalExistingTypesAndDirectoryPreflight(t *testing.T) {
	for _, kind := range []string{"file", "file-link", "directory-link", "dangling-link", "directory"} {
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
			case "file-link":
				blockWrite(t, other, []byte("user target\n"))
				if err := os.Symlink(other, target); err != nil {
					t.Fatal(err)
				}
			case "directory-link":
				if err := os.Mkdir(other, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, target); err != nil {
					t.Fatal(err)
				}
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
			if before.Mode()&os.ModeSymlink != 0 {
				a, _ := os.Readlink(target)
				if a != other {
					t.Fatalf("link changed: %q", a)
				}
			}
			if kind == "file" {
				blockEqual(t, target, []byte("user file\n"))
			}
			if kind == "file-link" {
				blockEqual(t, other, []byte("user target\n"))
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

func TestManagedBlockSpliceRetainsBinarySurroundingsAndMode(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	_ = root
	content, err := blockContent("user-vimrc", paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	prefix := bytes.Repeat([]byte("personal config\r\n"), 8192)
	prefix = append(prefix, []byte("앞\x00뒤\n")...)
	suffix := append(append([]byte{}, prefix...), []byte("no final newline")...)
	old := []byte("old block\n")
	target := home + "/block-target"
	blockWrite(t, target, append(append(append([]byte{}, prefix...), old...), suffix...))
	if err := os.Chmod(target, 0640); err != nil {
		t.Fatal(err)
	}
	view := blockView{start: len(prefix), end: len(prefix) + len(old)}
	if err := writeAtomic(target, spliceBlock(blockRead(t, target), view, content), 0640); err != nil {
		t.Fatal(err)
	}
	blockEqual(t, target, append(append(append([]byte{}, prefix...), content...), suffix...))
	view.end = len(prefix) + len(content)
	if err := writeAtomic(target, spliceBlock(blockRead(t, target), view, nil), 0640); err != nil {
		t.Fatal(err)
	}
	blockEqual(t, target, append(append([]byte{}, prefix...), suffix...))
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0640 {
		t.Fatalf("mode: %v %v", info, err)
	}
}

func TestManagedBlockContentFailureLeavesUserBytes(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	target := home + "/.zshrc"
	original := []byte("original zshrc\n")
	blockWrite(t, target, original)
	m := managed{c: CLI{Root: root}, paths: paths}
	if err := m.installBlock(Resource{Kind: "block", Name: "invalid-block", Target: target, Source: "-"}, false); err == nil {
		t.Fatal("unknown block accepted")
	}
	blockEqual(t, target, original)
	if _, err := os.Lstat(paths.Resources + "/invalid-block.state"); !os.IsNotExist(err) {
		t.Fatalf("state created: %v", err)
	}
	if matches, err := filepath.Glob(target + ".tmp.*"); err != nil || len(matches) != 0 {
		t.Fatalf("temporary files: %v %v", matches, err)
	}
}

func TestManagedAtomicRenameFailurePreservesOriginalAndCleansTemp(t *testing.T) {
	root, home, _ := blockHome(t, "macos")
	_ = root
	target := home + "/occupied"
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	blockWrite(t, target+"/user-file", []byte("user bytes"))
	if err := writeAtomic(target, []byte("replacement"), 0640); err == nil {
		t.Fatal("rename over directory reported success")
	}
	blockEqual(t, target+"/user-file", []byte("user bytes"))
	if matches, err := filepath.Glob(target + ".tmp.*"); err != nil || len(matches) != 0 {
		t.Fatalf("temporary files: %v %v", matches, err)
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
	if matches, err := filepath.Glob(r.Target + ".tmp.*"); err != nil || len(matches) != 0 {
		t.Fatalf("temp remnants: %v %v", matches, err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockEqual(t, r.Target, blockRead(t, r.Source))
	if got := blockState(t, paths, r.Name).Backup; got != state.Backup {
		t.Fatalf("backup identity changed: %q", got)
	}
	_ = home
}

func TestManagedBlockWriteFailureKeepsUserBytesPendingAndRetries(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	r := failureResource(t, root, "user-zshrc")
	original := []byte("original zshrc\n")
	blockWrite(t, r.Target, original)
	var out bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out}, paths: paths, atomicWrite: func(string, []byte, os.FileMode) error { return errors.New("injected atomic write failure") }}
	if err := m.installBlock(r, false); err == nil {
		t.Fatal("block write failure reported success")
	}
	blockEqual(t, r.Target, original)
	if blockState(t, paths, r.Name).Status != "pending" {
		t.Fatal("failed block marked active")
	}
	if strings.Contains(out.String(), "Added Selfishell block") {
		t.Fatal("false success")
	}
	if matches, err := filepath.Glob(r.Target + ".tmp.*"); err != nil || len(matches) != 0 {
		t.Fatalf("temp remnants: %v %v", matches, err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	if !bytes.Contains(blockRead(t, home+"/.zshrc"), []byte("# >>> Selfishell initialize >>>")) {
		t.Fatal("retry did not add block")
	}
}

func TestManagedBlockRemovalWriteFailureKeepsBlockAndStateForRetry(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "user-zshrc")
	before := blockRead(t, r.Target)
	state := blockState(t, paths, r.Name)
	m := managed{c: CLI{Root: root}, paths: paths, atomicWrite: func(string, []byte, os.FileMode) error { return errors.New("injected block removal write failure") }}
	if err := m.removeResource(ResourceState{Resource: r, State: state}, false); err == nil {
		t.Fatal("removal reported success")
	}
	blockEqual(t, r.Target, before)
	if blockState(t, paths, r.Name).Status != "pending" {
		t.Fatal("failed removal cleared state")
	}
	if matches, err := filepath.Glob(r.Target + ".tmp.*"); err != nil || len(matches) != 0 {
		t.Fatalf("temp remnants: %v %v", matches, err)
	}
	// Repair through install first, then normal uninstall.
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockOK(t, root, "uninstall", "--yes")
	if bytes.Contains(blockRead(t, home+"/.zshrc"), []byte("# >>> Selfishell initialize >>>")) {
		t.Fatal("retry left block")
	}
}

func TestManagedRemoveFailureRetainsFileAndResourceRecord(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "zshrc-config")
	before := blockRead(t, r.Target)
	state := blockState(t, paths, r.Name)
	m := managed{c: CLI{Root: root}, paths: paths, removePath: func(string) error { return errors.New("injected removal failure") }}
	if err := m.removeResource(ResourceState{Resource: r, State: state}, false); err == nil {
		t.Fatal("removal reported success")
	}
	blockEqual(t, r.Target, before)
	if _, err := os.Lstat(paths.Resources + "/" + r.Name + ".state"); err != nil {
		t.Fatal(err)
	}
	blockEqual(t, paths.State+"/configured", []byte("1\n"))
}

func TestManagedOverwriteWriteFailureKeepsConflictBackupAndRetries(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "vimrc")
	modified := []byte("user_modified_vimrc\n")
	blockWrite(t, r.Target, modified)
	var out bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out}, paths: paths, actions: map[string]string{r.Name: "overwrite"}, atomicWrite: func(string, []byte, os.FileMode) error { return errors.New("injected copy failure") }}
	if err := m.installFile(r, false); err == nil {
		t.Fatal("failed overwrite reported success")
	}
	blockEqual(t, r.Target, modified)
	if strings.Contains(out.String(), "Updated managed file: "+r.Target) {
		t.Fatal("false success")
	}
	state := blockState(t, paths, r.Name)
	if state.Status != "pending" {
		t.Fatalf("state %v", state)
	}
	entries, err := os.ReadDir(paths.State + "/backups")
	if err != nil || len(entries) != 1 {
		t.Fatalf("backup: %v %v", entries, err)
	}
	blockEqual(t, paths.State+"/backups/"+entries[0].Name(), modified)
	m.atomicWrite = nil
	if err := m.installFile(r, false); err != nil {
		t.Fatalf("retry: %v", err)
	}
	blockEqual(t, r.Target, blockRead(t, r.Source))
	if blockState(t, paths, r.Name).Status != "active" {
		t.Fatal("retry did not activate")
	}
}

func TestManagedGhosttyPreflightRejectsUserSymlinkBeforeConfiguration(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	target := home + "/.config/ghostty/config.ghostty"
	referent := home + "/personal-ghostty"
	blockWrite(t, referent, []byte("font-size = 14\n"))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(referent, target); err != nil {
		t.Fatal(err)
	}
	blockWrite(t, paths.State+"/ghostty", []byte("1\n"))
	code, out, _ := blockRun(t, root, "", "install", "--skip-packages", "--yes")
	if code != 1 || strings.Contains(out, "Skipping package and tool installation") {
		t.Fatalf("preflight: %d %q", code, out)
	}
	if dest, err := os.Readlink(target); err != nil || dest != referent {
		t.Fatalf("link: %q %v", dest, err)
	}
	blockEqual(t, referent, []byte("font-size = 14\n"))
	if _, err := os.Lstat(paths.Config); !os.IsNotExist(err) {
		t.Fatalf("managed config created: %v", err)
	}
	if _, err := os.Lstat(paths.Resources); !os.IsNotExist(err) {
		t.Fatalf("resource state created: %v", err)
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
		})
	}
}

func TestManagedPendingBlockStateRefreshFailureNeverClaimsUnchanged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("state permission failure requires an unprivileged process")
	}
	root, _, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	statePath := paths.Resources + "/user-zshrc.state"
	state := blockState(t, paths, "user-zshrc")
	state.Status = "pending"
	if err := WriteState(statePath, state); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(paths.Resources, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(paths.Resources, 0700) })
	code, out, _ := blockRun(t, root, "", "install", "--skip-packages", "--yes")
	if code != 1 || strings.Contains(out, "items unchanged") || strings.Contains(out, "Selfishell configuration installed.") {
		t.Fatalf("refresh failure: %d %q", code, out)
	}
	if err := os.Chmod(paths.Resources, 0700); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	if blockState(t, paths, "user-zshrc").Status != "active" {
		t.Fatal("retry left pending")
	}
}

func TestManagedBlockRemovalReadFailurePreservesTargetAndState(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read permission failure requires an unprivileged process")
	}
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	r := failureResource(t, root, "user-zshrc")
	before := blockRead(t, r.Target)
	state := blockRead(t, paths.Resources+"/user-zshrc.state")
	if err := os.Chmod(r.Target, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(r.Target, 0644) })
	code, _, _ := blockRun(t, root, "", "uninstall", "--yes")
	if code != 1 {
		t.Fatalf("read failure accepted: %d", code)
	}
	if err := os.Chmod(r.Target, 0644); err != nil {
		t.Fatal(err)
	}
	blockEqual(t, r.Target, before)
	blockEqual(t, paths.Resources+"/user-zshrc.state", state)
	if matches, err := filepath.Glob(r.Target + ".tmp.*"); err != nil || len(matches) != 0 {
		t.Fatalf("temp remnants: %v %v", matches, err)
	}
	_ = home
}
