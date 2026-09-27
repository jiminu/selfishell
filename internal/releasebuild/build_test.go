package releasebuild

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveModesIgnoreCheckoutPermissions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/config/nested", 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"packages.conf", "dependencies.conf", "config/nested/settings"} {
		if err := os.WriteFile(root+"/"+name, []byte("contents\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	binary := root + "/selfishell"
	if err := os.WriteFile(binary, []byte("binary"), 0700); err != nil {
		t.Fatal(err)
	}
	archive := root + "/release.tar.gz"
	if err := writeArchive(root, binary, "1.0.0", archive); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	want := map[string]int64{"VERSION": 0644, "bin/selfishell": 0755, "packages.conf": 0644, "dependencies.conf": 0644, "config/": 0755, "config/nested/": 0755, "config/nested/settings": 0644}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if expected, ok := want[h.Name]; ok {
			if h.Mode != expected {
				t.Errorf("%s mode %04o, want %04o", h.Name, h.Mode, expected)
			}
			delete(want, h.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing archive members: %v", want)
	}
}

func TestInvalidVersionDoesNotCreateOutput(t *testing.T) {
	for _, version := range []string{"", "v1.2.3", "01.2.3", "1.2.3+local"} {
		output := filepath.Join(t.TempDir(), "absent")
		if err := Build(context.Background(), ".", version, output); err == nil || !strings.Contains(err.Error(), "semantic version") {
			t.Errorf("version %q: error %v", version, err)
		}
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			t.Errorf("version %q created output: %v", version, err)
		}
	}
}
