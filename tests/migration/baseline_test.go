package migration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func baselineScenario(t *testing.T, home, cli, tools, scenario string) map[string]capture {
	t.Helper()
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if scenario == "existing" {
		mustFS(t, os.MkdirAll(filepath.Join(home, ".config/nvim"), 0700))
		mustFS(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export PERSONAL=kept\r\n"), 0600))
		mustFS(t, os.WriteFile(filepath.Join(home, ".vimrc"), []byte("set number"), 0600))
		mustFS(t, os.WriteFile(filepath.Join(home, ".config/nvim/init.lua"), []byte("personal editor\x00bytes\n"), 0600))
		mustFS(t, os.WriteFile(filepath.Join(home, ".config/starship.toml"), nil, 0600))
	}
	env := []string{"PATH=" + tools}
	initial := mustSnapshot(t, home)
	result := map[string]capture{}
	steps := []struct {
		name   string
		status int
		args   []string
	}{{"help", 0, []string{"help"}}, {"version", 0, []string{"version"}}, {"unknown", 2, []string{"unknown command with spaces"}}, {"extra-argument", 2, []string{"version", "extra"}}, {"dry-run", 0, []string{"install", "--skip-packages", "--dry-run", "--yes"}}, {"install", 0, []string{"install", "--skip-packages", "--yes"}}, {"reinstall", 0, []string{"install", "--skip-packages", "--yes"}}, {"update-config", 0, []string{"update", "--tools-only", "--skip-packages", "--yes"}}, {"restore", 0, []string{"uninstall", "--restore", "--yes"}}}
	for _, step := range steps {
		got, err := captureCommand(home, cli, step.args, env)
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		requireStatus(t, step.name, got, step.status)
		result[step.name] = got
		if step.name == "dry-run" && !bytes.Equal(initial, got.Home) {
			t.Fatal("dry-run changed HOME")
		}
		if step.name == "install" {
			for _, path := range []string{".local/state/selfishell/configured", ".local/state/selfishell/resources/user-zshrc.state"} {
				if _, err := os.Stat(filepath.Join(home, path)); err != nil {
					t.Fatal(err)
				}
			}
			if info, err := os.Lstat(filepath.Join(home, ".config/nvim")); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("nvim link: %v %v", info, err)
			}
		}
		if step.name == "reinstall" && !bytes.Equal(result["install"].Home, got.Home) {
			t.Fatal("reinstall changed HOME")
		}
	}
	if scenario == "existing" {
		for path, want := range map[string][]byte{".zshrc": []byte("export PERSONAL=kept\r\n"), ".vimrc": []byte("set number"), ".config/nvim/init.lua": []byte("personal editor\x00bytes\n"), ".config/starship.toml": {}} {
			got, err := os.ReadFile(filepath.Join(home, path))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("restored %s: %v %q", path, err, got)
			}
		}
	}
	return result
}

func TestBaseline(t *testing.T) {
	t.Run("fixed reference repeats", func(t *testing.T) {
		root := t.TempDir()
		release := filepath.Join(root, "reference")
		if err := exportCommit(repoRoot(), referenceCommit, release); err != nil {
			t.Fatal(err)
		}
		mustFS(t, os.Mkdir(filepath.Join(release, ".git"), 0700))
		tools := fixtureTools(t, root)
		cli := filepath.Join(release, "bin/selfishell")
		for _, scenario := range []string{"empty", "existing"} {
			var first map[string]capture
			for pass := 0; pass < 2; pass++ {
				home := filepath.Join(root, "home")
				mustFS(t, os.RemoveAll(home))
				got := baselineScenario(t, home, cli, tools, scenario)
				if pass == 0 {
					first = got
				} else {
					for name, want := range first {
						requireEqual(t, scenario+"/"+name, want, got[name])
					}
				}
			}
		}
	})
	t.Run("legacy reproducible native archive and purge", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "legacy-source")
		if err := exportCommit(repoRoot(), legacyCommit, source); err != nil {
			t.Fatal(err)
		}
		builds := []string{filepath.Join(root, "first-build"), filepath.Join(root, "second-build")}
		for _, build := range builds {
			cmd := []string{"/bin/bash", filepath.Join(source, "scripts/build-release.sh"), "--version", "1.3.1", "--output", build}
			got, err := runCommand(root, cmd, nil, nil, 120*time.Second)
			if err != nil || got.Status != 0 {
				t.Fatalf("build: %v %+v", err, got)
			}
		}
		first, err := snapshot(builds[0])
		if err != nil {
			t.Fatal(err)
		}
		second, err := snapshot(builds[1])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatal("legacy release builds differ")
		}
		if got, err := os.ReadFile(filepath.Join(builds[0], "VERSION")); err != nil || string(got) != "1.3.1\n" {
			t.Fatalf("VERSION %q: %v", got, err)
		}
		sums, err := os.ReadFile(filepath.Join(builds[0], "SHA256SUMS"))
		if err != nil {
			t.Fatal(err)
		}
		for _, platform := range []string{"linux", "macos"} {
			for _, arch := range []string{"amd64", "arm64"} {
				name := fmt.Sprintf("selfishell-1.3.1-%s-%s.tar.gz", platform, arch)
				hash, err := hashFile(filepath.Join(builds[0], name))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(sums), hash+"  "+name) && !strings.Contains(string(sums), hash+" "+name) {
					t.Fatalf("missing valid checksum for %s", name)
				}
			}
		}
		releaseRoot := filepath.Join(root, "releases/download/v1.3.1")
		if err := copyTree(builds[0], releaseRoot); err != nil {
			t.Fatal(err)
		}
		home := filepath.Join(root, "home")
		mustFS(t, os.MkdirAll(home, 0700))
		prefix := filepath.Join(home, ".local")
		install := []string{"/bin/bash", filepath.Join(source, "install.sh"), "--version", "1.3.1", "--prefix", prefix}
		got, err := runCommand(home, install, nil, []string{"SELFISHELL_RELEASE_ROOT=file://" + filepath.Join(root, "releases")}, 120*time.Second)
		if err != nil || got.Status != 0 {
			t.Fatalf("legacy install: %v %+v", err, got)
		}
		mustFS(t, os.RemoveAll(source))
		if _, err := os.Lstat(source); !os.IsNotExist(err) {
			t.Fatalf("legacy source still present: %v", err)
		}
		cli := filepath.Join(prefix, "bin/selfishell")
		got, err = captureCommand(home, cli, []string{"version"}, nil)
		if err != nil || got.Status != 0 || !bytes.Equal(got.Stdout, []byte("selfishell 1.3.1\n")) {
			t.Fatalf("installed after source removal: %v %+v", err, got)
		}
		if link, err := os.Readlink(filepath.Join(prefix, "bin/sfs")); err != nil || link != "selfishell" {
			t.Fatalf("sfs link %q: %v", link, err)
		}
		tools := fixtureTools(t, root)
		baselineScenario(t, home, cli, tools, "existing")
		got, err = captureCommand(home, cli, []string{"uninstall", "--restore", "--purge", "--yes"}, []string{"PATH=" + tools})
		if err != nil || got.Status != 0 {
			t.Fatalf("purge: %v %+v", err, got)
		}
		if _, err := os.Lstat(cli); !os.IsNotExist(err) {
			t.Fatalf("purge retained CLI: %v", err)
		}
		t.Logf("native legacy archive executed: %s/%s", runtime.GOOS, runtime.GOARCH)
	})
}
