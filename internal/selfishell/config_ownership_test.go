package selfishell

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func identicalUserFile(t *testing.T) (string, string, Resource, Paths, []byte) {
	t.Helper()
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	paths, e := UserPaths()
	if e != nil {
		t.Fatal(e)
	}
	resources, e := ResourcesForPlatform(root, "macos", false)
	if e != nil {
		t.Fatal(e)
	}
	var r Resource
	for _, item := range resources {
		if item.Name == "zsh-history" {
			r = item
		}
	}
	data, e := os.ReadFile(r.Source)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(filepath.Dir(r.Target), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(r.Target, data, 0600); e != nil {
		t.Fatal(e)
	}
	return root, home, r, paths, data
}
func assertOriginalRestored(t *testing.T, root, home string, r Resource, want []byte) {
	t.Helper()
	code, _, stderr := testCLI(t, root, home, "uninstall", "--restore", "--yes")
	if code != 0 {
		t.Fatalf("uninstall %d %s", code, stderr)
	}
	data, e := os.ReadFile(r.Target)
	if e != nil || !bytes.Equal(data, want) {
		t.Fatalf("original absent or changed: %v", e)
	}
	info, e := os.Stat(r.Target)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("original mode %v %v", info, e)
	}
}
func assertOriginalBackup(t *testing.T, paths Paths, r Resource, want []byte) State {
	t.Helper()
	state, e := ReadState(paths.Resources + "/" + r.Name + ".state")
	if e != nil || state.Status != "active" || state.Backup == "-" {
		t.Fatalf("state %+v %v", state, e)
	}
	data, e := os.ReadFile(state.Backup)
	if e != nil || !bytes.Equal(data, want) {
		t.Fatalf("original backup absent or changed: %v", e)
	}
	info, e := os.Stat(state.Backup)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("backup mode %v %v", info, e)
	}
	managedInfo, e := os.Stat(r.Target)
	if e != nil || managedInfo.Mode().Perm() != 0644 {
		t.Fatalf("managed mode %v %v", managedInfo, e)
	}
	return state
}
func TestIdenticalExistingFileKeepsOriginalThroughReinstallAndRestore(t *testing.T) {
	root, home, r, paths, original := identicalUserFile(t)
	code, _, stderr := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("install %d %s", code, stderr)
	}
	first := assertOriginalBackup(t, paths, r, original)
	code, _, stderr = testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("reinstall %d %s", code, stderr)
	}
	second := assertOriginalBackup(t, paths, r, original)
	if second.Backup != first.Backup {
		t.Fatalf("original backup changed %q -> %q", first.Backup, second.Backup)
	}
	assertOriginalRestored(t, root, home, r, original)
}
func TestPendingIdenticalFileBeforeBackupMoveRecovers(t *testing.T) {
	root, home, r, paths, original := identicalUserFile(t)
	backup := r.Target + ".backup.fixed"
	checksum, e := checksumBytes(original)
	if e != nil {
		t.Fatal(e)
	}
	if e = WriteState(paths.Resources+"/"+r.Name+".state", State{"file", "pending", r.Target, "-", backup, checksum}); e != nil {
		t.Fatal(e)
	}
	code, _, stderr := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("recover %d %s", code, stderr)
	}
	state := assertOriginalBackup(t, paths, r, original)
	if state.Backup != backup {
		t.Fatalf("backup changed %q", state.Backup)
	}
	assertOriginalRestored(t, root, home, r, original)
}
func TestActiveIdenticalFileWithMissingOriginalBackupRepairsOwnership(t *testing.T) {
	root, home, r, paths, original := identicalUserFile(t)
	backup := r.Target + ".backup.missing"
	checksum, e := checksumBytes(original)
	if e != nil {
		t.Fatal(e)
	}
	if e = WriteState(paths.Resources+"/"+r.Name+".state", State{"file", "active", r.Target, "-", backup, checksum}); e != nil {
		t.Fatal(e)
	}
	code, _, stderr := testCLI(t, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("repair %d %s", code, stderr)
	}
	state := assertOriginalBackup(t, paths, r, original)
	if state.Backup != backup {
		t.Fatalf("backup changed %q", state.Backup)
	}
	assertOriginalRestored(t, root, home, r, original)
}
func TestFreshIdenticalFileRejectsBackupCollisionBeforeShortcut(t *testing.T) {
	root, home, r, paths, original := identicalUserFile(t)
	_ = home
	var collision string
	var out, errOut bytes.Buffer
	m := managed{c: CLI{Root: root, Out: &out, Err: &errOut}, paths: paths, afterBackupChoice: func(path string) { collision = path; os.WriteFile(path, []byte("foreign backup"), 0600) }}
	if e := m.installFile(r, false); e == nil {
		t.Fatal("fresh file adopted an occupied backup")
	}
	source, e := os.ReadFile(r.Target)
	if e != nil || !bytes.Equal(source, original) {
		t.Fatalf("original changed %v", e)
	}
	foreign, e := os.ReadFile(collision)
	if e != nil || string(foreign) != "foreign backup" {
		t.Fatalf("collision changed %q %v", foreign, e)
	}
	state, e := ReadState(paths.Resources + "/" + r.Name + ".state")
	if e != nil || state.Status != "pending" {
		t.Fatalf("retry state %+v %v", state, e)
	}
}
