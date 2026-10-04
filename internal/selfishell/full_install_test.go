package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackagePhaseFailuresLeaveConfigurationUnchanged(t *testing.T) {
	const required = "package ubuntu required apt fixture-unavailable\n"
	const unpinned = "package ubuntu required apt fixture-apt\npackage all required mise absent\n"
	for _, tc := range []struct {
		name, packages, want, calls string
		args                        []string
	}{
		{"install required failure", required, "Could not update apt package indexes", "dpkg-query\napt-get update\n", []string{"install", "--yes"}},
		{"update required failure", required, "Could not update apt package indexes", "dpkg-query\napt-get update\n", []string{"update", "--tools-only"}},
		{"install missing pin", unpinned, "missing approved mise version: absent", "", []string{"install", "--yes"}},
		{"update missing pin", unpinned, "missing approved mise version: absent", "", []string{"update", "--tools-only", "--yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := isolatedUpdateHome(t)
			root, bin := t.TempDir(), t.TempDir()
			for _, name := range []string{"config", "dependencies.conf"} {
				if err := os.Symlink(filepath.Join(testRelease(t), name), filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
			}
			writeTestFile(t, root+"/packages.conf", tc.packages, 0600)
			if tc.args[0] == "update" {
				writeTestFile(t, home+"/.local/state/selfishell/configured", "1\n", 0600)
			}
			for name, body := range map[string]string{
				"dpkg-query": "#!/bin/sh\nprintf 'dpkg-query\\n' >>\"$HOME/package-calls\"\nexit 1\n",
				"apt-get":    "#!/bin/sh\nprintf 'apt-get %s\\n' \"$*\" >>\"$HOME/package-calls\"\nexit 1\n",
				"sudo":       "#!/bin/sh\nexec \"$@\"\n",
				"os-release": "ID=ubuntu\n",
			} {
				writeTestFile(t, bin+"/"+name, body, 0700)
			}
			t.Setenv("PATH", bin+":/usr/bin:/bin")
			t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
			t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", bin+"/os-release")
			t.Setenv("SELFISHELL_TEST_TTY", "")
			code, out, stderr := commandResult(root, tc.args...)
			if code != 1 || strings.Contains(out, "synchronized") || !strings.Contains(stderr, tc.want) {
				t.Fatalf("%d %q %q", code, out, stderr)
			}
			if tc.args[0] == "update" && !strings.Contains(stderr, "retry with: selfishell update --tools-only\n") {
				t.Fatalf("missing tools retry guidance: %q", stderr)
			}
			if calls, _ := os.ReadFile(home + "/package-calls"); string(calls) != tc.calls {
				t.Fatalf("package calls %q, want %q", calls, tc.calls)
			}
			if _, err := os.Lstat(home + "/.zshrc"); !os.IsNotExist(err) {
				t.Fatalf("package failure applied configuration: %v", err)
			}
		})
	}
}

func TestFullInstallPreflightsMiseTargetBeforePackageCommands(t *testing.T) {
	root, home := testRelease(t), t.TempDir()
	bin := t.TempDir()
	for name, body := range map[string]string{
		"apt-get":    "#!/bin/sh\nprintf called >\"$HOME/package-called\"\n",
		"dpkg-query": "#!/bin/sh\nexit 1\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	isolateHome(t, home)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", filepath.Join(bin, "os-release"))
	os.WriteFile(filepath.Join(bin, "os-release"), []byte("ID=ubuntu\n"), 0600)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	if err := os.MkdirAll(home+"/.config/mise/config.toml", 0700); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run([]string{"install", "--yes"})
	if code != 1 || !strings.Contains(stderr.String(), "user mise config path") {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, err := os.Lstat(home + "/package-called"); !os.IsNotExist(err) {
		t.Fatalf("package command ran before preflight: %v", err)
	}
}

func TestFullInstallDryRunAndCancellationDoNotMutate(t *testing.T) {
	root, home := testRelease(t), t.TempDir()
	isolateHome(t, home)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	var out, stderr bytes.Buffer
	cli := CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}
	if code := cli.Run([]string{"install", "--dry-run"}); code != 0 {
		t.Fatalf("dry-run %d: %s", code, stderr.String())
	}
	if !strings.Contains(out.String(), "Would sync required mise tools") || !strings.Contains(out.String(), "Would sync declared Neovim plugins") {
		t.Fatalf("missing package or editor plan: %s", out.String())
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("dry-run mutated HOME: %v %v", entries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cli.Context = ctx
	out.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"install", "--yes"}); code != 1 || !strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf("cancelled install: %d %s", code, stderr.String())
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("cancelled install mutated HOME: %v %v", entries, err)
	}
}

func TestInstallLoginShellListedAndSafe(t *testing.T) {
	root, home, fixture := testRelease(t), t.TempDir(), t.TempDir()
	bin := fixture + "/bin"
	listed := fixture + "/listed/zsh"
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(listed), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"zsh":  "#!/bin/sh\nexit 0\n",
		"id":   "#!/bin/sh\nprintf 'fixture-user\\n'\n",
		"chsh": "#!/bin/sh\n[ -f \"$HOME/.local/state/selfishell/configured\" ] && [ -f \"$HOME/.local/state/selfishell/ghostty\" ] || exit 42\nprintf '%s\\n' \"$*\" >\"$HOME/chsh-args\"\n",
	} {
		if err := os.WriteFile(bin+"/"+name, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(listed, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	shells := fixture + "/shells"
	if err := os.WriteFile(shells, []byte("/bin/sh\n"+listed+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	isolateHome(t, home)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("SELFISHELL_TEST_SHELLS_FILE", shells)
	t.Setenv("SELFISHELL_TEST_TERMINAL", "/dev/null")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	var out, stderr bytes.Buffer
	c := CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}
	if code := c.Run([]string{"install", "--skip-packages", "--yes"}); code != 0 {
		t.Fatalf("install %d: %s", code, stderr.String())
	}
	if data, err := os.ReadFile(home + "/chsh-args"); err != nil || string(data) != "-s "+listed+" fixture-user\n" {
		t.Fatalf("chsh args %q: %v", data, err)
	}
	if err := os.Remove(home + "/chsh-args"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", "/opt/homebrew/bin/zsh")
	if code := c.Run([]string{"install", "--skip-packages", "--yes"}); code != 0 {
		t.Fatalf("repeat %d: %s", code, stderr.String())
	}
	if _, err := os.Lstat(home + "/chsh-args"); !os.IsNotExist(err) {
		t.Fatal("changed existing Zsh shell")
	}
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("SELFISHELL_TEST_TERMINAL", fixture+"/missing-terminal")
	out.Reset()
	if code := c.Run([]string{"install", "--skip-packages", "--yes"}); code != 0 || !strings.Contains(out.String(), "chsh -s "+listed) {
		t.Fatalf("terminal guard: %d %s", code, out.String())
	}
	if _, err := os.Lstat(home + "/chsh-args"); !os.IsNotExist(err) {
		t.Fatal("called chsh without terminal")
	}
	if err := os.WriteFile(shells, []byte("/bin/sh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := c.Run([]string{"install", "--skip-packages", "--yes"}); code != 0 || !strings.Contains(out.String(), "Zsh is not listed") {
		t.Fatalf("unlisted guard: %d %s", code, out.String())
	}
	if _, err := os.Lstat(home + "/chsh-args"); !os.IsNotExist(err) {
		t.Fatal("called chsh with unlisted shell")
	}
	if err := os.WriteFile(shells, []byte(listed+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_TERMINAL", "/dev/null")
	out.Reset()
	c.defaultShell(true, true)
	if !strings.Contains(out.String(), "Would set login shell to: "+listed) {
		t.Fatalf("dry shell: %s", out.String())
	}
	if _, err := os.Lstat(home + "/chsh-args"); !os.IsNotExist(err) {
		t.Fatal("dry-run called chsh")
	}
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	c.In = strings.NewReader("n\n")
	c.defaultShell(false, false)
	if _, err := os.Lstat(home + "/chsh-args"); !os.IsNotExist(err) {
		t.Fatal("denied shell confirmation called chsh")
	}
	os.WriteFile(bin+"/chsh", []byte("#!/bin/sh\nexit 1\n"), 0700)
	c.In = strings.NewReader("y\n")
	out.Reset()
	c.defaultShell(false, false)
	if !strings.Contains(out.String(), "Could not set login shell") {
		t.Fatalf("chsh failure: %s", out.String())
	}
}

func TestFullInstallAppliesConfigurationAfterPackagesAndKeepsGhosttyChoice(t *testing.T) {
	_, _, root, manifest, home, _ := neovimFixture(t)
	original := []byte("alias own='yes'\r\n")
	if err := os.WriteFile(home+"/.zshrc", original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root + "/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(testRelease(t)+"/config", root+"/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(manifest, root+"/dependencies.conf"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/packages.conf", []byte("package ubuntu required apt example\npackage ubuntu optional apt optional-example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := home + "/bin"
	if err := os.WriteFile(bin+"/dpkg-query", []byte("#!/bin/sh\ncase \"$*\" in *optional-example*) exit 1;; esac\nprintf 'install ok installed\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin+"/apt-get", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$HOME/apt-calls\"\ncase \"$1\" in update) exit 0;; *) exit 1;; esac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin+"/sudo", []byte("#!/bin/sh\nexec \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin+"/apt-cache", []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", home+"/os-release")
	if err := os.WriteFile(home+"/os-release", []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", "/bin/zsh")
	var out, stderr bytes.Buffer
	c := CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}
	if code := c.Run([]string{"install", "--yes"}); code != 0 {
		t.Fatalf("full install %d: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Skipped optional packages: optional-example") {
		t.Fatalf("optional skip missing: %s", stderr.String())
	}
	if _, err := os.Stat(home + "/.zshrc"); err != nil {
		t.Fatal("config not installed", err)
	}
	if data, err := os.ReadFile(home + "/.zshrc"); err != nil || !bytes.HasSuffix(data, original) {
		t.Fatalf("existing user loader changed: %q %v", data, err)
	}
	if data, err := os.ReadFile(home + "/state/selfishell/configured"); err != nil || string(data) != "1\n" {
		t.Fatalf("configured marker: %q %v", data, err)
	}
	if data, err := os.ReadFile(home + "/state/selfishell/ghostty"); err != nil || string(data) != "0\n" {
		t.Fatalf("Ghostty marker: %q %v", data, err)
	}
	if data, err := os.ReadFile(home + "/apt-calls"); err != nil || string(data) != "update\n" {
		t.Fatalf("optional unavailable apt attempted install: %q %v", data, err)
	}
	out.Reset()
	stderr.Reset()
	if code := c.Run([]string{"install", "--yes"}); code != 0 {
		t.Fatalf("repeat %d: %s", code, stderr.String())
	}
	if !strings.Contains(out.String(), "items unchanged") {
		t.Fatalf("missing repeat report: %s", out.String())
	}
}

func TestFullInstallSavedGhosttyChoiceControlsCaskPlan(t *testing.T) {
	for _, tc := range []struct {
		name, command, platform, choice string
		explicit, want                  bool
	}{
		{"install-declined", "install", "macos", "0\n", false, false},
		{"install-enabled", "install", "macos", "1\n", false, true},
		{"install-missing", "install", "macos", "", false, true},
		{"install-exact-content", "install", "macos", "1", false, false},
		{"update-declined", "update", "macos", "0\n", false, false},
		{"update-enabled", "update", "macos", "1\n", false, true},
		{"update-missing", "update", "macos", "", false, false},
		{"explicit-declined", "install", "macos", "0\n", true, true},
		{"explicit-directory", "install", "macos", "directory", true, true},
		{"explicit-symlink", "install", "macos", "symlink", true, true},
		{"ubuntu-directory", "install", "ubuntu", "directory", false, false},
		{"ubuntu-symlink", "install", "ubuntu", "symlink", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home, paths := blockHome(t, tc.platform)
			blockWrite(t, paths.State+"/configured", []byte("1\n"))
			choice := paths.State + "/ghostty"
			var err error
			switch tc.choice {
			case "":
			case "directory":
				err = os.Mkdir(choice, 0700)
			case "symlink":
				blockWrite(t, home+"/external-choice", []byte("0\n"))
				err = os.Symlink(home+"/external-choice", choice)
			default:
				blockWrite(t, choice, []byte(tc.choice))
			}
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.Lstat(choice)
			args := []string{tc.command, "--dry-run"}
			if tc.command == "update" {
				args = append(args, "--tools-only")
			}
			if tc.explicit {
				args = append(args, "--ghostty")
			}
			code, out, stderr := blockRun(t, root, "", args...)
			if code != 0 {
				t.Fatalf("choice %q: %d %s", tc.choice, code, stderr)
			}
			if got := strings.Contains(out, "Would install optional Homebrew cask: ghostty"); got != tc.want {
				t.Fatalf("choice %q cask plan %t: %s", tc.choice, got, out)
			}
			if strings.Contains(out, "[Y/n]") {
				t.Fatalf("unexpected choice prompt: %s", out)
			}
			if after, err := os.Lstat(choice); before == nil {
				if !os.IsNotExist(err) {
					t.Fatal("preview saved a new choice", err)
				}
			} else if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("preview replaced the choice", err)
			}
			if tc.choice == "0\n" || tc.choice == "1\n" || tc.choice == "1" {
				blockEqual(t, choice, []byte(tc.choice))
			}
		})
	}
}

func TestFullInstallGhosttyFailureAppearsInOptionalSummary(t *testing.T) {
	_, _, root, manifest, home, _ := neovimFixture(t)
	if err := os.RemoveAll(root + "/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(testRelease(t)+"/config", root+"/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(manifest, root+"/dependencies.conf"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/packages.conf", []byte("package macos optional cask fixture-optional\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home+"/bin/brew", []byte("#!/bin/sh\ncase \"$1\" in install) exit 1;; esac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", home+"/bin:/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	paths, err := UserPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.State, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.State+"/ghostty", []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run([]string{"install", "--yes"})
	if code != 0 || !strings.Contains(stderr.String(), "Skipped optional packages: fixture-optional ghostty\n") {
		t.Fatalf("full install %d, optional summary %q", code, stderr.String())
	}
}

func TestFullInstallUnsupportedPlatformLeavesHomeUntouched(t *testing.T) {
	root, home := testRelease(t), t.TempDir()
	isolateHome(t, home)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	releaseFile := t.TempDir() + "/os-release"
	if err := os.WriteFile(releaseFile, []byte("ID=fedora\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", releaseFile)
	procFile := filepath.Join(filepath.Dir(releaseFile), "proc-version")
	writeTestFile(t, procFile, "Linux fixture\n", 0600)
	t.Setenv("SELFISHELL_TEST_PROC_VERSION_FILE", procFile)
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, Out: &out, Err: &stderr}).Run([]string{"install", "--yes"})
	if code != 1 || !strings.Contains(stderr.String(), "unavailable on unsupported-linux") {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("unsupported platform mutated HOME: %v %v", entries, err)
	}
}
