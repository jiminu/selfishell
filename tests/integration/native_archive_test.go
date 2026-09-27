package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/releasebuild"
)

const nativeArchiveVersion = "1.3.2"

var nativeOnce sync.Once
var nativeDir string
var nativeErr error

// nativeAssetDir builds once per test process, so lifecycle tests consume these exact bytes.
func nativeAssetDir(t *testing.T) string {
	t.Helper()
	privateNativeHome(t)
	nativeOnce.Do(func() {
		nativeDir, nativeErr = os.MkdirTemp("", "selfishell-native-assets-")
		if nativeErr != nil {
			return
		}
		nativeErr = releasebuild.Build(context.Background(), repoRoot(), nativeArchiveVersion, nativeDir)
	})
	if nativeErr != nil {
		t.Fatal(nativeErr)
	}
	return nativeDir
}

func privateNativeHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	for key, path := range map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data"), "XDG_STATE_HOME": filepath.Join(home, "state"), "XDG_CACHE_HOME": filepath.Join(home, "cache"), "MISE_DATA_DIR": filepath.Join(home, "mise-data"), "MISE_CACHE_DIR": filepath.Join(home, "mise-cache"), "MISE_CONFIG_DIR": filepath.Join(home, "mise-config"), "MISE_STATE_DIR": filepath.Join(home, "mise-state"), "GOCACHE": filepath.Join(home, "go-cache"),
	} {
		t.Setenv(key, path)
	}
}

type archiveMember struct {
	mode int64
	kind byte
	link string
	data []byte
}

func readReleaseArchive(t *testing.T, path string, normalized bool) map[string]archiveMember {
	t.Helper()
	f, err := os.Open(path)
	mustFS(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	mustFS(t, err)
	if normalized && !gz.ModTime.Equal(time.Unix(946684800, 0)) {
		t.Errorf("gzip timestamp %s", gz.ModTime)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	members := map[string]archiveMember{}
	last := ""
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		mustFS(t, err)
		name := strings.TrimSuffix(strings.TrimPrefix(h.Name, "./"), "/")
		if name <= last {
			t.Fatalf("unordered or duplicate archive member %q after %q", name, last)
		}
		last = name
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
			t.Fatalf("unsafe archive member %q", name)
		}
		if normalized && (h.Uid != 0 || h.Gid != 0 || !h.ModTime.Equal(time.Unix(946684800, 0))) {
			t.Errorf("%s nonnormalized metadata: uid=%d gid=%d mtime=%s", name, h.Uid, h.Gid, h.ModTime)
		}
		data, err := io.ReadAll(tr)
		mustFS(t, err)
		members[name] = archiveMember{h.Mode, h.Typeflag, h.Linkname, data}
	}
	return members
}
func releaseAssetNames(version string) []string {
	names := []string{}
	for _, platform := range []string{"linux", "macos"} {
		for _, arch := range []string{"amd64", "arm64"} {
			names = append(names, fmt.Sprintf("selfishell-%s-%s-%s.tar.gz", version, platform, arch))
		}
	}
	return names
}
func assertAssetSet(t *testing.T, dir, version string) {
	t.Helper()
	names := releaseAssetNames(version)
	entries, err := os.ReadDir(dir)
	mustFS(t, err)
	got := []string{}
	for _, e := range entries {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := append(append([]string{}, names...), "SHA256SUMS", "VERSION")
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("artifact set:\ngot %q\nwant %q", got, want)
	}
	versionBytes, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	mustFS(t, err)
	if string(versionBytes) != version+"\n" {
		t.Fatalf("VERSION %q", versionBytes)
	}
	sums, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	mustFS(t, err)
	var expected strings.Builder
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, name))
		mustFS(t, err)
		fmt.Fprintf(&expected, "%x  %s\n", sha256.Sum256(b), name)
	}
	if string(sums) != expected.String() {
		t.Fatalf("SHA256SUMS mismatch: %q", sums)
	}
}
func assertConfigPayload(t *testing.T, archive, version string, native bool) map[string]archiveMember {
	t.Helper()
	m := readReleaseArchive(t, archive, native)
	for _, name := range []string{"config/shared/zsh/common.zsh", "config/macos/zshrc", "config/ubuntu/zshrc"} {
		if _, ok := m[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	for name := range m {
		if strings.HasPrefix(name, "common/") || strings.HasPrefix(name, "mac/") || strings.HasPrefix(name, "ubuntu/") {
			t.Errorf("unexpected payload root %s", name)
		}
	}
	if !native {
		return m
	}
	expected := map[string]bool{"VERSION": true, "packages.conf": true, "dependencies.conf": true, "bin/selfishell": true, "bin/sfs": true}
	err := filepath.WalkDir(filepath.Join(repoRoot(), "config"), func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(repoRoot(), path)
		if e != nil {
			return e
		}
		expected[filepath.ToSlash(rel)] = true
		return nil
	})
	mustFS(t, err)
	if len(m) != len(expected) {
		t.Errorf("payload entry count %d want %d", len(m), len(expected))
	}
	for name := range expected {
		if _, ok := m[name]; !ok {
			t.Errorf("missing payload %s", name)
		}
	}
	for name := range m {
		if !expected[name] {
			t.Errorf("unexpected payload %s", name)
		}
	}
	if v := m["VERSION"]; v.kind != tar.TypeReg || v.mode != 0644 || string(v.data) != version+"\n" {
		t.Errorf("archive VERSION type/mode/data %+v", v)
	}
	if s := m["bin/sfs"]; s.kind != tar.TypeSymlink || s.mode != 0777 || s.link != "selfishell" {
		t.Errorf("sfs link %+v", s)
	}
	if b := m["bin/selfishell"]; b.kind != tar.TypeReg || b.mode != 0755 {
		t.Errorf("binary mode/type %+v", b)
	}
	for name := range expected {
		if name == "VERSION" || strings.HasPrefix(name, "bin/") {
			continue
		}
		src := filepath.Join(repoRoot(), filepath.FromSlash(name))
		info, e := os.Lstat(src)
		mustFS(t, e)
		entry := m[name]
		if entry.mode != int64(info.Mode().Perm()) {
			t.Errorf("%s mode %o want %o", name, entry.mode, info.Mode().Perm())
		}
		switch {
		case info.IsDir():
			if entry.kind != tar.TypeDir {
				t.Errorf("%s directory type %d", name, entry.kind)
			}
		case info.Mode()&os.ModeSymlink != 0:
			link, e := os.Readlink(src)
			mustFS(t, e)
			if entry.kind != tar.TypeSymlink || entry.link != link {
				t.Errorf("%s link %q want %q", name, entry.link, link)
			}
		default:
			b, e := os.ReadFile(src)
			mustFS(t, e)
			if entry.kind != tar.TypeReg || !bytes.Equal(entry.data, b) {
				t.Errorf("%s data/type differs", name)
			}
		}
	}
	return m
}
func inspectNativeBinary(t *testing.T, platform, arch, version string, b []byte, executeHost bool) {
	t.Helper()
	releaseRoot := t.TempDir()
	mustFS(t, os.MkdirAll(filepath.Join(releaseRoot, "bin"), 0755))
	mustFS(t, os.WriteFile(filepath.Join(releaseRoot, "VERSION"), []byte(version+"\n"), 0644))
	path := filepath.Join(releaseRoot, "bin", "selfishell")
	mustFS(t, os.WriteFile(path, b, 0755))
	switch platform {
	case "linux":
		f, e := elf.Open(path)
		mustFS(t, e)
		defer f.Close()
		want := elf.EM_X86_64
		if arch == "arm64" {
			want = elf.EM_AARCH64
		}
		if f.Machine != want {
			t.Errorf("ELF machine %v want %v", f.Machine, want)
		}
	case "macos":
		f, e := macho.Open(path)
		mustFS(t, e)
		defer f.Close()
		want := macho.CpuAmd64
		if arch == "arm64" {
			want = macho.CpuArm64
		}
		if f.Cpu != want {
			t.Errorf("Mach-O CPU %v want %v", f.Cpu, want)
		}
	}
	info, e := buildinfo.ReadFile(path)
	mustFS(t, e)
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	goos := platform
	if goos == "macos" {
		goos = "darwin"
	}
	for k, v := range map[string]string{"GOOS": goos, "GOARCH": arch, "CGO_ENABLED": "0"} {
		if settings[k] != v {
			t.Errorf("%s=%q want %q", k, settings[k], v)
		}
	}
	if arch == "amd64" && settings["GOAMD64"] != "v1" {
		t.Errorf("GOAMD64 %q", settings["GOAMD64"])
	}
	if arch == "arm64" && settings["GOARM64"] != "v8.0" {
		t.Errorf("GOARM64 %q", settings["GOARM64"])
	}
	if executeHost && (platform == runtime.GOOS || platform == "macos" && runtime.GOOS == "darwin") {
		if arch == runtime.GOARCH {
			cmd := exec.Command(path, "version")
			out, e := cmd.CombinedOutput()
			if e != nil || string(out) != "selfishell "+version+"\n" {
				t.Errorf("native version: %q %v", out, e)
			}
			cmd = exec.Command(path, "help")
			out, e = cmd.CombinedOutput()
			if e != nil || !bytes.Contains(out, []byte("selfishell")) {
				t.Errorf("native help: %q %v", out, e)
			}
		}
	}
}
func TestNativeReleaseArtifacts(t *testing.T) {
	dir := nativeAssetDir(t)
	assertAssetSet(t, dir, nativeArchiveVersion)
	for _, platform := range []string{"linux", "macos"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := fmt.Sprintf("selfishell-%s-%s-%s.tar.gz", nativeArchiveVersion, platform, arch)
			m := assertConfigPayload(t, filepath.Join(dir, name), nativeArchiveVersion, true)
			inspectNativeBinary(t, platform, arch, nativeArchiveVersion, m["bin/selfishell"].data, true)
		}
	}
}
func TestNativeReleaseReproducibleWithHostileEnvironment(t *testing.T) {
	first := nativeAssetDir(t)
	second := t.TempDir()
	t.Setenv("GOOS", "plan9")
	t.Setenv("GOARCH", "386")
	t.Setenv("GOFLAGS", "-tags=unapproved")
	t.Setenv("CGO_ENABLED", "1")
	mustFS(t, releasebuild.Build(context.Background(), repoRoot(), nativeArchiveVersion, second))
	assertAssetSet(t, second, nativeArchiveVersion)
	for _, name := range append(releaseAssetNames(nativeArchiveVersion), "SHA256SUMS", "VERSION") {
		a, e := os.ReadFile(filepath.Join(first, name))
		mustFS(t, e)
		b, e := os.ReadFile(filepath.Join(second, name))
		mustFS(t, e)
		if !bytes.Equal(a, b) {
			t.Errorf("not reproducible: %s", name)
		}
	}
}
func TestProductionNativeBuilderContract(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, "first")
	second := filepath.Join(home, "second")
	for _, out := range []string{first, second} {
		if out == second {
			time.Sleep(time.Second)
		}
		cmd := exec.Command("bash", filepath.Join(repoRoot(), "scripts/build-release.sh"), "--version", "0.2.2", "--output", out)
		cmd.Env = append(baseEnv(home, t.TempDir()), "PATH="+filepath.Join(runtime.GOROOT(), "bin")+":/usr/bin:/bin:/usr/sbin:/sbin", "GOTOOLCHAIN=local", "GOCACHE="+filepath.Join(home, "go-cache"))
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("production builder: %v %s", e, b)
		}
	}
	assertAssetSet(t, first, "0.2.2")
	assertAssetSet(t, second, "0.2.2")
	for _, name := range releaseAssetNames("0.2.2") {
		a, e := os.ReadFile(filepath.Join(first, name))
		mustFS(t, e)
		b, e := os.ReadFile(filepath.Join(second, name))
		mustFS(t, e)
		if !bytes.Equal(a, b) {
			t.Errorf("native archive not reproducible: %s", name)
		}
	}
	a, e := os.ReadFile(filepath.Join(first, "SHA256SUMS"))
	mustFS(t, e)
	b, e := os.ReadFile(filepath.Join(second, "SHA256SUMS"))
	mustFS(t, e)
	if !bytes.Equal(a, b) {
		t.Error("native checksums differ")
	}
	assertConfigPayload(t, filepath.Join(first, "selfishell-0.2.2-linux-amd64.tar.gz"), "0.2.2", true)
}
func TestCanceledNativeBuildDoesNotPublish(t *testing.T) {
	privateNativeHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := filepath.Join(t.TempDir(), "absent")
	if e := releasebuild.Build(ctx, repoRoot(), nativeArchiveVersion, out); e == nil {
		t.Fatal("canceled build succeeded")
	}
	if _, e := os.Lstat(out); !os.IsNotExist(e) {
		t.Fatalf("canceled build created output: %v", e)
	}
}
func TestUnavailableNativeToolchain(t *testing.T) {
	privateNativeHome(t)
	t.Setenv("PATH", t.TempDir())
	out := filepath.Join(t.TempDir(), "absent")
	e := releasebuild.Build(context.Background(), repoRoot(), nativeArchiveVersion, out)
	if e == nil || !strings.Contains(e.Error(), "Go toolchain required") {
		t.Fatalf("error %v", e)
	}
	if _, e := os.Lstat(out); !os.IsNotExist(e) {
		t.Fatalf("missing toolchain created output: %v", e)
	}
}

func TestNativeBuilderCLIOptions(t *testing.T) {
	home := t.TempDir()
	toolBin := t.TempDir()
	mustFS(t, os.Symlink(filepath.Join(runtime.GOROOT(), "bin", "go"), filepath.Join(toolBin, "go")))
	toolPath := toolBin + string(os.PathListSeparator) + "/usr/bin:/bin"
	toolEnv := []string{
		"PATH=" + toolPath, "GOTOOLCHAIN=local", "GOCACHE=" + filepath.Join(home, "go-cache"),
		"MISE_DATA_DIR=" + filepath.Join(home, "mise-data"), "MISE_CACHE_DIR=" + filepath.Join(home, "mise-cache"),
		"MISE_CONFIG_DIR=" + filepath.Join(home, "mise-config"), "MISE_STATE_DIR=" + filepath.Join(home, "mise-state"),
	}
	noGo := t.TempDir()
	dirname, err := exec.LookPath("dirname")
	mustFS(t, err)
	mustFS(t, os.Symlink(dirname, filepath.Join(noGo, "dirname")))
	for _, args := range [][]string{{}, {"--bogus"}, {"--version"}, {"--version", "v1.2.3"}} {
		cmd := exec.Command("/bin/bash", append([]string{filepath.Join(repoRoot(), "scripts/build-release.sh")}, args...)...)
		cmd.Env = append(baseEnv(home, t.TempDir()), "PATH="+noGo)
		b, e := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(e, &exit) || exit.ExitCode() != 2 {
			t.Errorf("without Go args %q: status %v output %s", args, e, b)
		}
	}
	script := filepath.Join(repoRoot(), "scripts/build-release.sh")
	for _, args := range [][]string{{}, {"--bogus"}, {"--version"}, {"--version", "v1.2.3"}} {
		cmd := exec.Command("bash", append([]string{script}, args...)...)
		cmd.Dir = home
		cmd.Env = append(baseEnv(home, t.TempDir()), toolEnv...)
		b, e := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(e, &exit) || exit.ExitCode() != 2 {
			t.Errorf("args %q: status %v output %s", args, e, b)
		}
	}
	ownedOut, err := os.MkdirTemp(repoRoot(), "native-option-output-")
	mustFS(t, err)
	t.Cleanup(func() { os.RemoveAll(ownedOut) })
	out := filepath.Base(ownedOut)
	cmd := exec.Command("bash", script, "--version", nativeArchiveVersion, "--output", out)
	cmd.Dir = home
	cmd.Env = append(baseEnv(home, t.TempDir()), toolEnv...)
	b, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("relative output: %v %s", e, b)
	}
	assertAssetSet(t, ownedOut, nativeArchiveVersion)
}

func TestWrongNativeToolchain(t *testing.T) {
	privateNativeHome(t)
	bin := t.TempDir()
	goStub := filepath.Join(bin, "go")
	mustFS(t, os.WriteFile(goStub, []byte("#!/bin/sh\nprintf 'go1.26.9\\n'\n"), 0755))
	t.Setenv("PATH", bin)
	out := filepath.Join(t.TempDir(), "absent")
	e := releasebuild.Build(context.Background(), repoRoot(), nativeArchiveVersion, out)
	if e == nil || !strings.Contains(e.Error(), "Go 1.27.1 required") {
		t.Fatalf("error %v", e)
	}
	if _, e := os.Lstat(out); !os.IsNotExist(e) {
		t.Fatalf("wrong toolchain created output: %v", e)
	}
}

func TestNativeReleaseIgnoresSourcePathAndMtime(t *testing.T) {
	privateNativeHome(t)
	copied := filepath.Join(t.TempDir(), "source with spaces")
	mustFS(t, os.MkdirAll(copied, 0755))
	for _, name := range []string{"cmd", "internal", "config"} {
		mustFS(t, copyTree(filepath.Join(repoRoot(), name), filepath.Join(copied, name)))
	}
	for _, name := range []string{"go.mod", "packages.conf", "dependencies.conf"} {
		mustFS(t, copyFile(filepath.Join(repoRoot(), name), filepath.Join(copied, name)))
	}
	old := time.Unix(100000000, 0)
	mustFS(t, filepath.WalkDir(copied, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type().IsRegular() {
			return os.Chtimes(path, old, old)
		}
		return nil
	}))
	out := filepath.Join(copied, "dist")
	mustFS(t, releasebuild.Build(context.Background(), copied, nativeArchiveVersion, ""))
	for _, name := range append(releaseAssetNames(nativeArchiveVersion), "SHA256SUMS", "VERSION") {
		a, e := os.ReadFile(filepath.Join(nativeAssetDir(t), name))
		mustFS(t, e)
		b, e := os.ReadFile(filepath.Join(out, name))
		mustFS(t, e)
		if !bytes.Equal(a, b) {
			t.Errorf("source path/mtime altered %s", name)
		}
	}
}

func TestFailedPublishHasNoVerifiedManifest(t *testing.T) {
	privateNativeHome(t)
	out := t.TempDir()
	blocker := filepath.Join(out, releaseAssetNames(nativeArchiveVersion)[0])
	mustFS(t, os.Mkdir(blocker, 0700))
	mustFS(t, os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte("stale\n"), 0644))
	if e := releasebuild.Build(context.Background(), repoRoot(), nativeArchiveVersion, out); e == nil {
		t.Fatal("blocked publish succeeded")
	}
	if _, e := os.Lstat(filepath.Join(out, "SHA256SUMS")); !os.IsNotExist(e) {
		t.Fatalf("published stale or partial checksums: %v", e)
	}
	if info, e := os.Stat(blocker); e != nil || !info.IsDir() {
		t.Fatalf("overwrote unrelated blocker: %v", e)
	}
}

func TestNativeStageUsesOutputFilesystem(t *testing.T) {
	privateNativeHome(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "chosen output")
	finished := make(chan error, 1)
	go func() { finished <- releasebuild.Build(context.Background(), repoRoot(), nativeArchiveVersion, out) }()
	found := false
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(30 * time.Second)
	for !found {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
			t.Fatal("build completed without a private stage beside output")
		case <-ticker.C:
			entries, err := os.ReadDir(parent)
			mustFS(t, err)
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), "selfishell-native-release-") {
					found = true
				}
			}
		case <-deadline:
			t.Fatal("no private stage beside output")
		}
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(parent)
	mustFS(t, err)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "selfishell-native-release-") {
			t.Errorf("staging directory left behind: %s", e.Name())
		}
	}
	assertAssetSet(t, out, nativeArchiveVersion)
}

func TestNativeAssetFixtureCleanedAtSuiteExit(t *testing.T) {
	temp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestNativeReleaseArtifacts$", "-test.count=1")
	cmd.Env = append(os.Environ(), "TMPDIR="+temp)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child tests: %v %s", err, b)
	}
	entries, err := os.ReadDir(temp)
	mustFS(t, err)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "selfishell-native-assets-") {
			t.Fatalf("test process left native artifact fixture: %s", entry.Name())
		}
	}
}
