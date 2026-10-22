package integration_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
)

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
