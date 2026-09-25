package selfishell

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestChecksum(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	vectors := [][]byte{nil, []byte("a\r\nb\r\n"), []byte("no final newline"), {0, 255, 128, 0, 10}, bytes.Repeat([]byte{0, 1, 128, 255, 13, 10}, 10000)}
	for _, data := range vectors {
		path := dir + "/input with spaces"
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("cksum")
		cmd.Stdin = bytes.NewReader(data)
		output, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(string(output))
		want := fields[0] + ":" + fields[1]
		got, err := Checksum(context.Background(), path)
		if err != nil || got != want {
			t.Fatalf("%d bytes: %q %v want %q", len(data), got, err, want)
		}
	}
	if got, err := Checksum(context.Background(), dir+"/missing"); got != "" || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %q %v", got, err)
	}
	if got, err := Checksum(context.Background(), dir); got != "" || err == nil {
		t.Fatalf("directory: %q %v", got, err)
	}
	link := dir + "/link"
	if err := os.Symlink(dir+"/input with spaces", link); err != nil {
		t.Fatal(err)
	}
	if got, err := Checksum(context.Background(), link); got != "" || err == nil {
		t.Fatalf("replaced link: %q %v", got, err)
	}
}

func TestChecksumFailure(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/input"
	if err := os.WriteFile(path, []byte("bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("PATH", dir)
	fixture, err := os.ReadFile("../../tests/fixtures/go_migration/cksum.bash")
	if err != nil {
		t.Fatal(err)
	}
	// Keep /bin/sh available for the fixture without exposing the host cksum.
	fixture = bytes.Replace(fixture, []byte("#!/usr/bin/env bash"), []byte("#!/bin/sh"), 1)
	if err := os.WriteFile(dir+"/cksum", fixture, 0700); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"failed", "malformed", "overflow"} {
		t.Setenv("SELFISHELL_TEST_CKSUM_CASE", scenario)
		if got, err := Checksum(context.Background(), path); got != "" || err == nil {
			t.Fatalf("failed checksum returned success: %q %v", got, err)
		}
	}
}

func TestResources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/alias/../config space")
	t.Setenv("XDG_STATE_HOME", home+"/state")
	t.Setenv("XDG_CACHE_HOME", home+"/cache")
	t.Setenv("XDG_DATA_HOME", home+"/data")
	all, err := ManagedResources("/release/alias/..")
	if err != nil || len(all) != 33 {
		t.Fatalf("resources: %d %v", len(all), err)
	}
	if all[0].Target != home+"/alias/../config space/selfishell/zsh/zshrc" {
		t.Fatal("resource paths were cleaned")
	}
	for _, tc := range []struct {
		platform string
		ghostty  bool
		count    int
		zsh      string
		zshenv   bool
	}{
		{"macos", false, 30, "macos", false}, {"macos", true, 32, "macos", false},
		{"ubuntu", false, 31, "ubuntu", true}, {"ubuntu", true, 31, "ubuntu", true},
		{"ubuntu-wsl", false, 31, "ubuntu", true},
	} {
		resources, err := ResourcesForPlatform("/release", tc.platform, tc.ghostty)
		if err != nil || len(resources) != tc.count {
			t.Fatalf("%s ghostty=%v: %d %v", tc.platform, tc.ghostty, len(resources), err)
		}
		if resources[0].Source != "/release/config/"+tc.zsh+"/zshrc" {
			t.Fatal(resources[0])
		}
		found := false
		for _, r := range resources {
			if r.Name == "user-zshenv" {
				found = true
			}
		}
		if found != tc.zshenv {
			t.Fatal("wrong zshenv selection")
		}
	}
	if resources, err := ResourcesForPlatform("/release", "unsupported-linux", false); resources != nil || err == nil {
		t.Fatalf("unsupported: %v %v", resources, err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("resource discovery wrote files")
	}
}

func TestLoadResourceStates(t *testing.T) {
	dir := t.TempDir()
	state := stateFixture()
	resources := []Resource{{Kind: "file", Name: "first", Target: "/a", Source: "/source"}, {Kind: "link", Name: "last", Target: "/b", Source: "/reference"}}
	if err := WriteState(dir+"/first.state", state); err != nil {
		t.Fatal(err)
	}
	records, err := LoadResourceStates(dir, resources)
	if err != nil || len(records) != 1 || records[0].Resource != resources[0] || records[0].State != state {
		t.Fatalf("records: %+v %v", records, err)
	}
	if err := os.WriteFile(dir+"/last.state", []byte("2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if records, err := LoadResourceStates(dir, resources); records != nil || !errors.Is(err, ErrMalformedState) {
		t.Fatalf("partial enumeration escaped: %+v %v", records, err)
	}
	if err := os.Remove(dir + "/last.state"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir+"/missing", dir+"/last.state"); err != nil {
		t.Fatal(err)
	}
	if records, err := LoadResourceStates(dir, resources); records != nil || !errors.Is(err, ErrMalformedState) {
		t.Fatalf("dangling state silently skipped: %+v %v", records, err)
	}
	for _, names := range [][]Resource{nil, {resources[0], resources[0]}, {resources[0], {Kind: "file", Name: "../escape", Target: "/x", Source: "/y"}}} {
		if records, err := LoadResourceStates(dir, names); records != nil || err == nil {
			t.Fatalf("invalid declaration: %+v %v", records, err)
		}
	}
	if records, err := LoadResourceStates(dir+"/first.state/invalid", resources); records != nil || err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("enumeration I/O failure skipped: %+v %v", records, err)
	}
}

func TestLoadResourceStatesUnreadableEntry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires an unprivileged test process")
	}
	dir := t.TempDir()
	resources := []Resource{{Kind: "file", Name: "first", Target: "/a", Source: "/b"}, {Kind: "file", Name: "last", Target: "/c", Source: "/d"}}
	for _, r := range resources {
		if err := WriteState(dir+"/"+r.Name+".state", stateFixture()); err != nil {
			t.Fatal(err)
		}
	}
	path := dir + "/last.state"
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	if records, err := LoadResourceStates(dir, resources); records != nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable last entry returned a partial list: %+v %v", records, err)
	}
}
