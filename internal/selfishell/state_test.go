package selfishell

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func stateFixture() State {
	return State{Kind: "file", Status: "pending", Target: "/home/a file", Reference: "-", Backup: "/home/original.backup.20000101000000.1", Checksum: "123:45"}
}

func TestReadState(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/file.state"
	if state, err := ReadState(path); !errors.Is(err, fs.ErrNotExist) || state != (State{}) {
		t.Fatalf("missing: %+v %v", state, err)
	}
	valid := "2\nfile\npending\n/home/a file\n-\n/home/original.backup.20000101000000.1\n123:45\n"
	cases := []struct {
		name, content string
		valid         bool
	}{
		{"valid", valid, true}, {"extra-lines", valid + "ignored\n", true},
		{"empty", "", false}, {"short", "2\n", false}, {"no-final-LF", strings.TrimSuffix(valid, "\n"), false},
		{"CRLF", strings.ReplaceAll(valid, "\n", "\r\n"), false},
		{"version", strings.Replace(valid, "2\n", "3\n", 1), false},
		{"kind", strings.Replace(valid, "file\n", "directory\n", 1), false},
		{"status", strings.Replace(valid, "pending\n", "done\n", 1), false},
		{"target", strings.Replace(valid, "/home/a file", "", 1), false},
		{"NUL", strings.Replace(valid, "/home/a file", "/home/a\x00file", 1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			state, err := ReadState(path)
			if tc.valid {
				if err != nil || state != stateFixture() {
					t.Fatalf("got %+v %v", state, err)
				}
			} else if !errors.Is(err, ErrMalformedState) || state != (State{}) {
				t.Fatalf("malformed: %+v %v", state, err)
			}
		})
	}
	if err := os.WriteFile(path, []byte("2\nblock\nactive\n target \\name\n\n\n\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := ReadState(path)
	if err != nil || state.Target != " target \\name" || state.Reference != "" || state.Backup != "" || state.Checksum != "" {
		t.Fatalf("empty optional/literal fields: %+v %v", state, err)
	}
}

func TestStatePathTypesAndPermissions(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/state"
	target := dir + "/target"
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"symlink", "dangling", "directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			switch kind {
			case "symlink":
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "dangling":
				if err := os.Symlink(dir+"/missing", path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscall.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			}
			defer os.Remove(path)
			if state, err := ReadState(path); !errors.Is(err, ErrMalformedState) || state != (State{}) {
				t.Fatalf("%s: %+v %v", kind, state, err)
			}
			if err := WriteState(path, stateFixture()); !errors.Is(err, ErrMalformedState) {
				t.Fatalf("write %s: %v", kind, err)
			}
		})
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "untouched" {
		t.Fatalf("referent changed: %q %v", data, err)
	}
	if state, err := ReadState(target + "/child"); err == nil || errors.Is(err, fs.ErrNotExist) || state != (State{}) {
		t.Fatalf("ENOTDIR treated as missing: %+v %v", state, err)
	}
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires an unprivileged test process")
	}
	if err := os.WriteFile(path, []byte("2\nfile\nactive\nx\n-\n-\n0:0\n"), 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	if state, err := ReadState(path); !errors.Is(err, fs.ErrPermission) || state != (State{}) {
		t.Fatalf("unreadable: %+v %v", state, err)
	}
	if err := WriteState(path, stateFixture()); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable write: %v", err)
	}
}

func TestWriteState(t *testing.T) {
	path := t.TempDir() + "/nested/resource.state"
	state := stateFixture()
	if err := WriteState(path, state); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != 0600 {
		t.Fatalf("private state mode: %v", before.Mode())
	}
	if got, err := ReadState(path); err != nil || got != state {
		t.Fatalf("read %+v %v", got, err)
	}
	if err := WriteState(path, state); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("identical write replaced state")
	}
	state.Status = "active"
	if err := WriteState(path, state); err != nil {
		t.Fatal(err)
	}
	got, err := ReadState(path)
	if err != nil || got != state {
		t.Fatalf("pending activation changed backup: %+v %v", got, err)
	}
	for _, bad := range []State{{}, {Kind: "file", Status: "active", Target: "bad\npath"}, {Kind: "link", Status: "active", Target: "x", Reference: "bad\x00path"}} {
		missing := t.TempDir() + "/absent/resource.state"
		if err := WriteState(missing, bad); !errors.Is(err, ErrMalformedState) {
			t.Fatalf("invalid state accepted: %v", err)
		}
		if _, err := os.Stat(filepath.Dir(missing)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("invalid state made directory: %v", err)
		}
	}
}

// Faults occur on real temporary files; the existing state must survive every
// pre-commit failure, including a partial write followed by an I/O error.
type failingStateFile struct {
	*os.File
	failure string
}

var stateIOError = errors.New("injected state I/O failure")

func (f failingStateFile) Write(data []byte) (int, error) {
	if f.failure == "write" {
		n, _ := f.File.Write(data[:len(data)/2])
		return n, stateIOError
	}
	if f.failure == "short" {
		return f.File.Write(data[:len(data)/2])
	}
	return f.File.Write(data)
}
func (f failingStateFile) Sync() error {
	if f.failure == "sync" {
		return stateIOError
	}
	return f.File.Sync()
}
func (f failingStateFile) Close() error {
	err := f.File.Close()
	if f.failure == "close" {
		return stateIOError
	}
	return err
}

func TestStateWriteFailures(t *testing.T) {
	for _, failure := range []string{"create", "write", "short", "sync", "close", "rename"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			path := dir + "/resource.state"
			state := stateFixture()
			if err := WriteState(path, state); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			info, _ := os.Stat(path)
			state.Status = "active"
			create := func(dir, pattern string) (stateFile, error) {
				if failure == "create" {
					return nil, stateIOError
				}
				f, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				return failingStateFile{File: f, failure: failure}, nil
			}
			rename := func(from, to string) error {
				if failure == "rename" {
					return stateIOError
				}
				return os.Rename(from, to)
			}
			err := writeState(path, state, create, rename)
			if !errors.Is(err, stateIOError) && !(failure == "short" && errors.Is(err, io.ErrShortWrite)) {
				t.Fatalf("expected injected failure, got %v", err)
			}
			after, _ := os.ReadFile(path)
			afterInfo, _ := os.Stat(path)
			if !bytes.Equal(before, after) || !os.SameFile(info, afterInfo) {
				t.Fatal("failed write replaced existing state")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Fatalf("temporary leaked: %v", entries)
			}
			if err := WriteState(path, state); err != nil {
				t.Fatalf("retry: %v", err)
			}
		})
	}
}

func TestStateInterruptedWriter(t *testing.T) {
	if path := os.Getenv("SELFISHELL_GO_INTERRUPTED_STATE"); path != "" {
		_ = writeState(path, stateFixture(), func(dir, pattern string) (stateFile, error) { return os.CreateTemp(dir, pattern) }, func(from, to string) error {
			os.Stdout.WriteString("ready\n")
			time.Sleep(time.Hour)
			return os.Rename(from, to)
		})
		os.Exit(0)
	}
	dir := t.TempDir()
	path := dir + "/resource.state"
	old := stateFixture()
	old.Status = "active"
	if err := WriteState(path, old); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	executable, _ := os.Executable()
	cmd := exec.Command(executable, "-test.run=^TestStateInterruptedWriter$")
	cmd.Env = append(os.Environ(), "SELFISHELL_GO_INTERRUPTED_STATE="+path, "HOME="+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	ready := make(chan error, 1)
	go func() { data := make([]byte, 6); _, err := io.ReadFull(out, data); ready <- err }()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not reach commit")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("interruption corrupted existing state")
	}
	if err := WriteState(path, stateFixture()); err != nil {
		t.Fatalf("retry after interruption: %v", err)
	}
	got, err := ReadState(path)
	if err != nil || got != stateFixture() {
		t.Fatalf("retry state: %+v %v", got, err)
	}
}

func TestWriteStatePreservesSymlinkParentMeaning(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/physical/child", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir+"/physical/child", dir+"/alias"); err != nil {
		t.Fatal(err)
	}
	path := dir + "/alias/../state/resource.state"
	if err := WriteState(path, stateFixture()); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadState(dir + "/physical/state/resource.state"); err != nil || got != stateFixture() {
		t.Fatalf("filesystem meaning changed: %+v %v", got, err)
	}
	if _, err := os.Stat(dir + "/state"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("created lexical parent instead: %v", err)
	}
}
