//go:build linux && (amd64 || arm64)

package selfishell

import (
	"syscall"
	"unsafe"
)

func moveBackupNoReplace(source, destination string) error {
	old, e := syscall.BytePtrFromString(source)
	if e != nil {
		return e
	}
	new, e := syscall.BytePtrFromString(destination)
	if e != nil {
		return e
	}
	const atFDCWD = ^uintptr(99) // -100 on Linux.
	const renameNoReplace = 1
	_, _, errno := syscall.Syscall6(renameat2Trap, atFDCWD, uintptr(unsafe.Pointer(old)), atFDCWD, uintptr(unsafe.Pointer(new)), renameNoReplace, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
