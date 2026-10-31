// Package terminal detects terminals and owns the Ghostty and Windows Terminal
// integrations, including the Windows fonts they select.
package terminal

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

// Width is the stream's terminal width; it falls back to 80 columns for plain streams and unset PTY sizes.
func Width(stream any) int {
	file, ok := stream.(*os.File)
	if ok {
		var size struct{ Rows, Columns, X, Y uint16 }
		_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, file.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)), 0, 0, 0)
		if errno == 0 && size.Columns > 0 {
			return int(size.Columns)
		}
	}
	return 80
}
