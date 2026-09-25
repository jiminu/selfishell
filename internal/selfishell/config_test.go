package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCLI(t *testing.T, root, home string, args ...string) (int, string, string) {
	t.Helper()
	isolateHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	var out, err bytes.Buffer
	code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &err}).Run(args)
	return code, out.String(), err.String()
}
func TestManifestValidationBeforeInstall(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	os.Mkdir(home, 0700)
	os.WriteFile(filepath.Join(root, "packages.conf"), []byte("package all required mise starship\nexecute unsafe\n"), 0600)
	os.WriteFile(filepath.Join(root, "dependencies.conf"), []byte("git zinit v1 all all https://example.org/repo abc .local/share/zinit/zinit.git zinit.zsh\n"), 0600)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(err, "Unknown package manifest record") {
		t.Fatalf("%d %s", code, err)
	}
	if _, e := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(e) {
		t.Fatal("install mutated HOME")
	}
}
func TestInstallRequiresSkipPackages(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--yes")
	if code != 1 || !strings.Contains(err, "--skip-packages") {
		t.Fatalf("%d %s", code, err)
	}
}
func TestInstallAndRestoreExistingResources(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	original := []byte("alias own='yes'\r\n")
	os.WriteFile(filepath.Join(home, ".zshrc"), original, 0600)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("install %d %s", code, err)
	}
	installed, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if !bytes.HasSuffix(installed, original) {
		t.Fatalf("lost original bytes: %q", installed)
	}
	info, _ := os.Stat(filepath.Join(home, ".zshrc"))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	code, _, err = testCLI(t, root, home, "uninstall", "--restore", "--yes")
	if code != 0 {
		t.Fatalf("uninstall %d %s", code, err)
	}
	restored, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if !bytes.Equal(restored, original) {
		t.Fatalf("restored %q", restored)
	}
}
func testRelease(t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	root := filepath.Clean(filepath.Join(cwd, "../.."))
	return root
}
func TestInstallDryRunHasNoEffects(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--dry-run")
	if code != 0 {
		t.Fatal(code, err)
	}
	entries, e := os.ReadDir(home)
	if e != nil || len(entries) != 0 {
		t.Fatalf("dry-run changed HOME: %v %v", entries, e)
	}
}
func TestChangedManagedFileStopsBeforeOtherWrites(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(code, err)
	}
	paths, _ := UserPaths()
	target := paths.Config + "/zsh/runtime.zsh"
	os.WriteFile(target, []byte("user change"), 0600)
	later := paths.Config + "/zsh/common.zsh"
	os.Remove(later)
	code, _, err = testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(err, "Managed file was modified") {
		t.Fatalf("%d %s", code, err)
	}
	if _, e := os.Stat(later); !os.IsNotExist(e) {
		t.Fatal("preflight allowed earlier writes")
	}
}
func TestPendingFileInstallRecoversOriginalBackup(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	resources, _ := ResourcesForPlatform(root, "macos", false)
	r := resources[0]
	paths, _ := UserPaths()
	os.MkdirAll(filepath.Dir(r.Target), 0700)
	os.WriteFile(r.Target, []byte("original"), 0600)
	backup := r.Target + ".backup.fixed"
	if e := WriteState(paths.Resources+"/"+r.Name+".state", State{"file", "pending", r.Target, "-", backup, "123:1"}); e != nil {
		t.Fatal(e)
	}
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(code, err)
	}
	original, e := os.ReadFile(backup)
	if e != nil || string(original) != "original" {
		t.Fatalf("backup %q %v", original, e)
	}
	state, e := ReadState(paths.Resources + "/" + r.Name + ".state")
	if e != nil || state.Backup != backup || state.Status != "active" {
		t.Fatalf("%+v %v", state, e)
	}
}
func TestUninstallPreflightPreservesAllResources(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(code, err)
	}
	isolateHome(t, home)
	paths, _ := UserPaths()
	protected := paths.Config + "/zsh/zshrc"
	os.WriteFile(protected, []byte("changed"), 0600)
	code, _, err = testCLI(t, root, home, "uninstall", "--yes")
	if code != 1 || !strings.Contains(err, "Uninstall cancelled") {
		t.Fatalf("%d %s", code, err)
	}
	if _, e := os.Lstat(home + "/.vimrc"); e != nil {
		t.Fatal("earlier user entrypoint removed", e)
	}
}

func TestUninstallMalformedBlockExplainsPreservation(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	if code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes"); code != 0 {
		t.Fatalf("install: %s", err)
	}
	os.WriteFile(filepath.Join(home, ".zshrc"), []byte("user changed shell config\n"), 0600)
	code, _, err := testCLI(t, root, home, "uninstall", "--restore", "--yes")
	want := "selfishell: Cannot manage the Selfishell user-zshrc block in: " + home + "/.zshrc\n" +
		"selfishell: Preserving the file. Remove conflicting Selfishell markers and retry.\n" +
		"selfishell: Uninstall cancelled because managed resources were changed.\n"
	if code != 1 || err != want {
		t.Fatalf("uninstall code %d, stderr %q; want %q", code, err, want)
	}
}
func TestLiteralHomePathThroughSymlinkDotDot(t *testing.T) {
	root := testRelease(t)
	base := t.TempDir()
	other := filepath.Join(base, "other")
	os.Mkdir(other, 0700)
	os.Mkdir(filepath.Join(other, "child"), 0700)
	os.Symlink(filepath.Join(other, "child"), filepath.Join(base, "link"))
	home := base + "/link/../home"
	os.Mkdir(filepath.Join(other, "home"), 0700)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(code, err)
	}
	if _, e := os.Stat(filepath.Join(other, "home", ".zshrc")); e != nil {
		t.Fatal("literal HOME resolved incorrectly", e)
	}
	if _, e := os.Stat(filepath.Join(base, "home", ".zshrc")); !os.IsNotExist(e) {
		t.Fatal("cleaned path was used")
	}
}
func TestInteractiveSkipAndOverwriteModifiedFile(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	isolateHome(t, home)
	paths, _ := UserPaths()
	target := paths.Config + "/zsh/runtime.zsh"
	os.WriteFile(target, []byte("user change"), 0600)
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	var out, stderr bytes.Buffer
	c := CLI{Root: root, In: strings.NewReader("y\nn\n"), Out: &out, Err: &stderr}
	code = c.Run([]string{"install", "--skip-packages"})
	if code != 0 || !strings.Contains(out.String(), "Skipped modified managed file") {
		t.Fatalf("skip %d %s %s", code, out.String(), stderr.String())
	}
	data, _ := os.ReadFile(target)
	if string(data) != "user change" {
		t.Fatal("skip overwrote file")
	}
	out.Reset()
	stderr.Reset()
	c.In = strings.NewReader("y\ny\n")
	code = c.Run([]string{"install", "--skip-packages"})
	if code != 0 || !strings.Contains(out.String(), "Backed up modified managed file") {
		t.Fatalf("overwrite %d %s %s", code, out.String(), stderr.String())
	}
}
func TestUninstallOccupiedRestoreTargetKeepsBackup(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ResourcesForPlatform(root, "macos", false)
	var r Resource
	for _, item := range resources {
		if item.Name == "user-vimrc" {
			r = item
		}
	}
	backup := r.Target + ".backup.fixed"
	content, _ := blockContent(r.Name, paths.Config)
	checksum, _ := checksumBytes(content)
	os.WriteFile(backup, []byte("original"), 0600)
	os.WriteFile(r.Target, content, 0600)
	WriteState(paths.Resources+"/"+r.Name+".state", State{"block", "active", r.Target, "-", backup, checksum})
	code, _, err := testCLI(t, root, home, "uninstall", "--restore", "--yes")
	if code != 1 || !strings.Contains(err, "Restore target is occupied") {
		t.Fatalf("%d %s", code, err)
	}
	if _, e := os.Stat(backup); e != nil {
		t.Fatal("backup lost", e)
	}
}
func TestPendingLinkAndBlockRecovery(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ResourcesForPlatform(root, "macos", false)
	var link, block Resource
	for _, r := range resources {
		if r.Name == "user-starship" {
			link = r
		}
		if r.Name == "user-vimrc" {
			block = r
		}
	}
	os.MkdirAll(filepath.Dir(link.Target), 0700)
	backup := link.Target + ".backup.fixed"
	os.WriteFile(backup, []byte("original"), 0600)
	WriteState(paths.Resources+"/"+link.Name+".state", State{"link", "pending", link.Target, link.Source, backup, "-"})
	WriteState(paths.Resources+"/"+block.Name+".state", State{"block", "pending", block.Target, "-", "-", "0:0"})
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(code, err)
	}
	dest, e := os.Readlink(link.Target)
	if e != nil || dest != link.Source {
		t.Fatalf("link %q %v", dest, e)
	}
	if _, e := os.Stat(backup); e != nil {
		t.Fatal("backup lost", e)
	}
	state, e := ReadState(paths.Resources + "/" + block.Name + ".state")
	if e != nil || state.Status != "active" {
		t.Fatal(state, e)
	}
}
func TestChangedPathTypeBlocksUninstall(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	isolateHome(t, home)
	paths, _ := UserPaths()
	target := paths.Config + "/zsh/runtime.zsh"
	os.Remove(target)
	os.Symlink("elsewhere", target)
	code, _, err = testCLI(t, root, home, "uninstall", "--yes")
	if code != 1 || !strings.Contains(err, "changed type") {
		t.Fatalf("%d %s", code, err)
	}
}
func TestMiseTrustAndDryRun(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	tools := t.TempDir()
	log := filepath.Join(t.TempDir(), "mise.log")
	mise := filepath.Join(tools, "mise")
	os.WriteFile(mise, []byte(`#!/bin/sh
printf "%s\n" "$*" >> "$SELFISHELL_TEST_MISE_LOG"
pwd >> "$SELFISHELL_TEST_MISE_LOG"
`), 0700)
	t.Setenv("PATH", tools+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_MISE_LOG", log)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--dry-run")
	if code != 0 {
		t.Fatal(err)
	}
	if _, e := os.Stat(log); !os.IsNotExist(e) {
		t.Fatal("dry-run invoked mise")
	}
	code, _, err = testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	data, e := os.ReadFile(log)
	if e != nil || !strings.Contains(string(data), "trust "+home+"/.config/mise/conf.d/selfishell.toml") || !strings.Contains(string(data), root+"/config/shared") {
		t.Fatalf("trust %q %v", data, e)
	}
}
func TestLoginShellUsesListedZshAndFakeChsh(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	tools := t.TempDir()
	zsh := filepath.Join(tools, "zsh")
	os.WriteFile(zsh, []byte("#!/bin/sh\n"), 0700)
	shells := filepath.Join(t.TempDir(), "shells")
	os.WriteFile(shells, []byte(zsh+"\n"), 0600)
	terminal := filepath.Join(t.TempDir(), "terminal")
	os.WriteFile(terminal, nil, 0600)
	log := filepath.Join(t.TempDir(), "chsh.log")
	os.WriteFile(filepath.Join(tools, "chsh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SELFISHELL_TEST_CHSH_LOG\"\nprintf chsh-stdout\nprintf chsh-stderr >&2\n"), 0700)
	t.Setenv("PATH", tools+":/usr/bin:/bin")
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("SELFISHELL_TEST_SHELLS_FILE", shells)
	t.Setenv("SELFISHELL_TEST_TERMINAL", terminal)
	t.Setenv("SELFISHELL_TEST_CHSH_LOG", log)
	var out, stderr bytes.Buffer
	c := CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}
	if code := c.Run([]string{"install", "--skip-packages", "--dry-run"}); code != 0 || !strings.Contains(out.String(), "Would set login shell to: "+zsh) {
		t.Fatalf("dry %d %s %s", code, out.String(), stderr.String())
	}
	if _, e := os.Stat(log); !os.IsNotExist(e) {
		t.Fatal("dry-run invoked chsh")
	}
	out.Reset()
	stderr.Reset()
	if code := c.Run([]string{"install", "--skip-packages", "--yes"}); code != 0 {
		t.Fatalf("install %d %s", code, stderr.String())
	}
	data, e := os.ReadFile(log)
	if e != nil || !strings.Contains(string(data), "-s "+zsh) {
		t.Fatalf("chsh %q %v", data, e)
	}
}
func TestPurgePreservesForeignSfsAndBackups(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	os.Mkdir(home, 0700)
	share := filepath.Join(base, ".local/share/selfishell")
	release := filepath.Join(share, "releases/v1")
	os.MkdirAll(release+"/bin", 0700)
	os.Symlink("releases/v1", filepath.Join(share, "current"))
	bin := filepath.Join(base, ".local/bin")
	os.MkdirAll(bin, 0700)
	os.Symlink(share+"/current/bin/selfishell", filepath.Join(bin, "selfishell"))
	os.Symlink("foreign", filepath.Join(bin, "sfs"))
	isolateHome(t, home)
	paths, _ := UserPaths()
	os.MkdirAll(paths.State+"/backups", 0700)
	os.WriteFile(paths.State+"/backups/edited", []byte("user"), 0600)
	var out, stderr bytes.Buffer
	c := CLI{Root: release, In: strings.NewReader(""), Out: &out, Err: &stderr}
	code := c.Run([]string{"uninstall", "--purge", "--yes"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if dest, e := os.Readlink(filepath.Join(bin, "sfs")); e != nil || dest != "foreign" {
		t.Fatal("foreign link changed", dest, e)
	}
	if _, e := os.Stat(paths.State + "/backups/edited"); e != nil {
		t.Fatal("backup lost", e)
	}
}
func TestDependencyManifestOverrideIsValidatedBeforeMutation(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "dependencies.conf")
	os.WriteFile(override, []byte("unknown dependency record\n"), 0600)
	t.Setenv("SELFISHELL_DEPENDENCIES_FILE", override)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(err, "invalid manifest record") {
		t.Fatalf("%d %s", code, err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatalf("mutated HOME: %v", entries)
	}
}
func TestPurgeRefusesForeignCLILinkBeforeMutation(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	os.Mkdir(home, 0700)
	share := filepath.Join(base, ".local/share/selfishell")
	release := filepath.Join(share, "releases/v1")
	os.MkdirAll(release+"/bin", 0700)
	os.Symlink("releases/v1", share+"/current")
	bin := filepath.Join(base, ".local/bin")
	os.MkdirAll(bin, 0700)
	os.Symlink("/foreign/current/bin/selfishell", bin+"/selfishell")
	isolateHome(t, home)
	var out, stderr bytes.Buffer
	c := CLI{Root: release, In: strings.NewReader(""), Out: &out, Err: &stderr}
	code := c.Run([]string{"uninstall", "--purge", "--yes"})
	if code != 1 || !strings.Contains(stderr.String(), "Refusing to remove non-Selfishell path") {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, e := os.Lstat(bin + "/selfishell"); e != nil {
		t.Fatal("foreign link removed", e)
	}
}
func TestPurgeDryRunLeavesEverythingAndShowsScope(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	os.Mkdir(home, 0700)
	share := filepath.Join(base, ".local/share/selfishell")
	release := filepath.Join(share, "releases/v1")
	os.MkdirAll(release+"/bin", 0700)
	os.Symlink("releases/v1", share+"/current")
	bin := filepath.Join(base, ".local/bin")
	os.MkdirAll(bin, 0700)
	os.Symlink(share+"/current/bin/selfishell", bin+"/selfishell")
	isolateHome(t, home)
	var out, stderr bytes.Buffer
	c := CLI{Root: release, In: strings.NewReader(""), Out: &out, Err: &stderr}
	code := c.Run([]string{"uninstall", "--purge", "--dry-run"})
	if code != 0 || !strings.Contains(out.String(), "Would remove Selfishell releases") {
		t.Fatalf("%d %s %s", code, out.String(), stderr.String())
	}
	if _, e := os.Lstat(bin + "/selfishell"); e != nil {
		t.Fatal("dry-run removed link", e)
	}
}

func isolateHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(name, "")
	}
}
func TestSavedGhosttyChoiceWinsOverYes(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	os.MkdirAll(paths.State, 0700)
	os.WriteFile(paths.State+"/ghostty", []byte("0\n"), 0600)
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	if _, e := os.Stat(paths.Config + "/ghostty/config.ghostty"); !os.IsNotExist(e) {
		t.Fatal("saved choice ignored")
	}
}
func TestModifiedBlockRejectsYes(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	path := home + "/.vimrc"
	data, _ := os.ReadFile(path)
	os.WriteFile(path, bytes.Replace(data, []byte("Selfishell vimrc >>>"), []byte("Selfishell vimrc >>> user"), 1), 0600)
	code, _, err = testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 1 || !strings.Contains(err, "Cannot manage") {
		t.Fatalf("%d %s", code, err)
	}
}
func TestNewUserMiseFileIsPrivate(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, err := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(err)
	}
	info, e := os.Stat(home + "/.config/mise/config.toml")
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v %v", info, e)
	}
}
func TestUninstallCleanupPreservesUntrackedUserPath(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	os.MkdirAll(filepath.Dir(paths.Config), 0700)
	os.WriteFile(paths.Config, []byte("personal"), 0600)
	code, _, err := testCLI(t, root, home, "uninstall", "--yes")
	if code != 0 {
		t.Fatal(code, err)
	}
	data, e := os.ReadFile(paths.Config)
	if e != nil || string(data) != "personal" {
		t.Fatalf("untracked path removed: %q %v", data, e)
	}
}
func TestUninstallCleanupPreservesUntrackedSymlink(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	os.MkdirAll(filepath.Dir(paths.Config), 0700)
	target := filepath.Join(t.TempDir(), "personal")
	os.WriteFile(target, []byte("personal"), 0600)
	os.Symlink(target, paths.Config)
	code, _, err := testCLI(t, root, home, "uninstall", "--yes")
	if code != 0 {
		t.Fatal(code, err)
	}
	dest, e := os.Readlink(paths.Config)
	if e != nil || dest != target {
		t.Fatalf("untracked link removed: %q %v", dest, e)
	}
}
func TestRemovalRevalidatesAfterWholeSetPreflight(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ResourcesForPlatform(root, "macos", false)
	r := resources[0]
	os.MkdirAll(filepath.Dir(r.Target), 0700)
	os.WriteFile(r.Target, []byte("managed"), 0600)
	checksum, _ := Checksum(context.Background(), r.Target)
	state := State{"file", "active", r.Target, "-", "-", checksum}
	WriteState(paths.Resources+"/"+r.Name+".state", state)
	var out, stderr bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out, Err: &stderr}, paths: paths}
	record := ResourceState{Resource: r, State: state}
	if e := m.preflightUninstall(record, false); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(r.Target, []byte("user edit"), 0600)
	if e := m.removeResource(record, false); e == nil {
		t.Fatal("removed edited file")
	}
	data, e := os.ReadFile(r.Target)
	if e != nil || string(data) != "user edit" {
		t.Fatalf("user data lost %q %v", data, e)
	}
	if _, e := ReadState(paths.Resources + "/" + r.Name + ".state"); e != nil {
		t.Fatal("retry state lost", e)
	}
}
func TestPurgeRechecksCLILinkBeforeRemoval(t *testing.T) {
	base := t.TempDir()
	home := base + "/home"
	os.Mkdir(home, 0700)
	isolateHome(t, home)
	paths, _ := UserPaths()
	share := base + "/.local/share/selfishell"
	release := share + "/releases/v1"
	os.MkdirAll(release+"/bin", 0700)
	os.Symlink("releases/v1", share+"/current")
	bin := base + "/.local/bin"
	os.MkdirAll(bin, 0700)
	link := bin + "/selfishell"
	os.Symlink(share+"/current/bin/selfishell", link)
	if e := preflightPurge(release); e != nil {
		t.Fatal(e)
	}
	os.Remove(link)
	os.WriteFile(link, []byte("personal"), 0600)
	var out, stderr bytes.Buffer
	c := CLI{Root: release, Out: &out, Err: &stderr}
	if e := purgeFiles(c, paths); e == nil {
		t.Fatal("removed replacement CLI path")
	}
	data, e := os.ReadFile(link)
	if e != nil || string(data) != "personal" {
		t.Fatalf("user path lost: %q %v", data, e)
	}
}
