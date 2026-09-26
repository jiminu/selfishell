package selfishell

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

type archiveMember struct {
	name, target string
	kind         byte
	body         string
	mode         int64
}

func testArchive(t *testing.T, members ...archiveMember) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.tar.gz")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		mode := m.mode
		if mode == 0 {
			mode = 0644
		}
		h := &tar.Header{Name: m.name, Typeflag: m.kind, Mode: mode, Linkname: m.target, Size: int64(len(m.body))}
		if m.kind != tar.TypeReg && m.kind != tar.TypeRegA {
			h.Size = 0
		}
		if e := tw.WriteHeader(h); e != nil {
			t.Fatal(e)
		}
		if h.Size > 0 {
			if _, e := tw.Write([]byte(m.body)); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := tw.Close(); e != nil {
		t.Fatal(e)
	}
	if e := gz.Close(); e != nil {
		t.Fatal(e)
	}
	if e := f.Close(); e != nil {
		t.Fatal(e)
	}
	return path
}

func TestReleaseArchivePreflightsAllMembers(t *testing.T) {
	valid := []archiveMember{{"VERSION", "", tar.TypeReg, "2.0.0\n", 0644}, {"bin/selfishell", "", tar.TypeReg, "#!/bin/sh\n", 0755}, {"bin/sfs", "selfishell", tar.TypeSymlink, "", 0}}
	for _, bad := range []archiveMember{{"../outside", "", tar.TypeReg, "oops", 0}, {"/absolute", "", tar.TypeReg, "oops", 0}, {"bin/fifo", "", tar.TypeFifo, "", 0}, {"bin/device", "", tar.TypeChar, "", 0}, {"bin/escape", "../../../outside", tar.TypeSymlink, "", 0}, {"bin/dangling", "missing", tar.TypeSymlink, "", 0}, {"bin/cycle", "cycle", tar.TypeSymlink, "", 0}, {"bin/sfs/child", "", tar.TypeReg, "oops", 0}, {"VERSION", "", tar.TypeReg, "duplicate", 0}} {
		members := append(append([]archiveMember{}, valid...), bad)
		archive := testArchive(t, members...)
		stage := filepath.Join(t.TempDir(), "staging")
		if err := extractReleaseArchive(archive, stage); err == nil {
			t.Errorf("accepted unsafe member %+v", bad)
		}
		if _, err := os.Lstat(stage); !os.IsNotExist(err) {
			t.Errorf("wrote staging before complete preflight for %+v: %v", bad, err)
		}
	}
	if err := extractReleaseArchive(testArchive(t, archiveMember{"safe/../traversal", "", tar.TypeReg, "oops", 0}), filepath.Join(t.TempDir(), "stage")); err == nil {
		t.Fatal("accepted traversal-shaped member")
	}
	stage := filepath.Join(t.TempDir(), "staging")
	if err := extractReleaseArchive(testArchive(t, valid...), stage); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(stage + "/bin/sfs"); err != nil || target != "selfishell" {
		t.Fatalf("safe link %q, %v", target, err)
	}
}

func TestReleaseArchiveAcceptsLinksToImpliedDirectories(t *testing.T) {
	archive := testArchive(t,
		archiveMember{"config/shared/file", "", tar.TypeReg, "managed\n", 0644},
		archiveMember{"config-link", "config", tar.TypeSymlink, "", 0},
		archiveMember{"nested-alias", "config-link/shared/file", tar.TypeSymlink, "", 0},
	)
	stage := filepath.Join(t.TempDir(), "stage")
	if err := extractReleaseArchive(archive, stage); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config-link/shared/file", "nested-alias"} {
		data, err := os.ReadFile(stage + "/" + name)
		if err != nil || string(data) != "managed\n" {
			t.Fatalf("%s: %q, %v", name, data, err)
		}
	}
	for _, bad := range [][]archiveMember{
		{{"config/shared/file", "", tar.TypeReg, "managed\n", 0644}, {"config-link", "config", tar.TypeSymlink, "", 0}, {"config-link/new", "", tar.TypeReg, "bad", 0644}},
		{{"a", "b", tar.TypeSymlink, "", 0}, {"b", "a", tar.TypeSymlink, "", 0}},
		{{"VERSION", "", tar.TypeReg, "1.0.0\n", 0644}, {"config/shared/file", "", tar.TypeReg, "managed\n", 0644}, {"a", "config/shared", tar.TypeSymlink, "", 0}, {"b", "a/../VERSION", tar.TypeSymlink, "", 0}},
	} {
		stage := filepath.Join(t.TempDir(), "stage")
		if err := extractReleaseArchive(testArchive(t, bad...), stage); err == nil {
			t.Fatalf("accepted invalid links: %+v", bad)
		}
		if _, err := os.Lstat(stage); !os.IsNotExist(err) {
			t.Fatalf("wrote stage for invalid links: %v", err)
		}
	}
}
