package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/selfishell"
)

func TestUpdaterRejectsGoToolchainMetadataBeforeAnyEdits(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest, metadata := filepath.Join(root, "dependencies.conf"), filepath.Join(root, "metadata")
	original := "nvim-plugin example/plugin " + oldC + " all all https://example.invalid/plugin.git - - -\n"
	mod := "module example.invalid/test\n\ngo 1.27.1\n"
	mise := "[tools]\ngo = \"1.27.1\"\n"
	put(t, manifest, original)
	put(t, metadata, "nvim-plugin example/plugin "+newC+"\ngo-toolchain go 1.27.2\n")
	put(t, filepath.Join(root, "go.mod"), mod)
	put(t, filepath.Join(root, "mise.toml"), mise)
	var output bytes.Buffer
	p := selfishell.Process{Out: &output, Err: &output, Env: []string{"HOME=" + root, "PATH=" + filepath.Join(root, "no-tools")}}
	if status := runDependencyUpdate(context.Background(), root, []string{"--metadata", metadata}, p); status == 0 || !strings.Contains(output.String(), "invalid dependency metadata record: go-toolchain") {
		t.Fatalf("status=%d output=%s", status, output.String())
	}
	exact(t, manifest, original)
	exact(t, filepath.Join(root, "go.mod"), mod)
	exact(t, filepath.Join(root, "mise.toml"), mise)
}
