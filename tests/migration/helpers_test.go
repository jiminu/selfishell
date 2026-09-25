package migration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotPreservesTypesModesAndBytes(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte{'a', '\r', '\n', 0, 'b'}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", filepath.Join(root, "cycle")); err != nil {
		t.Fatal(err)
	}
	if err := makeFIFO(filepath.Join(root, "pipe")); err != nil {
		t.Fatal(err)
	}
	before := mustSnapshot(t, root)
	changes := []func(){
		func() { mustFS(t, os.WriteFile(file, []byte{'a', '\n', 0, 'b'}, 0600)) },
		func() { mustFS(t, os.WriteFile(file, []byte{'a', '\r', '\n', 0, 'b', '\n'}, 0600)) },
		func() { mustFS(t, os.Chmod(file, 0640)) },
		func() {
			mustFS(t, os.Remove(filepath.Join(root, "link")))
			mustFS(t, os.Symlink("missing", filepath.Join(root, "link")))
		},
		func() {
			mustFS(t, os.Remove(filepath.Join(root, "link")))
			mustFS(t, os.WriteFile(filepath.Join(root, "link"), nil, 0600))
		},
		func() { mustFS(t, os.Remove(filepath.Join(root, "link"))) },
	}
	for i, change := range changes {
		change()
		if bytes.Equal(before, mustSnapshot(t, root)) {
			t.Fatalf("change %d hidden", i)
		}
		mustFS(t, os.Remove(file))
		mustFS(t, os.WriteFile(file, []byte{'a', '\r', '\n', 0, 'b'}, 0600))
		if _, err := os.Lstat(filepath.Join(root, "link")); err == nil {
			mustFS(t, os.Remove(filepath.Join(root, "link")))
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		mustFS(t, os.Symlink("file", filepath.Join(root, "link")))
	}
	if err := os.Chmod(file, 0600|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, mustSnapshot(t, root)) {
		t.Fatal("setuid permission hidden")
	}
	mustFS(t, os.Chmod(file, 0600))
	outside := t.TempDir()
	mustFS(t, os.WriteFile(filepath.Join(outside, "value"), []byte("before"), 0600))
	mustFS(t, os.Symlink(outside, filepath.Join(root, "outside")))
	unchanged := mustSnapshot(t, root)
	mustFS(t, os.WriteFile(filepath.Join(outside, "value"), []byte("after"), 0600))
	if !bytes.Equal(unchanged, mustSnapshot(t, root)) {
		t.Fatal("followed symlink")
	}
	invalid := filepath.Join(root, "invalid")
	if err := os.Symlink(string([]byte{0xff}), invalid); err != nil {
		t.Fatal(err)
	}
	invalidBefore := mustSnapshot(t, root)
	mustFS(t, os.Remove(invalid))
	if err := os.Symlink(string([]byte{0xfe}), invalid); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(invalidBefore, mustSnapshot(t, root)) {
		t.Fatal("invalid UTF-8 link target lost")
	}
}

func TestComparisonRejectsEachDifference(t *testing.T) {
	a := capture{Status: 0, Stdout: []byte("out"), Stderr: []byte("err"), Home: []byte("home")}
	for name, b := range map[string]capture{"status": {Status: 1, Stdout: a.Stdout, Stderr: a.Stderr, Home: a.Home}, "stdout": {Status: 0, Stdout: []byte("bad"), Stderr: a.Stderr, Home: a.Home}, "stderr": {Status: 0, Stdout: a.Stdout, Stderr: []byte("bad"), Home: a.Home}, "home": {Status: 0, Stdout: a.Stdout, Stderr: a.Stderr, Home: []byte("bad")}} {
		if err := compareCapture(a, b); err == nil || !strings.Contains(err.Error(), name) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := compareCapture(a, a); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotDetectsBackupAndStateChanges(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".local/state/selfishell")
	if err := os.MkdirAll(filepath.Join(stateDir, "backups"), 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(stateDir, "file.state")
	mustFS(t, os.WriteFile(state, []byte("2\nfile\nactive\n/target\n/source\n-\n123:4\n"), 0600))
	before := mustSnapshot(t, root)
	backup := filepath.Join(stateDir, "backups/file.backup.20000101000000")
	mustFS(t, os.WriteFile(backup, []byte("original\n"), 0600))
	if bytes.Equal(before, mustSnapshot(t, root)) {
		t.Fatal("backup hidden")
	}
	mustFS(t, os.Remove(backup))
	mustFS(t, os.WriteFile(state, []byte("2\nfile\npending\n/target\n/source\n-\n123:4\n"), 0600))
	if bytes.Equal(before, mustSnapshot(t, root)) {
		t.Fatal("state change hidden")
	}
	if _, err := snapshot(filepath.Join(root, "absent")); !os.IsNotExist(err) {
		t.Fatalf("missing root: %v", err)
	}
}

func TestRunPreservesArgumentsInputStreamsAndStatus(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "script")
	mustFS(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" \"$2\"\ncat\nprintf 'error\\n' >&2\nexit 7\n"), 0700))
	got, err := runCommand(root, []string{"/bin/sh", script, "", "two words"}, []byte("input\x00bytes"), nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 7 || !bytes.Equal(got.Stdout, []byte("\ntwo words\ninput\x00bytes")) || !bytes.Equal(got.Stderr, []byte("error\n")) {
		t.Fatalf("%+v", got)
	}
}

func TestInvalidCandidateOverrideFails(t *testing.T) {
	t.Setenv("SELFISHELL_TEST_CLI", filepath.Join(t.TempDir(), "missing"))
	if _, err := candidateCLI(t); err == nil {
		t.Fatal("accepted missing override")
	}
}

func TestMissingHistoryDoesNotCreateExport(t *testing.T) {
	root := t.TempDir()
	got, runErr := runCommand(root, []string{"git", "init", "-q", root}, nil, nil, 5*time.Second)
	if runErr != nil || got.Status != 0 {
		t.Fatalf("git init: %v %+v", runErr, got)
	}
	dest := filepath.Join(root, "export")
	err := exportCommit(root, "3bbbfa0346ee74eb47f31a81ec666340a5ef6018", dest)
	if err == nil || !strings.Contains(err.Error(), "fetch-depth: 0") {
		t.Fatalf("%v", err)
	}
	if _, e := os.Lstat(dest); !os.IsNotExist(e) {
		t.Fatalf("partial export: %v", e)
	}
}

func TestRunCleansDescendantAfterLeaderExits(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "escaped")
	start := time.Now()
	got, err := runCommand(home, []string{"/bin/sh", "-c", "(/bin/sleep 2; /usr/bin/touch \"$1\") & exit 0", "sh", marker}, nil, nil, 3*time.Second)
	if got.Status != 0 || err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("leader execution: %+v %v", got, err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("inherited pipe blocked cleanup")
	}
	time.Sleep(2200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived cleanup: %v", err)
	}
}

func TestRunTimeoutKillsChildAndReturns(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "escaped")
	start := time.Now()
	_, err := runCommand(home, []string{"/bin/sh", "-c", "(/bin/sleep 2; /usr/bin/touch \"$1\") & wait", "sh", marker}, nil, nil, 200*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout blocked")
	}
	time.Sleep(2200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child survived timeout: %v", err)
	}
}

func TestPTYUsesPrivateTempAndCleansDescendant(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "escaped")
	start := time.Now()
	got, err := capturePTY(home, "/bin/sh", []string{"-c", "printf '%s\\n' \"$TMPDIR\"; (/bin/sleep 2; /usr/bin/touch \"$1\") & exit 0", "sh", marker}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 0 {
		t.Fatalf("status %d stderr %q", got.Status, got.Stderr)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("PTY reader blocked by descendant")
	}
	tmp := strings.TrimSpace(string(got.Stdout))
	if tmp == "" || tmp == os.TempDir() || strings.HasPrefix(tmp, home+string(os.PathSeparator)) {
		t.Fatalf("TMPDIR not private: %q", tmp)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("PTY temporary directory retained: %v", err)
	}
	time.Sleep(2200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("PTY descendant survived: %v", err)
	}
}

func mustSnapshot(t *testing.T, root string) []byte {
	t.Helper()
	b, e := snapshot(root)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func mustFS(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
