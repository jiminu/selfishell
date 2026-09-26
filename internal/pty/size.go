package pty

import (
	"os"
	"syscall"
	"unsafe"
)

// Resize sets the terminal dimensions before starting its child.
func Resize(slave *os.File, rows, columns uint16) error {
	size := [4]uint16{rows, columns, 0, 0}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, slave.Fd(), uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&size[0])))
	if errno != 0 {
		return errno
	}
	return nil
}
