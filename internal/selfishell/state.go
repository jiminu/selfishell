package selfishell

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"syscall"
)

var ErrMalformedState = errors.New("malformed managed state")

// State retains the six payload fields of Bash's seven-line v2 record.
// In particular, Backup survives pending -> active and repeated installations.
type State struct{ Kind, Status, Target, Reference, Backup, Checksum string }

func (s State) fields() []string {
	return []string{"2", s.Kind, s.Status, s.Target, s.Reference, s.Backup, s.Checksum}
}

func (s State) validate() error {
	switch s.Kind {
	case "file", "link", "block":
	default:
		return ErrMalformedState
	}
	switch s.Status {
	case "pending", "active":
	default:
		return ErrMalformedState
	}
	if s.Target == "" {
		return ErrMalformedState
	}
	for _, field := range s.fields() {
		if strings.ContainsAny(field, "\n\x00") {
			return ErrMalformedState
		}
	}
	return nil
}

// ReadState distinguishes a missing record (fs.ErrNotExist), an invalid record
// (ErrMalformedState), and I/O errors. No partially parsed state escapes.
func ReadState(path string) (State, error) {
	data, err := readStateFile(path)
	if err != nil {
		return State{}, err
	}
	fields := strings.SplitN(string(data), "\n", 8)
	if len(fields) != 8 || fields[0] != "2" || bytes.IndexByte(data, 0) >= 0 {
		return State{}, fmt.Errorf("%w: %s", ErrMalformedState, path)
	}
	state := State{Kind: fields[1], Status: fields[2], Target: fields[3], Reference: fields[4], Backup: fields[5], Checksum: fields[6]}
	if err := state.validate(); err != nil {
		return State{}, fmt.Errorf("%w: %s", err, path)
	}
	// Like Bash read, consume seven terminated fields and ignore trailing lines.
	return state, nil
}

func checkStatePath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: not a regular file: %s", ErrMalformedState, path)
	}
	return nil
}

func readStateFile(path string) ([]byte, error) {
	if err := checkStatePath(path); err != nil {
		return nil, err
	}
	// Do not follow a final symlink or block on a FIFO swapped in after Lstat.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ELOOP) {
		return nil, fmt.Errorf("%w: symlink: %s", ErrMalformedState, path)
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file: %s", ErrMalformedState, path)
	}
	return io.ReadAll(file)
}

// WriteState validates before creating anything, then atomically replaces a
// completed private file. Identical records retain their inode and timestamps.
func WriteState(path string, state State) error {
	return writeState(path, state, func(dir, pattern string) (stateFile, error) { return os.CreateTemp(dir, pattern) }, os.Rename)
}

// This small file boundary allows tests to exercise real partial-write, flush,
// close and rename failures without mutable global filesystem hooks.
type stateFile interface {
	io.Writer
	Sync() error
	Close() error
	Name() string
}

func writeState(path string, state State, create func(string, string) (stateFile, error), rename func(string, string) error) error {
	if err := state.validate(); err != nil {
		return fmt.Errorf("%w: %s", err, path)
	}
	data := []byte(strings.Join(state.fields(), "\n") + "\n")
	current, err := readStateFile(path)
	if err == nil && bytes.Equal(current, data) {
		return nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// filepath.Dir cleans symlink/.. components and can change the selected path.
	dir, name := ".", path
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		dir, name = path[:index], path[index+1:]
		if dir == "" {
			dir = "/"
		}
	}
	if name == "" {
		return fmt.Errorf("%w: empty state filename: %s", ErrMalformedState, path)
	}
	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}
	temporary, err := create(dir, name+".tmp.*")
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		temporary.Close()
		if !committed {
			os.Remove(temporary.Name())
		}
	}()
	n, err := temporary.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := checkStatePath(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := rename(temporary.Name(), path); err != nil {
		return err
	}
	committed = true
	return nil
}
