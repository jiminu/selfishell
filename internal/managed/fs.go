package managed

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Present reports whether path exists, without following a final symlink.
func Present(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// SiblingName returns an unused name target+suffix+random hex.
func SiblingName(target, suffix string) (string, error) {
	for i := 0; i < 20; i++ {
		var salt [8]byte
		if _, err := rand.Read(salt[:]); err != nil {
			return "", err
		}
		name := target + suffix + hex.EncodeToString(salt[:])
		if exists, err := Present(name); err != nil {
			return "", err
		} else if !exists {
			return name, nil
		}
	}
	return "", fmt.Errorf("no unused sibling for %s", target)
}

// MoveAside renames an existing target to an unused ".previous." sibling and
// reports the sibling; an absent target is left alone.
func MoveAside(target string) (string, bool, error) {
	exists, err := Present(target)
	if err != nil || !exists {
		return "", false, err
	}
	old, err := SiblingName(target, ".previous.")
	if err != nil {
		return "", false, err
	}
	if err := os.Rename(target, old); err != nil {
		return "", false, err
	}
	return old, true, nil
}

// RestoreEmpty renames old back to target only while target is absent.
func RestoreEmpty(old, target string) error {
	exists, err := Present(target)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("cannot restore over occupied target: %s", target)
	}
	return os.Rename(old, target)
}

// Exists is Lstat that reports a missing path as false without an error.
func Exists(path string) (os.FileInfo, bool, error) {
	i, e := os.Lstat(path)
	if errors.Is(e, fs.ErrNotExist) {
		return nil, false, nil
	}
	return i, e == nil, e
}
