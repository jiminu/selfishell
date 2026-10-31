package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
)

func testCLI(env *Environment, root, home string, args ...string) (int, string, string) {
	isolateHome(env, home)
	env.set("XDG_CONFIG_HOME", "")
	env.set("XDG_STATE_HOME", "")
	env.set("XDG_CACHE_HOME", "")
	env.set("XDG_DATA_HOME", "")
	env.set("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	env.set("SHELL", "/bin/zsh")
	var out, err bytes.Buffer
	code := (CLI{Env: *env, Root: root, In: strings.NewReader(""), Out: &out, Err: &err}).Run(args)
	return code, out.String(), err.String()
}

func TestInvalidPackageNameHasUsageStatusAtPublicConsumers(t *testing.T) {
	t.Parallel()
	var env Environment
	root, home, bin := t.TempDir(), t.TempDir(), t.TempDir()
	env.set("TMPDIR", t.TempDir())
	for _, name := range []string{"apt-get", "apt-cache", "dpkg-query", "sudo", "brew", "curl", "git", "mise", "chsh"} {
		if err := testutil.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called >\"$HOME/package-called\"\nexit 99\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	env.set("PATH", bin)
	env.set("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	env.set("SELFISHELL_TEST_MACHINE_ARCH", "x86_64")
	env.set("SELFISHELL_TEST_OS_RELEASE_FILE", filepath.Join(root, "os-release"))
	if err := testutil.WriteFile(filepath.Join(root, "os-release"), []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"MISE_DATA_DIR", "MISE_CACHE_DIR", "MISE_STATE_DIR", "MISE_CONFIG_DIR"} {
		env.set(name, filepath.Join(home, "mise", name))
	}
	if err := os.MkdirAll(filepath.Join(home, ".local/state/selfishell"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(filepath.Join(home, ".local/state/selfishell/configured"), nil, 0600); err != nil {
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
		if err := testutil.WriteFile(filepath.Join(root, "packages.conf"), []byte(record.line), 0600); err != nil {
			t.Fatal(err)
		}
		isolateHome(&env, home)
		for _, args := range [][]string{{"install", "--yes"}, {"update", "--tools-only", "--skip-packages", "--dry-run"}, {"status"}} {
			var out, stderr bytes.Buffer
			code := (CLI{Env: env, Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run(args)
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
	t.Parallel()
	var env Environment
	for _, retry := range []string{"restore", "without-restore"} {
		t.Run(retry, func(t *testing.T) {
			t.Parallel()
			env := env
			root := testRelease(t)
			home := t.TempDir()
			isolateHome(&env, home)
			paths, err := userPaths(env)
			if err != nil {
				t.Fatal(err)
			}
			target := paths.Config + "/zsh/aliases.zsh"
			original := []byte("personal aliases\n")
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			if err := testutil.WriteFile(target, original, 0600); err != nil {
				t.Fatal(err)
			}
			if code, _, stderr := testCLI(&env, root, home, "install", "--skip-packages", "--yes"); code != 0 {
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
				code, _, stderr := testCLI(&env, root, home, args...)
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
				if code, _, stderr := testCLI(&env, root, home, "uninstall", "--restore", "--yes"); code != 0 {
					t.Fatalf("restore retry: %s", stderr)
				}
				got, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatalf("original not restored: %q %v", got, err)
				}
			} else {
				if code, _, stderr := testCLI(&env, root, home, "uninstall", "--yes"); code != 0 {
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
	t.Parallel()
	var env Environment
	root := testRelease(t)
	base := t.TempDir()
	other := filepath.Join(base, "other")
	os.Mkdir(other, 0700)
	os.Mkdir(filepath.Join(other, "child"), 0700)
	os.Symlink(filepath.Join(other, "child"), filepath.Join(base, "link"))
	home := base + "/link/../home"
	os.Mkdir(filepath.Join(other, "home"), 0700)
	code, _, err := testCLI(&env, root, home, "install", "--skip-packages", "--yes")
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
	t.Parallel()
	var env Environment
	root := testRelease(t)
	home := t.TempDir()
	tools := t.TempDir()
	log := filepath.Join(t.TempDir(), "mise.log")
	mise := filepath.Join(tools, "mise")
	testutil.WriteFile(mise, []byte(`#!/bin/sh
printf "%s\n" "$*" >> "$SELFISHELL_TEST_MISE_LOG"
pwd >> "$SELFISHELL_TEST_MISE_LOG"
`), 0700)
	env.set("PATH", tools+":/usr/bin:/bin")
	env.set("SELFISHELL_TEST_MISE_LOG", log)
	code, _, err := testCLI(&env, root, home, "install", "--skip-packages", "--dry-run")
	if code != 0 {
		t.Fatal(err)
	}
	if _, e := os.Stat(log); !os.IsNotExist(e) {
		t.Fatal("dry-run invoked mise")
	}
	code, _, err = testCLI(&env, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	data, e := os.ReadFile(log)
	if e != nil || !strings.Contains(string(data), "trust "+home+"/.config/mise/conf.d/selfishell.toml") || !strings.Contains(string(data), root+"/config/shared") {
		t.Fatalf("trust %q %v", data, e)
	}
}

func TestPurgeRetainsDependenciesForReinstall(t *testing.T) {
	t.Parallel()
	var env Environment
	for _, backups := range []bool{false, true} {
		name := "without backups"
		if backups {
			name = "with backups"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := env
			home := t.TempDir()
			isolateHome(&env, home)
			paths, err := userPaths(env)
			if err != nil {
				t.Fatal(err)
			}
			share := paths.Data
			release := share + "/releases/1.0"
			writeTestFile(t, release+"/bin/selfishell", "fixture CLI\n", 0700)
			if err := os.Symlink("releases/1.0", share+"/current"); err != nil {
				t.Fatal(err)
			}
			manifest := home + "/dependencies.conf"
			source := home + "/source"
			directDownload(t, manifest, source, "1.0", ".local/bin/tool", false)
			operation := &PackageOperation{Process: Process{inherit: env, Out: new(bytes.Buffer), Err: new(bytes.Buffer)}}
			if err := installTool(operation, paths, manifest); err != nil {
				t.Fatal(err)
			}
			cli := home + "/.local/bin/selfishell"
			if err := os.Symlink(share+"/current/bin/selfishell", cli); err != nil {
				t.Fatal(err)
			}
			target := home + "/.local/bin/tool"
			installed := readTestFile(t, target)
			retained := map[string]string{
				"dependencies/tool":                               "1.0\n",
				"dependencies/jetbrainsmono-regular":              "3.4.1\n",
				"retained-fonts/jetbrainsmono-regular/3.4.0.json": `{"target":"retained-font","checksum":"retained-checksum"}`,
				"pending-fonts/jetbrainsmono-regular":             `{"target":"pending-font","version":"3.4.2","checksum":"pending-checksum"}`,
			}
			if backups {
				retained["backups/edited"] = "user backup\n"
			}
			for path, contents := range retained {
				writeTestFile(t, paths.State+"/"+path, contents, 0600)
			}
			writeTestFile(t, paths.State+"/configured", "1\n", 0600)
			writeTestFile(t, paths.State+"/old-state", "remove this\n", 0600)
			var out, stderr bytes.Buffer
			c := CLI{Env: env, Root: release, In: strings.NewReader(""), Out: &out, Err: &stderr}
			for _, dry := range []bool{true, false} {
				args := []string{"uninstall", "--purge", "--yes"}
				if dry {
					args = append(args, "--dry-run")
				}
				if code := c.Run(args); code != 0 {
					t.Fatalf("purge (dry=%v): %d %s", dry, code, stderr.String())
				}
				if got := readTestFile(t, target); got != installed {
					t.Fatalf("purge changed retained tool: %q", got)
				}
				for path, contents := range retained {
					if got := readTestFile(t, paths.State+"/"+path); got != contents {
						t.Fatalf("purge changed retained ownership or backup %s: %q", path, got)
					}
				}
				if dry {
					if _, err := os.Lstat(cli); err != nil {
						t.Fatal("dry run removed CLI", err)
					}
					if got := readTestFile(t, paths.State+"/configured"); got != "1\n" {
						t.Fatal("dry run changed setup marker")
					}
				}
			}
			for _, path := range []string{cli, share, paths.State + "/configured", paths.State + "/old-state"} {
				assertNoPath(t, path)
			}
			directDownload(t, manifest, source, "2.0", ".local/bin/tool", false)
			operation = &PackageOperation{Process: Process{inherit: env, Out: new(bytes.Buffer), Err: new(bytes.Buffer)}}
			if err := installTool(operation, paths, manifest); err != nil {
				t.Fatal(err)
			}
			if got := readTestFile(t, paths.State+"/dependencies/tool"); got != "2.0\n" {
				t.Fatalf("reinstall did not synchronize retained tool: %q", got)
			}
			if got := readTestFile(t, target); !strings.Contains(got, "echo 2.0") {
				t.Fatalf("reinstall left old executable: %q", got)
			}
			if !strings.Contains(output(operation), "Updated approved dependency: tool 2.0") {
				t.Fatalf("reinstall lost Selfishell ownership: %s", output(operation))
			}
		})
	}
}

func TestPurgePreservesReplacedStateRoot(t *testing.T) {
	t.Parallel()
	var env Environment
	for _, kind := range []string{"symlink", "file"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			env := env
			home := t.TempDir()
			isolateHome(&env, home)
			paths, err := userPaths(env)
			if err != nil {
				t.Fatal(err)
			}
			release := paths.Data + "/releases/1.0"
			writeTestFile(t, release+"/bin/selfishell", "fixture CLI\n", 0700)
			if err := os.Symlink("releases/1.0", paths.Data+"/current"); err != nil {
				t.Fatal(err)
			}
			cli := home + "/.local/bin/selfishell"
			if err := os.MkdirAll(filepath.Dir(cli), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(paths.Data+"/current/bin/selfishell", cli); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, paths.State+"/original", "initial state\n", 0600)
			// The root is replaced before Run, so this covers the preflight checks,
			// not purgeFiles' later recheck for a change during purge.
			if _, err := backupInventory(paths); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(paths.State, paths.State+".held"); err != nil {
				t.Fatal(err)
			}
			personal := home + "/personal-data"
			writeTestFile(t, personal+"/personal.txt", "user data\n", 0600)
			if kind == "symlink" {
				if err := os.Symlink(personal, paths.State); err != nil {
					t.Fatal(err)
				}
			} else {
				writeTestFile(t, paths.State, "user state file\n", 0600)
			}
			var out, stderr bytes.Buffer
			c := CLI{Env: env, Root: release, In: strings.NewReader(""), Out: &out, Err: &stderr}
			for _, args := range [][]string{{"uninstall", "--purge", "--dry-run", "--yes"}, {"uninstall", "--purge", "--yes"}} {
				if code := c.Run(args); code != 1 || !strings.Contains(stderr.String(), "state path is not a directory") {
					t.Fatalf("%v: code=%d errors=%s", args, code, stderr.String())
				}
			}
			if err := purgeFiles(c, paths); err == nil || !strings.Contains(err.Error(), "state path is not a directory") {
				t.Fatalf("apply did not reject replaced state: %v", err)
			}
			if _, err := os.Lstat(cli); err != nil {
				t.Fatal("removed CLI before state preflight", err)
			}
			if got := readTestFile(t, personal+"/personal.txt"); got != "user data\n" {
				t.Fatal("changed symlink destination", got)
			}
			if got := readTestFile(t, paths.State+".held/original"); got != "initial state\n" {
				t.Fatal("changed original state", got)
			}
			if kind == "file" && readTestFile(t, paths.State) != "user state file\n" {
				t.Fatal("changed replaced state file")
			}
		})
	}
}

func TestDependencyManifestOverrideIsValidatedBeforeMutation(t *testing.T) {
	t.Parallel()
	var env Environment
	root := testRelease(t)
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "dependencies.conf")
	testutil.WriteFile(override, []byte("unknown dependency record\n"), 0600)
	env.set("SELFISHELL_DEPENDENCIES_FILE", override)
	code, _, err := testCLI(&env, root, home, "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(err, "invalid manifest record") {
		t.Fatalf("%d %s", code, err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatalf("mutated HOME: %v", entries)
	}
}
func TestPurgeRefusesForeignCLILinkBeforeMutation(t *testing.T) {
	t.Parallel()
	var env Environment
	base := t.TempDir()
	home := filepath.Join(base, "home")
	os.Mkdir(home, 0700)
	share := filepath.Join(base, ".local/share/selfishell")
	release := filepath.Join(share, "releases/v1")
	os.MkdirAll(release+"/bin", 0700)
	testutil.WriteFile(release+"/bin/selfishell", nil, 0700)
	os.Symlink("releases/v1", share+"/current")
	foreign := filepath.Join(base, "foreign/bin/selfishell")
	os.MkdirAll(filepath.Dir(foreign), 0700)
	testutil.WriteFile(foreign, nil, 0700)
	bin := filepath.Join(base, ".local/bin")
	os.MkdirAll(bin, 0700)
	os.Symlink(foreign, bin+"/selfishell")
	isolateHome(&env, home)
	var out, stderr bytes.Buffer
	c := CLI{Env: env, Root: release, In: strings.NewReader(""), Out: &out, Err: &stderr}
	code := c.Run([]string{"uninstall", "--purge", "--yes"})
	if code != 1 || !strings.Contains(stderr.String(), "Refusing to remove non-Selfishell path") {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, e := os.Lstat(bin + "/selfishell"); e != nil {
		t.Fatal("foreign link removed", e)
	}
}

func isolateHome(env *Environment, home string) {
	// WSL integration fixtures opt in explicitly with a fake Windows environment.
	env.set("WSL_DISTRO_NAME", "")
	env.set("HOME", home)
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		env.set(name, "")
	}
}
func TestNewUserMiseFileIsPrivate(t *testing.T) {
	t.Parallel()
	var env Environment
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(&env, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	info, e := os.Stat(home + "/.config/mise/config.toml")
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v %v", info, e)
	}
}
func TestUninstallCleanupPreservesUntrackedUserPath(t *testing.T) {
	t.Parallel()
	var env Environment
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			env := env
			root := testRelease(t)
			home := t.TempDir()
			isolateHome(&env, home)
			paths, _ := userPaths(env)
			os.MkdirAll(filepath.Dir(paths.Config), 0700)
			target := filepath.Join(t.TempDir(), "personal")
			if kind == "file" {
				testutil.WriteFile(paths.Config, []byte("personal"), 0600)
			} else {
				testutil.WriteFile(target, []byte("personal"), 0600)
				os.Symlink(target, paths.Config)
			}
			code, _, err := testCLI(&env, root, home, "uninstall", "--yes")
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
	t.Parallel()
	var env Environment
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(&env, home)
	paths, _ := userPaths(env)
	resources, _ := ResourcesForPlatform(root, "macos", false, env)
	r := resources[0]
	os.MkdirAll(filepath.Dir(r.Target), 0700)
	testutil.WriteFile(r.Target, []byte("managed"), 0600)
	checksum, _ := Checksum(context.Background(), r.Target)
	state := State{"file", "active", r.Target, "-", "-", checksum}
	WriteState(paths.Resources+"/"+r.Name+".state", state)
	var out, stderr bytes.Buffer
	m := managed{c: CLI{Env: env, Root: root, Out: &out, Err: &stderr}, paths: paths}
	record := ResourceState{Resource: r, State: state}
	if e := m.preflightUninstall(record, false); e != nil {
		t.Fatal(e)
	}
	testutil.WriteFile(r.Target, []byte("user edit"), 0600)
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
	t.Parallel()
	var env Environment
	base := t.TempDir()
	home := base + "/home"
	os.Mkdir(home, 0700)
	isolateHome(&env, home)
	paths, _ := userPaths(env)
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
	testutil.WriteFile(link, []byte("personal"), 0600)
	var out, stderr bytes.Buffer
	c := CLI{Env: env, Root: release, Out: &out, Err: &stderr}
	if e := purgeFiles(c, paths); e == nil {
		t.Fatal("removed replacement CLI path")
	}
	data, e := os.ReadFile(link)
	if e != nil || string(data) != "personal" {
		t.Fatalf("user path lost: %q %v", data, e)
	}
}
