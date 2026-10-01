package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestGoBuildCacheLifetime(t *testing.T) {
	t.Parallel()
	if os.Getenv("SELFISHELL_TEST_CACHE_CHILD") == "1" {
		mustFS(t, testutil.WriteFile(filepath.Join(testGoCache, "child-marker"), []byte("compiled fixture"), 0600))
		return
	}
	for _, mode := range []string{"private", "shared", "relative"} {
		t.Run(mode, func(t *testing.T) {
			home, temp, shared := t.TempDir(), t.TempDir(), t.TempDir()
			cache := ""
			if mode == "shared" {
				cache = shared
			} else if mode == "relative" {
				cache = "relative-cache"
			}
			mustFS(t, testutil.WriteFile(filepath.Join(shared, "owner-marker"), []byte("retained"), 0600))
			cmd := exec.Command(os.Args[0], "-test.run=^TestGoBuildCacheLifetime$", "-test.count=1")
			cmd.Dir = home
			cmd.Env = withEnv(baseEnv(home, temp), "SELFISHELL_TEST_CACHE_CHILD=1", "SELFISHELL_TEST_GO_CACHE="+cache)
			out, err := cmd.CombinedOutput()
			if mode == "relative" {
				if err == nil || !bytes.Contains(out, []byte("SELFISHELL_TEST_GO_CACHE must be absolute")) {
					t.Fatalf("relative cache: %v %s", err, out)
				}
			} else if err != nil {
				t.Fatalf("child tests: %v %s", err, out)
			}
			entries, err := os.ReadDir(temp)
			mustFS(t, err)
			if len(entries) != 0 {
				t.Fatalf("test process left temporary cache state: %v", entries)
			}
			if got := readBytes(t, filepath.Join(shared, "owner-marker")); string(got) != "retained" {
				t.Fatalf("caller-owned cache changed: %q", got)
			}
			if mode == "shared" {
				if got := readBytes(t, filepath.Join(shared, "child-marker")); string(got) != "compiled fixture" {
					t.Fatalf("shared cache was not retained: %q", got)
				}
			}
		})
	}
}

func TestSnapshotPreservesTypesModesAndBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := testutil.WriteFile(file, []byte{'a', '\r', '\n', 0, 'b'}, 0600); err != nil {
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
		func() { mustFS(t, testutil.WriteFile(file, []byte{'a', '\n', 0, 'b'}, 0600)) },
		func() { mustFS(t, testutil.WriteFile(file, []byte{'a', '\r', '\n', 0, 'b', '\n'}, 0600)) },
		func() { mustFS(t, os.Chmod(file, 0640)) },
		func() {
			mustFS(t, os.Remove(filepath.Join(root, "link")))
			mustFS(t, os.Symlink("missing", filepath.Join(root, "link")))
		},
		func() {
			mustFS(t, os.Remove(filepath.Join(root, "link")))
			mustFS(t, testutil.WriteFile(filepath.Join(root, "link"), nil, 0600))
		},
		func() { mustFS(t, os.Remove(filepath.Join(root, "link"))) },
	}
	for i, change := range changes {
		change()
		if bytes.Equal(before, mustSnapshot(t, root)) {
			t.Fatalf("change %d hidden", i)
		}
		mustFS(t, os.Remove(file))
		mustFS(t, testutil.WriteFile(file, []byte{'a', '\r', '\n', 0, 'b'}, 0600))
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
	mustFS(t, testutil.WriteFile(filepath.Join(outside, "value"), []byte("before"), 0600))
	mustFS(t, os.Symlink(outside, filepath.Join(root, "outside")))
	unchanged := mustSnapshot(t, root)
	mustFS(t, testutil.WriteFile(filepath.Join(outside, "value"), []byte("after"), 0600))
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

func TestCaptureCleansDescendants(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, tail string
		pty        bool
		timeout    time.Duration
		want       error
	}{
		{"leader exits", "exit 0", false, 3 * time.Second, exec.ErrWaitDelay},
		{"timeout", "wait", false, 200 * time.Millisecond, context.DeadlineExceeded},
		{"PTY", "exit 0", true, 0, exec.ErrWaitDelay},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			tmpRecord, marker, group := filepath.Join(home, "tmpdir"), filepath.Join(home, "escaped"), filepath.Join(home, "group")
			args := []string{"-c", "printf '%s\\n' \"$TMPDIR\" >\"$1\"; echo $$ >\"$3\"; (/bin/sleep 2; /usr/bin/touch \"$2\") & " + tc.tail, "sh", tmpRecord, marker, group}
			start := time.Now()
			var err error
			if tc.pty {
				_, err = capturePTY(home, "/bin/sh", args, nil)
			} else {
				_, err = runCommand(home, append([]string{"/bin/sh"}, args...), nil, nil, tc.timeout)
			}
			if elapsed := time.Since(start); !errors.Is(err, tc.want) || elapsed > 2*time.Second {
				t.Fatalf("capture returned %v after %v", err, elapsed)
			}
			tmp := strings.TrimSpace(string(readBytes(t, tmpRecord)))
			if tmp == "" || tmp == os.TempDir() || strings.HasPrefix(tmp, home+string(os.PathSeparator)) {
				t.Fatalf("TMPDIR not private: %q", tmp)
			}
			if _, err := os.Stat(tmp); !os.IsNotExist(err) {
				t.Fatalf("temporary directory retained: %v", err)
			}
			waitProcessGroupGone(t, group)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("descendant survived cleanup: %v", err)
			}
		})
	}
}

// waitProcessGroupGone polls instead of outwaiting the descendant: a survivor
// keeps its group alive until it writes the marker, so the marker check still fails.
func waitProcessGroupGone(t *testing.T, pidFile string) {
	t.Helper()
	b, err := os.ReadFile(pidFile)
	mustFS(t, err)
	pgid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	mustFS(t, err)
	for deadline := time.Now().Add(10 * time.Second); !errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("process group %d outlived cleanup", pgid)
		}
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
