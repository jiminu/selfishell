//go:build linux

package migration_test

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func openPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	var unlock int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), uintptr(syscall.TIOCSPTLCK), uintptr(unsafe.Pointer(&unlock)))
	if errno != 0 {
		master.Close()
		return nil, nil, errno
	}
	var number uint32
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), uintptr(syscall.TIOCGPTN), uintptr(unsafe.Pointer(&number)))
	if errno != 0 {
		master.Close()
		return nil, nil, errno
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR, 0)
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
