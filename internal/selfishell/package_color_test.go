//go:build darwin || linux

package selfishell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/pty"
)

func packageTestPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	return master, slave
}

func TestInstallAndUpdateTerminalSummary(t *testing.T) {
	for _, command := range []string{"install", "update"} {
		t.Run(command, func(t *testing.T) {
			for _, noColor := range []string{"", "1"} {
				t.Run("NO_COLOR="+noColor, func(t *testing.T) {
					platform := "macos"
					if runtime.GOOS == "linux" {
						platform = "ubuntu"
					}
					root, _, paths := blockHome(t, platform)
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
			c.report("Configuration", reportSuccess, "Updated managed file: example.zsh")
			c.error("example failure")
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
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CI", "")
	t.Setenv("NO_COLOR", "")
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
	(CLI{Err: slave}).error("example failure")
	readPackagePTY(t, master, "\x1b[31mselfishell:\x1b[0m example failure\n")
	master2, slave2 := packageTestPTY(t)
	defer master2.Close()
	defer slave2.Close()
	f.op.Process.Env = append(f.op.Process.Env, "NO_COLOR=1")
	t.Setenv("NO_COLOR", "1")
	f.op.Process.Out = slave2
	if err := f.op.InstallApt(context.Background(), "required", true, "needed"); err != nil {
		t.Fatal(err)
	}
	(CLI{Err: slave2}).error("example failure")
	got = readPackagePTY(t, master2, "selfishell: example failure\n")
	if strings.Contains(got, "\x1b[") {
		t.Fatal(got)
	}
}

func TestProgressTerminalFailure(t *testing.T) {
	isolateHome(t, t.TempDir())
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CI", "")
	t.Setenv("NO_COLOR", "1")
	master, slave := packageTestPTY(t)
	defer master.Close()
	defer slave.Close()
	ui := newProgress(slave, slave, Paths{})
	defer ui.pause()
	c := CLI{Out: slave, Err: slave, progress: ui}
	p := Process{Out: slave, Err: slave, progress: ui}
	ui.stage("Synchronizing tools")
	code, err := p.Run(context.Background(), "/bin/sh", "-c", "printf 'successful tool chatter\\n'")
	if code != 0 || err != nil {
		t.Fatalf("successful tool: %d %v", code, err)
	}
	c.report("Tools", reportSuccess, "Installed example tool")
	code, err = p.Run(context.Background(), "/bin/sh", "-c", "printf 'failure detail\\n' >&2; exit 7")
	if code != 7 || err != nil {
		t.Fatalf("failed tool: %d %v", code, err)
	}
	// Failure details must be visible before the final summary.
	got := readPackagePTY(t, master, "failure detail\n")
	if !strings.Contains(got, "Synchronizing tools") || strings.Contains(got, "successful tool chatter") {
		t.Fatalf("unexpected progress output: %q", got)
	}
	c.error("installation failed")
	ui.finish()
	fmt.Fprintln(slave, "END")
	got = readPackagePTY(t, master, "END\n")
	if !strings.Contains(got, "selfishell: installation failed") || !strings.Contains(got, "Installed example tool") || strings.Contains(got, "✓") || strings.Contains(got, "\x1b[") {
		t.Fatalf("failure summary or animation after failure: %q", got)
	}
}

func TestConfigurationPromptInterrupt(t *testing.T) {
	if root := os.Getenv("SELFISHELL_TEST_PROMPT_ROOT"); root != "" {
		c := CLI{Root: root, In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
		prepared, err := c.prepareConfig(DetectPlatform().Name, false, false, true)
		if err != nil {
			t.Fatal(err)
		}
		// A file changed after preflight must prompt while configuration progress runs.
		r := failureResource(t, root, "zsh-common")
		blockWrite(t, r.Target, []byte("# user edit after preflight\n"))
		c.progress = newProgress(c.Out, c.Err, prepared.paths)
		prepared.m.c.progress = c.progress
		defer c.progress.finish()
		if err := c.applyManagedResources(&prepared); err != nil {
			t.Fatal(err)
		}
		t.Fatal("configuration continued after interrupt")
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	platform := "macos"
	if runtime.GOOS == "linux" {
		platform = "ubuntu"
	}
	root, _, _ := blockHome(t, platform)
	blockOK(t, root, "install", "--skip-packages", "--yes")
	t.Setenv("SELFISHELL_TEST_PROMPT_ROOT", root)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CI", "")
	master, slave := packageTestPTY(t)
	defer master.Close()
	defer slave.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestConfigurationPromptInterrupt$")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	// Only the child retains the slave; macOS can otherwise stall while exiting.
	if err := slave.Close(); err != nil {
		t.Fatal(err)
	}
	got := readPackagePTY(t, master, "Overwrite with default config? [y/N] ")
	if !strings.Contains(got, "Applying configuration") {
		t.Fatalf("configuration progress did not run: %q", got)
	}
	if _, err := master.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGINT {
		t.Fatalf("prompt did not stop on Ctrl-C: %v (timeout: %v)", err, ctx.Err())
	}
	blockEqual(t, failureResource(t, root, "zsh-common").Target, []byte("# user edit after preflight\n"))
}
