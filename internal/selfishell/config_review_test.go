package selfishell

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/report"
	"github.com/jiminu/selfishell/internal/testutil"
)

func TestPendingLinkWithForeignTargetStaysPending(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(&env, home)
	paths, _ := managed.UserPaths(env.Get)
	resources, _ := ResourcesForPlatform(root, "macos", false, env)
	var link managed.Resource
	for _, r := range resources {
		if r.Name == "user-starship" {
			link = r
		}
	}
	os.MkdirAll(filepath.Dir(link.Target), 0700)
	backup := link.Target + ".backup.fixed"
	testutil.WriteFile(backup, []byte("original"), 0600)
	testutil.WriteFile(link.Target, []byte("foreign"), 0600)
	state := managed.State{Kind: "link", Status: "pending", Target: link.Target, Reference: link.Source, Backup: backup, Checksum: "-"}
	if e := managed.WriteState(paths.Resources+"/"+link.Name+".state", state); e != nil {
		t.Fatal(e)
	}
	var out, errOut bytes.Buffer
	m := lifecycle{Paths: paths, Reporter: report.ReportFunc(CLI{Env: env, Root: root, Out: &out, Err: &errOut}.report)}
	if e := m.installLink(link, false); e == nil {
		t.Fatal("foreign target falsely accepted")
	}
	got, e := managed.ReadState(paths.Resources + "/" + link.Name + ".state")
	if e != nil || got.Status != "pending" {
		t.Fatalf("state %+v %v", got, e)
	}
	data, e := os.ReadFile(link.Target)
	if e != nil || string(data) != "foreign" {
		t.Fatalf("foreign target changed: %q %v", data, e)
	}
}

func TestPurgeBackupErrorPreservesCLIAndReleases(t *testing.T) {
	t.Parallel()
	var env process.Environment
	for _, kind := range []string{"file", "unreadable-directory"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			env := env
			base := t.TempDir()
			home := base + "/home"
			os.Mkdir(home, 0700)
			isolateHome(&env, home)
			paths, _ := managed.UserPaths(env.Get)
			share := base + "/.local/share/selfishell"
			release := share + "/releases/v1"
			os.MkdirAll(release+"/bin", 0700)
			os.Symlink("releases/v1", share+"/current")
			bin := base + "/.local/bin"
			os.MkdirAll(bin, 0700)
			link := bin + "/selfishell"
			os.Symlink(share+"/current/bin/selfishell", link)
			os.MkdirAll(paths.State, 0700)
			if kind == "file" {
				testutil.WriteFile(paths.State+"/backups", []byte("foreign"), 0600)
			} else {
				os.Mkdir(paths.State+"/backups", 0000)
			}
			var out, errOut bytes.Buffer
			c := CLI{Env: env, Root: release, In: strings.NewReader(""), Out: &out, Err: &errOut}
			if code := c.Run([]string{"uninstall", "--purge", "--yes"}); code != 1 {
				t.Fatalf("purge accepted invalid backups: %s", errOut.String())
			}
			if _, e := os.Lstat(link); e != nil {
				t.Fatalf("CLI removed: %v", e)
			}
			if _, e := os.Stat(release); e != nil {
				t.Fatalf("releases removed: %v", e)
			}
		})
	}
}

func TestBackupCollisionAfterSelectionPreservesPendingState(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(&env, home)
	paths, _ := managed.UserPaths(env.Get)
	resources, _ := ResourcesForPlatform(root, "macos", false, env)
	r := resources[0]
	os.MkdirAll(filepath.Dir(r.Target), 0700)
	testutil.WriteFile(r.Target, []byte("original"), 0600)
	var occupied string
	var out, errOut bytes.Buffer
	m := lifecycle{Paths: paths, Reporter: report.ReportFunc(CLI{Env: env, Root: root, Out: &out, Err: &errOut}.report), Hooks: lifecycleHooks{BeforeBackupMove: func(_, destination string) {
		occupied = destination
		testutil.WriteFile(destination, []byte("late user data"), 0600)
	}}}
	if e := m.installFile(r, false); e == nil {
		t.Fatal("collision accepted")
	}
	source, _ := os.ReadFile(r.Target)
	collision, _ := os.ReadFile(occupied)
	if string(source) != "original" || string(collision) != "late user data" {
		t.Fatalf("source=%q collision=%q", source, collision)
	}
	state, e := managed.ReadState(paths.Resources + "/" + r.Name + ".state")
	if e != nil || state.Status != "pending" {
		t.Fatalf("state %+v %v", state, e)
	}
}
func TestPurgeRechecksBackupInventoryAtApply(t *testing.T) {
	t.Parallel()
	var env process.Environment
	base := t.TempDir()
	home := base + "/home"
	os.Mkdir(home, 0700)
	isolateHome(&env, home)
	paths, _ := managed.UserPaths(env.Get)
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
	testutil.WriteFile(paths.State+"/backups", []byte("late"), 0600)
	var out, errOut bytes.Buffer
	c := CLI{Env: env, Root: release, Out: &out, Err: &errOut}
	if e := purgeFiles(c, paths); e == nil {
		t.Fatal("invalid backup inventory accepted")
	}
	if _, e := os.Lstat(link); e != nil {
		t.Fatal("CLI removed", e)
	}
}
func TestFailedLinkRollbackPreservesLateTargetBackupAndState(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(&env, home)
	paths, _ := managed.UserPaths(env.Get)
	resources, _ := ResourcesForPlatform(root, "macos", false, env)
	var link managed.Resource
	for _, r := range resources {
		if r.Name == "user-starship" {
			link = r
		}
	}
	os.MkdirAll(filepath.Dir(link.Target), 0700)
	testutil.WriteFile(link.Target, []byte("original"), 0600)
	var out, errOut bytes.Buffer
	m := lifecycle{Paths: paths, Reporter: report.ReportFunc(CLI{Env: env, Root: root, Out: &out, Err: &errOut}.report), Hooks: lifecycleHooks{CreateLink: func(_, target string) error {
		testutil.WriteFile(target, []byte("late user data"), 0600)
		return errors.New("injected link failure")
	}}}
	if e := m.installLink(link, false); e == nil {
		t.Fatal("link failure ignored")
	}
	state, e := managed.ReadState(paths.Resources + "/" + link.Name + ".state")
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
