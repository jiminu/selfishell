package selfishell

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailedBlockStateRemovalCanRecoverThroughInstall(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	code, _, stderr := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(stderr)
	}
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ManagedResources(root)
	var block Resource
	for _, r := range resources {
		if r.Name == "user-vimrc" {
			block = r
		}
	}
	state, e := ReadState(paths.Resources + "/" + block.Name + ".state")
	if e != nil {
		t.Fatal(e)
	}
	var out, errOut bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out, Err: &errOut}, paths: paths, removeState: func(string) error { return errors.New("injected unlink failure") }}
	if e = m.removeResource(ResourceState{Resource: block, State: state}, false); e == nil {
		t.Fatal("expected unlink failure")
	}
	pending, e := ReadState(paths.Resources + "/" + block.Name + ".state")
	if e != nil || pending.Status != "pending" {
		t.Fatalf("state %+v %v", pending, e)
	}
	code, _, stderr = testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("repair install: %s", stderr)
	}
	code, _, stderr = testCLI(t, root, home, "uninstall", "--yes")
	if code != 0 {
		t.Fatalf("retry uninstall: %s", stderr)
	}
}

func TestPendingLinkWithForeignTargetStaysPending(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ResourcesForPlatform(root, "macos", false)
	var link Resource
	for _, r := range resources {
		if r.Name == "user-starship" {
			link = r
		}
	}
	os.MkdirAll(filepath.Dir(link.Target), 0700)
	backup := link.Target + ".backup.fixed"
	os.WriteFile(backup, []byte("original"), 0600)
	os.WriteFile(link.Target, []byte("foreign"), 0600)
	state := State{"link", "pending", link.Target, link.Source, backup, "-"}
	if e := WriteState(paths.Resources+"/"+link.Name+".state", state); e != nil {
		t.Fatal(e)
	}
	var out, errOut bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out, Err: &errOut}, paths: paths}
	if e := m.installLink(link, false); e == nil {
		t.Fatal("foreign target falsely accepted")
	}
	got, e := ReadState(paths.Resources + "/" + link.Name + ".state")
	if e != nil || got.Status != "pending" {
		t.Fatalf("state %+v %v", got, e)
	}
	data, e := os.ReadFile(link.Target)
	if e != nil || string(data) != "foreign" {
		t.Fatalf("foreign target changed: %q %v", data, e)
	}
}

func TestPurgeBackupErrorPreservesCLIAndReleases(t *testing.T) {
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
	os.MkdirAll(paths.State, 0700)
	os.WriteFile(paths.State+"/backups", []byte("foreign"), 0600)
	var out, errOut bytes.Buffer
	c := CLI{Root: release, In: strings.NewReader(""), Out: &out, Err: &errOut}
	if code := c.Run([]string{"uninstall", "--purge", "--yes"}); code != 1 {
		t.Fatalf("purge accepted invalid backups: %s", errOut.String())
	}
	if _, e := os.Lstat(link); e != nil {
		t.Fatalf("CLI removed: %v", e)
	}
	if _, e := os.Stat(release); e != nil {
		t.Fatalf("releases removed: %v", e)
	}
}

func TestBackupMoveAndCopyNeverReplaceCollision(t *testing.T) {
	root := t.TempDir()
	source := root + "/source"
	destination := root + "/backup"
	os.WriteFile(source, []byte("original"), 0600)
	os.WriteFile(destination, []byte("foreign"), 0600)
	if e := moveBackupNoReplace(source, destination); e == nil {
		t.Fatal("move replaced occupied backup")
	}
	if e := copyPreserve(source, destination); e == nil {
		t.Fatal("copy replaced occupied backup")
	}
	for path, want := range map[string]string{source: "original", destination: "foreign"} {
		data, e := os.ReadFile(path)
		if e != nil || string(data) != want {
			t.Fatalf("%s = %q %v", path, data, e)
		}
	}
}
func TestBackupNoReplaceCoversFilesLinksAndDirectories(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			source := root + "/source"
			backup := root + "/backup"
			switch kind {
			case "file":
				os.WriteFile(source, []byte("source"), 0600)
				os.WriteFile(backup, []byte("backup"), 0600)
			case "symlink":
				os.Symlink("source-target", source)
				os.Symlink("backup-target", backup)
			case "directory":
				os.Mkdir(source, 0700)
				os.WriteFile(source+"/source-child", []byte("source"), 0600)
				os.Mkdir(backup, 0700)
				os.WriteFile(backup+"/backup-child", []byte("backup"), 0600)
			}
			if e := moveBackupNoReplace(source, backup); e == nil {
				t.Fatal("occupied destination replaced")
			}
			if _, e := os.Lstat(source); e != nil {
				t.Fatal("source lost", e)
			}
			switch kind {
			case "file":
				data, _ := os.ReadFile(backup)
				if string(data) != "backup" {
					t.Fatal("backup overwritten")
				}
			case "symlink":
				dest, _ := os.Readlink(backup)
				if dest != "backup-target" {
					t.Fatal("link overwritten")
				}
			case "directory":
				if _, e := os.Stat(backup + "/backup-child"); e != nil {
					t.Fatal("directory overwritten", e)
				}
				if _, e := os.Stat(backup + "/source-child"); !os.IsNotExist(e) {
					t.Fatal("source nested in backup")
				}
			}
		})
	}
}
func TestBackupCollisionAfterSelectionPreservesPendingState(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ResourcesForPlatform(root, "macos", false)
	r := resources[0]
	os.MkdirAll(filepath.Dir(r.Target), 0700)
	os.WriteFile(r.Target, []byte("original"), 0600)
	var occupied string
	var out, errOut bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out, Err: &errOut}, paths: paths, beforeBackupMove: func(_, destination string) {
		occupied = destination
		os.WriteFile(destination, []byte("late user data"), 0600)
	}}
	if e := m.installFile(r, false); e == nil {
		t.Fatal("collision accepted")
	}
	source, _ := os.ReadFile(r.Target)
	collision, _ := os.ReadFile(occupied)
	if string(source) != "original" || string(collision) != "late user data" {
		t.Fatalf("source=%q collision=%q", source, collision)
	}
	state, e := ReadState(paths.Resources + "/" + r.Name + ".state")
	if e != nil || state.Status != "pending" {
		t.Fatalf("state %+v %v", state, e)
	}
}
func TestPurgeRechecksBackupInventoryAtApply(t *testing.T) {
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
	if _, e := backupInventory(paths); e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(paths.State, 0700)
	os.WriteFile(paths.State+"/backups", []byte("late"), 0600)
	var out, errOut bytes.Buffer
	c := CLI{Root: release, Out: &out, Err: &errOut}
	if e := purgeFiles(c, paths); e == nil {
		t.Fatal("invalid backup inventory accepted")
	}
	if _, e := os.Lstat(link); e != nil {
		t.Fatal("CLI removed", e)
	}
}
func TestRestoreMoveNeverReplacesLateOccupant(t *testing.T) {
	root := t.TempDir()
	backup := root + "/original.backup"
	target := root + "/original"
	os.WriteFile(backup, []byte("saved original"), 0600)
	os.WriteFile(target, []byte("late user file"), 0600)
	if e := moveBackupNoReplace(backup, target); e == nil {
		t.Fatal("restore replaced occupied target")
	}
	for path, want := range map[string]string{backup: "saved original", target: "late user file"} {
		data, e := os.ReadFile(path)
		if e != nil || string(data) != want {
			t.Fatalf("%s=%q %v", path, data, e)
		}
	}
}
func TestConflictCopyNeverReplacesLinkOrDirectory(t *testing.T) {
	for _, kind := range []string{"symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			source := root + "/edited"
			target := root + "/backup"
			os.WriteFile(source, []byte("edit"), 0600)
			if kind == "symlink" {
				os.Symlink("foreign", target)
			} else {
				os.Mkdir(target, 0700)
				os.WriteFile(target+"/foreign", []byte("foreign"), 0600)
			}
			if e := copyPreserve(source, target); e == nil {
				t.Fatal("conflict backup replaced existing path")
			}
			if kind == "symlink" {
				dest, _ := os.Readlink(target)
				if dest != "foreign" {
					t.Fatal("link changed")
				}
			} else {
				if _, e := os.Stat(target + "/foreign"); e != nil {
					t.Fatal("directory changed", e)
				}
			}
		})
	}
}
func TestUnreadableBackupDirectoryStopsPurge(t *testing.T) {
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
	os.MkdirAll(paths.State+"/backups", 0000)
	os.Chmod(paths.State+"/backups", 0000)
	var out, errOut bytes.Buffer
	c := CLI{Root: release, In: strings.NewReader(""), Out: &out, Err: &errOut}
	if code := c.Run([]string{"uninstall", "--purge", "--yes"}); code != 1 {
		t.Fatalf("unreadable backups accepted: %s", errOut.String())
	}
	if _, e := os.Lstat(link); e != nil {
		t.Fatal("CLI removed", e)
	}
}
func TestFailedLinkRollbackPreservesLateTargetBackupAndState(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ResourcesForPlatform(root, "macos", false)
	var link Resource
	for _, r := range resources {
		if r.Name == "user-starship" {
			link = r
		}
	}
	os.MkdirAll(filepath.Dir(link.Target), 0700)
	os.WriteFile(link.Target, []byte("original"), 0600)
	var out, errOut bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out, Err: &errOut}, paths: paths, createLink: func(_, target string) error {
		os.WriteFile(target, []byte("late user data"), 0600)
		return errors.New("injected link failure")
	}}
	if e := m.installLink(link, false); e == nil {
		t.Fatal("link failure ignored")
	}
	state, e := ReadState(paths.Resources + "/" + link.Name + ".state")
	if e != nil || state.Status != "pending" {
		t.Fatalf("retry state %+v %v", state, e)
	}
	backup, e := os.ReadFile(state.Backup)
	if e != nil || string(backup) != "original" {
		t.Fatalf("backup %q %v", backup, e)
	}
	target, e := os.ReadFile(link.Target)
	if e != nil || string(target) != "late user data" {
		t.Fatalf("late target %q %v", target, e)
	}
}
func TestFailedLinkCreationRestoresOriginalWithoutClaimingSuccess(t *testing.T) {
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, _ := UserPaths()
	resources, _ := ResourcesForPlatform(root, "macos", false)
	var link Resource
	for _, r := range resources {
		if r.Name == "user-starship" {
			link = r
		}
	}
	os.MkdirAll(filepath.Dir(link.Target), 0700)
	os.WriteFile(link.Target, []byte("original"), 0600)
	var out, errOut bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out, Err: &errOut}, paths: paths, createLink: func(string, string) error { return errors.New("injected link failure") }}
	if e := m.installLink(link, false); e == nil {
		t.Fatal("link failure reported success")
	}
	data, e := os.ReadFile(link.Target)
	if e != nil || string(data) != "original" {
		t.Fatalf("original %q %v", data, e)
	}
	if _, e := ReadState(paths.Resources + "/" + link.Name + ".state"); !os.IsNotExist(e) {
		t.Fatalf("state not cleared: %v", e)
	}
}
