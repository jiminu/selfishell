package testutil

import (
	"os"
	"path/filepath"
	"syscall"
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
	source, target, unrelated := filepath.Join(root, "source"), filepath.Join(root, "copy"), filepath.Join(root, "unrelated")
	if err := os.WriteFile(source, []byte("original executable"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_CLI", source)
	// The benchmark may already be using the exact prebuilt path.
	if !CopyCLI(t, source) {
		t.Fatal("prebuilt CLI not reused")
	}
	if err := os.WriteFile(unrelated, []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"absent", "symlink", "hardlink", "unrelated symlink", "restrictive umask"} {
		t.Run(mode, func(t *testing.T) {
			var err error
			switch mode {
			case "symlink":
				err = os.Symlink(source, target)
			case "hardlink":
				err = os.Link(source, target)
			case "unrelated symlink":
				err = os.Symlink(unrelated, target)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Remove(target) })
			if mode == "restrictive umask" {
				previous := syscall.Umask(0111)
				defer syscall.Umask(previous)
			}
			if !CopyCLI(t, target) {
				t.Fatal("prebuilt CLI not copied")
			}
			info, err := os.Lstat(target)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0755 {
				t.Fatalf("fixture CLI must be a regular executable: %v %v", info, err)
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "original executable" {
				t.Fatalf("fixture bytes: %q %v", data, err)
			}
			if err := os.WriteFile(target, []byte("fixture modified"), 0700); err != nil {
				t.Fatal(err)
			}
			for path, want := range map[string]string{source: "original executable", unrelated: "user data"} {
				if data, err := os.ReadFile(path); err != nil || string(data) != want {
					t.Fatalf("shared file changed: %s %q %v", path, data, err)
				}
			}
		})
	}
}
