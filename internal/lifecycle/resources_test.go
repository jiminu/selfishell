package lifecycle

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

func TestResources(t *testing.T) {
	t.Parallel()
	var env process.Environment
	home := t.TempDir()
	setEnv(&env, "HOME", home)
	setEnv(&env, "XDG_CONFIG_HOME", home+"/alias/../config space")
	setEnv(&env, "XDG_STATE_HOME", home+"/state")
	setEnv(&env, "XDG_CACHE_HOME", home+"/cache")
	setEnv(&env, "XDG_DATA_HOME", home+"/data")
	all, err := ManagedResources("/release/alias/..", env)
	if err != nil || len(all) == 0 {
		t.Fatalf("resources: %d %v", len(all), err)
	}
	if all[0].Target != home+"/alias/../config space/selfishell/zsh/zshrc" {
		t.Fatal("resource paths were cleaned")
	}
	for _, tc := range []struct {
		platform string
		ghostty  bool
		omitted  int
		zsh      string
	}{
		{"macos", false, 2, "macos"}, {"macos", true, 0, "macos"},
		{"ubuntu", false, 2, "ubuntu"}, {"ubuntu", true, 2, "ubuntu"},
		{"ubuntu-wsl", false, 2, "ubuntu"},
	} {
		resources, err := ResourcesForPlatform("/release", tc.platform, tc.ghostty, env)
		if err != nil || len(resources) != len(all)-tc.omitted {
			t.Fatalf("%s ghostty=%v: %d %v", tc.platform, tc.ghostty, len(resources), err)
		}
		if resources[0].Source != "/release/config/"+tc.zsh+"/zshrc" {
			t.Fatal(resources[0])
		}
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("resource discovery wrote files")
	}
}

func TestLoadResourceStates(t *testing.T) {
	dir := t.TempDir()
	state := stateFixture()
	resources := []managed.Resource{{Kind: "file", Name: "first", Target: "/a", Source: "/source"}, {Kind: "link", Name: "last", Target: "/b", Source: "/reference"}}
	if err := managed.WriteState(dir+"/first.state", state); err != nil {
		t.Fatal(err)
	}
	records, err := LoadResourceStates(dir, resources)
	if err != nil || len(records) != 1 || records[0].Resource != resources[0] || records[0].State != state {
		t.Fatalf("records: %+v %v", records, err)
	}
	if err := testutil.WriteFile(dir+"/last.state", []byte("2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if records, err := LoadResourceStates(dir, resources); records != nil || !errors.Is(err, managed.ErrMalformedState) {
		t.Fatalf("partial enumeration escaped: %+v %v", records, err)
	}
	if err := os.Remove(dir + "/last.state"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir+"/missing", dir+"/last.state"); err != nil {
		t.Fatal(err)
	}
	if records, err := LoadResourceStates(dir, resources); records != nil || !errors.Is(err, managed.ErrMalformedState) {
		t.Fatalf("dangling state silently skipped: %+v %v", records, err)
	}
	for _, names := range [][]managed.Resource{nil, {resources[0], resources[0]}, {resources[0], {Kind: "file", Name: "../escape", Target: "/x", Source: "/y"}}} {
		if records, err := LoadResourceStates(dir, names); records != nil || err == nil {
			t.Fatalf("invalid declaration: %+v %v", records, err)
		}
	}
	if records, err := LoadResourceStates(dir+"/first.state/invalid", resources); records != nil || err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("enumeration I/O failure skipped: %+v %v", records, err)
	}
}

func TestLoadResourceStatesUnreadableEntry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires an unprivileged test process")
	}
	dir := t.TempDir()
	resources := []managed.Resource{{Kind: "file", Name: "first", Target: "/a", Source: "/b"}, {Kind: "file", Name: "last", Target: "/c", Source: "/d"}}
	for _, r := range resources {
		if err := managed.WriteState(dir+"/"+r.Name+".state", stateFixture()); err != nil {
			t.Fatal(err)
		}
	}
	path := dir + "/last.state"
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	if records, err := LoadResourceStates(dir, resources); records != nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable last entry returned a partial list: %+v %v", records, err)
	}
}

func stateFixture() managed.State {
	return managed.State{Kind: "file", Status: "pending", Target: "/home/a file", Reference: "-", Backup: "/home/original.backup.20000101000000.1", Checksum: "123:45"}
}
