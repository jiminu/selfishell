package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

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
