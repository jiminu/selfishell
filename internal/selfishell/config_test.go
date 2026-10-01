package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCLI(t *testing.T, root, home string, args ...string) (int, string, string) {
	t.Helper()
	isolateHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	var out, err bytes.Buffer
	code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &err}).Run(args)
	return code, out.String(), err.String()
}

func TestInvalidPackageNameHasUsageStatusAtPublicConsumers(t *testing.T) {
	root, home, bin := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	for _, name := range []string{"apt-get", "apt-cache", "dpkg-query", "sudo", "brew", "curl", "git", "mise", "chsh"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called >\"$HOME/package-called\"\nexit 99\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	t.Setenv("SELFISHELL_TEST_MACHINE_ARCH", "x86_64")
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", filepath.Join(root, "os-release"))
	if err := os.WriteFile(filepath.Join(root, "os-release"), []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"MISE_DATA_DIR", "MISE_CACHE_DIR", "MISE_STATE_DIR", "MISE_CONFIG_DIR"} {
		t.Setenv(name, filepath.Join(home, "mise", name))
	}
	if err := os.MkdirAll(filepath.Join(home, ".local/state/selfishell"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".local/state/selfishell/configured"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, record := range []struct {
		line       string
		status     int
		diagnostic string
	}{
		{"package ubuntu required apt --allow-unauthenticated\n", 2, "Invalid package name: --allow-unauthenticated"},
		{"execute unsafe\n", 1, "Unknown package manifest record: execute"},
	} {
		if err := os.WriteFile(filepath.Join(root, "packages.conf"), []byte(record.line), 0600); err != nil {
			t.Fatal(err)
		}
		isolateHome(t, home)
		for _, args := range [][]string{{"install", "--yes"}, {"update", "--tools-only", "--skip-packages", "--dry-run"}, {"status"}} {
			var out, stderr bytes.Buffer
			code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run(args)
			diagnostic := stderr.String()
			if code != record.status || !strings.Contains(diagnostic, record.diagnostic) {
				t.Fatalf("%q: code=%d diagnostic=%q", args, code, diagnostic)
			}
			if _, err := os.Lstat(filepath.Join(home, "package-called")); !os.IsNotExist(err) {
				t.Fatalf("%q invoked apt-get: %v", args, err)
			}
		}
	}
}
func TestUninstallRestoreRequiresRecordedBackup(t *testing.T) {
	for _, retry := range []string{"restore", "without-restore"} {
		t.Run(retry, func(t *testing.T) {
			root := testRelease(t)
			home := t.TempDir()
			isolateHome(t, home)
			paths, err := UserPaths()
			if err != nil {
				t.Fatal(err)
			}
			target := paths.Config + "/zsh/aliases.zsh"
			original := []byte("personal aliases\n")
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, original, 0600); err != nil {
				t.Fatal(err)
			}
			if code, _, stderr := testCLI(t, root, home, "install", "--skip-packages", "--yes"); code != 0 {
				t.Fatalf("install: %s", stderr)
			}
			statePath := paths.Resources + "/aliases.state"
			state, err := ReadState(statePath)
			if err != nil || state.Backup == "-" {
				t.Fatalf("recorded backup: %+v %v", state, err)
			}
			held := state.Backup + ".held"
			if err := os.Rename(state.Backup, held); err != nil {
				t.Fatal(err)
			}
			managedBytes, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			earlier := paths.Config + "/zsh/zshrc"
			earlierBytes, err := os.ReadFile(earlier)
			if err != nil {
				t.Fatal(err)
			}
			for _, dry := range []bool{true, false} {
				args := []string{"uninstall", "--restore", "--yes"}
				if dry {
					args = append(args, "--dry-run")
				}
				code, _, stderr := testCLI(t, root, home, args...)
				if code != 1 || !strings.Contains(stderr, state.Backup) || !strings.Contains(stderr, "without --restore") {
					t.Fatalf("missing backup accepted or unclear: status=%d stderr=%q", code, stderr)
				}
				for path, want := range map[string][]byte{target: managedBytes, earlier: earlierBytes} {
					got, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(got, want) {
						t.Fatalf("failed preflight changed %s: %q %v", path, got, err)
					}
				}
				if _, err := os.Lstat(statePath); err != nil {
					t.Fatalf("failed preflight lost state: %v", err)
				}
			}
			if retry == "restore" {
				if err := os.Rename(held, state.Backup); err != nil {
					t.Fatal(err)
				}
				if code, _, stderr := testCLI(t, root, home, "uninstall", "--restore", "--yes"); code != 0 {
					t.Fatalf("restore retry: %s", stderr)
				}
				got, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatalf("original not restored: %q %v", got, err)
				}
			} else {
				if code, _, stderr := testCLI(t, root, home, "uninstall", "--yes"); code != 0 {
					t.Fatalf("uninstall without restore: %s", stderr)
				}
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("managed target retained: %v", err)
				}
				got, err := os.ReadFile(held)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatalf("held user backup changed: %q %v", got, err)
				}
			}
			if _, err := os.Lstat(statePath); !os.IsNotExist(err) {
				t.Fatalf("completed uninstall retained state: %v", err)
			}
		})
	}
}
func testRelease(t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	root := filepath.Clean(filepath.Join(cwd, "../.."))
	return root
}

func TestLiteralHomePathThroughSymlinkDotDot(t *testing.T) {
	root := testRelease(t)
	base := t.TempDir()
	other := filepath.Join(base, "other")
	os.Mkdir(other, 0700)
	os.Mkdir(filepath.Join(other, "child"), 0700)
	os.Symlink(filepath.Join(other, "child"), filepath.Join(base, "link"))
	home := base + "/link/../home"
	os.Mkdir(filepath.Join(other, "home"), 0700)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(code, err)
	}
	if _, e := os.Stat(filepath.Join(other, "home", ".zshrc")); e != nil {
		t.Fatal("literal HOME resolved incorrectly", e)
	}
	if _, e := os.Stat(filepath.Join(base, "home", ".zshrc")); !os.IsNotExist(e) {
		t.Fatal("cleaned path was used")
	}
}
func TestMiseTrustAndDryRun(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	tools := t.TempDir()
	log := filepath.Join(t.TempDir(), "mise.log")
	mise := filepath.Join(tools, "mise")
	os.WriteFile(mise, []byte(`#!/bin/sh
printf "%s\n" "$*" >> "$SELFISHELL_TEST_MISE_LOG"
pwd >> "$SELFISHELL_TEST_MISE_LOG"
`), 0700)
	t.Setenv("PATH", tools+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_MISE_LOG", log)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--dry-run")
	if code != 0 {
		t.Fatal(err)
	}
	if _, e := os.Stat(log); !os.IsNotExist(e) {
		t.Fatal("dry-run invoked mise")
	}
	code, _, err = testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	data, e := os.ReadFile(log)
	if e != nil || !strings.Contains(string(data), "trust "+home+"/.config/mise/conf.d/selfishell.toml") || !strings.Contains(string(data), root+"/config/shared") {
		t.Fatalf("trust %q %v", data, e)
	}
}
func TestPurgePreservesForeignSfsAndBackups(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	os.Mkdir(home, 0700)
	share := filepath.Join(base, ".local/share/selfishell")
	release := filepath.Join(share, "releases/v1")
	os.MkdirAll(release+"/bin", 0700)
	os.Symlink("releases/v1", filepath.Join(share, "current"))
	bin := filepath.Join(base, ".local/bin")
	os.MkdirAll(bin, 0700)
	os.Symlink(share+"/current/bin/selfishell", filepath.Join(bin, "selfishell"))
	os.Symlink("foreign", filepath.Join(bin, "sfs"))
	isolateHome(t, home)
	paths, _ := UserPaths()
	os.MkdirAll(paths.State+"/backups", 0700)
	os.WriteFile(paths.State+"/backups/edited", []byte("user"), 0600)
	var out, stderr bytes.Buffer
	c := CLI{Root: release, In: strings.NewReader(""), Out: &out, Err: &stderr}
	code := c.Run([]string{"uninstall", "--purge", "--yes"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if dest, e := os.Readlink(filepath.Join(bin, "sfs")); e != nil || dest != "foreign" {
		t.Fatal("foreign link changed", dest, e)
	}
	if _, e := os.Stat(paths.State + "/backups/edited"); e != nil {
		t.Fatal("backup lost", e)
	}
}
func TestDependencyManifestOverrideIsValidatedBeforeMutation(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "dependencies.conf")
	os.WriteFile(override, []byte("unknown dependency record\n"), 0600)
	t.Setenv("SELFISHELL_DEPENDENCIES_FILE", override)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(err, "invalid manifest record") {
		t.Fatalf("%d %s", code, err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatalf("mutated HOME: %v", entries)
	}
}
func TestPurgeRefusesForeignCLILinkBeforeMutation(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	os.Mkdir(home, 0700)
	share := filepath.Join(base, ".local/share/selfishell")
	release := filepath.Join(share, "releases/v1")
	os.MkdirAll(release+"/bin", 0700)
	os.Symlink("releases/v1", share+"/current")
	bin := filepath.Join(base, ".local/bin")
	os.MkdirAll(bin, 0700)
	os.Symlink("/foreign/current/bin/selfishell", bin+"/selfishell")
	isolateHome(t, home)
	var out, stderr bytes.Buffer
	c := CLI{Root: release, In: strings.NewReader(""), Out: &out, Err: &stderr}
	code := c.Run([]string{"uninstall", "--purge", "--yes"})
	if code != 1 || !strings.Contains(stderr.String(), "Refusing to remove non-Selfishell path") {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, e := os.Lstat(bin + "/selfishell"); e != nil {
		t.Fatal("foreign link removed", e)
	}
}

func isolateHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(name, "")
	}
}
func TestNewUserMiseFileIsPrivate(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	info, e := os.Stat(home + "/.config/mise/config.toml")
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v %v", info, e)
	}
}
func TestUninstallCleanupPreservesUntrackedUserPath(t *testing.T) {
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := testRelease(t)
			home := t.TempDir()
			isolateHome(t, home)
			paths, _ := UserPaths()
			os.MkdirAll(filepath.Dir(paths.Config), 0700)
			target := filepath.Join(t.TempDir(), "personal")
			if kind == "file" {
				os.WriteFile(paths.Config, []byte("personal"), 0600)
			} else {
				os.WriteFile(target, []byte("personal"), 0600)
				os.Symlink(target, paths.Config)
			}
			code, _, err := testCLI(t, root, home, "uninstall", "--yes")
			if code != 0 {
				t.Fatal(code, err)
			}
			data, e := os.ReadFile(paths.Config)
			if dest, _ := os.Readlink(paths.Config); e != nil || string(data) != "personal" || kind == "symlink" && dest != target {
				t.Fatalf("untracked %s removed: %q %q %v", kind, data, dest, e)
			}
		})
	}
}
func TestRemovalRevalidatesAfterWholeSetPreflight(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ResourcesForPlatform(root, "macos", false)
	r := resources[0]
	os.MkdirAll(filepath.Dir(r.Target), 0700)
	os.WriteFile(r.Target, []byte("managed"), 0600)
	checksum, _ := Checksum(context.Background(), r.Target)
	state := State{"file", "active", r.Target, "-", "-", checksum}
	WriteState(paths.Resources+"/"+r.Name+".state", state)
	var out, stderr bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out, Err: &stderr}, paths: paths}
	record := ResourceState{Resource: r, State: state}
	if e := m.preflightUninstall(record, false); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(r.Target, []byte("user edit"), 0600)
	if e := m.removeResource(record, false); e == nil {
		t.Fatal("removed edited file")
	}
	data, e := os.ReadFile(r.Target)
	if e != nil || string(data) != "user edit" {
		t.Fatalf("user data lost %q %v", data, e)
	}
	if _, e := ReadState(paths.Resources + "/" + r.Name + ".state"); e != nil {
		t.Fatal("retry state lost", e)
	}
}
func TestPurgeRechecksCLILinkBeforeRemoval(t *testing.T) {
	base := t.TempDir()
	home := base + "/home"
	os.Mkdir(home, 0700)
	isolateHome(t, home)
	paths, _ := UserPaths()
	share := base + "/.local/share/selfishell"
	release := share + "/releases/v1"
	os.MkdirAll(release+"/bin", 0700)
	os.Symlink("releases/v1", share+"/current")
	bin := base + "/.local/bin"
	os.MkdirAll(bin, 0700)
	link := bin + "/selfishell"
	os.Symlink(share+"/current/bin/selfishell", link)
	if e := preflightPurge(release); e != nil {
		t.Fatal(e)
	}
	os.Remove(link)
	os.WriteFile(link, []byte("personal"), 0600)
	var out, stderr bytes.Buffer
	c := CLI{Root: release, Out: &out, Err: &stderr}
	if e := purgeFiles(c, paths); e == nil {
		t.Fatal("removed replacement CLI path")
	}
	data, e := os.ReadFile(link)
	if e != nil || string(data) != "personal" {
		t.Fatalf("user path lost: %q %v", data, e)
	}
}
