package selfishell

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// rawParent and makeRawDir retain symlink/.. traversal from HOME and XDG values.
func rawParent(path string) string {
	at := strings.LastIndexByte(path, '/')
	if at < 0 {
		return "."
	}
	if at == 0 {
		return "/"
	}
	return path[:at]
}
func makeRawDir(path string) error {
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
func createRawTemp(target string) (*os.File, error) {
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
func replaceRaw(path string, data []byte, mode os.FileMode) error {
	if err := makeRawDir(rawParent(path)); err != nil {
		return err
	}
	f, err := createRawTemp(path)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func createRawOnce(path string, data []byte) error {
	if err := makeRawDir(rawParent(path)); err != nil {
		return err
	}
	f, err := createRawTemp(path)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Link(f.Name(), path); os.IsExist(err) {
		return nil
	}
	return err
}

func createRawExclusive(path string, data []byte, mode os.FileMode) error {
	if err := makeRawDir(rawParent(path)); err != nil {
		return err
	}
	f, err := createRawTemp(path)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Link(f.Name(), path)
}
