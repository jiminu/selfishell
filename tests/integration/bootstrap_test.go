package integration_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/pty"
	"github.com/jiminu/selfishell/internal/releasebuild"
	"github.com/jiminu/selfishell/internal/selfishell"
	"github.com/jiminu/selfishell/internal/testutil"
)

const nextNativeVersion = "1.3.3"
const prereleaseNativeVersion = "1.4.0-beta.2"

type nativeAssetFixture struct {
	once sync.Once
	dir  string
	err  error
}

var extraNative sync.Map

func nativeVersionAssets(t *testing.T, version string) string {
	t.Helper()
	if version == nativeArchiveVersion {
		return nativeAssetDir(t)
	}
	value, _ := extraNative.LoadOrStore(version, &nativeAssetFixture{})
	holder := value.(*nativeAssetFixture)
	holder.once.Do(func() {
		holder.dir, holder.err = os.MkdirTemp("", "selfishell-native-"+version+"-")
		if holder.err == nil {
			holder.err = releasebuild.Build(context.Background(), repoRoot(), version, holder.dir)
		}
	})
	if holder.err != nil {
		t.Fatal(holder.err)
	}
	return holder.dir
}

func hostArchive(version string) string {
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	return fmt.Sprintf("selfishell-%s-%s-%s.tar.gz", version, platform, runtime.GOARCH)
}

type bootstrapFixture struct {
	home, remote, prefix, share, cli, systembin string
	env                                         []string
}

func newBootstrapFixture(t *testing.T, versions ...string) *bootstrapFixture {
	t.Helper()
	home := t.TempDir()
	f := &bootstrapFixture{home: home, remote: t.TempDir(), prefix: filepath.Join(home, "prefix")}
	f.share = filepath.Join(f.prefix, "share/selfishell")
	f.cli = filepath.Join(f.prefix, "bin/selfishell")
	f.systembin = filepath.Join(home, "systembin")
	mustFS(t, os.Mkdir(f.systembin, 0700))
	for _, name := range []string{"bash", "curl", "awk", "sort", "mktemp", "sha256sum", "shasum", "tar", "find", "rm", "ln", "mv", "cp", "sed", "cat", "grep", "readlink", "dirname", "basename", "date", "cksum", "stat", "uname", "xargs", "chmod", "mkdir", "sleep", "tr", "head", "wc", "touch", "gzip", "cmp", "id", "cut", "dd", "tail", "git", "zsh", "vim"} {
		for _, dir := range []string{"/usr/bin", "/bin"} {
			source := filepath.Join(dir, name)
			if info, err := os.Stat(source); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
				mustFS(t, os.Symlink(source, filepath.Join(f.systembin, name)))
				break
			}
		}
	}
	osRelease := filepath.Join(home, "os-release")
	mustFS(t, testutil.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	f.env = []string{
		"SELFISHELL_RELEASE_ROOT=file://" + f.remote,
		"SELFISHELL_TEST_SYSTEM_NAME=Linux", "SELFISHELL_TEST_MACHINE_ARCH=" + runtime.GOARCH,
		"SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + osRelease,
		"MISE_DATA_DIR=" + home + "/mise/data", "MISE_CACHE_DIR=" + home + "/mise/cache", "MISE_CONFIG_DIR=" + home + "/mise/config", "MISE_STATE_DIR=" + home + "/mise/state",
		"MISE_OFFLINE=1", "SELFISHELL_ROOT=" + filepath.Join(home, "hostile-root"),
		"PATH=" + f.systembin,
	}
	for _, name := range []string{"go", "gcc", "cc", "clang", "apt-get", "brew", "sudo", "chsh"} {
		if path, err := resolveCommand(name, f.env); err == nil {
			t.Fatalf("ordinary child PATH exposes %s at %s", name, path)
		}
	}
	for _, v := range versions {
		f.addVersion(t, v)
	}
	return f
}
func (f *bootstrapFixture) addVersion(t *testing.T, version string) {
	t.Helper()
	assets := nativeVersionAssets(t, version)
	target := filepath.Join(f.remote, "download", "v"+version)
	mustFS(t, os.MkdirAll(target, 0700))
	for _, name := range []string{hostArchive(version), "SHA256SUMS", "VERSION"} {
		mustFS(t, copyFile(filepath.Join(assets, name), filepath.Join(target, name)))
	}
}
func (f *bootstrapFixture) latest(t *testing.T, version string) {
	t.Helper()
	target := filepath.Join(f.remote, "latest/download")
	mustFS(t, os.MkdirAll(target, 0700))
	mustFS(t, copyFile(filepath.Join(f.remote, "download", "v"+version, "VERSION"), filepath.Join(target, "VERSION")))
}
func (f *bootstrapFixture) run(t *testing.T, args ...string) capture {
	t.Helper()
	argv := append([]string{"/bin/bash", filepath.Join(repoRoot(), "install.sh"), "--prefix", f.prefix}, args...)
	got, err := runCommand(f.home, argv, nil, f.env, 35*time.Second)
	mustFS(t, err)
	return got
}
func (f *bootstrapFixture) cliRun(t *testing.T, args ...string) capture {
	t.Helper()
	got, err := runCommand(f.home, append([]string{f.cli}, args...), nil, f.env, 30*time.Second)
	mustFS(t, err)
	return got
}

// runPiped keeps the script on stdin while answers arrive through a private
// controlling terminal. A nil answer slice runs without any terminal, even
// when the developer launches the tests from an interactive shell.
func (f *bootstrapFixture) runPiped(t *testing.T, answers []byte, args ...string) capture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	argv := []string{"-c", `cat "$1" | bash -s -- "${@:2}"`, "bash", filepath.Join(repoRoot(), "install.sh"), "--prefix", f.prefix}
	cmd := exec.CommandContext(ctx, "/bin/bash", append(argv, args...)...)
	cmd.Dir = f.home
	cmd.Env = withEnv(baseEnv(f.home, t.TempDir()), f.env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var master, slave *os.File
	if answers != nil {
		var err error
		master, slave, err = pty.Open()
		mustFS(t, err)
		// Drain echoed input just like a real terminal. On macOS an exiting
		// session leader can otherwise block while draining terminal output.
		readDone := make(chan struct{})
		go func() { _, _ = io.Copy(io.Discard, master); close(readDone) }()
		defer func() {
			_ = slave.Close()
			_ = master.Close()
			select {
			case <-readDone:
			case <-time.After(time.Second):
				t.Error("bootstrap PTY reader did not exit after close")
			}
		}()
		cmd.Stdin = slave
		cmd.SysProcAttr.Setctty = true
		cmd.SysProcAttr.Ctty = 0
	}
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if master != nil {
			// Killing the process alone may leave terminal shutdown blocked.
			_ = master.Close()
		}
		return err
	}
	var out, errout bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	err := cmd.Start()
	if err == nil {
		if slave != nil {
			_ = slave.Close()
			_, writeErr := master.Write(answers)
			if writeErr != nil {
				_ = cmd.Cancel()
				_ = cmd.Wait()
				t.Fatal(writeErr)
			}
		}
		err = cmd.Wait()
	}
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	mustFS(t, ctx.Err())
	got := capture{Stdout: out.Bytes(), Stderr: errout.Bytes()}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		got.Status = exit.ExitCode()
	}
	return got
}
func requireOK(t *testing.T, got capture) {
	t.Helper()
	if bytes.Contains(got.Stderr, []byte("command not found")) {
		t.Fatalf("restricted PATH omitted a required utility: %q", got.Stderr)
	}
	if got.Status != 0 {
		t.Fatalf("status=%d stdout=%q stderr=%q", got.Status, got.Stdout, got.Stderr)
	}
}
func requireExit(t *testing.T, got capture, status int) {
	t.Helper()
	if got.Status != status {
		t.Fatalf("status=%d want=%d stdout=%q stderr=%q", got.Status, status, got.Stdout, got.Stderr)
	}
}
func requireLink(t *testing.T, path, target string) {
	t.Helper()
	got, e := os.Readlink(path)
	if e != nil || got != target {
		t.Fatalf("link %s=%q want %q: %v", path, got, target, e)
	}
}
func requireAbsent(t *testing.T, path string) {
	t.Helper()
	_, e := os.Lstat(path)
	if !os.IsNotExist(e) {
		t.Fatalf("path should be absent: %s: %v", path, e)
	}
}
func requireContains(t *testing.T, b []byte, s string) {
	t.Helper()
	if !bytes.Contains(b, []byte(s)) {
		t.Fatalf("missing %q in %q", s, b)
	}
}

// Raw curl output must reach users only inside an installer message.
func requireInstallerErrorsOnly(t *testing.T, stderr []byte) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimRight(string(stderr), "\n"), "\n") {
		if !strings.HasPrefix(line, "selfishell installer: ") {
			t.Fatalf("raw error output: %q", stderr)
		}
	}
}

type releaseServer struct {
	url      string
	mu       sync.Mutex
	requests []string
}

// serveHTTP serves the fixture's release root through real curl, answering the
// first request for once with interrupt.
func (f *bootstrapFixture) serveHTTP(t *testing.T, once string, interrupt http.HandlerFunc) *releaseServer {
	t.Helper()
	s := &releaseServer{}
	files := http.FileServer(http.Dir(f.remote))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		first := r.URL.Path == once && !slices.Contains(s.requests, r.URL.Path)
		s.requests = append(s.requests, r.URL.Path)
		s.mu.Unlock()
		if first {
			interrupt(w, r)
			return
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	s.url = server.URL
	f.env = append(f.env, "SELFISHELL_RELEASE_ROOT="+s.url)
	return s
}

func unavailable(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "busy", http.StatusServiceUnavailable)
}

// stall sends half of the file, then waits for curl to time out.
func (f *bootstrapFixture) stall(w http.ResponseWriter, r *http.Request) {
	body, err := os.ReadFile(filepath.Join(f.remote, filepath.FromSlash(r.URL.Path)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Write(body[:len(body)/2])
	w.(http.Flusher).Flush()
	<-r.Context().Done()
}

func (s *releaseServer) paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}
func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	mustFS(t, e)
	return b
}
func homeSnapshot(t *testing.T, home string) []byte {
	t.Helper()
	data, err := snapshot(home)
	mustFS(t, err)
	return data
}
func assertHomeSnapshot(t *testing.T, home string, before []byte) {
	t.Helper()
	if after := homeSnapshot(t, home); !bytes.Equal(before, after) {
		t.Fatal("failed bootstrap changed HOME state")
	}
}

func TestNativeBootstrapExactAndDefault(t *testing.T) {
	t.Parallel()
	f := newBootstrapFixture(t, nativeArchiveVersion, nextNativeVersion)
	f.latest(t, nextNativeVersion)
	// Force the host asset selector; no other target archive is offered.
	requireOK(t, f.run(t, "--version", nativeArchiveVersion))
	requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
	requireLink(t, f.cli, filepath.Join(f.share, "current/bin/selfishell"))
	requireLink(t, filepath.Join(f.prefix, "bin/sfs"), "selfishell")
	requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+nativeArchiveVersion+"\n")
	requireContains(t, f.cliRun(t, "help").Stdout, "selfishell")
	requireAbsent(t, filepath.Join(f.home, ".config/selfishell"))
	requireAbsent(t, filepath.Join(f.home, ".zshrc"))
	requireAbsent(t, filepath.Join(f.home, ".bashrc"))
	requireOK(t, f.run(t))
	requireLink(t, filepath.Join(f.share, "current"), "releases/"+nextNativeVersion)
	requireLink(t, filepath.Join(f.share, "previous"), "releases/"+nativeArchiveVersion)
	unknownRelease := filepath.Join(f.share, "releases/0.0.1")
	mustFS(t, os.Mkdir(unknownRelease, 0700))
	mustFS(t, testutil.WriteFile(filepath.Join(unknownRelease, "personal"), []byte("keep me\n"), 0600))
	requireOK(t, f.run(t, "--version", nextNativeVersion))
	if got := string(readBytes(t, filepath.Join(unknownRelease, "personal"))); got != "keep me\n" {
		t.Fatalf("unknown release changed: %q", got)
	}
	mustFS(t, os.Remove(filepath.Join(f.prefix, "bin/sfs")))
	requireOK(t, f.run(t, "--version", nextNativeVersion))
	requireLink(t, filepath.Join(f.prefix, "bin/sfs"), "selfishell")
	requireLink(t, filepath.Join(f.share, "previous"), "releases/"+nativeArchiveVersion)
	f.env = append(f.env, "SELFISHELL_RELEASE_ROOT=file:///definitely-unavailable")
	requireOK(t, f.cliRun(t, "rollback", "--yes"))
	requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
}

func TestNativeBootstrapMetadataAndPolicy(t *testing.T) {
	t.Parallel()
	t.Run("latest prerelease", func(t *testing.T) {
		f := newBootstrapFixture(t, prereleaseNativeVersion)
		tags := filepath.Join(f.home, "tags.json")
		mustFS(t, testutil.WriteFile(tags, []byte("[{\"name\":\"v"+prereleaseNativeVersion+"\"}]\n"), 0600))
		f.env = append(f.env, "SELFISHELL_RELEASE_TAGS_API_URL=file://"+tags)
		requireOK(t, f.run(t))
		requireLink(t, filepath.Join(f.share, "current"), "releases/"+prereleaseNativeVersion)
	})
	t.Run("missing metadata", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		got := f.run(t)
		if got.Status == 0 {
			t.Fatal("missing metadata accepted")
		}
		requireContains(t, got.Stderr, "Use --version VERSION to select one.")
		requireInstallerErrorsOnly(t, got.Stderr)
		requireAbsent(t, filepath.Join(f.share, "current"))
	})
	t.Run("unpublished tag", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		tags := filepath.Join(f.home, "tags.json")
		mustFS(t, testutil.WriteFile(tags, []byte("[{\"name\":\"v9.9.9-beta.1\"}]\n"), 0600))
		f.env = append(f.env, "SELFISHELL_RELEASE_TAGS_API_URL=file://"+tags)
		if got := f.run(t); got.Status == 0 {
			t.Fatal("unpublished tag selected")
		}
		requireAbsent(t, filepath.Join(f.share, "current"))
	})
	t.Run("setup-only options without setup", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		f.latest(t, nativeArchiveVersion)
		got := f.run(t, "--yes", "--skip-packages")
		requireOK(t, got)
		requireContains(t, got.Stderr, "--yes and --skip-packages apply only with --setup")
		requireInstallerErrorsOnly(t, got.Stderr)
		requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
		requireAbsent(t, filepath.Join(f.home, ".local/state/selfishell/configured"))
	})
	t.Run("curl policy", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		f.latest(t, nativeArchiveVersion)
		bin := filepath.Join(f.home, "fakebin")
		mustFS(t, os.Mkdir(bin, 0700))
		mustFS(t, testutil.WriteFile(filepath.Join(bin, "curl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$HOME/curl-calls\"\nexec /usr/bin/curl \"$@\"\n"), 0755))
		f.env = append(f.env, "PATH="+bin+":"+f.systembin)
		requireOK(t, f.run(t))
		calls := string(readBytes(t, filepath.Join(f.home, "curl-calls")))
		for _, s := range []string{"--connect-timeout 10", "--speed-limit 1024", "--speed-time 30", "--max-time 15", "--retry 3 --retry-max-time 60"} {
			if !strings.Contains(calls, s) {
				t.Errorf("missing curl policy %s: %s", s, calls)
			}
		}
		foundTransfer := false
		for _, line := range strings.Split(calls, "\n") {
			if strings.Contains(line, " -o ") && !strings.Contains(line, "--max-time") {
				foundTransfer = true
			}
		}
		if !foundTransfer {
			t.Fatalf("transfer had metadata timeout: %s", calls)
		}
	})
	t.Run("bad curl policy", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		f.env = append(f.env, "SELFISHELL_CURL_LOW_SPEED_TIME=invalid")
		got := f.run(t, "--version", nativeArchiveVersion)
		requireExit(t, got, 2)
		requireContains(t, got.Stderr, "must be positive integers")
		requireAbsent(t, filepath.Join(f.share, "current"))
	})
	t.Run("bad versions", func(t *testing.T) {
		f := newBootstrapFixture(t)
		for _, v := range []string{"01.2.3", "1.02.3", "1.2.3-alpha..1", "1.2.3-alpha.01", "", "v"} {
			got := f.run(t, "--version", v)
			if got.Status == 0 {
				t.Errorf("accepted %q", v)
			}
			requireContains(t, got.Stderr, "Invalid semantic version")
			requireAbsent(t, filepath.Join(f.share, "current"))
		}
	})
}

func TestNativeBootstrapDownloadErrors(t *testing.T) {
	t.Parallel()
	t.Run("curl missing", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		f.latest(t, nativeArchiveVersion)
		mustFS(t, os.Remove(filepath.Join(f.systembin, "curl")))
		for _, args := range [][]string{nil, {"--version", nativeArchiveVersion}} {
			got := f.run(t, args...)
			requireExit(t, got, 1)
			requireContains(t, got.Stderr, "curl is required")
			requireInstallerErrorsOnly(t, got.Stderr)
			requireAbsent(t, filepath.Join(f.share, "current"))
		}
	})
	t.Run("transient failure retried", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		archive := "/download/v" + nativeArchiveVersion + "/" + hostArchive(nativeArchiveVersion)
		server := f.serveHTTP(t, archive, unavailable)
		got := f.run(t, "--version", nativeArchiveVersion)
		requireOK(t, got)
		if n := len(slices.DeleteFunc(server.paths(), func(p string) bool { return p != archive })); n != 2 {
			t.Fatalf("archive requested %d times: %v", n, server.paths())
		}
		if bytes.Contains(got.Stderr, []byte("503")) {
			t.Fatalf("recovered attempt reported: %q", got.Stderr)
		}
		requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
	})
	t.Run("partial metadata retried", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		f.latest(t, nativeArchiveVersion)
		const version = "/latest/download/VERSION"
		server := f.serveHTTP(t, version, f.stall)
		f.env = append(f.env, "SELFISHELL_CURL_METADATA_MAX_TIME=1")
		requireOK(t, f.run(t))
		if n := len(slices.DeleteFunc(server.paths(), func(p string) bool { return p != version })); n != 2 {
			t.Fatalf("VERSION requested %d times: %v", n, server.paths())
		}
		requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
	})
	t.Run("explicit version not found", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		f.latest(t, nativeArchiveVersion)
		server := f.serveHTTP(t, "", nil)
		got := f.run(t, "--version", "9.9.9")
		requireExit(t, got, 1)
		url := server.url + "/download/v9.9.9/" + hostArchive("9.9.9")
		requireContains(t, got.Stderr, "Unable to download Selfishell 9.9.9 from "+url+": curl: (22)")
		requireContains(t, got.Stderr, "404")
		requireInstallerErrorsOnly(t, got.Stderr)
		for _, p := range server.paths() {
			if strings.Contains(p, "latest") {
				t.Fatalf("explicit version consulted latest: %v", server.paths())
			}
		}
		requireAbsent(t, filepath.Join(f.share, "current"))
	})
	t.Run("latest discovery reports curl error", func(t *testing.T) {
		f := newBootstrapFixture(t)
		bin := filepath.Join(f.home, "fakebin")
		mustFS(t, os.Mkdir(bin, 0700))
		script := "#!/bin/sh\necho 'Warning: Transient problem. Will retry.' >&2\necho 'curl: (5) Could not resolve proxy: proxy.invalid' >&2\nexit 5\n"
		mustFS(t, testutil.WriteFile(filepath.Join(bin, "curl"), []byte(script), 0755))
		f.env = append(f.env, "PATH="+bin+":"+f.systembin)
		got := f.run(t)
		requireExit(t, got, 1)
		requireContains(t, got.Stderr, "Unable to determine the latest Selfishell release: curl: (5) Could not resolve proxy: proxy.invalid. Use --version VERSION to select one.")
		requireInstallerErrorsOnly(t, got.Stderr)
		if bytes.Contains(got.Stderr, []byte("Warning")) {
			t.Fatalf("curl transcript reported: %q", got.Stderr)
		}
	})
}

func TestNativeBootstrapFailuresAndOwnership(t *testing.T) {
	t.Parallel()
	t.Run("foreign release links", func(t *testing.T) {
		for _, name := range []string{"current", "previous"} {
			t.Run(name, func(t *testing.T) {
				f := newBootstrapFixture(t, nativeArchiveVersion)
				mustFS(t, os.MkdirAll(f.share, 0700))
				link := filepath.Join(f.share, name)
				mustFS(t, os.Symlink("/personal", link))
				before := homeSnapshot(t, f.home)
				got := f.run(t, "--version", nativeArchiveVersion)
				if got.Status == 0 {
					t.Fatal("foreign release link accepted")
				}
				requireLink(t, link, "/personal")
				assertHomeSnapshot(t, f.home, before)
			})
		}
	})
	t.Run("symlinked release", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		elsewhere := filepath.Join(f.home, "elsewhere")
		mustFS(t, os.MkdirAll(filepath.Join(elsewhere, "bin"), 0700))
		mustFS(t, testutil.WriteFile(filepath.Join(elsewhere, "VERSION"), []byte(nativeArchiveVersion+"\n"), 0644))
		mustFS(t, testutil.WriteFile(filepath.Join(elsewhere, "bin/selfishell"), []byte("#!/bin/sh\nexit 0\n"), 0755))
		mustFS(t, os.MkdirAll(filepath.Join(f.share, "releases"), 0700))
		release := filepath.Join(f.share, "releases", nativeArchiveVersion)
		mustFS(t, os.Symlink(elsewhere, release))
		got := f.run(t, "--version", nativeArchiveVersion)
		if got.Status == 0 {
			t.Fatal("symlinked release accepted")
		}
		requireContains(t, got.Stderr, "Release path is not a directory")
		requireAbsent(t, filepath.Join(f.share, "current"))
		requireLink(t, release, elsewhere)
		requireAbsent(t, filepath.Join(f.share, "previous"))
		requireAbsent(t, filepath.Join(f.home, ".local/state/selfishell"))
	})
	t.Run("checksum", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		requireOK(t, f.run(t, "--version", nativeArchiveVersion))
		before := string(readBytes(t, filepath.Join(f.share, "current/VERSION")))
		archive := filepath.Join(f.remote, "download", "v"+nativeArchiveVersion, hostArchive(nativeArchiveVersion))
		mustFS(t, testutil.AppendFile(archive, []byte("corruption")))
		beforeHome := homeSnapshot(t, f.home)
		requireExit(t, f.run(t, "--version", nativeArchiveVersion), 1)
		requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
		if got := string(readBytes(t, filepath.Join(f.share, "current/VERSION"))); got != before {
			t.Fatalf("active changed %q", got)
		}
		assertHomeSnapshot(t, f.home, beforeHome)
	})
	t.Run("foreign cli", func(t *testing.T) {
		for _, kind := range []string{"file", "link"} {
			t.Run(kind, func(t *testing.T) {
				f := newBootstrapFixture(t, nativeArchiveVersion)
				mustFS(t, os.MkdirAll(filepath.Dir(f.cli), 0700))
				if kind == "file" {
					mustFS(t, testutil.WriteFile(f.cli, []byte("user file"), 0600))
				} else {
					mustFS(t, os.Symlink("/usr/bin/true", f.cli))
				}
				before := homeSnapshot(t, f.home)
				requireExit(t, f.run(t, "--version", nativeArchiveVersion), 1)
				assertHomeSnapshot(t, f.home, before)
				if kind == "file" {
					if string(readBytes(t, f.cli)) != "user file" {
						t.Fatal("foreign file changed")
					}
				} else {
					requireLink(t, f.cli, "/usr/bin/true")
				}
				requireAbsent(t, filepath.Join(f.share, "current"))
			})
		}
	})
	t.Run("foreign sfs", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion, nextNativeVersion)
		sfs := filepath.Join(f.prefix, "bin/sfs")
		mustFS(t, os.MkdirAll(filepath.Dir(sfs), 0700))
		mustFS(t, testutil.WriteFile(sfs, []byte("user command\n"), 0600))
		got := f.run(t, "--version", nativeArchiveVersion)
		requireOK(t, got)
		requireContains(t, got.Stdout, "Leaving "+sfs+" in place")
		if string(readBytes(t, sfs)) != "user command\n" {
			t.Fatal("foreign sfs changed")
		}
		mustFS(t, os.Remove(sfs))
		mustFS(t, os.Symlink("/usr/bin/true", sfs))
		requireOK(t, f.run(t, "--version", nextNativeVersion))
		requireLink(t, sfs, "/usr/bin/true")
		requireOK(t, f.cliRun(t, "uninstall", "--purge", "--yes"))
		requireLink(t, sfs, "/usr/bin/true")
		requireAbsent(t, f.cli)
	})
}

func TestNativeBootstrapActivationFailure(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"retained previous", "absent previous", "replaced link", "replaced file", "replaced directory", "restoration failure"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newBootstrapFixture(t, nativeArchiveVersion, nextNativeVersion)
			requireOK(t, f.run(t, "--version", nativeArchiveVersion))
			currentVersion, requestedVersion := nextNativeVersion, nativeArchiveVersion
			if kind == "absent previous" {
				currentVersion, requestedVersion = nativeArchiveVersion, nextNativeVersion
			} else {
				requireOK(t, f.run(t, "--version", nextNativeVersion))
			}
			previous := filepath.Join(f.share, "previous")
			personal := filepath.Join(f.home, "personal")
			if kind == "replaced file" {
				mustFS(t, testutil.WriteFile(personal, []byte("personal data\n"), 0600))
			} else if kind == "replaced directory" {
				mustFS(t, os.Mkdir(personal, 0700))
			} else if kind == "replaced link" {
				mustFS(t, os.Symlink("/personal", personal))
			}
			bin := filepath.Join(f.home, "fakebin")
			mustFS(t, os.Mkdir(bin, 0700))
			mv, err := exec.LookPath("mv")
			mustFS(t, err)
			mustFS(t, testutil.WriteFile(filepath.Join(bin, "mv"), []byte(`#!/bin/bash
for destination; do :; done
if [[ "$destination" == "$SELFISHELL_FAIL_CURRENT" ]]; then
  if [[ "$SELFISHELL_FAILURE_KIND" == replaced* && -L "$SELFISHELL_FAIL_PREVIOUS" && "$(readlink "$SELFISHELL_FAIL_PREVIOUS")" == "$SELFISHELL_OLD_CURRENT" ]]; then
    rm "$SELFISHELL_FAIL_PREVIOUS"
    "$SELFISHELL_REAL_MV" "$SELFISHELL_PERSONAL" "$SELFISHELL_FAIL_PREVIOUS"
  fi
  exit 73
fi
if [[ "$SELFISHELL_FAILURE_KIND" == "restoration failure" && "$destination" == "$SELFISHELL_FAIL_PREVIOUS" && "$(readlink "$2")" == "$SELFISHELL_OLD_PREVIOUS" ]]; then
  exit 74
fi
exec "$SELFISHELL_REAL_MV" "$@"
`), 0755))
			f.env = append(f.env, "PATH="+bin+":"+f.systembin, "SELFISHELL_REAL_MV="+mv,
				"SELFISHELL_FAIL_CURRENT="+filepath.Join(f.share, "current"), "SELFISHELL_FAIL_PREVIOUS="+previous,
				"SELFISHELL_FAILURE_KIND="+kind, "SELFISHELL_OLD_CURRENT=releases/"+currentVersion,
				"SELFISHELL_OLD_PREVIOUS=releases/"+nativeArchiveVersion, "SELFISHELL_PERSONAL="+personal)
			got := f.run(t, "--version", requestedVersion)
			requireExit(t, got, 73)
			requireLink(t, filepath.Join(f.share, "current"), "releases/"+currentVersion)
			requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+currentVersion+"\n")
			switch kind {
			case "retained previous":
				requireLink(t, previous, "releases/"+nativeArchiveVersion)
				requireOK(t, f.cliRun(t, "rollback", "--yes"))
				requireContains(t, f.cliRun(t, "version").Stdout, "selfishell "+nativeArchiveVersion+"\n")
			case "absent previous":
				requireAbsent(t, previous)
			case "replaced link":
				requireLink(t, previous, "/personal")
			case "replaced file":
				if string(readBytes(t, previous)) != "personal data\n" {
					t.Fatal("replaced previous file changed")
				}
			case "replaced directory":
				info, err := os.Lstat(previous)
				mustFS(t, err)
				if !info.IsDir() {
					t.Fatalf("replaced previous directory changed: %v", info.Mode())
				}
			case "restoration failure":
				requireLink(t, previous, "releases/"+currentVersion)
			}
			if strings.HasPrefix(kind, "replaced") || kind == "restoration failure" {
				requireContains(t, got.Stderr, "Failed to restore the previous release link")
			}
		})
	}
}

func TestNativeBootstrapStagingAndTermination(t *testing.T) {
	t.Parallel()
	t.Run("termination", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		bin := filepath.Join(f.home, "fakebin")
		mustFS(t, os.Mkdir(bin, 0700))
		mustFS(t, testutil.WriteFile(filepath.Join(bin, "curl"), []byte("#!/bin/sh\nkill -TERM \"$PPID\"\n"), 0755))
		f.env = append(f.env, "PATH="+bin+":"+f.systembin)
		requireExit(t, f.run(t, "--version", nativeArchiveVersion), 143)
		requireAbsent(t, filepath.Join(f.share, "current"))
	})
	t.Run("concurrent staging", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		bin := filepath.Join(f.home, "fakebin")
		mustFS(t, os.Mkdir(bin, 0700))
		script := `#!/bin/bash
/usr/bin/tar "$@" || exit
while (($# > 0)); do
  if [[ "$1" == -C ]]; then staging="$2"; fi
  shift
done
releases="${staging%/*}"
version="${staging##*/.}"
version="${version%%.tmp.*}"
cp -R "$staging" "$releases/$version"
`
		mustFS(t, testutil.WriteFile(filepath.Join(bin, "tar"), []byte(script), 0755))
		f.env = append(f.env, "PATH="+bin+":"+f.systembin)
		requireOK(t, f.run(t, "--version", nativeArchiveVersion))
		requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
		entries, e := os.ReadDir(filepath.Join(f.share, "releases", nativeArchiveVersion))
		mustFS(t, e)
		for _, entry := range entries {
			if strings.Contains(entry.Name(), ".tmp.") {
				t.Fatalf("nested stage: %s", entry.Name())
			}
		}
	})
	t.Run("stale staging", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		requireOK(t, f.run(t, "--version", nativeArchiveVersion))
		releases := filepath.Join(f.share, "releases")
		stale := filepath.Join(releases, ".9.9.9.tmp.stale")
		fresh := filepath.Join(releases, ".9.9.9.tmp.fresh")
		emptySuffix := filepath.Join(releases, ".9.9.9.tmp.")
		newlineStage := filepath.Join(releases, ".9.9.9.tmp.fresh\npersonal")
		foreignStage := filepath.Join(releases, ".personal.tmp.docs")
		foreignRelease := filepath.Join(releases, "9.9.9")
		obsoleteRelease := filepath.Join(releases, "0.9.0")
		mustFS(t, os.Mkdir(stale, 0700))
		mustFS(t, os.Mkdir(fresh, 0700))
		mustFS(t, os.Mkdir(emptySuffix, 0700))
		mustFS(t, os.Mkdir(newlineStage, 0700))
		mustFS(t, os.Mkdir(foreignStage, 0700))
		mustFS(t, os.Mkdir(foreignRelease, 0700))
		mustFS(t, testutil.WriteFile(filepath.Join(foreignRelease, "personal"), []byte("keep me\n"), 0600))
		mustFS(t, os.MkdirAll(filepath.Join(obsoleteRelease, "bin"), 0700))
		mustFS(t, testutil.WriteFile(filepath.Join(obsoleteRelease, "VERSION"), []byte("0.9.0\n"), 0600))
		mustFS(t, testutil.WriteFile(filepath.Join(obsoleteRelease, "bin/selfishell"), []byte("#!/bin/sh\n"), 0755))
		old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		mustFS(t, os.Chtimes(stale, old, old))
		mustFS(t, os.Chtimes(emptySuffix, old, old))
		mustFS(t, os.Chtimes(newlineStage, old, old))
		mustFS(t, os.Chtimes(foreignStage, old, old))
		requireOK(t, f.run(t, "--version", nativeArchiveVersion))
		requireAbsent(t, stale)
		if _, e := os.Stat(emptySuffix); e != nil {
			t.Errorf("empty-suffix staging deleted: %v", e)
		}
		requireAbsent(t, obsoleteRelease)
		if got := string(readBytes(t, filepath.Join(foreignRelease, "personal"))); got != "keep me\n" {
			t.Fatalf("foreign release changed: %q", got)
		}
		if _, e := os.Stat(foreignStage); e != nil {
			t.Fatalf("foreign staging deleted: %v", e)
		}
		info, e := os.Stat(fresh)
		if e != nil || !info.IsDir() {
			t.Fatalf("fresh stage removed: %v", e)
		}
	})
}

// The helper remains intentionally executable as a process: the smoke interface
// must consume the exact bytes supplied by its caller.
func TestExactReleaseSmoke(t *testing.T) {
	t.Parallel()
	dir, version := os.Getenv("SELFISHELL_TEST_RELEASE_DIR"), os.Getenv("SELFISHELL_TEST_RELEASE_VERSION")
	if dir == "" && version == "" {
		t.Skip("set SELFISHELL_TEST_RELEASE_DIR and SELFISHELL_TEST_RELEASE_VERSION to smoke prebuilt assets")
	}
	if dir == "" || version == "" {
		t.Fatal("both prebuilt release variables are required")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("SELFISHELL_TEST_RELEASE_DIR must be absolute")
	}
	if !validSmokeVersion(version) {
		t.Fatalf("invalid release version %q", version)
	}
	assertAssetSet(t, dir, version)
	smokePrebuiltArchive(t, dir, version)
}
func validSmokeVersion(v string) bool { return selfishell.ValidReleaseVersion(v) }

func TestNativeBootstrapPipedSetup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, system, answers, ghostty string
		status                         int
	}{
		{name: "accept setup", system: "Linux", answers: "y\n"},
		{name: "decline setup", system: "Linux", answers: "n\n", status: 1},
		{name: "accept Ghostty", system: "Darwin", answers: "y\ny\n", ghostty: "1\n"},
		{name: "decline Ghostty", system: "Darwin", answers: "y\nn\n", ghostty: "0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBootstrapFixture(t, nativeArchiveVersion)
			f.env = append(f.env, "SELFISHELL_TEST_SYSTEM_NAME="+tc.system)
			got := f.runPiped(t, []byte(tc.answers), "--version", nativeArchiveVersion, "--setup", "--skip-packages")
			requireExit(t, got, tc.status)
			requireContains(t, got.Stdout, "Install Selfishell configuration?")
			configured := filepath.Join(f.home, ".local/state/selfishell/configured")
			if tc.status != 0 {
				requireAbsent(t, configured)
				requireAbsent(t, filepath.Join(f.home, ".zshrc"))
				return
			}
			requireContains(t, readBytes(t, configured), "1\n")
			requireContains(t, readBytes(t, filepath.Join(f.home, ".zshrc")), "# >>> Selfishell initialize >>>")
			if tc.ghostty != "" {
				requireContains(t, got.Stdout, "Install Ghostty terminal and managed configuration")
				choice := filepath.Join(f.home, ".local/state/selfishell/ghostty")
				if got := string(readBytes(t, choice)); got != tc.ghostty {
					t.Fatalf("Ghostty choice %q want %q", got, tc.ghostty)
				}
				// Reinstallation must reuse the choice without another question.
				reinstalled := f.runPiped(t, []byte("y\n"), "--version", nativeArchiveVersion, "--setup", "--skip-packages")
				requireOK(t, reinstalled)
				if bytes.Contains(reinstalled.Stdout, []byte("Install Ghostty terminal")) {
					t.Fatal("reinstallation asked for the saved Ghostty choice")
				}
				if got := string(readBytes(t, choice)); got != tc.ghostty {
					t.Fatalf("reinstallation changed Ghostty choice to %q", got)
				}
			}
		})
	}
	t.Run("no terminal requires yes", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		got := f.runPiped(t, nil, "--version", nativeArchiveVersion, "--setup", "--skip-packages")
		requireExit(t, got, 2)
		requireContains(t, got.Stderr, "interactive terminal")
		requireContains(t, got.Stderr, "--yes")
		requireAbsent(t, filepath.Join(f.home, ".zshrc"))
		requireAbsent(t, filepath.Join(f.home, ".local/state/selfishell"))
	})
	t.Run("yes needs no terminal", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		got := f.runPiped(t, nil, "--version", nativeArchiveVersion, "--setup", "--skip-packages", "--yes")
		requireOK(t, got)
		if bytes.Contains(got.Stdout, []byte("Install Selfishell configuration?")) {
			t.Fatal("--yes asked for confirmation")
		}
		requireContains(t, readBytes(t, filepath.Join(f.home, ".local/state/selfishell/configured")), "1\n")
	})
}

func TestNativeBootstrapSetupAndPurge(t *testing.T) {
	t.Parallel()
	t.Run("explicit setup and dry run", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		requireOK(t, f.run(t, "--version", nativeArchiveVersion, "--setup", "--skip-packages", "--yes"))
		zshrc := filepath.Join(f.home, ".zshrc")
		info, e := os.Lstat(zshrc)
		mustFS(t, e)
		if !info.Mode().IsRegular() {
			t.Fatalf("setup created nonregular zshrc: %v", info.Mode())
		}
		requireContains(t, readBytes(t, zshrc), "# >>> Selfishell initialize >>>")
		if got := string(readBytes(t, filepath.Join(f.home, ".local/state/selfishell/configured"))); got != "1\n" {
			t.Fatalf("configured %q", got)
		}
		before, e := snapshot(f.home)
		mustFS(t, e)
		requireOK(t, f.cliRun(t, "uninstall", "--restore", "--purge", "--dry-run"))
		after, e := snapshot(f.home)
		mustFS(t, e)
		if !bytes.Equal(before, after) {
			t.Fatal("purge dry-run changed private HOME")
		}
		requireLink(t, f.cli, filepath.Join(f.share, "current/bin/selfishell"))
		requireContains(t, readBytes(t, zshrc), "# >>> Selfishell initialize >>>")
	})
	t.Run("purge removes managed state", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		requireOK(t, f.run(t, "--version", nativeArchiveVersion, "--setup", "--skip-packages", "--yes"))
		cache := filepath.Join(f.home, ".cache/selfishell")
		mustFS(t, os.MkdirAll(cache, 0700))
		mustFS(t, testutil.WriteFile(filepath.Join(cache, "test"), []byte("cache\n"), 0600))
		requireOK(t, f.cliRun(t, "uninstall", "--restore", "--purge", "--yes"))
		requireAbsent(t, f.cli)
		requireAbsent(t, filepath.Join(f.prefix, "bin/sfs"))
		requireAbsent(t, f.share)
		requireAbsent(t, filepath.Join(f.home, ".local/state/selfishell"))
		requireAbsent(t, cache)
		zshrc := filepath.Join(f.home, ".zshrc")
		info, e := os.Lstat(zshrc)
		mustFS(t, e)
		if !info.Mode().IsRegular() || info.Size() != 0 {
			t.Fatalf("purge altered user-owned empty zshrc: %v", info)
		}
	})
	t.Run("retains modified backups", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		requireOK(t, f.run(t, "--version", nativeArchiveVersion, "--setup", "--skip-packages", "--yes"))
		backups := filepath.Join(f.home, ".local/state/selfishell/backups")
		mustFS(t, os.MkdirAll(backups, 0700))
		file := filepath.Join(backups, "vimrc.backup.20260101000000")
		mustFS(t, testutil.WriteFile(file, []byte("user edit\n"), 0600))
		requireOK(t, f.cliRun(t, "uninstall", "--restore", "--purge", "--dry-run"))
		requireOK(t, f.cliRun(t, "uninstall", "--restore", "--purge", "--yes"))
		if got := string(readBytes(t, file)); got != "user edit\n" {
			t.Fatalf("backup changed %q", got)
		}
		entries, e := os.ReadDir(filepath.Dir(backups))
		mustFS(t, e)
		if len(entries) != 1 || entries[0].Name() != "backups" {
			t.Fatalf("purge retained nonbackup state: %v", entries)
		}
	})
	t.Run("restore without purge keeps CLI", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		requireOK(t, f.run(t, "--version", nativeArchiveVersion, "--setup", "--skip-packages", "--yes"))
		requireOK(t, f.cliRun(t, "uninstall", "--restore", "--yes"))
		requireLink(t, f.cli, filepath.Join(f.share, "current/bin/selfishell"))
	})
	t.Run("foreign cli rejects before uninstall", func(t *testing.T) {
		f := newBootstrapFixture(t, nativeArchiveVersion)
		requireOK(t, f.run(t, "--version", nativeArchiveVersion, "--setup", "--skip-packages", "--yes"))
		mustFS(t, os.Remove(f.cli))
		mustFS(t, os.Symlink("/usr/bin/true", f.cli))
		currentBinary := filepath.Join(f.share, "current/bin/selfishell")
		got, e := runCommand(f.home, []string{currentBinary, "uninstall", "--restore", "--purge", "--yes"}, nil, f.env, 25*time.Second)
		mustFS(t, e)
		requireExit(t, got, 1)
		requireLink(t, f.cli, "/usr/bin/true")
		zshrc := filepath.Join(f.home, ".zshrc")
		info, e := os.Lstat(zshrc)
		mustFS(t, e)
		if !info.Mode().IsRegular() {
			t.Fatalf("preflight changed zshrc type: %v", info.Mode())
		}
		requireContains(t, readBytes(t, zshrc), "# >>> Selfishell initialize >>>")
	})
}
