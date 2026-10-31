package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

// setEnv, writeTestFile and readTestFile copy the internal/selfishell test helpers.
func setEnv(e *process.Environment, key, value string) {
	*e = e.With(map[string]string{key: value})
}

func writeTestFile(t *testing.T, path, value string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
