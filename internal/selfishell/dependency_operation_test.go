package selfishell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dependencyFixture(t *testing.T) (*PackageOperation, Paths, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home+"/data")
	t.Setenv("XDG_STATE_HOME", home+"/state")
	paths, err := UserPaths()
	if err != nil {
		t.Fatal(err)
	}
	manifest := home + "/dependencies.conf"
	return &PackageOperation{Process: Process{Out: new(bytes.Buffer), Err: new(bytes.Buffer)}}, paths, manifest, home
}

func TestDirectDownloadChecksumAndState(t *testing.T) {
	op, paths, manifest, home := dependencyFixture(t)
	payload := []byte("#!/bin/sh\necho tool\n")
	source := home + "/source"
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	line := fmt.Sprintf("download tool 1.0 linux amd64 file://%s %x .local/bin/tool raw\n", source, sum)
	if err := os.WriteFile(manifest, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "tool", "linux", "amd64", false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(home + "/.local/bin/tool")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("target: %q %v", got, err)
	}
	state, err := os.ReadFile(paths.State + "/dependencies/tool")
	if err != nil || string(state) != "1.0\n" {
		t.Fatalf("state: %q %v", state, err)
	}
	if !strings.Contains(op.Process.Out.(*bytes.Buffer).String(), "Installed approved dependency: tool 1.0") {
		t.Fatal("missing success report")
	}
}

func TestDirectDownloadChecksumFailureKeepsManagedTarget(t *testing.T) {
	op, paths, manifest, home := dependencyFixture(t)
	target := home + "/.local/bin/tool"
	state := paths.State + "/dependencies/tool"
	for _, path := range []string{target, state} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(target, []byte("old"), 0755)
	os.WriteFile(state, []byte("0.9\n"), 0600)
	source := home + "/source"
	os.WriteFile(source, []byte("new"), 0600)
	os.WriteFile(manifest, []byte(fmt.Sprintf("download tool 1.0 linux amd64 file://%s %064d .local/bin/tool raw\n", source, 0)), 0600)
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "tool", "linux", "amd64", false); err == nil {
		t.Fatal("checksum accepted")
	}
	got, _ := os.ReadFile(target)
	version, _ := os.ReadFile(state)
	if string(got) != "old" || string(version) != "0.9\n" {
		t.Fatalf("changed target/state: %q %q", got, version)
	}
}
