package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
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
func minimalStatusDiagnosis(got capture) error {
	if got.Status != 1 {
		return fmt.Errorf("status exit %d want 1: stdout %q stderr %q", got.Status, got.Stdout, got.Stderr)
	}
	if !bytes.Contains(got.Stdout, []byte("[ERROR] Tool: mise is missing (direct)")) {
		return fmt.Errorf("status omitted missing mise diagnosis: %q", got.Stdout)
	}
	return nil
}

func setupAndCheck(t *testing.T, f *bootstrapFixture, version string, original []byte) string {
	t.Helper()
	user := filepath.Join(f.home, ".zshrc")
	mustFS(t, testutil.WriteFile(user, original, 0600))
	requireOK(t, f.run(t, "--version", version, "--setup", "--yes", "--skip-packages"))
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+version+"\n")
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
	requireContains(t, status.Stdout, "Current: "+version)
	if err := minimalStatusDiagnosis(status); err != nil {
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
	mustFS(t, testutil.WriteFile(user, original, 0600))
	requireOK(t, f.run(t, "--version", version))
	if got := readBytes(t, user); !bytes.Equal(got, original) {
		t.Fatalf("CLI-only bootstrap changed user bytes: %q", got)
	}
	requireAbsent(t, filepath.Join(f.home, ".config/selfishell"))
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+version+"\n")
	// Guard every system/package/login command even though this flow requests --skip-packages.
	bin := filepath.Join(f.home, "fakebin")
	mustFS(t, os.Mkdir(bin, 0700))
	for _, name := range []string{"apt-get", "apt", "brew", "sudo", "chsh"} {
		mustFS(t, testutil.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf '%s\\n' called >>\"$HOME/system-command-called\"\nexit 70\n"), 0755))
	}
	f.env = append(f.env, "PATH="+bin+":"+f.systembin)
	setupAndCheck(t, f, version, original)
	if got := string(readBytes(t, filepath.Join(f.home, ".local/state/selfishell/resources/user-zshrc.state"))); !strings.HasPrefix(got, "3\n") {
		t.Fatalf("state is not v3: %q", got)
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
	requireAbsent(t, filepath.Join(f.home, "system-command-called"))
}
func TestNativeBuiltArchiveSmoke(t *testing.T) {
	t.Parallel()
	dir := nativeAssetDir(t)
	assertAssetSet(t, dir, nativeArchiveVersion)
	smokePrebuiltArchive(t, dir, nativeArchiveVersion)
}
