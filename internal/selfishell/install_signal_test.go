package selfishell

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
	release := fixture + "/release"
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
	build := exec.Command("go", "build", "-o", release+"/bin/selfishell", "./cmd/selfishell")
	build.Dir = source
	build.Env = withEnvironment(Process{Env: os.Environ()}, map[string]string{
		"HOME": home, "GOTOOLCHAIN": "local", "GOCACHE": home + "/.cache/go-build", "GOMODCACHE": home + "/go/pkg/mod", "GOPROXY": "off",
	}).Env
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real candidate: %v\n%s", err, output)
	}
	bin := fixture + "/fake-bin"
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"apt-get":    "#!/bin/sh\nexit 0\n",
		"dpkg-query": "#!/bin/sh\nprintf started >\"$HOME/child-started\"\nexec /bin/sleep 30\n",
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
	run := func(args []string, tty bool) (*exec.Cmd, *signalBuffer, chan error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, release+"/bin/selfishell", args...)
		cmd.Env = append([]string{}, env...)
		if tty {
			cmd.Env = append(cmd.Env, "SELFISHELL_TEST_TTY=1")
		}
		var output signalBuffer
		cmd.Stdout, cmd.Stderr = &output, &output
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait(); _ = stdin.Close(); cancel() }()
		return cmd, &output, done
	}
	t.Run("blocked confirmation keeps default SIGINT", func(t *testing.T) {
		cmd, output, done := run([]string{"install"}, true)
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
		cmd, output, done := run([]string{"install", "--yes"}, false)
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
}
