package selfishell

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

type signalBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *signalBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}
func (b *signalBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.data.String() }

func TestRealInstallSignalHandlingIsScoped(t *testing.T) {
	source, fixture, home := testRelease(t), t.TempDir(), t.TempDir()
	isolateHome(t, home)
	share := fixture + "/selfishell"
	release := share + "/releases/1.0.0"
	if err := os.MkdirAll(release+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source+"/config", release+"/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release+"/dependencies.conf", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release+"/packages.conf", []byte("package ubuntu required apt demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	buildNativeTestCLI(t, source, home, release+"/bin/selfishell")
	fixtureFile(t, release+"/VERSION", "1.0.0\n", 0600)
	if err := os.Symlink("releases/1.0.0", share+"/current"); err != nil {
		t.Fatal(err)
	}
	bin := fixture + "/fake-bin"
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"apt-get":    "#!/bin/sh\nexit 0\n",
		"dpkg-query": "#!/bin/sh\nprintf '%s\\n' \"$$\" >\"$HOME/child-started\"\nexec /bin/sleep 30\n",
	} {
		if err := os.WriteFile(bin+"/"+name, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	osRelease := fixture + "/os-release"
	if err := os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env := withEnvironment(Process{Env: os.Environ()}, map[string]string{
		"HOME": home, "PATH": bin + ":/usr/bin:/bin", "SHELL": "/bin/zsh",
		"SELFISHELL_TEST_SYSTEM_NAME": "Linux", "SELFISHELL_TEST_OS_RELEASE_FILE": osRelease,
	}).Env
	run := func(t *testing.T, args []string, tty bool) (*exec.Cmd, *signalBuffer, chan error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, release+"/bin/selfishell", args...)
		cmd.Env = append([]string{}, env...)
		if tty {
			cmd.Env = append(cmd.Env, "SELFISHELL_TEST_TTY=1")
		}
		var output signalBuffer
		cmd.Stdout, cmd.Stderr = &output, &output
		cmd.WaitDelay = time.Second
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); cancel(); _ = stdin.Close() })
		done := make(chan error, 1)
		go func() { done <- cmd.Wait(); _ = stdin.Close(); cancel() }()
		return cmd, &output, done
	}
	t.Run("blocked confirmation keeps default SIGINT", func(t *testing.T) {
		cmd, output, done := run(t, []string{"install"}, true)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && !strings.Contains(output.String(), "Install Selfishell configuration?") {
			time.Sleep(10 * time.Millisecond)
		}
		if !strings.Contains(output.String(), "Install Selfishell configuration?") {
			t.Fatalf("prompt never appeared: %s", output.String())
		}
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("prompt ignored SIGINT")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("prompt hung after SIGINT")
		}
	})
	t.Run("blocked package child is canceled and reaped", func(t *testing.T) {
		cmd, output, done := run(t, []string{"install", "--yes"}, false)
		marker := filepath.Join(home, "child-started")
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("package child did not start: %v %s", err, output.String())
		}
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err == nil || !strings.Contains(output.String(), "context canceled") {
				t.Fatalf("child cancellation: %v %s", err, output.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("package child hung after SIGINT")
		}
	})
	t.Run("full update forwards cancellation to package phase", func(t *testing.T) {
		binary, err := os.ReadFile(release + "/bin/selfishell")
		if err != nil {
			t.Fatal(err)
		}
		members := []archiveMember{
			{"VERSION", "", 0, "2.0.0\n", 0644},
			{"bin/selfishell", "", 0, string(binary), 0755},
			{"packages.conf", "", 0, "package ubuntu required apt demo\n", 0644},
			{"dependencies.conf", "", 0, "", 0644},
		}
		if err := filepath.WalkDir(source+"/config", func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err == nil {
				members = append(members, archiveMember{strings.TrimPrefix(path, source+"/"), "", 0, string(data), 0644})
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		publishReleaseFixture(t, "2.0.0", members...)
		env = withEnvironment(Process{Env: env}, map[string]string{"SELFISHELL_RELEASE_ROOT": os.Getenv("SELFISHELL_RELEASE_ROOT")}).Env
		fixtureFile(t, home+"/.local/state/selfishell/configured", "1\n", 0600)
		for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
			t.Run(sig.String(), func(t *testing.T) {
				marker := home + "/child-started"
				_ = os.Remove(marker)
				if err := atomicReleaseLink("releases/1.0.0", share+"/current"); err != nil {
					t.Fatal(err)
				}
				cmd, output, done := run(t, []string{"update", "--version", "2.0.0", "--yes"}, false)
				pid := 0
				t.Cleanup(func() {
					if pid > 0 {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
				})
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					if data, err := os.ReadFile(marker); err == nil {
						pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
						if pid > 0 {
							break
						}
					}
					time.Sleep(10 * time.Millisecond)
				}
				if pid == 0 {
					t.Fatalf("continuation package did not start: %s", output.String())
				}
				if err := cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-done:
					if err == nil || cmd.ProcessState.ExitCode() != 130 || !strings.Contains(output.String(), "context canceled") || !strings.Contains(output.String(), "Activated Selfishell CLI: 1.0.0 -> 2.0.0") {
						t.Fatalf("continuation cancellation: %v %s", err, output.String())
					}
				case <-time.After(2 * time.Second):
					t.Fatal("continuation hung after signal")
				}
				if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
					t.Fatalf("continuation left package child running: pid=%d err=%v", pid, err)
				}
				pid = 0
			})
		}
	})
}

func TestRealReleaseSignalHandlingIsScoped(t *testing.T) {
	fixture, home := t.TempDir(), t.TempDir()
	isolateHome(t, home)
	share := fixture + "/selfishell"
	release := share + "/releases/1.0.0"
	if err := os.MkdirAll(release+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	buildNativeTestCLI(t, testRelease(t), home, release+"/bin/selfishell")
	if err := os.WriteFile(release+"/VERSION", []byte("1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("releases/1.0.0", share+"/current"); err != nil {
		t.Fatal(err)
	}
	bin := fixture + "/fake-bin"
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin+"/curl", []byte(`#!/bin/sh
if [ "$SELFISHELL_TEST_CURL_READY" = 1 ]; then
  while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then printf '2.0.0\n' >"$2"; exit 0; fi
    shift
  done
  exit 90
fi
printf '%s\n' "$$" >"$SELFISHELL_TEST_CURL_PID"
exec /bin/sleep 30
`), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		args   []string
		signal os.Signal
		prompt bool
	}{
		{"available metadata SIGINT", []string{"version", "--available"}, os.Interrupt, false},
		{"update metadata SIGTERM", []string{"update", "--cli-only", "--yes"}, syscall.SIGTERM, false},
		{"exact transfer SIGTERM", []string{"update", "--cli-only", "--version", "2.0.0", "--yes"}, syscall.SIGTERM, false},
		{"confirmation after metadata keeps SIGINT", []string{"update", "--cli-only"}, os.Interrupt, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			temporary := t.TempDir()
			pidFile := temporary + "/curl.pid"
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, release+"/bin/selfishell", tc.args...)
			cmd.Env = withEnvironment(Process{Env: os.Environ()}, map[string]string{
				"HOME": home, "PATH": bin + ":/usr/bin:/bin", "TMPDIR": temporary,
				"SELFISHELL_RELEASE_ROOT":  "file:///selfishell-signal-fixture",
				"SELFISHELL_TEST_CURL_PID": pidFile, "SELFISHELL_TEST_TTY": "1",
				"SELFISHELL_TEST_CURL_READY": map[bool]string{true: "1", false: "0"}[tc.prompt],
			}).Env
			var output signalBuffer
			cmd.Stdout, cmd.Stderr, cmd.WaitDelay = &output, &output, time.Second
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			var waitErr error
			go func() { waitErr = cmd.Wait(); close(done) }()
			pid := 0
			t.Cleanup(func() {
				if pid > 0 {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
				_ = cmd.Process.Kill()
				cancel()
				<-done
			})
			deadline := time.Now().Add(2 * time.Second)
			ready := false
			for time.Now().Before(deadline) {
				if tc.prompt {
					ready = strings.Contains(output.String(), "Update Selfishell CLI to 2.0.0?")
				} else if data, err := os.ReadFile(pidFile); err == nil {
					pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
					ready = err == nil && pid > 0
				}
				if ready {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !ready {
				t.Fatalf("release phase did not start: %s", output.String())
			}
			if err := cmd.Process.Signal(tc.signal); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
				if waitErr == nil {
					t.Fatalf("interrupted CLI reported success: %s", output.String())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("CLI hung after signal")
			}
			if pid > 0 {
				if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
					t.Fatalf("transfer child was not canceled and reaped: pid=%d err=%v", pid, err)
				}
				pid = 0
				_ = os.Remove(pidFile)
			}
			entries, err := os.ReadDir(temporary)
			if err != nil || len(entries) != 0 {
				t.Fatalf("interrupted release left temporary downloads: %v %v", entries, err)
			}
			current, err := os.Readlink(share + "/current")
			if err != nil || current != "releases/1.0.0" {
				t.Fatalf("interrupted release activated: %q %v", current, err)
			}
			if entries, err := os.ReadDir(share + "/releases"); err != nil || len(entries) != 1 {
				t.Fatalf("interrupted release left a partial target or stage: %v %v", entries, err)
			}
		})
	}
}

func buildNativeTestCLI(t *testing.T, source, home, target string) {
	t.Helper()
	if testutil.CopyCLI(t, target) {
		return
	}
	build := exec.Command("go", "build", "-o", target, "./cmd/selfishell")
	build.Dir = source
	build.Env = withEnvironment(Process{Env: os.Environ()}, map[string]string{
		"HOME": home, "GOTOOLCHAIN": "local", "GOCACHE": testutil.GoCache(t), "GOMODCACHE": home + "/go/pkg/mod", "GOPROXY": "off",
	}).Env
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test CLI: %v\n%s", err, output)
	}
}
