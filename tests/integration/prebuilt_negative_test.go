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
	f, e := os.Create(path)
	mustFS(t, e)
	gz := gzip.NewWriter(f)
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
			_, e = tw.Write(m.data)
			mustFS(t, e)
		}
	}
	mustFS(t, tw.Close())
	mustFS(t, gz.Close())
	mustFS(t, f.Close())
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
	mustFS(t, os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums.String()), 0644))
}
func TestPrebuiltSmokeRejectsWrongCPUAndPayload(t *testing.T) {
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

func TestMinimalSmokeDoctorRequiresMissingMiseDiagnosis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		got    capture
		reject bool
	}{
		{"unexpected success", capture{Status: 0, Stdout: []byte("Selfishell doctor\n[OK] Tool: mise detected\n")}, true},
		{"missing diagnosis", capture{Status: 1, Stdout: []byte("Selfishell doctor\n")}, true},
		{"expected missing mise", capture{Status: 1, Stdout: []byte("Selfishell doctor\n[ERROR] Tool: mise is missing (direct)\n")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := minimalDoctorDiagnosis(tc.got)
			if (err != nil) != tc.reject {
				t.Fatalf("doctor diagnosis error %v, reject want %t", err, tc.reject)
			}
		})
	}
}
