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

func TestProgressTerminalFailure(t *testing.T) {
	t.Parallel()
	var env Environment
	isolateHome(&env, t.TempDir())
	setEnv(&env, "TERM", "xterm-256color")
	setEnv(&env, "CI", "")
	setEnv(&env, "NO_COLOR", "1")
	master, slave := packageTestPTY(t)
	defer master.Close()
	defer slave.Close()
	ui := newProgress(slave, slave, Paths{}, env)
	defer ui.Pause()
	c := CLI{Env: env, Out: slave, Err: slave, progress: ui}
	p := Process{inherit: env, Out: slave, Err: slave, progress: ui}
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
	if !strings.Contains(got, "Synchronizing tools") || strings.Contains(got, "successful tool chatter") || strings.Contains(got, "Installed example tool") {
		t.Fatalf("unexpected progress output: %q", got)
	}
	c.error("installation failed")
	ui.finish()
	fmt.Fprintln(slave, "END")
	got = readPackagePTY(t, master, "END\n")
	if !strings.Contains(got, "selfishell: installation failed") || !strings.Contains(got, "Completed changes before stopping:") || !strings.Contains(got, "Installed example tool") || strings.Contains(got, "✓") || strings.Contains(got, "\x1b[") {
		t.Fatalf("failure summary or animation after failure: %q", got)
	}
}

func TestConfigurationPromptInterrupt(t *testing.T) {
	t.Parallel()
	if root := os.Getenv("SELFISHELL_TEST_PROMPT_ROOT"); root != "" {
		env := ProcessEnvironment() // the parent's fixture environment
		c := CLI{Env: env, Root: root, In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
		prepared, err := c.prepareConfig(detectPlatform(env).Name, false, false, true, false, false)
		if err != nil {
			t.Fatal(err)
		}
		// A file changed after preflight must prompt while configuration progress runs.
		r := failureResource(t, env, root, "zsh-common")
		blockWrite(t, r.Target, []byte("# user edit after preflight\n"))
		c.progress = newProgress(c.Out, c.Err, prepared.paths, env)
		prepared.m.c.progress = c.progress
		defer c.progress.finish()
		if err := c.applyManagedResources(&prepared); err != nil {
			t.Fatal(err)
		}
		t.Fatal("configuration continued after interrupt")
	}

	var env Environment
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	platform := "macos"
	if runtime.GOOS == "linux" {
		platform = "ubuntu"
	}
	root, _, _ := blockHome(t, &env, platform)
	blockOK(t, env, root, "install", "--skip-packages", "--yes")
	setEnv(&env, "SELFISHELL_TEST_PROMPT_ROOT", root)
	setEnv(&env, "TERM", "xterm-256color")
	setEnv(&env, "CI", "")
	master, slave := packageTestPTY(t)
	defer master.Close()
	defer slave.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestConfigurationPromptInterrupt$")
	cmd.Env = env.List()
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
	blockEqual(t, failureResource(t, env, root, "zsh-common").Target, []byte("# user edit after preflight\n"))
}
