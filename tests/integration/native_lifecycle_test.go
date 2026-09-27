package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func inspectPrebuiltHost(t *testing.T, dir, version string) {
	t.Helper()
	for _, platform := range []string{"linux", "macos"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := fmt.Sprintf("selfishell-%s-%s-%s.tar.gz", version, platform, arch)
			members := assertConfigPayload(t, filepath.Join(dir, name), version, true)
			inspectNativeBinary(t, platform, arch, version, members["bin/selfishell"].data, false)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

func prebuiltFixture(t *testing.T, dir, version string) *bootstrapFixture {
	t.Helper()
	f := newBootstrapFixture(t)
	target := filepath.Join(f.remote, "download", "v"+version)
	mustFS(t, os.MkdirAll(target, 0700))
	for _, name := range []string{hostArchive(version), "SHA256SUMS", "VERSION"} {
		mustFS(t, copyFile(filepath.Join(dir, name), filepath.Join(target, name)))
	}
	return f
}
func assertNoSourceLinks(t *testing.T, home, removed string) {
	t.Helper()
	for _, path := range []string{home + "/.zshrc", home + "/.vimrc", home + "/.config/selfishell/zsh/zshrc", home + "/.config/selfishell/vim/vimrc"} {
		info, e := os.Lstat(path)
		if os.IsNotExist(e) {
			continue
		}
		mustFS(t, e)
		if info.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(path)
			mustFS(t, e)
			if strings.Contains(target, repoRoot()) || removed != "" && strings.Contains(target, removed) {
				t.Fatalf("managed link points to source: %s -> %s", path, target)
			}
		}
	}
}
func assertRestoredOriginal(t *testing.T, f *bootstrapFixture, original []byte, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(f.home, ".zshrc")
	got := readBytes(t, path)
	if !bytes.Equal(got, original) {
		t.Fatalf("restore changed original user bytes: %q", got)
	}
	info, e := os.Lstat(path)
	mustFS(t, e)
	if !info.Mode().IsRegular() || info.Mode().Perm() != mode {
		t.Fatalf("restore type/mode: %v", info.Mode())
	}
	requireAbsent(t, filepath.Join(f.home, ".config/selfishell/zsh/zshrc"))
}
func minimalDoctorDiagnosis(got capture) error {
	if got.Status != 1 {
		return fmt.Errorf("doctor status %d want 1: stdout %q stderr %q", got.Status, got.Stdout, got.Stderr)
	}
	if !bytes.Contains(got.Stdout, []byte("[ERROR] Tool: mise is missing (direct)")) {
		return fmt.Errorf("doctor omitted missing mise diagnosis: %q", got.Stdout)
	}
	return nil
}

func setupAndCheck(t *testing.T, f *bootstrapFixture, version string, original []byte) string {
	t.Helper()
	user := filepath.Join(f.home, ".zshrc")
	mustFS(t, os.WriteFile(user, original, 0600))
	requireOK(t, f.run(t, "--version", version, "--setup", "--yes", "--skip-packages"))
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+version+"\n")
	help := f.cliRun(t, "help")
	requireOK(t, help)
	requireContains(t, help.Stdout, "selfishell")
	info, e := os.Lstat(user)
	mustFS(t, e)
	if !info.Mode().IsRegular() {
		t.Fatalf("zshrc not user-owned: %v", info.Mode())
	}
	requireContains(t, readBytes(t, user), "# >>> Selfishell initialize >>>")
	requireContains(t, readBytes(t, user), string(original))
	if got := string(readBytes(t, filepath.Join(f.home, ".local/state/selfishell/configured"))); got != "1\n" {
		t.Fatalf("configured %q", got)
	}
	status := f.cliRun(t, "status")
	if status.Status != 0 && status.Status != 1 {
		t.Fatalf("status command: %d %q", status.Status, status.Stderr)
	}
	requireContains(t, status.Stdout, "Current: "+version)
	doctor := f.cliRun(t, "doctor")
	if err := minimalDoctorDiagnosis(doctor); err != nil {
		t.Fatal(err)
	}
	assertNoSourceLinks(t, f.home, "")
	return user
}
func smokePrebuiltArchive(t *testing.T, dir, version string) {
	t.Helper()
	inspectPrebuiltHost(t, dir, version)
	f := prebuiltFixture(t, dir, version)
	if _, err := resolveCommand("go", f.env); err == nil {
		t.Fatal("installed child PATH contains Go")
	}
	original := []byte("alias personal='kept'\r\n")
	user := filepath.Join(f.home, ".zshrc")
	mustFS(t, os.WriteFile(user, original, 0600))
	requireOK(t, f.run(t, "--version", version))
	if got := readBytes(t, user); !bytes.Equal(got, original) {
		t.Fatalf("CLI-only bootstrap changed user bytes: %q", got)
	}
	requireAbsent(t, filepath.Join(f.home, ".config/selfishell"))
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+version+"\n")
	requireContains(t, f.cliRun(t, "help").Stdout, "selfishell")
	setupAndCheck(t, f, version, original)
	if got := string(readBytes(t, filepath.Join(f.home, ".local/state/selfishell/resources/user-zshrc.state"))); !strings.HasPrefix(got, "2\n") {
		t.Fatalf("state is not v2: %q", got)
	}
	// Release phase is unavailable: tools-only --skip-packages must use installed bytes.
	f.env = append(f.env, "SELFISHELL_RELEASE_ROOT=file:///definitely-unavailable")
	requireOK(t, f.cliRun(t, "update", "--tools-only", "--skip-packages", "--yes"))
	requireContains(t, readBytes(t, user), string(original))
	requireOK(t, f.cliRun(t, "uninstall", "--restore", "--yes"))
	assertRestoredOriginal(t, f, original, 0600)
	requireOK(t, f.cliRun(t, "uninstall", "--purge", "--yes"))
	requireAbsent(t, f.cli)
	requireAbsent(t, f.share)
}
func TestNativeBuiltArchiveSmoke(t *testing.T) {
	dir := nativeAssetDir(t)
	assertAssetSet(t, dir, nativeArchiveVersion)
	smokePrebuiltArchive(t, dir, nativeArchiveVersion)
}

func TestNativeGoToGoExactUpdateRollbackRestore(t *testing.T) {
	f := newBootstrapFixture(t, nativeArchiveVersion, nextNativeVersion)
	original := []byte("alias mine='kept'\r\n")
	setupAndCheck(t, f, nativeArchiveVersion, original)
	beforeState := readBytes(t, filepath.Join(f.home, ".local/state/selfishell/resources/user-zshrc.state"))
	requireOK(t, f.cliRun(t, "update", "--version", nextNativeVersion, "--yes", "--skip-packages"))
	requireLink(t, filepath.Join(f.share, "current"), "releases/"+nextNativeVersion)
	requireLink(t, filepath.Join(f.share, "previous"), "releases/"+nativeArchiveVersion)
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+nextNativeVersion+"\n")
	requireContains(t, f.cliRun(t, "status").Stdout, "Current: "+nextNativeVersion+" | Rollback: "+nativeArchiveVersion)
	if got := readBytes(t, filepath.Join(f.home, ".local/state/selfishell/resources/user-zshrc.state")); !bytes.HasPrefix(got, []byte("2\n")) {
		t.Fatalf("post-update state %q", got)
	}
	if !bytes.HasPrefix(beforeState, []byte("2\n")) {
		t.Fatalf("initial state %q", beforeState)
	}
	assertNoSourceLinks(t, f.home, "")
	f.env = append(f.env, "SELFISHELL_RELEASE_ROOT=file:///definitely-unavailable")
	requireOK(t, f.cliRun(t, "rollback", "--yes"))
	requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+nativeArchiveVersion+"\n")
	requireOK(t, f.cliRun(t, "uninstall", "--restore", "--yes"))
	assertRestoredOriginal(t, f, original, 0600)
}

func TestPrebuiltSmokeRejectsBadInputs(t *testing.T) {
	// Execute the entrypoint as a child so t.Setenv cannot mask parent variables.
	for _, tc := range []struct {
		name, dir, version string
		want               string
	}{
		{"missing directory", "", "1.3.2", "both prebuilt release variables"},
		{"relative directory", "relative", "1.3.2", "must be absolute"},
		{"invalid version", "/tmp", "01.2.3", "invalid release version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestExactReleaseSmoke$", "-test.count=1", "-test.v")
			cmd.Env = append(os.Environ(), "SELFISHELL_TEST_RELEASE_DIR="+tc.dir, "SELFISHELL_TEST_RELEASE_VERSION="+tc.version)
			out, e := cmd.CombinedOutput()
			if e == nil || !bytes.Contains(out, []byte(tc.want)) {
				t.Fatalf("expected %q failure: %v %s", tc.want, e, out)
			}
		})
	}
}

func TestNativeCompleteReleaseLifecycle(t *testing.T) {
	f := newBootstrapFixture(t, nativeArchiveVersion, nextNativeVersion)
	bin := filepath.Join(f.home, "fakebin")
	mustFS(t, os.Mkdir(bin, 0700))
	for _, name := range []string{"starship", "mise", "fzf", "zoxide", "rg", "jq", "nvim", "tree-sitter", "node", "python", "uv", "gh", "lazygit", "gcc", "build-essential"} {
		mustFS(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0755))
	}
	// Guard every system/package/login command even though this flow requests --skip-packages.
	for _, name := range []string{"apt-get", "apt", "brew", "sudo", "chsh"} {
		mustFS(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf '%s\\n' called >>\"$HOME/system-command-called\"\nexit 70\n"), 0755))
	}
	f.env = append(f.env, "PATH="+bin+":"+f.systembin)
	mustFS(t, os.WriteFile(filepath.Join(bin, "dpkg-query"), []byte("#!/bin/sh\nprintf 'ca-certificates\\tii \\t1.0\\n'\n"), 0755))
	mustFS(t, os.MkdirAll(filepath.Join(f.home, ".local/bin"), 0700))
	mustFS(t, os.WriteFile(filepath.Join(f.home, ".local/bin/mise"), []byte("#!/bin/sh\nexit 0\n"), 0755))
	zinit := filepath.Join(f.home, ".local/share/zinit/zinit.git")
	mustFS(t, os.MkdirAll(zinit, 0700))
	mustFS(t, os.WriteFile(filepath.Join(zinit, "zinit.zsh"), []byte(":\n"), 0644))
	manifest := filepath.Join(f.home, "dependencies.conf")
	source := strings.Split(string(readBytes(t, filepath.Join(repoRoot(), "dependencies.conf"))), "\n")
	var rewritten strings.Builder
	for _, line := range source {
		if !strings.HasPrefix(line, "zsh-plugin ") {
			if line != "" {
				rewritten.WriteString(line + "\n")
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			t.Fatalf("bad plugin declaration %q", line)
		}
		plugin := filepath.Join(f.home, ".local/share/zinit/plugins", strings.ReplaceAll(fields[1], "/", "---"))
		mustFS(t, os.MkdirAll(plugin, 0700))
		for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "test"}, {"commit", "--quiet", "--allow-empty", "-m", "initial"}} {
			got, e := runCommand(f.home, append([]string{"git", "-C", plugin}, args...), nil, f.env, 10*time.Second)
			mustFS(t, e)
			requireOK(t, got)
		}
		rev, e := runCommand(f.home, []string{"git", "-C", plugin, "rev-parse", "HEAD"}, nil, f.env, 10*time.Second)
		mustFS(t, e)
		requireOK(t, rev)
		fmt.Fprintf(&rewritten, "zsh-plugin %s %s all all - - - -\n", fields[1], strings.TrimSpace(string(rev.Stdout)))
	}
	mustFS(t, os.WriteFile(manifest, []byte(rewritten.String()), 0600))
	f.env = append(f.env, "SELFISHELL_DEPENDENCIES_FILE="+manifest)
	original := []byte("original zshrc\n")
	mustFS(t, os.WriteFile(filepath.Join(f.home, ".zshrc"), original, 0600))
	requireOK(t, f.run(t, "--version", nativeArchiveVersion, "--setup", "--yes", "--skip-packages"))
	requireOK(t, f.cliRun(t, "doctor"))
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+nativeArchiveVersion)
	zshrc := filepath.Join(f.home, ".zshrc")
	info, e := os.Lstat(zshrc)
	mustFS(t, e)
	if !info.Mode().IsRegular() {
		t.Fatal("original zshrc no longer regular")
	}
	requireContains(t, readBytes(t, zshrc), "# >>> Selfishell initialize >>>")
	requireOK(t, f.cliRun(t, "update", "--cli-only", "--version", nextNativeVersion, "--yes"))
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+nextNativeVersion)
	f.env = append(f.env, "SELFISHELL_RELEASE_ROOT=file:///network-must-not-be-used")
	requireOK(t, f.cliRun(t, "rollback", "--yes"))
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+nativeArchiveVersion)
	requireOK(t, f.cliRun(t, "uninstall", "--restore", "--yes"))
	assertRestoredOriginal(t, f, original, 0600)
	requireAbsent(t, filepath.Join(f.home, "system-command-called"))
}

func TestPrebuiltSmokeRejectsCorruptedArchive(t *testing.T) {
	source := nativeAssetDir(t)
	dir := t.TempDir()
	for _, name := range append(releaseAssetNames(nativeArchiveVersion), "SHA256SUMS", "VERSION") {
		mustFS(t, copyFile(filepath.Join(source, name), filepath.Join(dir, name)))
	}
	archive := filepath.Join(dir, hostArchive(nativeArchiveVersion))
	file, e := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0)
	mustFS(t, e)
	_, e = file.WriteString("corrupt")
	mustFS(t, e)
	mustFS(t, file.Close())
	cmd := exec.Command(os.Args[0], "-test.run=^TestExactReleaseSmoke$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), "SELFISHELL_TEST_RELEASE_DIR="+dir, "SELFISHELL_TEST_RELEASE_VERSION="+nativeArchiveVersion)
	out, e := cmd.CombinedOutput()
	if e == nil || !bytes.Contains(out, []byte("SHA256SUMS mismatch")) {
		t.Fatalf("corrupt exact asset accepted: %v %s", e, out)
	}
}

func TestPrebuiltSmokeConsumesSuppliedArbitraryVersion(t *testing.T) {
	dir := nativeVersionAssets(t, nextNativeVersion)
	cmd := exec.Command(os.Args[0], "-test.run=^TestExactReleaseSmoke$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(),
		"SELFISHELL_TEST_RELEASE_DIR="+dir,
		"SELFISHELL_TEST_RELEASE_VERSION="+nextNativeVersion,
		"PATH="+t.TempDir())
	out, e := cmd.CombinedOutput()
	if e != nil || !bytes.Contains(out, []byte("--- PASS: TestExactReleaseSmoke")) {
		t.Fatalf("prebuilt arbitrary-version smoke: %v %s", e, out)
	}
}
