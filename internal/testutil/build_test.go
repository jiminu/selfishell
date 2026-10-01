package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoCacheOwnership(t *testing.T) {
	borrowed := t.TempDir()
	t.Setenv("GOCACHE", borrowed)
	var private string
	t.Run("private", func(t *testing.T) {
		t.Setenv("SELFISHELL_TEST_GO_CACHE", "")
		private = GoCache(t)
		if private == borrowed || !filepath.IsAbs(private) {
			t.Fatalf("unselected caller cache used: %q", private)
		}
	})
	if _, err := os.Stat(private); !os.IsNotExist(err) {
		t.Fatalf("private cache survived test cleanup: %v", err)
	}
	t.Run("borrowed", func(t *testing.T) {
		t.Setenv("SELFISHELL_TEST_GO_CACHE", borrowed)
		if err := os.WriteFile(filepath.Join(GoCache(t), "marker"), []byte("retained"), 0600); err != nil {
			t.Fatal(err)
		}
	})
	if data, err := os.ReadFile(filepath.Join(borrowed, "marker")); err != nil || string(data) != "retained" {
		t.Fatalf("borrowed cache removed: %q %v", data, err)
	}
}

func TestCopyCLIKeepsFixtureIndependent(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "copy")
	if err := os.WriteFile(source, []byte("original executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_CLI", source)
	if !CopyCLI(t, target) {
		t.Fatal("prebuilt CLI not copied")
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0755 {
		t.Fatalf("fixture CLI must be a regular executable: %v %v", info, err)
	}
	if err := os.WriteFile(target, []byte("fixture modified"), 0700); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "original executable" {
		t.Fatalf("source changed: %q %v", data, err)
	}
}
