package integration_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestFoundation(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	root := t.TempDir()
	release := filepath.Join(root, "release with spaces")
	copyCLIFixture(t, release, cli)
	mustFS(t, os.Remove(filepath.Join(release, "VERSION")))
	mustFS(t, testutil.WriteFile(filepath.Join(release, ".git"), nil, 0600))
	entry := filepath.Join(release, "bin/selfishell")
	home := t.TempDir()
	before := mustSnapshot(t, home)
	direct := filepath.Join(root, "direct")
	chained := filepath.Join(root, "sfs")
	mustFS(t, os.Symlink(entry, direct))
	mustFS(t, os.Symlink("direct", chained))
	env := []string{"SELFISHELL_ROOT=/wrong/root", "SELFISHELL_RELEASE_ROOT=file://" + filepath.Join(root, "unavailable-release-metadata"), "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	for _, args := range [][]string{nil, {""}, {"help"}, {"--help"}, {"-h"}} {
		got, err := captureCommand(home, chained, args, env)
		mustFS(t, err)
		requireStatus(t, "help", got, 0)
		requireContains(t, got.Stdout, "Usage:\n  selfishell <command>")
		if len(got.Stderr) != 0 {
			t.Fatalf("help stderr: %q", got.Stderr)
		}
	}
	for _, args := range [][]string{{"update", "--cli-only", "--version", "1.2.3", "--yes"}, {"rollback", "--yes"}} {
		got, err := captureCommand(home, entry, args, env)
		mustFS(t, err)
		if got.Status != 1 || len(got.Stdout) != 0 || !bytes.Contains(got.Stderr, []byte("requires a versioned Selfishell installation")) {
			t.Fatalf("%v: %+v", args, got)
		}
	}
	for _, command := range []string{"install", "uninstall", "update", "rollback"} {
		got, err := captureCommand(home, entry, []string{command, "--help"}, env)
		mustFS(t, err)
		requireStatus(t, command+" help", got, 0)
	}
	if !bytes.Equal(before, mustSnapshot(t, home)) {
		t.Fatal("foundation commands mutated HOME")
	}
	installed := filepath.Join(root, "installed")
	mustFS(t, os.MkdirAll(filepath.Join(installed, "bin"), 0700))
	installedCLI := filepath.Join(installed, "bin/selfishell")
	mustFS(t, copyFile(cli, installedCLI))
	mustFS(t, testutil.WriteFile(filepath.Join(installed, "VERSION"), []byte("0.0.0-test\n"), 0600))
	mustFS(t, os.RemoveAll(release))
	noTools := []string{"SELFISHELL_ROOT=/wrong/root", "PATH=" + filepath.Join(root, "no-tools")}
	got, err := captureCommand(home, installedCLI, []string{"version"}, noTools)
	mustFS(t, err)
	if got.Status != 0 || string(got.Stdout) != "selfishell 0.0.0-test\n" || len(got.Stderr) != 0 {
		t.Fatalf("installed version: %+v", got)
	}
	got, err = captureCommand(home, installedCLI, []string{"help"}, noTools)
	mustFS(t, err)
	requireStatus(t, "installed help", got, 0)
}
