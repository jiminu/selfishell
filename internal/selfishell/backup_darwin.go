//go:build darwin

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
	const renameatxNP = 488
	const atFDCWD = ^uintptr(1) // -2 on Darwin.
	const renameExcl = 4
	_, _, errno := syscall.Syscall6(renameatxNP, atFDCWD, uintptr(unsafe.Pointer(old)), atFDCWD, uintptr(unsafe.Pointer(new)), renameExcl, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
