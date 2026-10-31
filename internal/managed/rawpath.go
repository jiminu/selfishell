package managed

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// RawParent returns the directory part of path without cleaning it. RawParent
// and MakeRawDir retain symlink/.. traversal from HOME and XDG values.
func RawParent(path string) string {
	at := strings.LastIndexByte(path, '/')
	if at < 0 {
		return "."
	}
	if at == 0 {
		return "/"
	}
	return path[:at]
}

// MakeRawDir creates each missing directory of path, keeping its spelling.
func MakeRawDir(path string) error {
	current := ""
	if strings.HasPrefix(path, "/") {
		current = "/"
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		if current == "" || current == "/" {
			current += part
		} else {
			current += "/" + part
		}
		err := os.Mkdir(current, 0777)
		if err == nil {
			continue
		}
		if !os.IsExist(err) {
			return err
		}
		info, e := os.Stat(current)
		if e != nil {
			return e
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", current)
		}
	}
	return nil
}

// CreateRawTemp creates a private, exclusive temporary sibling of target.
func CreateRawTemp(target string) (*os.File, error) {
	for attempt := 0; attempt < 20; attempt++ {
		var salt [8]byte
		if _, err := rand.Read(salt[:]); err != nil {
			return nil, err
		}
		path := target + ".tmp." + hex.EncodeToString(salt[:])
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		return f, err
	}
	return nil, fmt.Errorf("could not create temporary file for %s", target)
}

// writeRaw stages data in a temporary sibling that publish renames or links
// into place; the temporary name is always removed. A nil mode keeps
// CreateRawTemp's 0600.
func writeRaw(path string, data []byte, mode *os.FileMode, publish func(temp string) error) error {
	if err := MakeRawDir(RawParent(path)); err != nil {
		return err
	}
	f, err := CreateRawTemp(path)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if mode != nil {
		if err = f.Chmod(*mode); err != nil {
			f.Close()
			return err
		}
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return publish(f.Name())
}
func replaceRaw(path string, data []byte, mode os.FileMode) error {
	return writeRaw(path, data, &mode, func(temp string) error { return os.Rename(temp, path) })
}

// createRawOnce leaves an existing path, even one created concurrently, in place.
func createRawOnce(path string, data []byte) error {
	return writeRaw(path, data, nil, func(temp string) error {
		if err := os.Link(temp, path); !os.IsExist(err) {
			return err
		}
		return nil
	})
}

// CreateRawExclusive writes path with mode only if it does not exist yet.
func CreateRawExclusive(path string, data []byte, mode os.FileMode) error {
	return writeRaw(path, data, &mode, func(temp string) error { return os.Link(temp, path) })
}

// WriteAtomic replaces path with data and mode through a renamed temporary file.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	return replaceRaw(path, data, mode)
}

// WriteOnce creates path with data unless it exists, even concurrently.
func WriteOnce(path string, data []byte) error { return createRawOnce(path, data) }
