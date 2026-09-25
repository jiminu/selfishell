package selfishell

import (
	"os"
	"syscall"
	"unsafe"
)

// IsTerminal checks a real terminal rather than treating every character device
// (including /dev/null) as one. Darwin/Linux ioctl constants live beside it.
func IsTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	var state syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, file.Fd(), terminalRequest, uintptr(unsafe.Pointer(&state)), 0, 0, 0)
	return errno == 0
}
