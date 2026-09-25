//go:build darwin

package migration_test

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

func openPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	ioctl := func(req uint) error {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), uintptr(req), 0)
		if errno != 0 {
			return errno
		}
		return nil
	}
	if err = ioctl(syscall.TIOCPTYGRANT); err != nil {
		master.Close()
		return nil, nil, err
	}
	if err = ioctl(syscall.TIOCPTYUNLK); err != nil {
		master.Close()
		return nil, nil, err
	}
	var name [128]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), uintptr(syscall.TIOCPTYGNAME), uintptr(unsafe.Pointer(&name[0])))
	if errno != 0 {
		master.Close()
		return nil, nil, errno
	}
	slave, err := os.OpenFile(string(bytes.TrimRight(name[:], "\x00")), os.O_RDWR, 0)
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
