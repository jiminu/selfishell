package selfishell

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

func TestStatusToolsContinuesAfterQueryFailureWithoutMissingHint(t *testing.T) {
	t.Parallel()
	var env process.Environment
	for _, requirement := range []string{"required", "optional"} {
		t.Run(requirement, func(t *testing.T) {
			t.Parallel()
			env := env
			root, paths, warnings, bin := inventoryFixture(t, &env)
			testutil.SetEnv(&env, "PATH", bin)
			fixtureFile(t, bin+"/dpkg-query", "#!/bin/sh\nprintf 'database locked\\n' >&2\nexit 2\n", 0700)
			fixtureFile(t, bin+"/brew", "#!/bin/sh\nprintf '{\"formulae\":[{\"name\":\"present\",\"versions\":[\"1.0\"]}],\"casks\":[]}'\n", 0700)
			var out bytes.Buffer
			cli := CLI{Env: env, Out: &out}
			problem, err := cli.statusTools([]Package{
				{Name: "probe", Manager: "apt", Requirement: requirement},
				{Name: "present", Manager: "formula", Requirement: "required"},
			}, inventory(t, env, root, paths, warnings), Platform{Name: "ubuntu", Arch: "amd64"}, true)
			if err != nil || !problem {
				t.Fatalf("query failure aborted listing or passed diagnosis: problem=%v error=%v output=%s", problem, err, &out)
			}
			for _, want := range []string{"database locked", "Installed: unknown", "present | Installed: 1.0", "Tools: 1 present, 1 unknown"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("diagnosis omitted %q: %s", want, &out)
				}
			}
			if strings.Contains(out.String(), "missing") || strings.Contains(out.String(), "selfishell update --tools-only") {
				t.Fatalf("query failure incorrectly recommended installation: %s", &out)
			}
		})
	}
}

// Zsh plugins are git direct packages; drifted or dirty checkouts are missing.
func TestStatusToolsReportsGitPluginCheckouts(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root, paths, warnings, _ := inventoryFixture(t, &env)
	repo := t.TempDir()
	testutil.GitCommand(t, repo, "init", "--quiet")
	fixtureFile(t, repo+"/p.plugin.zsh", ":\n", 0600)
	testutil.GitCommand(t, repo, "add", "p.plugin.zsh")
	testutil.GitCommand(t, repo, "commit", "--quiet", "-m", "initial")
	sha := testutil.GitCommand(t, repo, "rev-parse", "HEAD")
	data := env.Get("XDG_DATA_HOME") + "/selfishell/zsh-plugins/"
	var manifest strings.Builder
	var packages []Package
	for _, name := range []string{"present", "drifted", "dirty", "absent"} {
		fmt.Fprintf(&manifest, "git %s %s all all %s %s .local/share/selfishell/zsh-plugins/%s p.plugin.zsh\n", name, sha, repo, sha, name)
		packages = append(packages, Package{Name: name, Manager: "direct", Requirement: "required"})
		if name != "absent" {
			testutil.GitCommand(t, repo, "clone", "--quiet", repo, data+name)
			fixtureFile(t, paths.State+"/dependencies/"+name, sha+"\n", 0600)
		}
	}
	testutil.GitCommand(t, data+"drifted", "commit", "--quiet", "--allow-empty", "-m", "drift")
	fixtureFile(t, data+"dirty/p.plugin.zsh", "changed\n", 0600)
	fixtureFile(t, root+"/dependencies.conf", manifest.String(), 0600)
	var out bytes.Buffer
	problem, err := (CLI{Env: env, Out: &out}).statusTools(packages, inventory(t, env, root, paths, warnings), Platform{Name: "ubuntu", Arch: "amd64"}, true)
	if err != nil || !problem {
		t.Fatalf("plugin issues passed diagnosis: %v %s", err, &out)
	}
	for _, want := range []string{"present | Installed: " + sha[:12] + " | Source: selfishell | Approved: " + sha[:12] + "\n", "Tool: drifted is missing (direct)", "Tool: dirty is missing (direct)", "Tool: absent is missing (direct)", "Tools: 1 present, 3 required missing"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("plugin diagnosis omitted %q: %s", want, &out)
		}
	}
}

func compactDiagnosticFixture(t *testing.T, env *process.Environment, platform string, ghostty bool) (string, managed.Paths) {
	t.Helper()
	source, home, paths := blockHome(t, env, platform)
	root := t.TempDir()
	if err := copyTree(source+"/config", root+"/config"); err != nil {
		t.Fatal(err)
	}
	blockWrite(t, root+"/dependencies.conf", blockRead(t, source+"/dependencies.conf"))
	blockWrite(t, root+"/packages.conf", nil)
	if platform == "macos" {
		choice := "0\n"
		if ghostty {
			choice = "1\n"
		}
		blockWrite(t, paths.State+"/ghostty", []byte(choice))
	}
	blockOK(t, *env, root, "install", "--skip-packages", "--yes")
	for _, name := range []string{"apt-get", "brew", "xcode-select", "gcc"} {
		path := home + "/tools/" + name
		blockWrite(t, path, []byte("#!/bin/sh\nprintf 'fixture 1.0\\n'\n"))
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return root, paths
}

func TestStatusDetectsMissingInstallationRecordForSelectedPlatform(t *testing.T) {
	t.Parallel()
	var env process.Environment
	for _, tc := range []struct {
		platform string
		ghostty  bool
		name     string
	}{
		{"ubuntu", false, "zsh-common"},
		{"macos", false, "zsh-common"},
		{"macos", true, "ghostty-config"},
	} {
		t.Run(tc.platform+tc.name, func(t *testing.T) {
			t.Parallel()
			env := env
			root, paths := compactDiagnosticFixture(t, &env, tc.platform, tc.ghostty)
			blockOK(t, env, root, "status")
			state := blockState(t, paths, tc.name)
			before := blockRead(t, state.Target)
			if err := os.Remove(paths.Resources + "/" + tc.name + ".state"); err != nil {
				t.Fatal(err)
			}
			code, out, stderr := blockRun(t, env, root, "", "status")
			if code != 1 || stderr != "" || !strings.Contains(out, "[MISSING] Installation record: ~/.local/state/selfishell/resources/"+tc.name+".state") || strings.Count(out, "[MISSING]") != 1 {
				t.Fatalf("missing record or optional/platform false positive: code=%d out=%q errors=%q", code, out, stderr)
			}
			if !strings.Contains(out, "1 issues") || strings.Count(out, "Review installation records") != 1 {
				t.Fatal(out)
			}
			blockEqual(t, state.Target, before)
			if _, err := os.Stat(paths.Resources + "/" + tc.name + ".state"); !os.IsNotExist(err) {
				t.Fatalf("diagnostic wrote state: %v", err)
			}
		})
	}
}

func TestDiagnosticsRequireRegularConfiguredMarker(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root, paths := compactDiagnosticFixture(t, &env, "ubuntu", false)
	marker := paths.State + "/configured"
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(marker, 0700); err != nil {
		t.Fatal(err)
	}
	_, out, stderr := blockRun(t, env, root, "", "status")
	if stderr != "" || strings.Contains(out, "configuration is installed") || strings.Contains(out, "Tools:") || strings.Contains(out, "C compiler") {
		t.Fatalf("status treated a directory marker as completed setup: out=%q errors=%q", out, stderr)
	}
}

func TestDiagnosticsSeparateOptionalMissingToolsAndShowOneHint(t *testing.T) {
	t.Parallel()
	var env process.Environment
	for _, required := range []bool{false, true} {
		t.Run(map[bool]string{true: "required", false: "optional"}[required], func(t *testing.T) {
			t.Parallel()
			env := env
			root, _ := compactDiagnosticFixture(t, &env, "ubuntu", false)
			packages := "package all optional direct fzf-tab\n"
			wantCode := 0
			want := "[INFO] Tools: 0 present, 1 optional not installed"
			if required {
				packages += "package all required direct mise\n"
				wantCode = 1
				want = "[ERROR] Tools: 0 present, 1 required missing, 1 optional not installed"
			}
			blockWrite(t, root+"/packages.conf", []byte(packages))
			code, out, stderr := blockRun(t, env, root, "", "status")
			if code != wantCode || stderr != "" || !strings.Contains(out, want) || strings.Contains(out, "[WARN] Tools") || strings.Count(out, "selfishell update --tools-only") != 1 {
				t.Fatalf("tool severity/count/hint: code=%d out=%q errors=%q", code, out, stderr)
			}
		})
	}
}

func TestDoctorIsHiddenStatusAlias(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root, paths := compactDiagnosticFixture(t, &env, "ubuntu", false)
	blockWrite(t, blockState(t, paths, "zsh-common").Target, []byte("personal edit\n"))
	wantCode, wantOut, wantErr := blockRun(t, env, root, "", "status", "--verbose")
	code, out, stderr := blockRun(t, env, root, "", "doctor", "--verbose")
	if code != 1 || code != wantCode || out != wantOut || stderr != wantErr {
		t.Fatalf("doctor diverged from status: code=%d/%d out=%q want %q errors=%q/%q", code, wantCode, out, wantOut, stderr, wantErr)
	}
	for _, want := range []string{"[OK] C compiler: gcc", "System: Ubuntu (", "[CHANGED] ~/.config/selfishell/zsh/common.zsh", "Tools: 0 present"} {
		if !strings.Contains(out, want) {
			t.Fatalf("merged status omitted %q: %s", want, out)
		}
	}
	if help := blockOK(t, env, root, "help"); strings.Contains(help, "doctor") {
		t.Fatalf("help lists the hidden alias: %s", help)
	}
}

func TestHomeDisplayPreservesExternalPaths(t *testing.T) {
	for _, tc := range []struct{ message, want string }{
		{"/home/u/.config", "~/.config"},
		{"open /home/u/.state", "open ~/.state"},
		{"Log: /mnt/home/u/state", "Log: /mnt/home/u/state"},
		{"open \"/home/u/file\"", "open \"~/file\""},
		{"/home/u/link -> /mnt/home/u/reference", "~/link -> /mnt/home/u/reference"},
		{"/home/user/file", "/home/user/file"},
		{"/home/u/a /home/u/b", "~/a ~/b"},
	} {
		if got := displayHome(tc.message, "/home/u"); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.message, got, tc.want)
		}
	}
}
