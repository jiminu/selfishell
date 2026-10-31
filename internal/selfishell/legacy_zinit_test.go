package selfishell

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRemoveLegacyZinit(t *testing.T) {
	for _, shape := range []string{"owned", "dry_run", "unknown_plugin", "polaris_content", "missing_state", "symlink", "removal_failure"} {
		t.Run(shape, func(t *testing.T) {
			op, paths, _, home := dependencyFixture(t)
			dir, state := home+"/data/zinit", paths.State+"/dependencies/zinit"
			fixtureFile(t, dir+"/zinit.git/zinit.zsh", ":\n", 0600)
			for name := range legacyZinitPlugins {
				fixtureFile(t, dir+"/plugins/"+name+"/.git/HEAD", "ref\n", 0600)
			}
			for _, empty := range []string{"/polaris/bin", "/polaris/man/man1", "/completions"} {
				mustDir(t, dir+empty)
			}
			fixtureFile(t, state, "v3.17.1\n", 0600)
			switch shape {
			case "unknown_plugin":
				mustDir(t, dir+"/plugins/user---plugin")
			case "polaris_content":
				fixtureFile(t, dir+"/polaris/bin/tool", "#!/bin/sh\n", 0700)
			case "missing_state":
				if err := os.Remove(state); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(dir, home+"/real-zinit"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(home+"/real-zinit", dir); err != nil {
					t.Fatal(err)
				}
			case "removal_failure":
				op.dependencyFault = func(at string) error { return errors.New("forced " + at) }
			}
			if err := op.RemoveLegacyZinit(context.Background(), paths, shape == "dry_run"); err != nil {
				t.Fatal(err)
			}
			var note bytes.Buffer
			(CLI{Out: &note}).legacyZinitNote(paths)
			out, warnings := output(op), op.Process.Err.(*bytes.Buffer).String()
			switch shape {
			case "owned":
				assertNoPath(t, dir)
				assertNoPath(t, state)
				if !strings.Contains(out, "Removed the legacy Zinit checkout") || note.Len() != 0 {
					t.Fatalf("output=%q note=%q", out, note.String())
				}
				return
			case "dry_run":
				if !strings.Contains(out, "Would remove the legacy Zinit checkout") {
					t.Fatalf("no preview: %q", out)
				}
			case "removal_failure":
				if !strings.Contains(warnings, "Could not remove the legacy Zinit checkout") {
					t.Fatalf("failure not reported: %q", warnings)
				}
				op.dependencyFault = nil
				if err := op.RemoveLegacyZinit(context.Background(), paths, false); err != nil {
					t.Fatal(err)
				}
				assertNoPath(t, dir)
				assertNoPath(t, state)
				return
			default:
				if out != "" || !strings.Contains(note.String(), "Legacy Zinit checkout remains: ~/data/zinit;") {
					t.Fatalf("output=%q note=%q", out, note.String())
				}
			}
			if _, err := os.Stat(dir + "/zinit.git/zinit.zsh"); err != nil {
				t.Fatalf("checkout changed: %v", err)
			}
			if _, err := os.Lstat(state); shape != "missing_state" && err != nil {
				t.Fatalf("state removed: %v", err)
			}
		})
	}
}

func mustDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
}
