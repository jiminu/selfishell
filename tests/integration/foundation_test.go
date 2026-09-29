package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFoundation(t *testing.T) {
	t.Parallel()
	cli, err := testCLI(t)
	mustFS(t, err)
	root := t.TempDir()
	release := filepath.Join(root, "release with spaces")
	copyCLIFixture(t, release, cli)
	mustFS(t, os.Remove(filepath.Join(release, "VERSION")))
	mustFS(t, os.WriteFile(filepath.Join(release, ".git"), nil, 0600))
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
	for _, tc := range []struct {
		args           []string
		status         int
		stdout, stderr string
	}{
		{[]string{"help", ""}, 2, "", "selfishell: help does not accept arguments\n"},
		{[]string{"help", "extra"}, 2, "", "selfishell: help does not accept arguments\n"},
		{[]string{"unknown"}, 2, "", "selfishell: Unknown command: unknown\nselfishell: Run 'selfishell help' to see available commands.\n"},
		{[]string{" unknown "}, 2, "", "selfishell: Unknown command:  unknown \nselfishell: Run 'selfishell help' to see available commands.\n"},
		{[]string{"version"}, 0, "selfishell development\n", ""},
		{[]string{"--version"}, 0, "selfishell development\n", ""},
		{[]string{"-v"}, 0, "selfishell development\n", ""},
		{[]string{"version", ""}, 0, "selfishell development\n", ""},
		{[]string{"version", "", "extra"}, 0, "selfishell development\n", ""},
		{[]string{"version", "extra"}, 2, "", "selfishell: Usage: selfishell version [--available]\n"},
		{[]string{"version", "help", "extra"}, 0, "Usage: selfishell version [--available]\n", ""},
		{[]string{"version", "--help"}, 0, "Usage: selfishell version [--available]\n", ""},
		{[]string{"version", "-h"}, 0, "Usage: selfishell version [--available]\n", ""},
		{[]string{"version", "--available", "extra"}, 2, "", "selfishell: Usage: selfishell version [--available]\n"},
	} {
		t.Run(fmt.Sprint(tc.args), func(t *testing.T) {
			got, err := captureCommand(home, chained, tc.args, env)
			mustFS(t, err)
			requireStatus(t, "command", got, tc.status)
			if string(got.Stdout) != tc.stdout || string(got.Stderr) != tc.stderr {
				t.Fatalf("stdout=%q stderr=%q; expected %q / %q", got.Stdout, got.Stderr, tc.stdout, tc.stderr)
			}
		})
	}
	mustFS(t, os.Remove(filepath.Join(release, ".git")))
	for _, tc := range []struct{ contents, want string }{
		{"1.2.3\n", "selfishell 1.2.3\n"},
		{"1.2.3\n\n", "selfishell 1.2.3\n"},
		{" v1 \r\n", "selfishell  v1 \r\n"},
		{"", "selfishell \n"},
	} {
		mustFS(t, os.WriteFile(filepath.Join(release, "VERSION"), []byte(tc.contents), 0600))
		got, err := captureCommand(home, entry, []string{"version"}, env)
		mustFS(t, err)
		if got.Status != 0 || string(got.Stdout) != tc.want || len(got.Stderr) != 0 {
			t.Fatalf("version: %+v", got)
		}
	}
	mustFS(t, os.Remove(filepath.Join(release, "VERSION")))
	missing, err := captureCommand(home, entry, []string{"version"}, env)
	mustFS(t, err)
	if missing.Status != 1 || len(missing.Stdout) != 0 || !bytes.Contains(missing.Stderr, []byte("Version file not found:")) {
		t.Fatalf("missing version: %+v", missing)
	}
	for _, noColor := range []string{"", "1"} {
		got, err := capturePTY(home, entry, []string{"unknown"}, append(append([]string{}, env...), "NO_COLOR="+noColor))
		mustFS(t, err)
		if got.Status != 2 || len(got.Stdout) != 0 || !bytes.Contains(got.Stderr, []byte("Unknown command: unknown")) || bytes.Contains(got.Stderr, []byte("\x1b[31m")) != (noColor == "") {
			t.Fatalf("PTY color NO_COLOR=%q: %+v", noColor, got)
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
	mustFS(t, os.WriteFile(filepath.Join(installed, "VERSION"), []byte("0.0.0-test\n"), 0600))
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
