package selfishell

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
)

func identicalUserFile(t *testing.T, env *Environment) (string, string, Resource, Paths, []byte) {
	t.Helper()
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(env, home)
	paths, e := userPaths(env.Get)
	if e != nil {
		t.Fatal(e)
	}
	resources, e := ResourcesForPlatform(root, "macos", false, *env)
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
	if e = testutil.WriteFile(r.Target, data, 0600); e != nil {
		t.Fatal(e)
	}
	return root, home, r, paths, data
}
func assertOriginalRestored(t *testing.T, env Environment, root, home string, r Resource, want []byte) {
	t.Helper()
	code, _, stderr := testCLI(&env, root, home, "uninstall", "--restore", "--yes")
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
	t.Parallel()
	var env Environment
	root, home, r, paths, original := identicalUserFile(t, &env)
	code, _, stderr := testCLI(&env, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("install %d %s", code, stderr)
	}
	first := assertOriginalBackup(t, paths, r, original)
	code, _, stderr = testCLI(&env, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("reinstall %d %s", code, stderr)
	}
	second := assertOriginalBackup(t, paths, r, original)
	if second.Backup != first.Backup {
		t.Fatalf("original backup changed %q -> %q", first.Backup, second.Backup)
	}
	assertOriginalRestored(t, env, root, home, r, original)
}
func TestPendingIdenticalFileBeforeBackupMoveRecovers(t *testing.T) {
	t.Parallel()
	var env Environment
	root, home, r, paths, original := identicalUserFile(t, &env)
	backup := r.Target + ".backup.fixed"
	checksum, e := checksumBytes(original)
	if e != nil {
		t.Fatal(e)
	}
	if e = WriteState(paths.Resources+"/"+r.Name+".state", State{Kind: "file", Status: "pending", Target: r.Target, Reference: "-", Backup: backup, Checksum: checksum}); e != nil {
		t.Fatal(e)
	}
	code, _, stderr := testCLI(&env, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("recover %d %s", code, stderr)
	}
	state := assertOriginalBackup(t, paths, r, original)
	if state.Backup != backup {
		t.Fatalf("backup changed %q", state.Backup)
	}
	assertOriginalRestored(t, env, root, home, r, original)
}
func TestActiveIdenticalFileWithMissingOriginalBackupRepairsOwnership(t *testing.T) {
	t.Parallel()
	var env Environment
	root, home, r, paths, original := identicalUserFile(t, &env)
	backup := r.Target + ".backup.missing"
	checksum, e := checksumBytes(original)
	if e != nil {
		t.Fatal(e)
	}
	if e = WriteState(paths.Resources+"/"+r.Name+".state", State{Kind: "file", Status: "active", Target: r.Target, Reference: "-", Backup: backup, Checksum: checksum}); e != nil {
		t.Fatal(e)
	}
	code, _, stderr := testCLI(&env, root, home, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("repair %d %s", code, stderr)
	}
	state := assertOriginalBackup(t, paths, r, original)
	if state.Backup != backup {
		t.Fatalf("backup changed %q", state.Backup)
	}
	assertOriginalRestored(t, env, root, home, r, original)
}
