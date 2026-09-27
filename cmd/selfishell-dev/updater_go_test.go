package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/jiminu/selfishell/internal/selfishell"
)

func TestUpdaterGoPatchPins(t *testing.T) {
	for _, tc := range []struct {
		name, candidate, miseVersion, want string
		failure                            bool
	}{
		{"patch", "1.27.2", "1.27.1", "1.27.2", false},
		{"current", "1.27.1", "1.27.1", "1.27.1", false},
		{"older", "1.27.0", "1.27.1", "1.27.1", false},
		{"next series", "1.28.0", "1.27.1", "1.27.1", true},
		{"prerelease", "1.27.2rc1", "1.27.1", "1.27.1", true},
		{"mismatched pins", "1.27.2", "1.26.9", "1.27.1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			manifest, metadata := filepath.Join(root, "dependencies.conf"), filepath.Join(root, "metadata")
			mod := "module example.invalid/test\n\ngo 1.27.1\n"
			mise := "[tools]\ngo = \"" + tc.miseVersion + "\" # toolchain\n"
			put(t, manifest, "# unchanged\n")
			put(t, metadata, "go-toolchain go "+tc.candidate+"\n")
			put(t, filepath.Join(root, "go.mod"), mod)
			put(t, filepath.Join(root, "mise.toml"), mise)
			var output bytes.Buffer
			p := selfishell.Process{Out: &output, Err: &output, Env: []string{"HOME=" + root, "PATH=" + filepath.Join(root, "no-tools")}}
			for attempt := 0; attempt < 2; attempt++ {
				status := runDependencyUpdate(context.Background(), root, []string{"--metadata", metadata}, p)
				if (status != 0) != tc.failure {
					t.Fatalf("status=%d, failure=%v: %s", status, tc.failure, output.String())
				}
			}
			exact(t, manifest, "# unchanged\n")
			exact(t, filepath.Join(root, "go.mod"), "module example.invalid/test\n\ngo "+tc.want+"\n")
			if tc.failure {
				exact(t, filepath.Join(root, "mise.toml"), mise)
			} else {
				exact(t, filepath.Join(root, "mise.toml"), "[tools]\ngo = \""+tc.want+"\" # toolchain\n")
			}
		})
	}
}

func TestUpdaterRejectsMalformedGoPinsBeforeAnyEdits(t *testing.T) {
	for _, tc := range []struct{ name, mod, mise string }{
		{"extra directive", "module example.invalid/test\n\ngo 1.27.1\ngo broken\n", "[tools]\ngo = \"1.27.1\"\n"},
		{"invalid mise suffix", "module example.invalid/test\n\ngo 1.27.1\n", "[tools]\ngo = \"1.27.1\" garbage\n"},
		{"extra invalid mise pin", "module example.invalid/test\n\ngo 1.27.1\n", "[tools]\ngo = \"1.27.1\"\ngo = broken\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			manifest, metadata := filepath.Join(root, "dependencies.conf"), filepath.Join(root, "metadata")
			original := "nvim-plugin example/plugin " + oldC + " all all https://example.invalid/plugin.git - - -\n"
			put(t, manifest, original)
			put(t, metadata, "nvim-plugin example/plugin "+newC+"\ngo-toolchain go 1.27.2\n")
			put(t, filepath.Join(root, "go.mod"), tc.mod)
			put(t, filepath.Join(root, "mise.toml"), tc.mise)
			var output bytes.Buffer
			p := selfishell.Process{Out: &output, Err: &output, Env: []string{"HOME=" + root, "PATH=" + filepath.Join(root, "no-tools")}}
			if status := runDependencyUpdate(context.Background(), root, []string{"--metadata", metadata}, p); status == 0 {
				t.Fatal("accepted malformed toolchain pins")
			}
			exact(t, manifest, original)
			exact(t, filepath.Join(root, "go.mod"), tc.mod)
			exact(t, filepath.Join(root, "mise.toml"), tc.mise)
		})
	}
}
