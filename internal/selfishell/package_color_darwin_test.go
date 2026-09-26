//go:build darwin

package selfishell

import (
	"bytes"
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func packageTestPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range []uint{syscall.TIOCPTYGRANT, syscall.TIOCPTYUNLK} {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), uintptr(req), 0)
		if errno != 0 {
			master.Close()
			t.Fatal(errno)
		}
	}
	var name [128]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), uintptr(syscall.TIOCPTYGNAME), uintptr(unsafe.Pointer(&name[0])))
	if errno != 0 {
		master.Close()
		t.Fatal(errno)
	}
	slave, err := os.OpenFile(string(bytes.TrimRight(name[:], "\x00")), os.O_RDWR, 0)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	return master, slave
}

func TestPackageAdapterColorsTerminalStreams(t *testing.T) {
	f := newPackageFixture(t)
	master, slave := packageTestPTY(t)
	defer master.Close()
	f.op.Process.Out = slave
	f.op.Process.Err = slave
	if err := f.op.InstallApt(context.Background(), "required", true, "needed"); err != nil {
		t.Fatal(err)
	}
	if err := f.op.InstallHomebrew(context.Background(), "optional", "formula", false, "needed"); err != nil {
		t.Fatal(err)
	}
	var b [1024]byte
	n, err := master.Read(b[:])
	if err != nil {
		t.Fatal(err)
	}
	slave.Close()
	got := string(b[:n])
	if !strings.Contains(got, "\x1b[36mWould install required apt packages:\x1b[0m") || !strings.Contains(got, "\x1b[33mselfishell: warning:\x1b[0m") {
		t.Fatal(got)
	}
	master2, slave2 := packageTestPTY(t)
	defer master2.Close()
	f.op.Process.Env = append(f.op.Process.Env, "NO_COLOR=1")
	f.op.Process.Out = slave2
	if err := f.op.InstallApt(context.Background(), "required", true, "needed"); err != nil {
		t.Fatal(err)
	}
	n, err = master2.Read(b[:])
	if err != nil {
		t.Fatal(err)
	}
	slave2.Close()
	if strings.Contains(string(b[:n]), "\x1b[") {
		t.Fatal(string(b[:n]))
	}
}
