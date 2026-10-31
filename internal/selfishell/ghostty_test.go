package selfishell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/process"
)

// Ghostty detection must not see the developer's /Applications.
func TestMain(m *testing.M) {
	os.Setenv("SELFISHELL_TEST_APPLICATIONS_DIR", filepath.Join(os.TempDir(), "selfishell-test-no-applications"))
	os.Exit(m.Run())
}

func TestInstallNotesOverriddenGhosttySettings(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root, _, paths := blockHome(t, &env, "macos")
	entrypoint := filepath.Dir(paths.Config) + "/ghostty/config.ghostty"
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0700); err != nil {
		t.Fatal(err)
	}
	blockWrite(t, entrypoint, []byte("theme = Nord\nfont-family = Fira Code\n"))
	note := "Selfishell's Ghostty defaults override these settings in " + entrypoint + ": theme. Move them to user.ghostty to keep them."
	for _, args := range [][]string{{"install", "--skip-packages", "--yes", "--dry-run"}, {"install", "--skip-packages", "--yes"}} {
		if out := blockOK(t, env, root, args...); !strings.Contains(out, note) {
			t.Fatalf("%v: missing note in %s", args, out)
		}
	}
}
