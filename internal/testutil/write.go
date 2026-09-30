package testutil

import (
	"os"
	"syscall"
)

// WriteFile is os.WriteFile under syscall.ForkLock. A concurrent fork would
// inherit the write descriptor until its exec, and executing the file
// meanwhile fails with ETXTBSY on Linux.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	return os.WriteFile(path, data, perm)
}

// AppendFile appends to an existing file under the same lock as WriteFile.
func AppendFile(path string, data []byte) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
