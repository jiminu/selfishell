//go:build darwin

package selfishell

import (
	"bytes"
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
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

func readPackagePTY(t *testing.T, master *os.File, expected string) string {
	t.Helper()
	if err := syscall.SetNonblock(int(master.Fd()), true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var got strings.Builder
	var chunk [8]byte // Force fragmented reads instead of relying on PTY buffering.
	for time.Now().Before(deadline) {
		n, err := syscall.Read(int(master.Fd()), chunk[:])
		if n > 0 {
			got.Write(chunk[:n])
			if strings.Contains(strings.ReplaceAll(got.String(), "\r\n", "\n"), expected) {
				return got.String()
			}
		}
		if err != nil && err != syscall.EAGAIN && err != syscall.EWOULDBLOCK {
			t.Fatalf("PTY read: %v; partial output %q", err, got.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("PTY output incomplete after deadline: %q", got.String())
	return ""
}

func TestPackageAdapterColorsTerminalStreams(t *testing.T) {
	f := newPackageFixture(t)
	master, slave := packageTestPTY(t)
	defer master.Close()
	defer slave.Close()
	f.op.Process.Out = slave
	f.op.Process.Err = slave
	if err := f.op.InstallApt(context.Background(), "required", true, "needed"); err != nil {
		t.Fatal(err)
	}
	if err := f.op.InstallHomebrew(context.Background(), "optional", "formula", false, "needed"); err != nil {
		t.Fatal(err)
	}
	got := readPackagePTY(t, master, "\x1b[36mWould install required apt packages:\x1b[0m needed\n\x1b[33mselfishell: warning:\x1b[0m Homebrew is required to install packages.\n")
	if !strings.Contains(got, "\x1b[36mWould install required apt packages:\x1b[0m") || !strings.Contains(got, "\x1b[33mselfishell: warning:\x1b[0m") {
		t.Fatal(got)
	}
	master2, slave2 := packageTestPTY(t)
	defer master2.Close()
	defer slave2.Close()
	f.op.Process.Env = append(f.op.Process.Env, "NO_COLOR=1")
	f.op.Process.Out = slave2
	if err := f.op.InstallApt(context.Background(), "required", true, "needed"); err != nil {
		t.Fatal(err)
	}
	got = readPackagePTY(t, master2, "Would install required apt packages: needed\n")
	if strings.Contains(got, "\x1b[") {
		t.Fatal(got)
	}
}
