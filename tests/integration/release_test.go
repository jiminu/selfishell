package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func releaseEnv(t *testing.T, home, remote string) []string {
	t.Helper()
	osRelease := filepath.Join(home, "os-release")
	mustFS(t, testutil.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	return []string{
		"SELFISHELL_RELEASE_ROOT=file://" + remote,
		"SELFISHELL_TEST_SYSTEM_NAME=Linux",
		"SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease,
		"SELFISHELL_TEST_PROC_VERSION_FILE=" + osRelease,
		"MISE_DATA_DIR=" + home + "/mise/data",
		"MISE_CACHE_DIR=" + home + "/mise/cache",
		"MISE_CONFIG_DIR=" + home + "/mise/config",
		"MISE_STATE_DIR=" + home + "/mise/state",
	}
}

func installedFixture(t *testing.T, home, version, executable string) (string, string) {
	t.Helper()
	share := filepath.Join(home, ".local/share/selfishell")
	root := filepath.Join(share, "releases", version)
	mustFS(t, os.MkdirAll(root+"/bin", 0700))
	mustFS(t, copyFile(executable, root+"/bin/selfishell"))
	mustFS(t, testutil.WriteFile(root+"/VERSION", []byte(version+"\n"), 0644))
	for _, name := range []string{"config", "packages.conf", "dependencies.conf"} {
		from, to := filepath.Join(repoRoot(), name), filepath.Join(root, name)
		if name == "config" {
			mustFS(t, copyTree(from, to))
		} else {
			mustFS(t, copyFile(from, to))
		}
	}
	mustFS(t, os.Symlink("releases/"+version, share+"/current"))
	return root, share
}

func archiveFixture(t *testing.T, remote, version, executable, marker string, packagesOverride ...string) {
	t.Helper()
	payload := t.TempDir()
	mustFS(t, os.MkdirAll(payload+"/bin", 0700))
	mustFS(t, copyFile(executable, payload+"/bin/selfishell"))
	mustFS(t, testutil.WriteFile(payload+"/VERSION", []byte(version+"\n"), 0644))
	for _, name := range []string{"config", "packages.conf", "dependencies.conf"} {
		from, to := filepath.Join(repoRoot(), name), filepath.Join(payload, name)
		if name == "config" {
			mustFS(t, copyTree(from, to))
		} else {
			mustFS(t, copyFile(from, to))
		}
	}
	if len(packagesOverride) != 0 {
		mustFS(t, testutil.WriteFile(payload+"/packages.conf", []byte(packagesOverride[0]), 0644))
	}
	if marker != "" {
		mustFS(t, testutil.AppendFile(payload+"/config/shared/vimrc", []byte("\n\" "+marker+"\n")))
	}
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	name := fmt.Sprintf("selfishell-%s-%s-%s.tar.gz", version, platform, runtime.GOARCH)
	dir := filepath.Join(remote, "download", "v"+version)
	mustFS(t, os.MkdirAll(dir, 0700))
	archive := filepath.Join(dir, name)
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	w := tar.NewWriter(gz)
	err := filepath.WalkDir(payload, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == payload {
			return nil
		}
		rel, err := filepath.Rel(payload, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			h.Name += "/"
		}
		if err := w.WriteHeader(h); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, in)
			closeErr := in.Close()
			if err != nil {
				return err
			}
			return closeErr
		}
		return nil
	})
	mustFS(t, err)
	mustFS(t, w.Close())
	mustFS(t, gz.Close())
	mustFS(t, testutil.WriteFile(archive, data.Bytes(), 0644))
	digest := sha256.Sum256(data.Bytes())
	mustFS(t, testutil.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(hex.EncodeToString(digest[:])+"  "+name+"\n"), 0644))
}

func TestGoUpdateContinuationForwardsArgumentsStreamsAndStatus(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	home := t.TempDir()
	root, share := installedFixture(t, home, "1.0.0", cli)
	remote := t.TempDir()
	stub := filepath.Join(t.TempDir(), "selfishell")
	mustFS(t, testutil.WriteFile(stub, []byte("#!/bin/sh\nprintf 'argv:%s\\n' \"$*\"\ncat\nprintf 'child-stderr\\n' >&2\nexit 7\n"), 0755))
	archiveFixture(t, remote, "2.0.0", stub, "")
	got, err := runCommand(home, []string{root + "/bin/selfishell", "update", "--version", "2.0.0", "--yes", "--skip-packages"}, []byte("child-input\n"), releaseEnv(t, home, remote), 20*time.Second)
	mustFS(t, err)
	if got.Status != 7 || !bytes.Contains(got.Stdout, []byte("argv:update --continue-after-cli-update --yes --skip-packages\nchild-input\n")) || !bytes.Contains(got.Stderr, []byte("child-stderr\n")) {
		t.Fatalf("continuation: status %d stdout %q stderr %q", got.Status, got.Stdout, got.Stderr)
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/2.0.0" {
		t.Fatalf("wrong selected release: %s", current)
	}
}

func TestGoToGoUpdateUsesNewRootAndOfflineRollback(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	home := t.TempDir()
	root, share := installedFixture(t, home, "1.0.0", cli)
	remote := t.TempDir()
	archiveFixture(t, remote, "2.0.0", cli, "go-new-root")
	env := releaseEnv(t, home, remote)
	original := []byte("\" personal Vim configuration\r\n")
	vimrc := filepath.Join(home, ".config/selfishell/vim/vimrc")
	mustFS(t, os.MkdirAll(filepath.Dir(vimrc), 0700))
	mustFS(t, testutil.WriteFile(vimrc, original, 0640))
	statePath := filepath.Join(home, ".local/state/selfishell/resources/vimrc.state")
	backupPath := func() string {
		t.Helper()
		fields := strings.Split(strings.TrimSuffix(string(readBytes(t, statePath)), "\n"), "\n")
		if len(fields) != 7 || fields[5] == "-" || fields[5] == "" {
			t.Fatalf("missing original backup: %q", fields)
		}
		return fields[5]
	}
	setup, err := runCommand(home, []string{root + "/bin/selfishell", "install", "--skip-packages", "--yes"}, nil, env, 20*time.Second)
	mustFS(t, err)
	if setup.Status != 0 {
		t.Fatalf("setup: %d %q", setup.Status, setup.Stderr)
	}
	backup := backupPath()
	assertBackup := func() {
		t.Helper()
		if got := backupPath(); got != backup {
			t.Fatalf("backup path changed: %q -> %q", backup, got)
		}
		if got := readBytes(t, backup); !bytes.Equal(got, original) {
			t.Fatalf("original backup changed: %q", got)
		}
		info, err := os.Lstat(backup)
		mustFS(t, err)
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0640 {
			t.Fatalf("backup type/mode changed: %v", info.Mode())
		}
	}
	assertBackup()
	updated, err := runCommand(home, []string{root + "/bin/selfishell", "update", "--version", "2.0.0", "--yes", "--skip-packages"}, nil, env, 25*time.Second)
	mustFS(t, err)
	if updated.Status != 0 || strings.Count(string(updated.Stdout), "Selfishell updated:") != 1 {
		t.Fatalf("update: %d %q %q", updated.Status, updated.Stdout, updated.Stderr)
	}
	assertBackup()
	managed, err := os.ReadFile(home + "/.config/selfishell/vim/vimrc")
	mustFS(t, err)
	if !bytes.Contains(managed, []byte("go-new-root")) {
		t.Fatal("continuation used old configuration")
	}
	current, _ := os.Readlink(share + "/current")
	previous, _ := os.Readlink(share + "/previous")
	if current != "releases/2.0.0" || previous != "releases/1.0.0" {
		t.Fatalf("links after update: %s %s", current, previous)
	}
	status, err := runCommand(home, []string{share + "/current/bin/selfishell", "status"}, nil, env, 10*time.Second)
	mustFS(t, err)
	if !bytes.Contains(status.Stdout, []byte("Current: 2.0.0 | Rollback: 1.0.0")) {
		t.Fatalf("updated status: %q %q", status.Stdout, status.Stderr)
	}
	rolled, err := runCommand(home, []string{share + "/current/bin/selfishell", "rollback", "--yes"}, nil, append(env, "SELFISHELL_RELEASE_ROOT=file:///unavailable"), 20*time.Second)
	mustFS(t, err)
	if rolled.Status != 0 {
		t.Fatalf("rollback: %d %q %q", rolled.Status, rolled.Stdout, rolled.Stderr)
	}
	current, _ = os.Readlink(share + "/current")
	if current != "releases/1.0.0" {
		t.Fatalf("rollback changed wrong release: %s", current)
	}
	status, err = runCommand(home, []string{share + "/current/bin/selfishell", "status"}, nil, env, 10*time.Second)
	mustFS(t, err)
	if !bytes.Contains(status.Stdout, []byte("Current: 1.0.0 | Rollback: 2.0.0")) {
		t.Fatalf("rolled status: %q %q", status.Stdout, status.Stderr)
	}
	assertBackup()
	restored, err := runCommand(home, []string{share + "/current/bin/selfishell", "uninstall", "--restore", "--yes"}, nil, append(env, "SELFISHELL_RELEASE_ROOT=file:///unavailable"), 20*time.Second)
	mustFS(t, err)
	requireOK(t, restored)
	if got := readBytes(t, vimrc); !bytes.Equal(got, original) {
		t.Fatalf("restore changed original Vim bytes: %q", got)
	}
	info, err := os.Lstat(vimrc)
	mustFS(t, err)
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0640 {
		t.Fatalf("restore changed Vim type/mode: %v", info.Mode())
	}
	requireAbsent(t, backup)
	requireAbsent(t, statePath)
}

func TestContinuationUsesResolvedExecutableWhenCurrentChanges(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	home := t.TempDir()
	root, share := installedFixture(t, home, "1.0.0", cli)
	remote := t.TempDir()
	archiveFixture(t, remote, "2.0.0", cli, "intended-root")
	env := releaseEnv(t, home, remote)
	setup, err := runCommand(home, []string{root + "/bin/selfishell", "install", "--skip-packages", "--yes"}, nil, env, 20*time.Second)
	mustFS(t, err)
	if setup.Status != 0 {
		t.Fatalf("setup: %d %q", setup.Status, setup.Stderr)
	}
	// Materialize the intended target, then simulate another process changing current.
	update, err := runCommand(home, []string{root + "/bin/selfishell", "update", "--version", "2.0.0", "--cli-only", "--yes"}, nil, env, 20*time.Second)
	mustFS(t, err)
	if update.Status != 0 {
		t.Fatalf("CLI update: %d %q", update.Status, update.Stderr)
	}
	mustFS(t, os.Remove(share+"/current"))
	mustFS(t, os.Symlink("releases/1.0.0", share+"/current"))
	continued, err := runCommand(home, []string{share + "/releases/2.0.0/bin/selfishell", "update", "--continue-after-cli-update", "--yes", "--skip-packages"}, nil, env, 20*time.Second)
	mustFS(t, err)
	if continued.Status != 0 {
		t.Fatalf("continuation: %d %q", continued.Status, continued.Stderr)
	}
	managed, err := os.ReadFile(home + "/.config/selfishell/vim/vimrc")
	mustFS(t, err)
	if !bytes.Contains(managed, []byte("intended-root")) {
		t.Fatal("continued from changed current instead of executable root")
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/1.0.0" {
		t.Fatalf("continuation rewrote current: %s", current)
	}
}

func TestDefaultUpdateWithoutSetupOnlyChangesCLI(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	home := t.TempDir()
	root, share := installedFixture(t, home, "1.0.0", cli)
	remote := t.TempDir()
	archiveFixture(t, remote, "2.0.0", cli, "unused-no-setup")
	got, err := runCommand(home, []string{root + "/bin/selfishell", "update", "--version", "2.0.0", "--yes"}, nil, releaseEnv(t, home, remote), 20*time.Second)
	mustFS(t, err)
	if got.Status != 0 || !bytes.Contains(got.Stdout, []byte("configuration is not installed; skipping tools and configuration")) || strings.Count(string(got.Stdout), "Selfishell updated:") != 1 {
		t.Fatalf("no setup: %d %q %q", got.Status, got.Stdout, got.Stderr)
	}
	if bytes.Index(got.Stdout, []byte("configuration is not installed; skipping tools and configuration")) > bytes.Index(got.Stdout, []byte("Selfishell updated:")) {
		t.Fatalf("skip message follows transition: %q", got.Stdout)
	}
	if _, err := os.Lstat(home + "/.config/selfishell"); !os.IsNotExist(err) {
		t.Fatalf("no setup installed configuration: %v", err)
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/2.0.0" {
		t.Fatalf("no setup release: %s", current)
	}
}

func TestDefaultUpdateRejectsUnreadableSetupMarkerWithoutTransition(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	home := t.TempDir()
	root, share := installedFixture(t, home, "1.0.0", cli)
	marker := home + "/.local/state/selfishell/configured"
	mustFS(t, os.MkdirAll(marker, 0700))
	remote := t.TempDir()
	archiveFixture(t, remote, "2.0.0", cli, "unused-bad-marker")
	got, err := runCommand(home, []string{root + "/bin/selfishell", "update", "--version", "2.0.0", "--skip-packages", "--yes"}, nil, releaseEnv(t, home, remote), 20*time.Second)
	mustFS(t, err)
	if got.Status != 1 || bytes.Contains(got.Stdout, []byte("Selfishell updated")) || bytes.Contains(got.Stdout, []byte("skipping tools and configuration")) || !bytes.Contains(got.Stderr, []byte("configured marker")) {
		t.Fatalf("bad marker claimed success: %d %q %q", got.Status, got.Stdout, got.Stderr)
	}
	current, err := os.Readlink(share + "/current")
	mustFS(t, err)
	if current != "releases/2.0.0" {
		t.Fatalf("CLI activation was not preserved after child failure: %s", current)
	}
}

func TestContinuationRequiredPhaseFailureKeepsChildStatusAndNoSuccess(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	home := t.TempDir()
	root, share := installedFixture(t, home, "1.0.0", cli)
	remote := t.TempDir()
	archiveFixture(t, remote, "2.0.0", cli, "", "package ubuntu required apt fixture-required\n")
	env := releaseEnv(t, home, remote)
	setup, err := runCommand(home, []string{root + "/bin/selfishell", "install", "--skip-packages", "--yes"}, nil, env, 20*time.Second)
	mustFS(t, err)
	if setup.Status != 0 {
		t.Fatalf("setup: %d %q", setup.Status, setup.Stderr)
	}
	bin := filepath.Join(home, "fakebin")
	mustFS(t, os.Mkdir(bin, 0700))
	for name, body := range map[string]string{
		"dpkg-query": "#!/bin/sh\nexit 1\n",
		"apt-get":    "#!/bin/sh\nexit 1\n",
		"sudo":       "#!/bin/sh\nexec \"$@\"\n",
	} {
		mustFS(t, testutil.WriteFile(bin+"/"+name, []byte(body), 0755))
	}
	got, err := runCommand(home, []string{root + "/bin/selfishell", "update", "--version", "2.0.0", "--yes"}, nil, append(env, "PATH="+bin+":/usr/bin:/bin"), 20*time.Second)
	mustFS(t, err)
	if got.Status != 1 || bytes.Contains(got.Stdout, []byte("Selfishell updated:")) || !bytes.Contains(got.Stderr, []byte("apt")) {
		t.Fatalf("child failure: %d %q %q", got.Status, got.Stdout, got.Stderr)
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/2.0.0" {
		t.Fatalf("CLI not activated before child failure: %s", current)
	}
}

func TestContinuationPreservesTerminalOutput(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	home := t.TempDir()
	root, _ := installedFixture(t, home, "1.0.0", cli)
	remote := t.TempDir()
	stub := filepath.Join(t.TempDir(), "selfishell")
	mustFS(t, testutil.WriteFile(stub, []byte("#!/bin/sh\nif [ -t 1 ]; then printf 'child-stdout-is-tty\\n'; else printf 'child-lost-tty\\n'; fi\n"), 0755))
	archiveFixture(t, remote, "2.0.0", stub, "")
	got, err := capturePTYOutput(home, root+"/bin/selfishell", []string{"update", "--version", "2.0.0", "--yes"}, releaseEnv(t, home, remote))
	mustFS(t, err)
	if got.Status != 0 || !bytes.Contains(got.Stdout, []byte("child-stdout-is-tty")) || bytes.Contains(got.Stdout, []byte("child-lost-tty")) {
		t.Fatalf("tty: %d %q %q", got.Status, got.Stdout, got.Stderr)
	}
}

func TestToolsOnlyOverwritesChangedSourceWithConflictBackupAndChecksum(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	home := t.TempDir()
	root, share := installedFixture(t, home, "1.0.0", cli)
	remote := t.TempDir()
	archiveFixture(t, remote, "2.0.0", cli, "changed-source-vimrc")
	env := releaseEnv(t, home, remote)
	setup, err := runCommand(home, []string{root + "/bin/selfishell", "install", "--skip-packages", "--yes"}, nil, env, 20*time.Second)
	mustFS(t, err)
	if setup.Status != 0 {
		t.Fatalf("setup: %d %q", setup.Status, setup.Stderr)
	}
	selected, err := runCommand(home, []string{root + "/bin/selfishell", "update", "--cli-only", "--version", "2.0.0", "--yes"}, nil, env, 20*time.Second)
	mustFS(t, err)
	if selected.Status != 0 {
		t.Fatalf("CLI selection: %d %q", selected.Status, selected.Stderr)
	}
	target := home + "/.config/selfishell/vim/vimrc"
	state := home + "/.local/state/selfishell/resources/vimrc.state"
	before, err := os.ReadFile(state)
	mustFS(t, err)
	mustFS(t, testutil.WriteFile(target, []byte("user_modified_vimrc\n"), 0600))
	updated, err := runCommand(home, []string{share + "/current/bin/selfishell", "update", "--tools-only", "--skip-packages"}, []byte("y\n"), append(env, "SELFISHELL_TEST_TTY=1"), 20*time.Second)
	mustFS(t, err)
	if updated.Status != 0 || !bytes.Contains(updated.Stdout, []byte("Selfishell tools and configuration synchronized")) {
		t.Fatalf("changed source: %d %q %q", updated.Status, updated.Stdout, updated.Stderr)
	}
	if bytes.Count(updated.Stdout, []byte("[y/N]")) != 1 || !bytes.Contains(updated.Stdout, []byte("Managed file was modified:")) {
		t.Fatalf("expected only the modified-file prompt: %q", updated.Stdout)
	}
	source, err := os.ReadFile(share + "/releases/2.0.0/config/shared/vimrc")
	mustFS(t, err)
	managed, err := os.ReadFile(target)
	mustFS(t, err)
	if !bytes.Equal(managed, source) {
		t.Fatal("managed file did not take new release bytes")
	}
	backups, err := filepath.Glob(home + "/.local/state/selfishell/backups/vimrc.backup.*")
	mustFS(t, err)
	if len(backups) != 1 {
		t.Fatalf("missing conflict backup: %v", backups)
	}
	backup, err := os.ReadFile(backups[0])
	mustFS(t, err)
	if string(backup) != "user_modified_vimrc\n" {
		t.Fatalf("backup lost personal bytes: %q", backup)
	}
	after, err := os.ReadFile(state)
	mustFS(t, err)
	if bytes.Equal(before, after) {
		t.Fatal("state checksum not refreshed")
	}
	cksum, err := runCommand(home, []string{"cksum"}, source, nil, 10*time.Second)
	mustFS(t, err)
	fields := strings.Fields(string(cksum.Stdout))
	lines := strings.Split(strings.TrimSuffix(string(after), "\n"), "\n")
	if cksum.Status != 0 || len(fields) != 2 || len(lines) < 7 || lines[6] != fields[0]+":"+fields[1] {
		t.Fatalf("state checksum does not match new source: %q vs %q", after, cksum.Stdout)
	}
}
