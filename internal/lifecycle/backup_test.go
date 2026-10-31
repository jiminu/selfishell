package lifecycle

import (
	"os"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestBackupMoveAndCopyNeverReplaceCollision(t *testing.T) {
	t.Parallel()
	create := func(path, kind, label string) {
		switch kind {
		case "file":
			testutil.WriteFile(path, []byte(label), 0600)
		case "symlink":
			os.Symlink(label+"-target", path)
		case "directory":
			os.Mkdir(path, 0700)
			testutil.WriteFile(path+"/"+label+"-child", []byte(label), 0600)
		}
	}
	unchanged := func(t *testing.T, path, kind, label string) {
		t.Helper()
		data, _ := os.ReadFile(path)
		dest, _ := os.Readlink(path)
		_, child := os.Stat(path + "/" + label + "-child")
		if kind == "file" && string(data) != label || kind == "symlink" && dest != label+"-target" || kind == "directory" && child != nil {
			t.Fatalf("%s %s changed", kind, path)
		}
	}
	for _, op := range []string{"move", "copy"} {
		for _, kind := range []string{"file", "symlink", "directory"} {
			t.Run(op+"/"+kind, func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				source, backup := root+"/source", root+"/backup"
				sourceKind, apply := kind, moveBackupNoReplace
				if op == "copy" {
					sourceKind, apply = "file", copyPreserve
				}
				create(source, sourceKind, "source")
				create(backup, kind, "backup")
				if e := apply(source, backup); e == nil {
					t.Fatal("occupied backup replaced")
				}
				unchanged(t, source, sourceKind, "source")
				unchanged(t, backup, kind, "backup")
				if _, e := os.Lstat(backup + "/source-child"); kind == "directory" && !os.IsNotExist(e) {
					t.Fatal("source nested in backup")
				}
			})
		}
	}
}
