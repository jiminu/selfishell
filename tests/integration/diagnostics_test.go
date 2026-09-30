package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiagnostics(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "releases", "1.2.3")
	copyCLIFixture(t, release, cli)
	entry := filepath.Join(release, "bin/selfishell")
	mustFS(t, os.WriteFile(filepath.Join(release, "VERSION"), []byte("1.2.3\n"), 0600))
	tools := fixtureTools(t, root)
	manager := filepath.Join(root, "manager")
	mustFS(t, os.MkdirAll(manager, 0700))
	mustFS(t, os.WriteFile(filepath.Join(manager, "apt-get"), []byte("#!/bin/sh\nexit 0\n"), 0700))
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
		{"status-missing-package-manager", "status", 1, nil},
		{"status-unsupported", "status", 1, nil},
		{"status-unsupported-architecture", "status", 1, nil},
		{"status-ubuntu-wsl", "status", 1, nil},
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
		{"status-file-replaced-by-same-content-symlink", "status", 1, func(home string) {
			state := filepath.Join(home, ".local/state/selfishell/resources")
			mustFS(t, os.MkdirAll(state, 0700))
			personal := filepath.Join(home, "personal-vimrc")
			content := []byte("same managed bytes\n")
			mustFS(t, os.WriteFile(personal, content, 0600))
			checksum, err := runCommand(home, []string{"cksum"}, content, nil, 10*time.Second)
			if err != nil || checksum.Status != 0 {
				t.Fatalf("cksum: %v %s", err, checksum.Stderr)
			}
			fields := strings.Fields(string(checksum.Stdout))
			if len(fields) != 2 {
				t.Fatalf("cksum output: %q", checksum.Stdout)
			}
			target := filepath.Join(home, "vimrc")
			mustFS(t, os.Symlink(personal, target))
			mustFS(t, os.WriteFile(filepath.Join(state, "vimrc.state"), []byte(fmt.Sprintf("2\nfile\nactive\n%s\n-\n-\n%s:%s\n", target, fields[0], fields[1])), 0600))
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
			if tc.name == "status-unsupported-architecture" {
				arch = "mips64"
			}
			if tc.name == "status-ubuntu-wsl" {
				mustFS(t, os.WriteFile(proc, []byte("Linux microsoft WSL2\n"), 0600))
			} else {
				mustFS(t, os.WriteFile(proc, []byte("Linux\n"), 0600))
			}
			path := tools + ":" + manager
			if tc.name == "status-missing-package-manager" {
				path = tools
			}
			env := []string{"PATH=" + path, "SELFISHELL_TEST_SYSTEM_NAME=Linux", "SELFISHELL_TEST_MACHINE_ARCH=" + arch, "SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + proc}
			args := []string{tc.command}
			if tc.name == "status-help" {
				args = append(args, "--help")
			}
			if tc.name == "status-unsupported" {
				mustFS(t, os.WriteFile(osRelease, []byte("ID=fedora\n"), 0600))
			} else {
				mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
			}
			before := mustSnapshot(t, home)
			got, err := captureCommand(home, entry, args, env)
			if err != nil {
				t.Fatal(err)
			}
			requireStatus(t, "CLI", got, tc.status)
			if !bytes.Equal(before, got.Home) {
				t.Fatal("CLI mutated HOME")
			}
			expected := map[string]string{
				"status-empty":                                 "Selfishell configuration is not installed.",
				"status-help":                                  "Usage: selfishell status [--verbose]",
				"status-missing-package-manager":               "[ERROR] System: Ubuntu",
				"status-unsupported":                           "[ERROR] Platform: Unsupported Linux distribution",
				"status-unsupported-architecture":              "[ERROR] Architecture: mips64",
				"status-ubuntu-wsl":                            "[OK] System: Ubuntu on WSL",
				"status-ghostty-user-override":                 "[OK] Configuration: 1 paths intact",
				"status-pending":                               "[PENDING] ~/vimrc",
				"status-changed-file":                          "[CHANGED] ~/vimrc",
				"status-file-replaced-by-same-content-symlink": "[CHANGED] ~/vimrc",
				"status-changed-link":                          "[CHANGED] ~/link",
				"status-changed-block":                         "[CHANGED] ~/.vimrc",
				"status-malformed-and-good":                    "[MALFORMED]",
			}[tc.name]
			if expected == "" || !bytes.Contains(got.Stdout, []byte(expected)) {
				t.Fatalf("missing diagnostic %q: %s", expected, got.Stdout)
			}

			if tc.name == "status-empty" || tc.name == "status-missing-package-manager" {
				if n := strings.Count(string(got.Stdout), "selfishell install"); n != 1 {
					t.Fatalf("install hint shown %d times: %s", n, got.Stdout)
				}
			}
			if tc.name == "status-malformed-and-good" && (!strings.Contains(string(got.Stdout), "[MALFORMED]") || !strings.Contains(string(got.Stdout), "Configuration: 1 intact, 1 issues")) {
				t.Fatalf("status did not account for both resources: %s", got.Stdout)
			}
		})
	}
}

func TestConfiguredDiagnostics(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "releases", "2.0.0")
	copyCLIFixture(t, release, cli)
	entry := filepath.Join(release, "bin/selfishell")
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
		{"status", []string{"status"}, 0}, {"status-verbose", []string{"status", "--verbose"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(root, "home")
			mustFS(t, os.RemoveAll(home))
			mustFS(t, os.MkdirAll(home, 0700))
			setup, e := captureCommand(home, entry, []string{"install", "--skip-packages", "--yes"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "configuration fixture", setup, 0)
			state := filepath.Join(home, ".local/state/selfishell")
			mustFS(t, os.MkdirAll(filepath.Join(state, "resources"), 0700))
			mustFS(t, os.WriteFile(filepath.Join(state, "configured"), []byte("1\n"), 0600))
			target := filepath.Join(home, "managed")
			mustFS(t, os.WriteFile(target, []byte("managed"), 0600))
			mustFS(t, os.WriteFile(filepath.Join(state, "resources/aliases.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", filepath.Join(home, "link"), target)), 0600))
			mustFS(t, os.Symlink(target, filepath.Join(home, "link")))
			before := mustSnapshot(t, home)
			got, err := captureCommand(home, entry, tc.args, env)
			if err != nil {
				t.Fatal(err)
			}
			requireStatus(t, "CLI", got, tc.status)
			requireContains(t, got.Stdout, "Selfishell configuration is installed.")
			if !bytes.HasPrefix(got.Stdout, []byte("[CLI] Current: 2.0.0 | Rollback: none\n[INFO] Selfishell configuration is installed.\n")) {
				t.Fatalf("inconsistent diagnostic header: %s", got.Stdout)
			}
			requireContains(t, got.Stdout, "[OK] System: Ubuntu (amd64), 4 checks passed")
			requireContains(t, got.Stdout, "Configuration: 31 paths intact")
			requireContains(t, got.Stdout, "Optional tool: optional is not installed")
			requireContains(t, got.Stdout, "[INFO] Tools: 1 present, 1 optional not installed")
			if tc.name == "status-verbose" {
				requireContains(t, got.Stdout, "[OK] C compiler:")
				requireContains(t, got.Stdout, "[OK] ~/link")
				requireContains(t, got.Stdout, "[TOOL] git | Installed: 2.0")
				requireContains(t, got.Stdout, "[TOOL] optional | Installed: missing")
			} else if bytes.Contains(got.Stdout, []byte("[OK] C compiler:")) || bytes.Contains(got.Stdout, []byte("[OK] ~/link")) || bytes.Contains(got.Stdout, []byte("[TOOL]")) {
				t.Fatalf("healthy details were not collapsed: %s", got.Stdout)
			}
			if !bytes.Equal(before, got.Home) {
				t.Fatal("CLI mutated HOME")
			}
		})
	}
}

func TestStatusPlugins(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	copyCLIFixture(t, release, cli)
	entry := filepath.Join(release, "bin/selfishell")
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
	for _, name := range []string{"missing", "clean", "detached", "packed", "fallback", "unborn", "broken", "invalid-config", "drift", "dirty", "untracked", "mixed"} {
		t.Run(name, func(t *testing.T) {
			home := filepath.Join(root, "home")
			mustFS(t, os.RemoveAll(home))
			mustFS(t, os.MkdirAll(home, 0700))
			setup, e := captureCommand(home, entry, []string{"install", "--skip-packages", "--yes"}, env)
			mustFS(t, e)
			requireStatus(t, "configuration fixture", setup, 0)
			state := filepath.Join(home, ".local/state/selfishell")
			mustFS(t, os.WriteFile(filepath.Join(state, "configured"), []byte("1\n"), 0600))
			zinit := filepath.Join(home, ".local/share/zinit/zinit.git/zinit.zsh")
			mustFS(t, os.MkdirAll(filepath.Dir(zinit), 0700))
			mustFS(t, os.WriteFile(zinit, []byte("# zinit\n"), 0600))
			plugin := filepath.Join(home, ".local/share/zinit/plugins/test---plugin")
			revision := strings.Repeat("0", 40)
			if name != "missing" {
				mustFS(t, os.MkdirAll(plugin, 0700))
				runGit(plugin, "init", "--quiet")
				// Fixture commits must not leave background maintenance racing HOME snapshots.
				runGit(plugin, "config", "maintenance.auto", "false")
				runGit(plugin, "config", "user.email", "test@example.com")
				runGit(plugin, "config", "user.name", "test")
				mustFS(t, os.WriteFile(filepath.Join(plugin, "tracked"), []byte("first\n"), 0600))
				runGit(plugin, "add", "tracked")
				runGit(plugin, "commit", "--quiet", "-m", "first")
				revision = runGit(plugin, "rev-parse", "HEAD")
				switch name {
				case "detached":
					runGit(plugin, "checkout", "--quiet", "--detach")
				case "packed":
					runGit(plugin, "pack-refs", "--all")
				case "fallback":
					ref := runGit(plugin, "symbolic-ref", "HEAD")
					runGit(plugin, "symbolic-ref", "refs/heads/alias", ref)
					runGit(plugin, "symbolic-ref", "HEAD", "refs/heads/alias")
				case "unborn":
					runGit(plugin, "rm", "-q", "tracked")
					runGit(plugin, "symbolic-ref", "HEAD", "refs/heads/unborn")
				case "broken":
					mustFS(t, os.WriteFile(filepath.Join(plugin, ".git/HEAD"), []byte("invalid\n"), 0600))
				case "invalid-config":
					mustFS(t, os.WriteFile(filepath.Join(plugin, ".git/config"), []byte("[invalid\n"), 0600))
				case "untracked":
					mustFS(t, os.WriteFile(filepath.Join(plugin, "untracked"), []byte("user data\n"), 0600))
				}
				if name == "drift" {
					mustFS(t, os.WriteFile(filepath.Join(plugin, "tracked"), []byte("second\n"), 0600))
					runGit(plugin, "commit", "--quiet", "-am", "second")
				}
				if name == "dirty" || name == "mixed" {
					mustFS(t, os.WriteFile(filepath.Join(plugin, "tracked"), []byte("changed\n"), 0600))
				}
			}
			mustFS(t, os.WriteFile(filepath.Join(release, "dependencies.conf"), []byte(fmt.Sprintf("zsh-plugin test/plugin %s all all - - - -\n", revision)), 0600))
			if name == "mixed" {
				other := filepath.Join(home, ".local/share/zinit/plugins/test---other")
				runGit(root, "clone", "--quiet", plugin, other)
				runGit(other, "config", "maintenance.auto", "false")
				f, e := os.OpenFile(filepath.Join(release, "dependencies.conf"), os.O_APPEND|os.O_WRONLY, 0600)
				mustFS(t, e)
				_, e = fmt.Fprintf(f, "zsh-plugin test/other %s all all - - - -\n", strings.Repeat("0", 40))
				mustFS(t, e)
				mustFS(t, f.Close())
			}
			trace := filepath.Join(root, "git-trace")
			mustFS(t, os.WriteFile(trace, nil, 0600))
			diagnosticEnv := append(append([]string{}, env...), "GIT_TRACE="+trace, "GIT_DIR="+root+"/foreign", "GIT_WORK_TREE="+root+"/foreign")
			before := mustSnapshot(t, home)
			got, e := captureCommand(home, entry, []string{"status"}, diagnosticEnv)
			if e != nil {
				t.Fatal(e)
			}
			status := 0
			if name != "clean" && name != "detached" && name != "packed" && name != "fallback" {
				status = 1
			}
			requireStatus(t, "CLI", got, status)
			expected := map[string]string{
				"missing":        "Zsh plugins: 1 not provisioned (test/plugin)",
				"clean":          "Zsh plugins: provisioned",
				"detached":       "Zsh plugins: provisioned",
				"packed":         "Zsh plugins: provisioned",
				"fallback":       "Zsh plugins: provisioned",
				"unborn":         "Zsh plugins: 1 at an unapproved revision (test/plugin)",
				"broken":         "Zsh plugins: 1 at an unapproved revision (test/plugin)",
				"invalid-config": "Zsh plugins: 1 at an unapproved revision (test/plugin)",
				"untracked":      "Zsh plugins: 1 modified locally (test/plugin)",
				"drift":          "Zsh plugins: 1 at an unapproved revision (test/plugin)",
				"dirty":          "Zsh plugins: 1 modified locally (test/plugin)",
				"mixed":          "Zsh plugins: 1 modified locally (test/plugin)",
			}[name]
			requireContains(t, got.Stdout, expected)
			if name == "mixed" {
				requireContains(t, got.Stdout, "Zsh plugins: 1 at an unapproved revision (test/other)")
				if strings.Count(string(got.Stdout), "selfishell update --tools-only") != 1 {
					t.Fatalf("repeated plugin repair hint: %s", got.Stdout)
				}
			}
			calls, err := os.ReadFile(trace)
			mustFS(t, err)
			wantHeads := 0
			if name == "fallback" || name == "unborn" {
				wantHeads = 1
			}
			if got := strings.Count(string(calls), "rev-parse HEAD"); got != wantHeads {
				t.Fatalf("HEAD processes=%d want=%d: %s", got, wantHeads, calls)
			}
			if !bytes.Equal(before, got.Home) {
				t.Fatal("CLI mutated HOME")
			}
		})
	}
}

func TestStatusGhosttyChoice(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	copyCLIFixture(t, release, cli)
	entry := filepath.Join(release, "bin/selfishell")
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), nil, 0600))
	osRelease, proc := filepath.Join(root, "os-release"), filepath.Join(root, "proc-version")
	mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	tools := filepath.Join(root, "system-tools")
	mustFS(t, os.MkdirAll(tools, 0700))
	for _, name := range []string{"apt-get", "brew", "xcode-select", "gcc"} {
		mustFS(t, os.WriteFile(filepath.Join(tools, name), []byte("#!/bin/sh\nexit 0\n"), 0700))
	}
	for _, tc := range []struct {
		platform, choice string
		tracked, wantErr bool
		wantCode         int
	}{
		{platform: "macos", choice: "enabled", wantCode: 1},
		{platform: "macos", choice: "disabled"},
		{platform: "macos", choice: "missing"},
		{platform: "macos", choice: "directory", wantErr: true, wantCode: 1},
		{platform: "macos", choice: "fifo", wantErr: true, wantCode: 1},
		{platform: "macos", choice: "symlink", wantErr: true, wantCode: 1},
		{platform: "macos", choice: "dangling-symlink", wantErr: true, wantCode: 1},
		{platform: "macos", choice: "permission", wantErr: true, wantCode: 1},
		{platform: "macos", choice: "disabled", tracked: true, wantCode: 1},
		{platform: "macos", choice: "missing", tracked: true, wantCode: 1},
		{platform: "ubuntu", choice: "fifo"},
		{platform: "ubuntu-wsl", choice: "fifo"},
		{platform: "ubuntu", choice: "fifo", tracked: true, wantCode: 1},
		{platform: "ubuntu-wsl", choice: "fifo", tracked: true, wantCode: 1},
	} {
		t.Run(fmt.Sprintf("%s/%s/tracked=%t", tc.platform, tc.choice, tc.tracked), func(t *testing.T) {
			if tc.choice == "permission" && os.Geteuid() == 0 {
				t.Skip("root can read files without permission bits")
			}
			home := t.TempDir()
			state := filepath.Join(home, ".local/state/selfishell")
			mustFS(t, os.MkdirAll(state, 0700))
			choice := filepath.Join(state, "ghostty")
			mustFS(t, os.WriteFile(choice, []byte("0\n"), 0600))
			system, procVersion := "Linux", "Linux\n"
			if tc.platform == "macos" {
				system = "Darwin"
			} else if tc.platform == "ubuntu-wsl" {
				procVersion = "Linux microsoft WSL2\n"
			}
			mustFS(t, os.WriteFile(proc, []byte(procVersion), 0600))
			env := []string{"PATH=" + tools + ":/usr/bin:/bin:/usr/sbin:/sbin", "SELFISHELL_TEST_SYSTEM_NAME=" + system, "SELFISHELL_TEST_MACHINE_ARCH=arm64", "SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + proc}
			setup, e := captureCommand(home, entry, []string{"install", "--skip-packages", "--yes"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "configuration fixture", setup, 0)
			mustFS(t, os.Remove(choice))
			switch tc.choice {
			case "enabled":
				mustFS(t, os.WriteFile(choice, []byte("1\n"), 0600))
			case "disabled", "permission":
				mustFS(t, os.WriteFile(choice, []byte("0\n"), 0600))
			case "directory":
				mustFS(t, os.Mkdir(choice, 0700))
			case "fifo":
				mustFS(t, makeFIFO(choice))
			case "symlink", "dangling-symlink":
				target := filepath.Join(home, "personal-choice")
				if tc.choice == "symlink" {
					mustFS(t, os.WriteFile(target, []byte("0\n"), 0600))
				}
				mustFS(t, os.Symlink(target, choice))
			}
			if tc.tracked {
				mustFS(t, os.WriteFile(filepath.Join(state, "resources/user-ghostty.state"), []byte("malformed\n"), 0600))
			}
			before := mustSnapshot(t, home)
			if tc.choice == "permission" {
				mustFS(t, os.Chmod(choice, 0000))
			}
			// runCommand kills the child on timeout and returns an error, never a passing exit status.
			got, e := runCommand(home, []string{entry, "status"}, nil, env, 3*time.Second)
			if tc.choice == "permission" {
				info, statErr := os.Lstat(choice)
				mustFS(t, statErr)
				if info.Mode().Perm() != 0 {
					t.Fatal("status changed choice permissions")
				}
				mustFS(t, os.Chmod(choice, 0600))
			}
			if e != nil {
				t.Fatalf("status did not finish: %v", e)
			}
			requireStatus(t, "status", got, tc.wantCode)
			if tc.wantErr {
				requireContains(t, got.Stderr, "selfishell:")
				requireContains(t, got.Stderr, "~/.local/state/selfishell/ghostty")
			} else {
				if len(got.Stderr) != 0 {
					t.Fatalf("unexpected diagnostic error: %s", got.Stderr)
				}
				for _, name := range []string{"ghostty-config", "user-ghostty"} {
					missing := "[MISSING] Installation record: ~/.local/state/selfishell/resources/" + name + ".state"
					if bytes.Contains(got.Stdout, []byte(missing)) != (tc.choice == "enabled") {
						t.Fatalf("incorrect Ghostty selection: %s", got.Stdout)
					}
				}
				if tc.tracked {
					requireContains(t, got.Stdout, "[MALFORMED] ~/.local/state/selfishell/resources/user-ghostty.state")
				}
			}
			if !bytes.Equal(before, mustSnapshot(t, home)) {
				t.Fatal("status mutated HOME")
			}
		})
	}
}

func TestDiagnosticsTTYColors(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	copyCLIFixture(t, release, cli)
	entry := filepath.Join(release, "bin/selfishell")
	home := filepath.Join(root, "home")
	mustFS(t, os.MkdirAll(home, 0700))
	state := filepath.Join(home, ".local/state/selfishell/resources")
	mustFS(t, os.MkdirAll(state, 0700))
	target := filepath.Join(home, "link")
	mustFS(t, os.WriteFile(filepath.Join(state, "user-nvim.state"), []byte(fmt.Sprintf("2\nlink\nactive\n%s\n%s\n-\n-\n", target, filepath.Join(home, "missing"))), 0600))
	before := mustSnapshot(t, home)
	for _, tc := range []struct {
		name, noColor, ci, term string
		redirect, color         bool
	}{
		{name: "TTY", term: "xterm", color: true},
		{name: "NO_COLOR", noColor: "1", term: "xterm"},
		{name: "CI", ci: "true", term: "xterm"},
		{name: "TERM=dumb", term: "dumb"},
		{name: "redirect", term: "xterm", redirect: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := []string{"NO_COLOR=" + tc.noColor, "CI=" + tc.ci, "TERM=" + tc.term, "SELFISHELL_TEST_SYSTEM_NAME=Darwin", "SELFISHELL_TEST_MACHINE_ARCH=arm64", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
			captureOutput := capturePTYOutput
			if tc.redirect {
				captureOutput = captureCommand
			}
			got, e := captureOutput(home, entry, []string{"status"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "CLI", got, 1)
			// brew is outside PATH, so the system check fails alongside the changed link.
			for _, want := range []struct{ marker, hint, color string }{
				{"[CHANGED]", "selfishell update --tools-only --skip-packages", "33"},
				{"[ERROR]", "selfishell install", "31"},
			} {
				requireStdout(t, got, want.marker)
				requireStdout(t, got, want.hint)
				if tc.color {
					requireStdout(t, got, "\x1b["+want.color+"m"+want.marker+"\x1b[0m")
					requireStdout(t, got, "\x1b[1m"+want.hint+"\x1b[0m")
				}
			}
			if !tc.color && (bytes.Contains(got.Stdout, []byte("\x1b")) || bytes.Contains(got.Stderr, []byte("\x1b"))) {
				t.Fatalf("unexpected ANSI: stdout=%q stderr=%q", got.Stdout, got.Stderr)
			}
			if !bytes.Equal(before, got.Home) {
				t.Fatal("CLI mutated HOME")
			}
		})
	}
}

func TestStatusRollbackMetadata(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	share := filepath.Join(root, "share", "selfishell")
	release := filepath.Join(share, "releases", "2.0.0")
	copyCLIFixture(t, release, cli)
	entry := filepath.Join(release, "bin/selfishell")
	mustFS(t, os.WriteFile(filepath.Join(release, "VERSION"), []byte("2.0.0\n"), 0600))
	old := filepath.Join(share, "releases", "1.0.0")
	mustFS(t, os.MkdirAll(filepath.Join(old, "bin"), 0700))
	mustFS(t, os.WriteFile(filepath.Join(old, "bin/selfishell"), []byte("#!/bin/sh\n"), 0700))
	home := filepath.Join(root, "home")
	mustFS(t, os.MkdirAll(home, 0700))
	before := mustSnapshot(t, home)
	// The header accepts exactly what `selfishell rollback` would restore.
	for _, tc := range []struct {
		name, rollback, version, expected string
		occupied                          bool
	}{
		{"none", "", "", "none", false}, {"valid", "releases/1.0.0", "1.0.0\n", "1.0.0", false}, {"corrupt", "releases/1.0.0", "wrong\n", "invalid", false}, {"missing", "releases/9.0.0", "", "invalid", false},
		{"foreign", "foreign/1.0.0", "1.0.0\n", "invalid", false}, {"occupied", "", "1.0.0\n", "invalid", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := filepath.Join(share, "previous")
			_ = os.Remove(previous)
			if tc.rollback != "" {
				mustFS(t, os.Symlink(tc.rollback, previous))
			}
			if tc.occupied {
				mustFS(t, os.WriteFile(previous, []byte("releases/1.0.0\n"), 0600))
			}
			_ = os.Remove(filepath.Join(old, "VERSION"))
			if tc.version != "" {
				mustFS(t, os.WriteFile(filepath.Join(old, "VERSION"), []byte(tc.version), 0600))
			}
			retained := mustSnapshot(t, share)
			env := []string{"SELFISHELL_TEST_SYSTEM_NAME=Darwin", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
			got, e := captureCommand(home, entry, []string{"status"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "status", got, 1)
			if !bytes.Contains(got.Stdout, []byte("Rollback: "+tc.expected+"\n")) {
				t.Fatalf("status rollback: %q", got.Stdout)
			}
			if !bytes.Equal(before, got.Home) {
				t.Fatal("status mutated HOME")
			}
			if !bytes.Equal(retained, mustSnapshot(t, share)) {
				t.Fatal("status changed retained releases")
			}
		})
	}
}

func TestDiagnosticsRejectMalformedDependencyWithoutMutation(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, os.MkdirAll(filepath.Join(release, "bin"), 0700))
	mustFS(t, copyFile(cli, filepath.Join(release, "bin/selfishell")))
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), []byte("package all required direct zinit\n"), 0600))
	injected := filepath.Join(root, "injected")
	mustFS(t, os.WriteFile(filepath.Join(release, "dependencies.conf"), []byte("download zinit invalid all all - - - - $(touch "+injected+")\n"), 0600))
	home := filepath.Join(root, "home")
	mustFS(t, os.MkdirAll(filepath.Join(home, ".local/state/selfishell"), 0700))
	mustFS(t, os.WriteFile(filepath.Join(home, ".local/state/selfishell/configured"), []byte("1\n"), 0600))
	before := mustSnapshot(t, home)
	got, e := captureCommand(home, filepath.Join(release, "bin/selfishell"), []string{"status"}, []string{"SELFISHELL_TEST_SYSTEM_NAME=Darwin", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"})
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, "status", got, 1)
	if !bytes.Contains(got.Stderr, []byte("invalid manifest record")) {
		t.Fatalf("status accepted malformed dependency: %s", got.Stderr)
	}
	if !bytes.Equal(before, got.Home) {
		t.Fatal("status mutated HOME")
	}
	if _, e := os.Lstat(injected); e == nil {
		t.Fatal("status executed dependency text")
	}
}

func TestStatusXcodeStub(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	const (
		works  = "#!/bin/sh\nprintf 'stub compiler\\n'\n"
		broken = "#!/bin/sh\nprintf 'license not accepted\\n' >&2\nexit 69\n"
	)
	for _, tc := range []struct {
		name, xcode, gcc, clang string
		brew                    bool
		status                  int
		want, reject            []string
	}{
		{"xcode_missing", "exit 2", works, "", true, 1, []string{"Xcode Command Line Tools are not installed"}, []string{"[OK] C compiler"}},
		{"gcc", "exit 0", works, "", true, 0, []string{"[OK] C compiler: gcc (stub compiler)", "4 checks passed\n"}, []string{"[ERROR]"}},
		{"clang_after_gcc_fails", "exit 0", broken, works, true, 0, []string{"[OK] C compiler: clang (stub compiler)", "4 checks passed\n"}, []string{"[ERROR]"}},
		{"all_fail", "exit 0", broken, broken, true, 1, []string{"[ERROR] C compiler: gcc and clang failed to run", "Run 'gcc --version'", "3 checks passed, 1 failed"}, []string{"[OK] C compiler", "license not accepted"}},
		{"none", "exit 0", "", "", true, 1, []string{"[ERROR] C compiler: gcc or clang was not found", "3 checks passed, 1 failed"}, []string{"failed to run"}},
		{"earlier_failure_kept", "exit 0", works, "", false, 1, []string{"[ERROR] Package manager: brew was not found", "[OK] C compiler: gcc", "3 checks passed, 1 failed"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			release := filepath.Join(root, "release")
			copyCLIFixture(t, release, cli)
			entry := filepath.Join(release, "bin/selfishell")
			mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), nil, 0600))
			tools := fixtureTools(t, root)
			bodies := map[string]string{"xcode-select": "#!/bin/sh\n" + tc.xcode + "\n", "gcc": tc.gcc, "clang": tc.clang}
			if tc.brew {
				bodies["brew"] = "#!/bin/sh\nexit 0\n"
			}
			for name, body := range bodies {
				if body != "" {
					mustFS(t, os.WriteFile(filepath.Join(tools, name), []byte(body), 0700))
				}
			}
			home := filepath.Join(root, "home")
			mustFS(t, os.MkdirAll(home, 0700))
			env := []string{"PATH=" + tools, "SELFISHELL_TEST_SYSTEM_NAME=Darwin", "SELFISHELL_TEST_MACHINE_ARCH=arm64"}
			setup, e := captureCommand(home, entry, []string{"install", "--skip-packages", "--yes"}, env)
			mustFS(t, e)
			requireStatus(t, "configuration fixture", setup, 0)
			mustFS(t, os.WriteFile(filepath.Join(home, ".local/state/selfishell/configured"), []byte("1\n"), 0600))
			before := mustSnapshot(t, home)
			got, e := captureCommand(home, entry, []string{"status", "--verbose"}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "CLI", got, tc.status)
			for _, want := range tc.want {
				requireContains(t, got.Stdout, want)
			}
			for _, reject := range tc.reject {
				if bytes.Contains(got.Stdout, []byte(reject)) {
					t.Fatalf("unexpected %q: %s", reject, got.Stdout)
				}
			}
			if !bytes.Equal(before, got.Home) {
				t.Fatal("CLI mutated HOME")
			}
		})
	}
}

func TestDiagnosticsLiteralXDGStatePath(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	copyCLIFixture(t, release, cli)
	entry := filepath.Join(release, "bin/selfishell")
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
	}{{"status", 1}} {
		t.Run(tc.command, func(t *testing.T) {
			got, e := captureCommand(home, entry, []string{tc.command}, env)
			if e != nil {
				t.Fatal(e)
			}
			requireStatus(t, "CLI", got, tc.status)
			if !bytes.Contains(got.Stdout, []byte("Selfishell configuration is installed.")) {
				t.Fatalf("CLI missed literal state: %s", got.Stdout)
			}
			if !bytes.Equal(beforeHome, got.Home) || !bytes.Equal(beforeState, mustSnapshot(t, actual)) {
				t.Fatal("CLI mutated state")
			}
		})
	}
}

func TestStatusInstalledResource(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	copyCLIFixture(t, release, cli)
	entry := filepath.Join(release, "bin/selfishell")
	mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), nil, 0600))
	tools := fixtureTools(t, root)
	for _, name := range []string{"apt-get", "gcc"} {
		mustFS(t, os.WriteFile(filepath.Join(tools, name), []byte("#!/bin/sh\nexit 0\n"), 0700))
	}
	osRelease := filepath.Join(root, "os-release")
	proc := filepath.Join(root, "proc-version")
	mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	mustFS(t, os.WriteFile(proc, []byte("Linux\n"), 0600))
	env := []string{"PATH=" + tools, "SELFISHELL_TEST_SYSTEM_NAME=Linux", "SELFISHELL_TEST_MACHINE_ARCH=x86_64", "SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + proc}
	home := filepath.Join(root, "home")
	mustFS(t, os.RemoveAll(home))
	mustFS(t, os.MkdirAll(home, 0700))
	config := filepath.Join(home, ".config")
	mustFS(t, os.MkdirAll(filepath.Join(config, "mise"), 0700))
	global := filepath.Join(config, "mise/config.toml")
	mustFS(t, os.WriteFile(global, []byte("user original\n"), 0600))
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
	mustFS(t, os.WriteFile(global, []byte("user modified\n"), 0600))
	run("reinstall", []string{"install", "--skip-packages", "--yes"}, 0)
	globalBytes, e := os.ReadFile(global)
	if e != nil || string(globalBytes) != "user modified\n" {
		t.Fatalf("reinstall changed user mise config: %v %q", e, globalBytes)
	}
	mustFS(t, os.WriteFile(filepath.Join(tools, "curl"), []byte("#!/bin/sh\nprintf 'called\\n' >>'"+filepath.Join(root, "curl-calls")+"'\nexit 1\n"), 0700))
	before := mustSnapshot(t, home)
	plain := run("plain", []string{"status"}, 0)
	requireContains(t, plain.Stdout, "[OK] Configuration:")
	verbose := run("verbose", []string{"status", "--verbose"}, 0)
	for _, name := range []string{"zsh/zshrc", "vim/vimrc", "nvim/init.lua"} {
		if !bytes.Contains(verbose.Stdout, []byte(name)) {
			t.Fatalf("verbose status missing %s", name)
		}
		if bytes.Contains(plain.Stdout, []byte(name)) {
			t.Fatalf("healthy path shown in compact status: %s", plain.Stdout)
		}
	}
	if bytes.Contains(plain.Stdout, []byte("config.toml")) {
		t.Fatal("user config.toml reported")
	}
	if !bytes.Equal(before, plain.Home) {
		t.Fatal("status mutated HOME")
	}
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
	if !bytes.Contains(changed.Stdout, []byte("[CHANGED] ~/.config/selfishell/nvim/init.lua")) {
		t.Fatalf("modified Neovim not reported: %s", changed.Stdout)
	}
	if _, e := os.Stat(filepath.Join(root, "curl-calls")); e == nil {
		t.Fatal("status invoked curl")
	}
	mustFS(t, copyFile(filepath.Join(release, "config/shared/nvim/init.lua"), nvim))
	run("uninstall", []string{"uninstall", "--restore", "--yes"}, 0)
	globalBytes, e = os.ReadFile(global)
	if e != nil || string(globalBytes) != "user modified\n" {
		t.Fatalf("user mise config lost: %v %q", e, globalBytes)
	}
}

func TestStatusListsUnknownTrackedResources(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, os.MkdirAll(filepath.Join(release, "bin"), 0700))
	mustFS(t, copyFile(cli, filepath.Join(release, "bin/selfishell")))
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
	if !bytes.Contains(got.Stdout, []byte("[CHANGED] ~/personal")) || !bytes.Contains(got.Stdout, []byte("[MALFORMED] ~/.local/state/selfishell/resources/unknown-bad.state")) || !bytes.Contains(got.Stdout, []byte("Configuration: 0 intact, 2 issues")) {
		t.Fatalf("incomplete tracked resource report: %s", got.Stdout)
	}
	if !bytes.Equal(before, got.Home) {
		t.Fatal("status mutated HOME")
	}
}
