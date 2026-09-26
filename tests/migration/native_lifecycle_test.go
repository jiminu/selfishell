package migration_test

import (
	"archive/tar"
	"bytes"
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func inspectPrebuiltHost(t *testing.T, dir, version string) {
	t.Helper()
	for _, platform := range []string{"linux", "macos"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := fmt.Sprintf("selfishell-%s-%s-%s.tar.gz", version, platform, arch)
			members := readReleaseArchive(t, filepath.Join(dir, name), true)
			if string(members["VERSION"].data) != version+"\n" {
				t.Fatalf("%s VERSION %q", name, members["VERSION"].data)
			}
			binary := members["bin/selfishell"]
			if binary.kind != tar.TypeReg || binary.mode != 0755 {
				t.Fatalf("%s binary kind/mode %d/%o", name, binary.kind, binary.mode)
			}
			path := filepath.Join(t.TempDir(), "selfishell")
			mustFS(t, os.WriteFile(path, binary.data, 0755))
			if platform == "linux" {
				f, e := elf.Open(path)
				mustFS(t, e)
				want := elf.EM_X86_64
				if arch == "arm64" {
					want = elf.EM_AARCH64
				}
				if f.Machine != want {
					t.Fatalf("%s ELF machine %v want %v", name, f.Machine, want)
				}
				mustFS(t, f.Close())
			} else {
				f, e := macho.Open(path)
				mustFS(t, e)
				want := macho.CpuAmd64
				if arch == "arm64" {
					want = macho.CpuArm64
				}
				if f.Cpu != want {
					t.Fatalf("%s Mach-O CPU %v want %v", name, f.Cpu, want)
				}
				mustFS(t, f.Close())
			}
			info, e := buildinfo.ReadFile(path)
			mustFS(t, e)
			settings := map[string]string{}
			for _, item := range info.Settings {
				settings[item.Key] = item.Value
			}
			goos := platform
			if goos == "macos" {
				goos = "darwin"
			}
			if settings["GOOS"] != goos || settings["GOARCH"] != arch || settings["CGO_ENABLED"] != "0" {
				t.Fatalf("%s build settings: %+v", name, settings)
			}
		}
	}

	path := filepath.Join(dir, hostArchive(version))
	members := readReleaseArchive(t, path, true)
	for _, name := range []string{"VERSION", "bin/selfishell", "bin/sfs", "packages.conf", "dependencies.conf", "config/shared/zsh/common.zsh", "config/macos/zshrc", "config/ubuntu/zshrc"} {
		if _, ok := members[name]; !ok {
			t.Fatalf("prebuilt archive missing %s", name)
		}
	}
	if string(members["VERSION"].data) != version+"\n" {
		t.Fatalf("prebuilt VERSION %q", members["VERSION"].data)
	}
	if m := members["bin/sfs"]; m.kind != tar.TypeSymlink || m.link != "selfishell" {
		t.Fatalf("prebuilt sfs: %+v", m)
	}
	if m := members["bin/selfishell"]; m.kind != tar.TypeReg || m.mode != 0755 {
		t.Fatalf("prebuilt executable: kind=%d mode=%o", m.kind, m.mode)
	}
	for name := range members {
		if strings.HasPrefix(name, "lib/") || strings.HasPrefix(name, "cmd/") || strings.HasPrefix(name, "internal/") {
			t.Fatalf("prebuilt archive carries source/runtime engine: %s", name)
		}
	}
	bin := filepath.Join(t.TempDir(), "selfishell")
	mustFS(t, os.WriteFile(bin, members["bin/selfishell"].data, 0755))
	switch runtime.GOOS {
	case "darwin":
		f, e := macho.Open(bin)
		mustFS(t, e)
		want := macho.CpuAmd64
		if runtime.GOARCH == "arm64" {
			want = macho.CpuArm64
		}
		if f.Cpu != want {
			t.Errorf("Mach-O CPU %v want %v", f.Cpu, want)
		}
		mustFS(t, f.Close())
	case "linux":
		f, e := elf.Open(bin)
		mustFS(t, e)
		want := elf.EM_X86_64
		if runtime.GOARCH == "arm64" {
			want = elf.EM_AARCH64
		}
		if f.Machine != want {
			t.Errorf("ELF CPU %v want %v", f.Machine, want)
		}
		mustFS(t, f.Close())
	default:
		t.Fatalf("unsupported runtime host %s", runtime.GOOS)
	}
	info, e := buildinfo.ReadFile(bin)
	mustFS(t, e)
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH || settings["CGO_ENABLED"] != "0" {
		t.Fatalf("prebuilt host settings: %+v", settings)
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
	if doctor.Status != 0 && doctor.Status != 1 {
		t.Fatalf("doctor status %d stderr %q", doctor.Status, doctor.Stderr)
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

func TestActualBashArchiveToGoAndOfflineRestore(t *testing.T) {
	f := newBootstrapFixture(t, nativeArchiveVersion)
	source := filepath.Join(t.TempDir(), "old-bash-export")
	mustFS(t, exportCommit(repoRoot(), fixedBashRelease, source))
	oldAssets := filepath.Join(t.TempDir(), "old-assets")
	mustFS(t, os.Mkdir(oldAssets, 0700))
	built, e := runCommand(f.home, []string{"/bin/bash", filepath.Join(source, "scripts/build-release.sh"), "--version", "1.3.1", "--output", oldAssets}, nil, f.env, 45*time.Second)
	mustFS(t, e)
	requireOK(t, built)
	oldTarget := filepath.Join(f.remote, "download/v1.3.1")
	mustFS(t, os.MkdirAll(oldTarget, 0700))
	for _, name := range []string{hostArchive("1.3.1"), "SHA256SUMS", "VERSION"} {
		mustFS(t, copyFile(filepath.Join(oldAssets, name), filepath.Join(oldTarget, name)))
	}
	original := []byte("alias old='retained'\r\n")
	mustFS(t, os.WriteFile(filepath.Join(f.home, ".zshrc"), original, 0640))
	originalVim := []byte("\" personal vimrc\n")
	vimrc := filepath.Join(f.home, ".config/selfishell/vim/vimrc")
	mustFS(t, os.MkdirAll(filepath.Dir(vimrc), 0700))
	mustFS(t, os.WriteFile(vimrc, originalVim, 0640))
	oldInstall := filepath.Join(source, "install.sh")
	oldBootstrap, e := runCommand(f.home, []string{"/bin/bash", oldInstall, "--prefix", f.prefix, "--version", "1.3.1", "--setup", "--skip-packages", "--yes"}, nil, f.env, 45*time.Second)
	mustFS(t, e)
	requireOK(t, oldBootstrap)
	oldState := readBytes(t, filepath.Join(f.home, ".local/state/selfishell/resources/user-zshrc.state"))
	if !bytes.HasPrefix(oldState, []byte("2\n")) {
		t.Fatalf("old state %q", oldState)
	}
	vimStatePath := filepath.Join(f.home, ".local/state/selfishell/resources/vimrc.state")
	oldVimState := strings.Split(strings.TrimSpace(string(readBytes(t, vimStatePath))), "\n")
	if len(oldVimState) != 7 || oldVimState[5] == "-" {
		t.Fatalf("old original backup missing: %q", oldVimState)
	}
	originalBackup := oldVimState[5]
	if got := readBytes(t, originalBackup); !bytes.Equal(got, originalVim) {
		t.Fatalf("old backup bytes %q", got)
	}
	mustFS(t, os.RemoveAll(source))
	requireOK(t, f.cliRun(t, "update", "--version", nativeArchiveVersion, "--yes", "--skip-packages"))
	requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
	requireLink(t, filepath.Join(f.share, "previous"), "releases/1.3.1")
	managed := readBytes(t, filepath.Join(f.home, ".config/selfishell/vim/vimrc"))
	nativeMembers := readReleaseArchive(t, filepath.Join(nativeAssetDir(t), hostArchive(nativeArchiveVersion)), true)
	if !bytes.Equal(managed, nativeMembers["config/shared/vimrc"].data) {
		t.Fatal("update did not use new release config")
	}
	if got := readBytes(t, filepath.Join(f.home, ".local/state/selfishell/resources/user-zshrc.state")); !bytes.HasPrefix(got, []byte("2\n")) {
		t.Fatalf("new state %q", got)
	}
	newVimState := strings.Split(strings.TrimSpace(string(readBytes(t, vimStatePath))), "\n")
	if len(newVimState) != 7 || newVimState[5] != originalBackup {
		t.Fatalf("Go update changed original backup identity: %q", newVimState)
	}
	assertNoSourceLinks(t, f.home, source)
	f.env = append(f.env, "SELFISHELL_RELEASE_ROOT=file:///definitely-unavailable")
	requireOK(t, f.cliRun(t, "rollback", "--yes"))
	requireLink(t, filepath.Join(f.share, "current"), "releases/1.3.1")
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell 1.3.1\n")
	requireContains(t, f.cliRun(t, "status").Stdout, "Current: 1.3.1 | Rollback: "+nativeArchiveVersion)
	requireOK(t, f.cliRun(t, "uninstall", "--restore", "--yes"))
	assertRestoredOriginal(t, f, original, 0640)
	if got := readBytes(t, vimrc); !bytes.Equal(got, originalVim) {
		t.Fatalf("Bash restore changed original Vim bytes: %q", got)
	}
	vimInfo, e := os.Lstat(vimrc)
	mustFS(t, e)
	if !vimInfo.Mode().IsRegular() || vimInfo.Mode().Perm() != 0640 {
		t.Fatalf("Bash restore changed Vim type/mode: %v", vimInfo.Mode())
	}
	requireAbsent(t, originalBackup)
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
