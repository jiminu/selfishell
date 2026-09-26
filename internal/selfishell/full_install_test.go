package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullInstallRequiredPackageFailureBeforeConfiguration(t *testing.T) {
	root, home := testRelease(t), t.TempDir()
	bin := t.TempDir()
	for name, body := range map[string]string{
		"apt-get":    "#!/bin/sh\nexit 1\n",
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
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run([]string{"install", "--yes"})
	if code != 1 || strings.Contains(stderr.String(), "--skip-packages") {
		t.Fatalf("full install did not reach package phase: code=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("required package failure changed configuration: %v", err)
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
		"chsh": "#!/bin/sh\nprintf '%s\\n' \"$*\" >\"$HOME/chsh-args\"\n",
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
	if err := os.WriteFile(bin+"/apt-get", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$HOME/apt-calls\"\nexit 1\n"), 0700); err != nil {
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
	if _, err := os.Lstat(home + "/apt-calls"); !os.IsNotExist(err) {
		t.Fatalf("optional unavailable apt attempted install: %v", err)
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

func TestOptionalMiseSkipReportsPackageNames(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	isolateHome(t, home)
	if err := os.MkdirAll(root+"/config/shared", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/config/shared/mise.toml", []byte("[tools]\neza = \"0.23.0\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	c := CLI{Root: root, Out: &out, Err: &stderr}
	o := &PackageOperation{Process: Process{Out: &out, Err: &stderr, Env: []string{"HOME=" + home, "PATH=" + t.TempDir()}}}
	paths, _ := UserPaths()
	if err := c.installPackages(o, paths, []Package{{Platform: "all", Requirement: "optional", Manager: "mise", Name: "eza"}}, "macos", "arm64", false); err != nil {
		t.Fatal(err)
	}
	if len(o.SkippedOptional) != 1 || o.SkippedOptional[0] != "eza" || !strings.Contains(stderr.String(), "Skipped optional packages: eza\n") {
		t.Fatalf("skipped %v, warning %q", o.SkippedOptional, stderr.String())
	}
}

func TestFullInstallRejectsMissingMisePinBeforePackageCommands(t *testing.T) {
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
	if err := os.WriteFile(root+"/packages.conf", []byte("package ubuntu required apt example\npackage all required mise absent\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := home + "/bin"
	if err := os.WriteFile(bin+"/dpkg-query", []byte("#!/bin/sh\nprintf called >\"$HOME/package-called\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", home+"/os-release")
	os.WriteFile(home+"/os-release", []byte("ID=ubuntu\n"), 0600)
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, Out: &out, Err: &stderr}).Run([]string{"install", "--yes"})
	if code != 1 || !strings.Contains(stderr.String(), "missing approved mise version: absent") {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, err := os.Lstat(home + "/package-called"); !os.IsNotExist(err) {
		t.Fatalf("package command ran first: %v", err)
	}
}

func TestFullInstallSavedGhosttyChoiceControlsCaskPlan(t *testing.T) {
	root, home := testRelease(t), t.TempDir()
	isolateHome(t, home)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	paths, _ := UserPaths()
	if err := os.MkdirAll(paths.State, 0700); err != nil {
		t.Fatal(err)
	}
	for _, choice := range []struct {
		value string
		want  bool
	}{{"0\n", false}, {"1\n", true}} {
		if err := os.WriteFile(paths.State+"/ghostty", []byte(choice.value), 0600); err != nil {
			t.Fatal(err)
		}
		var out, stderr bytes.Buffer
		code := (CLI{Root: root, Out: &out, Err: &stderr}).Run([]string{"install", "--dry-run", "--yes"})
		if code != 0 {
			t.Fatalf("choice %q: %d %s", choice.value, code, stderr.String())
		}
		if got := strings.Contains(out.String(), "Would install optional Homebrew cask: ghostty"); got != choice.want {
			t.Fatalf("choice %q cask plan %t: %s", choice.value, got, out.String())
		}
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
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, Out: &out, Err: &stderr}).Run([]string{"install", "--yes"})
	if code != 1 || !strings.Contains(stderr.String(), "unavailable on unsupported-linux") {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("unsupported platform mutated HOME: %v %v", entries, err)
	}
}
