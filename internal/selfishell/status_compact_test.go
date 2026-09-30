package selfishell

import (
	"os"
	"strings"
	"testing"
)

func TestCompactStatusReportsMissingRequiredToolAndChangedPath(t *testing.T) {
	source, _, paths := blockHome(t, "macos")
	root := t.TempDir()
	if err := copyTree(source+"/config", root+"/config"); err != nil {
		t.Fatal(err)
	}
	blockWrite(t, root+"/dependencies.conf", blockRead(t, source+"/dependencies.conf"))
	blockWrite(t, root+"/packages.conf", []byte("package all required direct mise\n"))
	blockOK(t, root, "install", "--skip-packages", "--yes")
	s := blockState(t, paths, "zsh-common")
	if err := os.WriteFile(s.Target, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := blockRun(t, root, "", "status")
	if code != 1 || stderr != "" || !strings.Contains(out, "[ERROR] Tool: mise is missing (direct)") || !strings.Contains(out, "[CHANGED] ~/.config/selfishell/zsh/common.zsh") {
		t.Fatalf("hidden problems: code=%d output=%q errors=%q", code, out, stderr)
	}
	if strings.Contains(out, "[OK] ~/.config/selfishell/") || !strings.Contains(out, "1 issues") {
		t.Fatalf("healthy details were not collapsed or issue count was lost: %s", out)
	}
}

func compactDiagnosticFixture(t *testing.T, platform string, ghostty bool) (string, Paths) {
	t.Helper()
	source, home, paths := blockHome(t, platform)
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
	blockOK(t, root, "install", "--skip-packages", "--yes")
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
			root, paths := compactDiagnosticFixture(t, tc.platform, tc.ghostty)
			blockOK(t, root, "status")
			state := blockState(t, paths, tc.name)
			before := blockRead(t, state.Target)
			if err := os.Remove(paths.Resources + "/" + tc.name + ".state"); err != nil {
				t.Fatal(err)
			}
			code, out, stderr := blockRun(t, root, "", "status")
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
	root, paths := compactDiagnosticFixture(t, "ubuntu", false)
	marker := paths.State + "/configured"
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(marker, 0700); err != nil {
		t.Fatal(err)
	}
	_, out, stderr := blockRun(t, root, "", "status")
	if stderr != "" || strings.Contains(out, "configuration is installed") || strings.Contains(out, "Tools:") || strings.Contains(out, "C compiler") {
		t.Fatalf("status treated a directory marker as completed setup: out=%q errors=%q", out, stderr)
	}
}

func TestDiagnosticsSeparateOptionalMissingToolsAndShowOneHint(t *testing.T) {
	for _, required := range []bool{false, true} {
		t.Run(map[bool]string{true: "required", false: "optional"}[required], func(t *testing.T) {
			root, _ := compactDiagnosticFixture(t, "ubuntu", false)
			packages := "package all optional direct zinit\n"
			wantCode := 0
			want := "[INFO] Tools: 0 present, 1 optional not installed"
			if required {
				packages += "package all required direct mise\n"
				wantCode = 1
				want = "[ERROR] Tools: 0 present, 1 required missing, 1 optional not installed"
			}
			blockWrite(t, root+"/packages.conf", []byte(packages))
			code, out, stderr := blockRun(t, root, "", "status")
			if code != wantCode || stderr != "" || !strings.Contains(out, want) || strings.Contains(out, "[WARN] Tools") || strings.Count(out, "selfishell update --tools-only") != 1 {
				t.Fatalf("tool severity/count/hint: code=%d out=%q errors=%q", code, out, stderr)
			}
		})
	}
}

func TestDoctorIsHiddenStatusAlias(t *testing.T) {
	root, paths := compactDiagnosticFixture(t, "ubuntu", false)
	blockWrite(t, blockState(t, paths, "zsh-common").Target, []byte("personal edit\n"))
	wantCode, wantOut, wantErr := blockRun(t, root, "", "status", "--verbose")
	code, out, stderr := blockRun(t, root, "", "doctor", "--verbose")
	if code != 1 || code != wantCode || out != wantOut || stderr != wantErr {
		t.Fatalf("doctor diverged from status: code=%d/%d out=%q want %q errors=%q/%q", code, wantCode, out, wantOut, stderr, wantErr)
	}
	for _, want := range []string{"[OK] C compiler: gcc", "System: Ubuntu (", "[CHANGED] ~/.config/selfishell/zsh/common.zsh", "Tools: 0 present"} {
		if !strings.Contains(out, want) {
			t.Fatalf("merged status omitted %q: %s", want, out)
		}
	}
	if help := blockOK(t, root, "help"); strings.Contains(help, "doctor") {
		t.Fatalf("help lists the hidden alias: %s", help)
	}
}

func TestStatusShortensPathsAndGroupsChangedConfigurationHint(t *testing.T) {
	root, paths := compactDiagnosticFixture(t, "ubuntu", false)
	for _, name := range []string{"zsh-common", "aliases"} {
		state := blockState(t, paths, name)
		blockWrite(t, state.Target, []byte("personal edit\n"))
	}
	code, out, stderr := blockRun(t, root, "", "status")
	if code != 1 || stderr != "" || strings.Contains(out, os.Getenv("HOME")) || !strings.Contains(out, "[CHANGED] ~/.config/selfishell/zsh/common.zsh") || strings.Count(out, "selfishell update --tools-only --skip-packages") != 1 {
		t.Fatalf("path shortening or grouped repair hint: code=%d out=%q errors=%q", code, out, stderr)
	}
}

func TestStatusDetectsLostInstallationRecordDirectory(t *testing.T) {
	root, paths := compactDiagnosticFixture(t, "ubuntu", false)
	if err := os.RemoveAll(paths.Resources); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := blockRun(t, root, "", "status")
	if code != 1 || stderr != "" || !strings.Contains(out, "Configuration: 0 intact, 31 issues") || strings.Count(out, "[MISSING]") != 31 || strings.Count(out, "Review installation records") != 1 {
		t.Fatalf("lost state directory: code=%d out=%q errors=%q", code, out, stderr)
	}
	if _, err := os.Stat(paths.Resources); !os.IsNotExist(err) {
		t.Fatalf("diagnostic recreated state directory: %v", err)
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

func TestDiagnosticErrorsShortenHomePath(t *testing.T) {
	compactDiagnosticFixture(t, "ubuntu", false)
	code, _, stderr := blockRun(t, os.Getenv("HOME")+"/missing-release", "", "status")
	if code != 1 || !strings.Contains(stderr, "~/missing-release/packages.conf") || strings.Contains(stderr, os.Getenv("HOME")) {
		t.Fatalf("diagnostic error path: code=%d errors=%q", code, stderr)
	}
}
