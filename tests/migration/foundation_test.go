package migration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const referenceCommit = "3bbbfa0346ee74eb47f31a81ec666340a5ef6018"
const legacyCommit = "d025710338036f1f54b948f1f3e5c17a0b3f7e38"

func TestFoundation(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release with spaces")
	if err = exportCommit(repoRoot(), referenceCommit, release); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	os.MkdirAll(filepath.Join(home, "tmp"), 0700)
	before := mustSnapshot(t, home)
	os.WriteFile(filepath.Join(release, ".git"), nil, 0600)
	direct := filepath.Join(root, "direct")
	chained := filepath.Join(root, "sfs")
	os.Symlink(entry, direct)
	os.Symlink("direct", chained)
	env := []string{"SELFISHELL_ROOT=/wrong/root", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	arguments := [][]string{{}, {""}, {"help"}, {"--help"}, {"-h"}, {"help", ""}, {"help", "extra"}, {"unknown"}, {" unknown "}, {"version"}, {"--version"}, {"-v"}, {"version", ""}, {"version", "", "extra"}, {"version", "extra"}, {"version", "help", "extra"}, {"version", "--help"}, {"version", "-h"}, {"version", "--available", "extra"}}
	compare := func(name, exe string, args []string, env []string, pty bool) {
		t.Helper()
		if err := os.WriteFile(entry, reference, 0755); err != nil {
			t.Fatal(err)
		}
		var want, got capture
		var e error
		if pty {
			want, e = capturePTY(home, exe, args, env)
		} else {
			want, e = captureCommand(home, exe, args, env)
		}
		if e != nil {
			t.Fatal(e)
		}
		if e = copyFile(candidate, entry); e != nil {
			t.Fatal(e)
		}
		if pty {
			got, e = capturePTY(home, exe, args, env)
		} else {
			got, e = captureCommand(home, exe, args, env)
		}
		if e != nil {
			t.Fatal(e)
		}
		requireEqual(t, name, want, got)
	}
	for i, args := range arguments {
		compare(fmt.Sprintf("argument-%02d-%q", i, args), chained, args, env, false)
	}
	os.Remove(filepath.Join(release, ".git"))
	versions := [][]byte{[]byte("1.2.3\n"), []byte("1.2.3\n\n"), []byte(" v1 \r\n"), {}, nil}
	for i, version := range versions {
		path := filepath.Join(release, "VERSION")
		if version == nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, version, 0600)
		}
		compare(fmt.Sprintf("version-%d", i), entry, []string{"version"}, env, false)
	}
	for _, noColor := range []string{"", "1"} {
		ptyEnv := append(append([]string{}, env...), "NO_COLOR="+noColor)
		if err := os.WriteFile(entry, reference, 0755); err != nil {
			t.Fatal(err)
		}
		want, e := capturePTY(home, entry, []string{"unknown"}, ptyEnv)
		if e != nil {
			t.Fatal(e)
		}
		if len(want.Stderr) == 0 || bytes.Contains(want.Stderr, []byte("\x1b[31m")) != (noColor == "") {
			t.Fatalf("reference PTY color NO_COLOR=%q: status=%d stdout=%q stderr=%q", noColor, want.Status, want.Stdout, want.Stderr)
		}
		copyFile(candidate, entry)
		got, e := capturePTY(home, entry, []string{"unknown"}, ptyEnv)
		if e != nil {
			t.Fatal(e)
		}
		requireEqual(t, "pty-NO_COLOR="+noColor, want, got)
	}
	for _, command := range []string{"status", "doctor", "update", "rollback"} {
		got, e := captureCommand(home, entry, []string{command}, env)
		if e != nil {
			t.Fatal(e)
		}
		if got.Status != 1 || len(got.Stdout) != 0 || !bytes.Contains(got.Stderr, []byte("not implemented in the Go candidate")) {
			t.Fatalf("%s: %+v", command, got)
		}
	}
	for _, command := range []string{"install", "uninstall"} {
		got, e := captureCommand(home, entry, []string{command, "--help"}, env)
		if e != nil {
			t.Fatal(e)
		}
		requireStatus(t, command+" help", got, 0)
	}
	if !bytes.Equal(before, mustSnapshot(t, home)) {
		t.Fatal("foundation commands mutated HOME")
	}
	installed := filepath.Join(root, "installed")
	if err := os.MkdirAll(filepath.Join(installed, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	installedCLI := filepath.Join(installed, "bin/selfishell")
	if err := copyFile(candidate, installedCLI); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed, "VERSION"), []byte("0.0.0-test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(release); err != nil {
		t.Fatal(err)
	}
	noTools := []string{"SELFISHELL_ROOT=/wrong/root", "PATH=" + filepath.Join(root, "no-tools")}
	got, e := captureCommand(home, installedCLI, []string{"version"}, noTools)
	if e != nil {
		t.Fatal(e)
	}
	if got.Status != 0 || !bytes.Equal(got.Stdout, []byte("selfishell 0.0.0-test\n")) || len(got.Stderr) != 0 {
		t.Fatalf("installed version: %+v", got)
	}
	got, e = captureCommand(home, installedCLI, []string{"help"}, noTools)
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, "installed help", got, 0)
}
