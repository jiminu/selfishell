//go:build darwin

package selfishell

import (
	"bytes"
	"context"
	"fmt"
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

func TestInstallAndUpdateTerminalSummary(t *testing.T) {
	for _, command := range []string{"install", "update"} {
		t.Run(command, func(t *testing.T) {
			for _, noColor := range []string{"", "1"} {
				t.Run("NO_COLOR="+noColor, func(t *testing.T) {
					root, _, paths := blockHome(t, "macos")
					t.Setenv("NO_COLOR", noColor)
					t.Setenv("TERM", "xterm-256color")
					t.Setenv("CI", "")
					blockOK(t, root, "install", "--skip-packages", "--yes")
					r := failureResource(t, root, "zsh-common")
					if err := os.Remove(r.Target); err != nil {
						t.Fatal(err)
					}
					master, slave := packageTestPTY(t)
					defer master.Close()
					defer slave.Close()
					args := []string{command, "--skip-packages", "--yes"}
					if command == "update" {
						args = append(args, "--tools-only")
					}
					cli := CLI{Root: root, Out: slave, Err: slave}
					if code := cli.Run(append(append([]string{}, args...), "--dry-run")); code != 0 {
						t.Fatalf("dry run exited %d", code)
					}
					fmt.Fprintln(slave, "DRYEND")
					dry := readPackagePTY(t, master, "DRYEND\n")
					if strings.Contains(dry, "\x1b[2K") || strings.Contains(dry, "\x1b[36mWould install managed file:") != (noColor == "") {
						t.Fatalf("dry-run output: %q", dry)
					}
					if _, err := os.Stat(r.Target); !os.IsNotExist(err) {
						t.Fatalf("dry run created target: %v", err)
					}
					if _, err := os.Stat(paths.State + "/logs"); !os.IsNotExist(err) {
						t.Fatalf("dry run created logs: %v", err)
					}
					code := (CLI{Root: root, Out: slave, Err: slave}).Run(args)
					fmt.Fprintln(slave, "END")
					got := readPackagePTY(t, master, "END\n")
					if code != 0 || !strings.Contains(got, "Configuration") || strings.Contains(got, "\x1b[32mInstalled managed file:\x1b[0m") != (noColor == "") {
						t.Fatalf("missing completion summary: code=%d output=%q", code, got)
					}
					if strings.Index(got, "Configuration") > strings.Index(got, "Installed managed file:") {
						t.Fatalf("action printed before summary: %q", got)
					}
				})
			}
		})
	}
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

func TestProgressPlainTerminalModes(t *testing.T) {
	for _, mode := range []string{"ci", "dumb"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")
			t.Setenv("CI", "")
			t.Setenv("TERM", "xterm-256color")
			if mode == "ci" {
				t.Setenv("CI", "true")
			} else {
				t.Setenv("TERM", "dumb")
			}
			master, slave := packageTestPTY(t)
			defer master.Close()
			defer slave.Close()
			ui := newProgress(slave, slave, Paths{State: t.TempDir()})
			ui.stage("Applying configuration")
			c := CLI{Out: slave, Err: slave, progress: ui}
			c.report("Configuration", "Updated managed file: example.zsh")
			c.complete("Complete")
			ui.finish()
			fmt.Fprintln(slave, "END")
			if got := readPackagePTY(t, master, "END\n"); strings.Contains(got, "\x1b") {
				t.Fatalf("terminal controls in plain mode: %q", got)
			}
		})
	}
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
