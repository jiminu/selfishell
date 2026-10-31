package release

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestReleaseArchivePreflightsAllMembers(t *testing.T) {
	t.Parallel()
	valid := []testutil.ArchiveMember{{Name: "VERSION", Kind: tar.TypeReg, Body: "2.0.0\n", Mode: 0644}, {Name: "bin/selfishell", Kind: tar.TypeReg, Body: "#!/bin/sh\n", Mode: 0755}, {Name: "bin/sfs", Target: "selfishell", Kind: tar.TypeSymlink}}
	for _, bad := range []testutil.ArchiveMember{{Name: "../outside", Kind: tar.TypeReg, Body: "oops"}, {Name: "/absolute", Kind: tar.TypeReg, Body: "oops"}, {Name: "bin/fifo", Kind: tar.TypeFifo}, {Name: "bin/device", Kind: tar.TypeChar}, {Name: "bin/escape", Target: "../../../outside", Kind: tar.TypeSymlink}, {Name: "bin/dangling", Target: "missing", Kind: tar.TypeSymlink}, {Name: "bin/cycle", Target: "cycle", Kind: tar.TypeSymlink}, {Name: "bin/sfs/child", Kind: tar.TypeReg, Body: "oops"}, {Name: "VERSION", Kind: tar.TypeReg, Body: "duplicate"}} {
		members := append(append([]testutil.ArchiveMember{}, valid...), bad)
		archive := testutil.Archive(t, members...)
		stage := filepath.Join(t.TempDir(), "staging")
		if err := extractReleaseArchive(archive, stage); err == nil {
			t.Errorf("accepted unsafe member %+v", bad)
		}
		if _, err := os.Lstat(stage); !os.IsNotExist(err) {
			t.Errorf("wrote staging before complete preflight for %+v: %v", bad, err)
		}
	}
	if err := extractReleaseArchive(testutil.Archive(t, testutil.ArchiveMember{Name: "safe/../traversal", Kind: tar.TypeReg, Body: "oops"}), filepath.Join(t.TempDir(), "stage")); err == nil {
		t.Fatal("accepted traversal-shaped member")
	}
	stage := filepath.Join(t.TempDir(), "staging")
	if err := extractReleaseArchive(testutil.Archive(t, valid...), stage); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(stage + "/bin/sfs"); err != nil || target != "selfishell" {
		t.Fatalf("safe link %q, %v", target, err)
	}
}

func TestReleaseArchiveAcceptsLinksToImpliedDirectories(t *testing.T) {
	t.Parallel()
	archive := testutil.Archive(t,
		testutil.ArchiveMember{Name: "config/shared/file", Kind: tar.TypeReg, Body: "managed\n", Mode: 0644},
		testutil.ArchiveMember{Name: "config-link", Target: "config", Kind: tar.TypeSymlink},
		testutil.ArchiveMember{Name: "nested-alias", Target: "config-link/shared/file", Kind: tar.TypeSymlink},
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
	for _, bad := range [][]testutil.ArchiveMember{
		{{Name: "config/shared/file", Kind: tar.TypeReg, Body: "managed\n", Mode: 0644}, {Name: "config-link", Target: "config", Kind: tar.TypeSymlink}, {Name: "config-link/new", Kind: tar.TypeReg, Body: "bad", Mode: 0644}},
		{{Name: "a", Target: "b", Kind: tar.TypeSymlink}, {Name: "b", Target: "a", Kind: tar.TypeSymlink}},
		{{Name: "VERSION", Kind: tar.TypeReg, Body: "1.0.0\n", Mode: 0644}, {Name: "config/shared/file", Kind: tar.TypeReg, Body: "managed\n", Mode: 0644}, {Name: "a", Target: "config/shared", Kind: tar.TypeSymlink}, {Name: "b", Target: "a/../VERSION", Kind: tar.TypeSymlink}},
	} {
		stage := filepath.Join(t.TempDir(), "stage")
		if err := extractReleaseArchive(testutil.Archive(t, bad...), stage); err == nil {
			t.Fatalf("accepted invalid links: %+v", bad)
		}
		if _, err := os.Lstat(stage); !os.IsNotExist(err) {
			t.Fatalf("wrote stage for invalid links: %v", err)
		}
	}
}

func TestReleaseArchiveAcceptsRepeatedFiniteDirectoryAlias(t *testing.T) {
	t.Parallel()
	archive := testutil.Archive(t,
		testutil.ArchiveMember{Name: "config/shared/file", Kind: tar.TypeReg, Body: "ok", Mode: 0600},
		testutil.ArchiveMember{Name: "root-link", Target: ".", Kind: tar.TypeSymlink},
		testutil.ArchiveMember{Name: "alias", Target: "root-link/root-link/config/shared/file", Kind: tar.TypeSymlink},
	)
	stage := filepath.Join(t.TempDir(), "stage")
	if err := extractReleaseArchive(archive, stage); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(stage + "/alias"); err != nil || string(data) != "ok" {
		t.Fatalf("staged alias %q: %v", data, err)
	}
}
