package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func rewriteSuppliedArchive(t *testing.T, path string, change func(map[string]archiveMember)) {
	t.Helper()
	members := readReleaseArchive(t, path, true)
	change(members)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	gz.ModTime = time.Unix(946684800, 0).UTC()
	gz.OS = 255
	tw := tar.NewWriter(gz)
	for _, name := range names {
		m := members[name]
		h := &tar.Header{Name: name, Mode: m.mode, Uid: 0, Gid: 0, ModTime: time.Unix(946684800, 0).UTC(), Format: tar.FormatPAX, Typeflag: m.kind, Linkname: m.link}
		if m.kind == tar.TypeDir {
			h.Name += "/"
		}
		if m.kind == tar.TypeReg {
			h.Size = int64(len(m.data))
		}
		mustFS(t, tw.WriteHeader(h))
		if m.kind == tar.TypeReg {
			_, e := tw.Write(m.data)
			mustFS(t, e)
		}
	}
	mustFS(t, tw.Close())
	mustFS(t, gz.Close())
	mustFS(t, testutil.WriteFile(path, data.Bytes(), 0644))
}
func copiedSuppliedAssets(t *testing.T) string {
	t.Helper()
	source := nativeAssetDir(t)
	dir := t.TempDir()
	for _, name := range append(releaseAssetNames(nativeArchiveVersion), "SHA256SUMS", "VERSION") {
		mustFS(t, copyFile(filepath.Join(source, name), filepath.Join(dir, name)))
	}
	return dir
}
func refreshSuppliedChecksums(t *testing.T, dir string) {
	t.Helper()
	var sums strings.Builder
	for _, name := range releaseAssetNames(nativeArchiveVersion) {
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(readBytes(t, filepath.Join(dir, name))), name)
	}
	mustFS(t, testutil.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums.String()), 0644))
}
func TestPrebuiltSmokeRejectsWrongCPUAndPayload(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, want string
		change     func(*testing.T, map[string]archiveMember)
	}{
		{"higher CPU", "GOAMD64", func(t *testing.T, m map[string]archiveMember) {
			b := m["bin/selfishell"]
			old := []byte("GOAMD64=v1")
			if !bytes.Contains(b.data, old) {
				t.Fatal("fixture has no GOAMD64=v1 setting")
			}
			b.data = bytes.ReplaceAll(b.data, old, []byte("GOAMD64=v2"))
			m["bin/selfishell"] = b
		}},
		{"missing foreign config", "missing payload config/ubuntu/zshrc", func(_ *testing.T, m map[string]archiveMember) { delete(m, "config/ubuntu/zshrc") }},
		{"wrong foreign mode", "mode", func(_ *testing.T, m map[string]archiveMember) {
			entry := m["config/shared/vimrc"]
			entry.mode = 0600
			m["config/shared/vimrc"] = entry
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := copiedSuppliedAssets(t)
			archive := filepath.Join(dir, "selfishell-"+nativeArchiveVersion+"-linux-amd64.tar.gz")
			rewriteSuppliedArchive(t, archive, func(m map[string]archiveMember) { tc.change(t, m) })
			refreshSuppliedChecksums(t, dir)
			cmd := exec.Command(os.Args[0], "-test.run=^TestExactReleaseSmoke$", "-test.count=1", "-test.v")
			cmd.Env = append(os.Environ(), "SELFISHELL_TEST_RELEASE_DIR="+dir, "SELFISHELL_TEST_RELEASE_VERSION="+nativeArchiveVersion, "PATH="+t.TempDir())
			out, e := cmd.CombinedOutput()
			if e == nil || !bytes.Contains(out, []byte(tc.want)) {
				t.Fatalf("accepted invalid supplied %s: %v %s", tc.name, e, out)
			}
		})
	}
}

func TestNativeBootstrapValidatesExecutable(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"fresh", "retained"} {
		for _, kind := range []string{"directory", "symlink", "non-executable"} {
			t.Run(source+" "+kind, func(t *testing.T) {
				t.Parallel()
				f := newBootstrapFixture(t, nativeArchiveVersion)
				release := filepath.Join(f.share, "releases", nativeArchiveVersion)
				if source == "fresh" {
					dir := filepath.Join(f.remote, "download", "v"+nativeArchiveVersion)
					archive := filepath.Join(dir, hostArchive(nativeArchiveVersion))
					rewriteSuppliedArchive(t, archive, func(m map[string]archiveMember) {
						entry := m["bin/selfishell"]
						switch kind {
						case "directory":
							entry.kind, entry.data = tar.TypeDir, nil
						case "symlink":
							m["bin/selfishell-real"] = entry
							entry.kind, entry.link, entry.data = tar.TypeSymlink, "selfishell-real", nil
						case "non-executable":
							entry.mode = 0644
						}
						m["bin/selfishell"] = entry
					})
					sum := fmt.Sprintf("%x  %s\n", sha256.Sum256(readBytes(t, archive)), hostArchive(nativeArchiveVersion))
					mustFS(t, testutil.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sum), 0644))
				} else {
					mustFS(t, os.MkdirAll(filepath.Join(release, "bin"), 0700))
					mustFS(t, testutil.WriteFile(filepath.Join(release, "VERSION"), []byte(nativeArchiveVersion+"\n"), 0644))
					exe := filepath.Join(release, "bin/selfishell")
					switch kind {
					case "directory":
						mustFS(t, os.Mkdir(exe, 0755))
					case "symlink":
						mustFS(t, testutil.WriteFile(exe+"-real", []byte("#!/bin/sh\nexit 0\n"), 0755))
						mustFS(t, os.Symlink("selfishell-real", exe))
					case "non-executable":
						mustFS(t, testutil.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0644))
					}
				}
				var before []byte
				if source == "retained" {
					before = homeSnapshot(t, release)
				}
				got := f.run(t, "--version", nativeArchiveVersion)
				if kind == "symlink" {
					// Native release validation follows links to regular executables.
					requireOK(t, got)
					requireLink(t, filepath.Join(release, "bin/selfishell"), "selfishell-real")
					requireLink(t, filepath.Join(f.share, "current"), "releases/"+nativeArchiveVersion)
					return
				}
				requireExit(t, got, 1)
				if source == "fresh" {
					requireContains(t, got.Stderr, "Release archive is invalid")
					requireAbsent(t, release)
				} else {
					requireContains(t, got.Stderr, "Existing release is incomplete")
					assertHomeSnapshot(t, release, before)
				}
				requireAbsent(t, filepath.Join(f.share, "current"))
				requireAbsent(t, filepath.Join(f.share, "previous"))
				requireAbsent(t, f.cli)
			})
		}
	}
}
