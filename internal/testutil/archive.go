package testutil

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// ArchiveMember is one tar entry; Kind defaults to a regular file, Mode to 0644.
type ArchiveMember struct {
	Name, Target string
	Kind         byte
	Body         string
	Mode         int64
}

// Archive writes a gzipped tar of members in a test temporary directory.
func Archive(t testing.TB, members ...ArchiveMember) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.tar.gz")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		mode := m.Mode
		if mode == 0 {
			mode = 0644
		}
		h := &tar.Header{Name: m.Name, Typeflag: m.Kind, Mode: mode, Linkname: m.Target, Size: int64(len(m.Body))}
		if m.Kind != tar.TypeReg && m.Kind != tar.TypeRegA {
			h.Size = 0
		}
		if e := tw.WriteHeader(h); e != nil {
			t.Fatal(e)
		}
		if h.Size > 0 {
			if _, e := tw.Write([]byte(m.Body)); e != nil {
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
