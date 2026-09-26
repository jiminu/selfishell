package migration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticsReference(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "releases", "1.2.3")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustFS(t, os.WriteFile(filepath.Join(release, "VERSION"), []byte("1.2.3\n"), 0600))
	tools := fixtureTools(t, root)
	osRelease := filepath.Join(root, "os-release")
	proc := filepath.Join(root, "proc-version")
	mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	mustFS(t, os.WriteFile(proc, []byte("Linux\n"), 0600))
	cases := []struct {
		name, command string
		status        int
		setup         func(string)
	}{
		{"status-empty", "status", 1, nil},
		{"status-help", "status", 0, nil},
		{"doctor-unconfigured", "doctor", 1, nil},
		{"doctor-unsupported", "doctor", 1, nil},
		{"doctor-unsupported-architecture", "doctor", 1, nil},
		{"doctor-ubuntu-wsl", "doctor", 1, nil},
		{"status-ghostty-user-override", "status", 0, func(home string) {
			state := filepath.Join(home, ".local/state/selfishell/resources")
			mustFS(t, os.MkdirAll(state, 0700))
			target := filepath.Join(home, "target")
			mustFS(t, os.WriteFile(target, []byte("intact"), 0600))
			link := filepath.Join(home, "link")
			mustFS(t, os.Symlink(target, link))
			mustFS(t, os.WriteFile(filepath.Join(state, "user-nvim.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", link, target)), 0600))
			ghostty := filepath.Join(home, ".config/ghostty/user.ghostty")
			mustFS(t, os.MkdirAll(filepath.Dir(ghostty), 0700))
			mustFS(t, os.Symlink(filepath.Join(home, "missing"), ghostty))
		}},
		{"status-pending", "status", 1, func(home string) {
			state := filepath.Join(home, ".local/state/selfishell/resources")
			mustFS(t, os.MkdirAll(state, 0700))
			mustFS(t, os.WriteFile(filepath.Join(state, "vimrc.state"), []byte(fmt.Sprintf("2\nfile\npending\n%s\n-\n-\n-\n", filepath.Join(home, "vimrc"))), 0600))
		}},
		{"status-changed-file", "status", 1, func(home string) {
			state := filepath.Join(home, ".local/state/selfishell/resources")
			mustFS(t, os.MkdirAll(state, 0700))
			target := filepath.Join(home, "vimrc")
			mustFS(t, os.WriteFile(target, []byte("changed"), 0600))
			mustFS(t, os.WriteFile(filepath.Join(state, "vimrc.state"), []byte(fmt.Sprintf("2\nfile\nactive\n%s\n-\n-\n0:0\n", target)), 0600))
		}},
		{"status-changed-link", "status", 1, func(home string) {
			state := filepath.Join(home, ".local/state/selfishell/resources")
			mustFS(t, os.MkdirAll(state, 0700))
			target := filepath.Join(home, "link")
			mustFS(t, os.WriteFile(target, []byte("replaced link"), 0600))
			mustFS(t, os.WriteFile(filepath.Join(state, "user-nvim.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", target, filepath.Join(home, "original"))), 0600))
		}},
		{"status-changed-block", "status", 1, func(home string) {
			state := filepath.Join(home, ".local/state/selfishell/resources")
			mustFS(t, os.MkdirAll(state, 0700))
			target := filepath.Join(home, ".vimrc")
			mustFS(t, os.WriteFile(target, []byte("personal vimrc\n"), 0600))
			mustFS(t, os.WriteFile(filepath.Join(state, "user-vimrc.state"), []byte(fmt.Sprintf("2\nblock\nactive\n%s\n-\n-\n0:0\n", target)), 0600))
		}},
		{"status-malformed-and-good", "status", 1, func(home string) {
			state := filepath.Join(home, ".local/state/selfishell/resources")
			mustFS(t, os.MkdirAll(state, 0700))
			mustFS(t, os.WriteFile(filepath.Join(state, "vimrc.state"), []byte("2\n"), 0600))
			target := filepath.Join(home, "good")
			mustFS(t, os.WriteFile(target, []byte("good"), 0600))
			// The second record checks that a malformed record does not stop listing.
			mustFS(t, os.WriteFile(filepath.Join(state, "aliases.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", filepath.Join(home, "link"), target)), 0600))
			mustFS(t, os.Symlink(target, filepath.Join(home, "link")))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(root, "home")
			mustFS(t, os.RemoveAll(home))
			mustFS(t, os.MkdirAll(home, 0700))
			if tc.setup != nil {
				tc.setup(home)
			}
			arch := "x86_64"
			if tc.name == "doctor-unsupported-architecture" {
				arch = "mips64"
			}
			if tc.name == "doctor-ubuntu-wsl" {
				mustFS(t, os.WriteFile(proc, []byte("Linux microsoft WSL2\n"), 0600))
			} else {
				mustFS(t, os.WriteFile(proc, []byte("Linux\n"), 0600))
			}
			env := []string{"PATH=" + tools, "SELFISHELL_TEST_SYSTEM_NAME=Linux", "SELFISHELL_TEST_MACHINE_ARCH=" + arch, "SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + proc}
			args := []string{tc.command}
			if tc.name == "status-help" {
				args = append(args, "--help")
			}
			if tc.name == "doctor-unsupported" {
				mustFS(t, os.WriteFile(osRelease, []byte("ID=fedora\n"), 0600))
			} else {
				mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
			}
			before := mustSnapshot(t, home)
			mustFS(t, os.WriteFile(entry, reference, 0755))
			want, err := captureCommand(home, entry, args, env)
			if err != nil {
				t.Fatal(err)
			}
			requireStatus(t, "reference", want, tc.status)
			if !bytes.Equal(before, want.Home) {
				t.Fatal("reference mutated HOME")
			}
			mustFS(t, copyFile(candidate, entry))
			got, err := captureCommand(home, entry, args, env)
			if err != nil {
				t.Fatal(err)
			}
			requireStatus(t, "candidate", got, tc.status)
			if !bytes.Equal(before, got.Home) {
				t.Fatal("candidate mutated HOME")
			}
			requireEqual(t, tc.name, want, got)
			if tc.name == "status-malformed-and-good" && (!strings.Contains(string(got.Stdout), "[MALFORMED]") || !strings.Contains(string(got.Stdout), "[OK]")) {
				t.Fatalf("status did not list both resources: %s", got.Stdout)
			}
		})
	}
}

func TestConfiguredDiagnosticsReference(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "releases", "2.0.0")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustFS(t, os.WriteFile(filepath.Join(release, "VERSION"), []byte("2.0.0\n"), 0600))
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), []byte("package all required apt git\npackage all optional apt optional\n"), 0600))
	tools := fixtureTools(t, root)
	for name, body := range map[string]string{
		"apt-get":    "#!/bin/sh\nexit 0\n",
		"dpkg-query": "#!/bin/sh\nprintf 'git\\tii \\t2.0\\n'\n",
		"gcc":        "#!/bin/sh\nprintf 'gcc fixture 1.0\\n'\n",
	} {
		mustFS(t, os.WriteFile(filepath.Join(tools, name), []byte(body), 0700))
	}
	osRelease := filepath.Join(root, "os-release")
	proc := filepath.Join(root, "proc-version")
	mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	mustFS(t, os.WriteFile(proc, []byte("Linux\n"), 0600))
	env := []string{"PATH=" + tools, "SELFISHELL_TEST_SYSTEM_NAME=Linux", "SELFISHELL_TEST_MACHINE_ARCH=x86_64", "SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + proc}
	for _, tc := range []struct {
		name   string
		args   []string
		status int
	}{
		{"status", []string{"status"}, 0}, {"status-verbose", []string{"status", "--verbose"}, 0}, {"doctor", []string{"doctor"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(root, "home")
			mustFS(t, os.RemoveAll(home))
			mustFS(t, os.MkdirAll(home, 0700))
			state := filepath.Join(home, ".local/state/selfishell")
			mustFS(t, os.MkdirAll(filepath.Join(state, "resources"), 0700))
			mustFS(t, os.WriteFile(filepath.Join(state, "configured"), []byte("1\n"), 0600))
			target := filepath.Join(home, "managed")
			mustFS(t, os.WriteFile(target, []byte("managed"), 0600))
			mustFS(t, os.WriteFile(filepath.Join(state, "resources/aliases.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", filepath.Join(home, "link"), target)), 0600))
			mustFS(t, os.Symlink(target, filepath.Join(home, "link")))
			before := mustSnapshot(t, home)
			mustFS(t, os.WriteFile(entry, reference, 0755))
			want, err := captureCommand(home, entry, tc.args, env)
			if err != nil {
				t.Fatal(err)
			}
			requireStatus(t, "reference", want, tc.status)
			if !bytes.Equal(before, want.Home) {
				t.Fatal("reference mutated HOME")
			}
			mustFS(t, copyFile(candidate, entry))
			got, err := captureCommand(home, entry, tc.args, env)
			if err != nil {
				t.Fatal(err)
			}
			requireStatus(t, "candidate", got, tc.status)
			if !bytes.Equal(before, got.Home) {
				t.Fatal("candidate mutated HOME")
			}
			requireEqual(t, tc.name, want, got)
		})
	}
}

func TestDoctorPluginsReference(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), nil, 0600))
	tools := fixtureTools(t, root)
	git, err := resolveCommand("git", baseEnv(root, os.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	mustFS(t, os.Symlink(git, filepath.Join(tools, "git")))
	for name, body := range map[string]string{"apt-get": "#!/bin/sh\nexit 0\n", "gcc": "#!/bin/sh\nprintf 'gcc fixture 1.0\\n'\n"} {
		mustFS(t, os.WriteFile(filepath.Join(tools, name), []byte(body), 0700))
	}
	osRelease := filepath.Join(root, "os-release")
	proc := filepath.Join(root, "proc-version")
	mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	mustFS(t, os.WriteFile(proc, []byte("Linux\n"), 0600))
	env := []string{"PATH=" + tools, "SELFISHELL_TEST_SYSTEM_NAME=Linux", "SELFISHELL_TEST_MACHINE_ARCH=x86_64", "SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + proc}
	runGit := func(dir string, args ...string) string {
		t.Helper()
		result, e := runCommand(root, append([]string{git, "-C", dir}, args...), nil, nil, 10*time.Second)
		if e != nil || result.Status != 0 {
			t.Fatalf("git %v: %v %s", args, e, result.Stderr)
		}
		return strings.TrimSpace(string(result.Stdout))
	}
	for _, name := range []string{"missing", "clean", "drift", "dirty"} {
		t.Run(name, func(t *testing.T) {
			home := filepath.Join(root, "home")
			mustFS(t, os.RemoveAll(home))
			mustFS(t, os.MkdirAll(home, 0700))
			state := filepath.Join(home, ".local/state/selfishell")
			mustFS(t, os.MkdirAll(state, 0700))
			mustFS(t, os.WriteFile(filepath.Join(state, "configured"), []byte("1\n"), 0600))
			zinit := filepath.Join(home, ".local/share/zinit/zinit.git/zinit.zsh")
			mustFS(t, os.MkdirAll(filepath.Dir(zinit), 0700))
			mustFS(t, os.WriteFile(zinit, []byte("# zinit\n"), 0600))
			plugin := filepath.Join(home, ".local/share/zinit/plugins/test---plugin")
			revision := strings.Repeat("0", 40)
			if name != "missing" {
				mustFS(t, os.MkdirAll(plugin, 0700))
				runGit(plugin, "init", "--quiet")
				runGit(plugin, "config", "user.email", "test@example.com")
				runGit(plugin, "config", "user.name", "test")
				mustFS(t, os.WriteFile(filepath.Join(plugin, "tracked"), []byte("first\n"), 0600))
				runGit(plugin, "add", "tracked")
				runGit(plugin, "commit", "--quiet", "-m", "first")
				revision = runGit(plugin, "rev-parse", "HEAD")
				if name == "drift" {
					mustFS(t, os.WriteFile(filepath.Join(plugin, "tracked"), []byte("second\n"), 0600))
					runGit(plugin, "commit", "--quiet", "-am", "second")
				}
				if name == "dirty" {
					mustFS(t, os.WriteFile(filepath.Join(plugin, "tracked"), []byte("changed\n"), 0600))
				}
			}
			mustFS(t, os.WriteFile(filepath.Join(release, "dependencies.conf"), []byte(fmt.Sprintf("zsh-plugin test/plugin %s all all - - - -\n", revision)), 0600))
			before := mustSnapshot(t, home)
			mustFS(t, os.WriteFile(entry, reference, 0755))
			want, e := captureCommand(home, entry, []string{"doctor"}, env)
			if e != nil {
				t.Fatal(e)
			}
			status := 0
			if name != "clean" {
				status = 1
			}
			requireStatus(t, "reference", want, status)
			if !bytes.Equal(before, want.Home) {
				t.Fatal("reference mutated HOME")
			}
			mustFS(t, copyFile(candidate, entry))
			got, e := captureCommand(home, entry, []string{"doctor"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "candidate", got, status)
			if !bytes.Equal(before, got.Home) {
				t.Fatal("candidate mutated HOME")
			}
			requireEqual(t, name, want, got)
		})
	}
}

func TestDiagnosticsTTYColors(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	mustFS(t, os.MkdirAll(home, 0700))
	state := filepath.Join(home, ".local/state/selfishell/resources")
	mustFS(t, os.MkdirAll(state, 0700))
	target := filepath.Join(home, "link")
	mustFS(t, os.WriteFile(filepath.Join(state, "user-nvim.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", target, filepath.Join(home, "missing"))), 0600))
	before := mustSnapshot(t, home)
	for _, noColor := range []string{"", "1"} {
		t.Run("NO_COLOR="+noColor, func(t *testing.T) {
			env := []string{"NO_COLOR=" + noColor, "SELFISHELL_TEST_SYSTEM_NAME=Darwin", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
			mustFS(t, os.WriteFile(entry, reference, 0755))
			want, e := capturePTYOutput(home, entry, []string{"status"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "reference", want, 1)
			if bytes.Contains(want.Stdout, []byte("\x1b[33m")) != (noColor == "") {
				t.Fatalf("reference color: %q", want.Stdout)
			}
			if !bytes.Equal(before, want.Home) {
				t.Fatal("reference mutated HOME")
			}
			mustFS(t, copyFile(candidate, entry))
			got, e := capturePTYOutput(home, entry, []string{"status"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "candidate", got, 1)
			if !bytes.Equal(before, got.Home) {
				t.Fatal("candidate mutated HOME")
			}
			requireEqual(t, "color", want, got)
		})
	}
}

func TestStatusRollbackMetadataReference(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	share := filepath.Join(root, "share", "selfishell")
	release := filepath.Join(share, "releases", "2.0.0")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustFS(t, os.WriteFile(filepath.Join(release, "VERSION"), []byte("2.0.0\n"), 0600))
	old := filepath.Join(share, "releases", "1.0.0")
	mustFS(t, os.MkdirAll(filepath.Join(old, "bin"), 0700))
	mustFS(t, os.WriteFile(filepath.Join(old, "bin/selfishell"), []byte("#!/bin/sh\n"), 0700))
	home := filepath.Join(root, "home")
	mustFS(t, os.MkdirAll(home, 0700))
	before := mustSnapshot(t, home)
	for _, tc := range []struct{ name, rollback, version, expected string }{
		{"none", "", "", "none"}, {"valid", "releases/1.0.0", "1.0.0\n", "1.0.0"}, {"corrupt", "releases/1.0.0", "wrong\n", "invalid"}, {"missing", "releases/9.0.0", "", "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := filepath.Join(share, "previous")
			_ = os.Remove(previous)
			if tc.rollback != "" {
				mustFS(t, os.Symlink(tc.rollback, previous))
			}
			_ = os.Remove(filepath.Join(old, "VERSION"))
			if tc.version != "" {
				mustFS(t, os.WriteFile(filepath.Join(old, "VERSION"), []byte(tc.version), 0600))
			}
			env := []string{"SELFISHELL_TEST_SYSTEM_NAME=Darwin", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
			mustFS(t, os.WriteFile(entry, reference, 0755))
			want, e := captureCommand(home, entry, []string{"status"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "reference", want, 1)
			if !bytes.Contains(want.Stdout, []byte("Rollback: "+tc.expected+"\n")) {
				t.Fatalf("reference rollback: %q", want.Stdout)
			}
			if !bytes.Equal(before, want.Home) {
				t.Fatal("reference mutated HOME")
			}
			mustFS(t, copyFile(candidate, entry))
			got, e := captureCommand(home, entry, []string{"status"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "candidate", got, 1)
			if !bytes.Equal(before, got.Home) {
				t.Fatal("candidate mutated HOME")
			}
			requireEqual(t, tc.name, want, got)
		})
	}
}

func TestDiagnosticsRejectMalformedDependencyWithoutMutation(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, os.MkdirAll(filepath.Join(release, "bin"), 0700))
	mustFS(t, copyFile(candidate, filepath.Join(release, "bin/selfishell")))
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), []byte("package all required direct zinit\n"), 0600))
	injected := filepath.Join(root, "injected")
	mustFS(t, os.WriteFile(filepath.Join(release, "dependencies.conf"), []byte("download zinit invalid all all - - - - $(touch "+injected+")\n"), 0600))
	home := filepath.Join(root, "home")
	mustFS(t, os.MkdirAll(filepath.Join(home, ".local/state/selfishell"), 0700))
	mustFS(t, os.WriteFile(filepath.Join(home, ".local/state/selfishell/configured"), []byte("1\n"), 0600))
	before := mustSnapshot(t, home)
	for _, command := range []string{"status", "doctor"} {
		got, e := captureCommand(home, filepath.Join(release, "bin/selfishell"), []string{command}, []string{"SELFISHELL_TEST_SYSTEM_NAME=Darwin", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"})
		if e != nil {
			t.Fatal(e)
		}
		requireStatus(t, command, got, 1)
		if !bytes.Contains(got.Stderr, []byte("invalid manifest record")) {
			t.Fatalf("%s accepted malformed dependency: %s", command, got.Stderr)
		}
		if !bytes.Equal(before, got.Home) {
			t.Fatalf("%s mutated HOME", command)
		}
		if _, e := os.Lstat(injected); e == nil {
			t.Fatalf("%s executed dependency text", command)
		}
	}
}

func TestDoctorXcodeStubReference(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), nil, 0600))
	tools := fixtureTools(t, root)
	for name, body := range map[string]string{"brew": "#!/bin/sh\nexit 0\n", "xcode-select": "#!/bin/sh\nexit 2\n", "gcc": "#!/bin/sh\nprintf 'stub compiler\\n'\n"} {
		mustFS(t, os.WriteFile(filepath.Join(tools, name), []byte(body), 0700))
	}
	home := filepath.Join(root, "home")
	mustFS(t, os.MkdirAll(filepath.Join(home, ".local/state/selfishell"), 0700))
	mustFS(t, os.WriteFile(filepath.Join(home, ".local/state/selfishell/configured"), []byte("1\n"), 0600))
	before := mustSnapshot(t, home)
	env := []string{"PATH=" + tools, "SELFISHELL_TEST_SYSTEM_NAME=Darwin", "SELFISHELL_TEST_MACHINE_ARCH=arm64"}
	mustFS(t, os.WriteFile(entry, reference, 0755))
	want, e := captureCommand(home, entry, []string{"doctor"}, env)
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, "reference", want, 1)
	if !bytes.Contains(want.Stdout, []byte("Xcode Command Line Tools are not installed")) || bytes.Contains(want.Stdout, []byte("[OK] C compiler")) {
		t.Fatalf("reference guard: %s", want.Stdout)
	}
	if !bytes.Equal(before, want.Home) {
		t.Fatal("reference mutated HOME")
	}
	mustFS(t, copyFile(candidate, entry))
	got, e := captureCommand(home, entry, []string{"doctor"}, env)
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, "candidate", got, 1)
	if !bytes.Equal(before, got.Home) {
		t.Fatal("candidate mutated HOME")
	}
	requireEqual(t, "xcode stub", want, got)
}

func TestDiagnosticsLiteralXDGStatePath(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), nil, 0600))
	home := filepath.Join(root, "home")
	mustFS(t, os.MkdirAll(home, 0700))
	traverse := filepath.Join(root, "traverse")
	actual := filepath.Join(root, "actual")
	mustFS(t, os.MkdirAll(traverse, 0700))
	mustFS(t, os.MkdirAll(filepath.Join(actual, "child"), 0700))
	mustFS(t, os.Symlink(filepath.Join(actual, "child"), filepath.Join(traverse, "link")))
	literal := filepath.Join(traverse, "link") + "/.."
	state := filepath.Join(actual, "selfishell")
	mustFS(t, os.MkdirAll(filepath.Join(state, "resources"), 0700))
	mustFS(t, os.WriteFile(filepath.Join(state, "configured"), []byte("1\n"), 0600))
	target := filepath.Join(home, "target")
	link := filepath.Join(home, "link")
	mustFS(t, os.WriteFile(target, []byte("intact"), 0600))
	mustFS(t, os.Symlink(target, link))
	mustFS(t, os.WriteFile(filepath.Join(state, "resources/user-nvim.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", link, target)), 0600))
	beforeHome := mustSnapshot(t, home)
	beforeState := mustSnapshot(t, actual)
	env := []string{"XDG_STATE_HOME=" + literal, "SELFISHELL_TEST_SYSTEM_NAME=Darwin", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	for _, tc := range []struct {
		command string
		status  int
	}{{"status", 0}, {"doctor", 1}} {
		t.Run(tc.command, func(t *testing.T) {
			mustFS(t, os.WriteFile(entry, reference, 0755))
			want, e := captureCommand(home, entry, []string{tc.command}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "reference", want, tc.status)
			if !bytes.Contains(want.Stdout, []byte("Selfishell configuration is installed.")) {
				t.Fatalf("reference missed literal state: %s", want.Stdout)
			}
			if !bytes.Equal(beforeHome, want.Home) || !bytes.Equal(beforeState, mustSnapshot(t, actual)) {
				t.Fatal("reference mutated state")
			}
			mustFS(t, copyFile(candidate, entry))
			got, e := captureCommand(home, entry, []string{tc.command}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "candidate", got, tc.status)
			if !bytes.Equal(beforeHome, got.Home) || !bytes.Equal(beforeState, mustSnapshot(t, actual)) {
				t.Fatal("candidate mutated state")
			}
			requireEqual(t, tc.command, want, got)
		})
	}
}

func TestStatusInstalledResourceReference(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), nil, 0600))
	tools := fixtureTools(t, root)
	osRelease := filepath.Join(root, "os-release")
	proc := filepath.Join(root, "proc-version")
	mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	mustFS(t, os.WriteFile(proc, []byte("Linux\n"), 0600))
	env := []string{"PATH=" + tools, "SELFISHELL_TEST_SYSTEM_NAME=Linux", "SELFISHELL_TEST_MACHINE_ARCH=x86_64", "SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + proc}
	outputs := map[string]capture{}
	for _, implementation := range []string{"bash", "go"} {
		home := filepath.Join(root, "home")
		mustFS(t, os.RemoveAll(home))
		mustFS(t, os.MkdirAll(home, 0700))
		config := filepath.Join(home, ".config")
		mustFS(t, os.MkdirAll(filepath.Join(config, "mise"), 0700))
		global := filepath.Join(config, "mise/config.toml")
		mustFS(t, os.WriteFile(global, []byte("user original\n"), 0600))
		if implementation == "bash" {
			mustFS(t, os.WriteFile(entry, reference, 0755))
		} else {
			mustFS(t, copyFile(candidate, entry))
		}
		run := func(name string, args []string, status int) capture {
			t.Helper()
			got, e := captureCommand(home, entry, args, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, name, got, status)
			return got
		}
		run("install", []string{"install", "--skip-packages", "--yes"}, 0)
		run("reinstall", []string{"install", "--skip-packages", "--yes"}, 0)
		mustFS(t, os.WriteFile(filepath.Join(tools, "curl"), []byte("#!/bin/sh\nprintf 'called\\n' >>'"+filepath.Join(root, "curl-calls")+"'\nexit 1\n"), 0700))
		before := mustSnapshot(t, home)
		plain := run("plain", []string{"status"}, 0)
		for _, name := range []string{"zsh/zshrc", "vim/vimrc", "nvim/init.lua"} {
			if !bytes.Contains(plain.Stdout, []byte(name)) {
				t.Fatalf("%s missing %s", implementation, name)
			}
		}
		if bytes.Contains(plain.Stdout, []byte("config.toml")) {
			t.Fatal("user config.toml reported")
		}
		if !bytes.Equal(before, plain.Home) {
			t.Fatal("status mutated HOME")
		}
		mustFS(t, os.WriteFile(global, []byte("user modified\n"), 0600))
		ghostty := filepath.Join(config, "ghostty/user.ghostty")
		mustFS(t, os.MkdirAll(filepath.Dir(ghostty), 0700))
		mustFS(t, os.Symlink(filepath.Join(root, "nonexistent"), ghostty))
		unchanged := run("user files ignored", []string{"status"}, 0)
		if !bytes.Equal(plain.Stdout, unchanged.Stdout) || !bytes.Equal(plain.Stderr, unchanged.Stderr) {
			t.Fatal("user config changed status output")
		}
		nvim := filepath.Join(config, "selfishell/nvim/init.lua")
		f, e := os.OpenFile(nvim, os.O_APPEND|os.O_WRONLY, 0)
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.WriteString("\n-- personal edit\n")
		mustFS(t, e)
		mustFS(t, f.Close())
		changed := run("changed Neovim", []string{"status"}, 1)
		if !bytes.Contains(changed.Stdout, []byte("[CHANGED] "+nvim)) {
			t.Fatalf("modified Neovim not reported: %s", changed.Stdout)
		}
		if _, e := os.Stat(filepath.Join(root, "curl-calls")); e == nil {
			t.Fatal("status invoked curl")
		}
		mustFS(t, copyFile(filepath.Join(release, "config/shared/nvim/init.lua"), nvim))
		run("uninstall", []string{"uninstall", "--restore", "--yes"}, 0)
		globalBytes, e := os.ReadFile(global)
		if e != nil || string(globalBytes) != "user modified\n" {
			t.Fatalf("user mise config lost: %v %q", e, globalBytes)
		}
		for name, got := range map[string]capture{"plain": plain, "unchanged": unchanged, "changed": changed} {
			if implementation == "bash" {
				outputs[name] = got
			} else {
				requireEqual(t, name, outputs[name], got)
			}
		}
	}
}

func TestStatusListsUnknownTrackedResources(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, os.MkdirAll(filepath.Join(release, "bin"), 0700))
	mustFS(t, copyFile(candidate, filepath.Join(release, "bin/selfishell")))
	home := filepath.Join(root, "home")
	state := filepath.Join(home, ".local/state/selfishell/resources")
	mustFS(t, os.MkdirAll(state, 0700))
	target := filepath.Join(home, "personal")
	mustFS(t, os.WriteFile(filepath.Join(state, "old-platform-link.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", target, filepath.Join(home, "approved"))), 0600))
	mustFS(t, os.WriteFile(filepath.Join(state, "unknown-bad.state"), []byte("2\n"), 0600))
	before := mustSnapshot(t, home)
	got, e := captureCommand(home, filepath.Join(release, "bin/selfishell"), []string{"status"}, []string{"SELFISHELL_TEST_SYSTEM_NAME=Darwin"})
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, "unknown tracked", got, 1)
	if !bytes.Contains(got.Stdout, []byte("[CHANGED] "+target)) || !bytes.Contains(got.Stdout, []byte("[MALFORMED] "+filepath.Join(state, "unknown-bad.state"))) || !bytes.Contains(got.Stdout, []byte("Managed paths: 2")) {
		t.Fatalf("incomplete tracked resource report: %s", got.Stdout)
	}
	if !bytes.Equal(before, got.Home) {
		t.Fatal("status mutated HOME")
	}
}
