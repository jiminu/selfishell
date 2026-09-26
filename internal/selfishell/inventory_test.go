package selfishell

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func inventoryFixture(t *testing.T) (string, Paths, *bytes.Buffer, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	for _, p := range []string{home, bin, filepath.Join(root, "config/shared")} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("SELFISHELL_DEPENDENCIES_FILE", filepath.Join(root, "dependencies.conf"))
	if err := os.WriteFile(filepath.Join(root, "dependencies.conf"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := UserPaths()
	if err != nil {
		t.Fatal(err)
	}
	return root, paths, &bytes.Buffer{}, bin
}
func fixtureFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
func inventory(t *testing.T, root string, paths Paths, warnings *bytes.Buffer) *ToolInventory {
	t.Helper()
	inv, err := NewToolInventory(root, paths, warnings)
	if err != nil {
		t.Fatal(err)
	}
	return inv
}
func wantTool(t *testing.T, got ToolResult, installed, source, approved string) {
	t.Helper()
	if got != (ToolResult{installed, source, approved}) {
		t.Fatalf("got %+v; want %q %q %q", got, installed, source, approved)
	}
}
func TestInventoryAptStatusesAndCache(t *testing.T) {
	root, paths, warnings, bin := inventoryFixture(t)
	fixtureFile(t, filepath.Join(bin, "dpkg-query"), "#!/bin/sh\nprintf 'call\\n' >>\"$HOME/dpkg-calls\"\nprintf 'git:amd64\\tii \\t2.43.0\\nvim\\trc \\t9.1\\nmake\\thi \\t4.3\\nless\\tiiR\\t590\\n'\n", 0700)
	inv := inventory(t, root, paths, warnings)
	for _, tc := range []struct{ name, version, source string }{{"git", "2.43.0", "apt"}, {"make", "4.3", "apt"}} {
		got, err := inv.Detect("apt", tc.name, "linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		wantTool(t, got, tc.version, tc.source, "package-manager")
	}
	for _, name := range []string{"vim", "less"} {
		got, err := inv.Detect("apt", name, "linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		if got.Source == "apt" {
			t.Fatalf("%s incorrectly reported as apt: %+v", name, got)
		}
	}
	calls, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "dpkg-calls"))
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "call\n" {
		t.Fatalf("calls %q", calls)
	}
}
func TestInventoryBrewJSONAndLegacy(t *testing.T) {
	for _, tc := range []struct{ name, json, calls string }{
		{"json", `{"formulae":[{"name":"starship","versions":["1.26.0"]}],"casks":[{"token":"ghostty","versions":["1.3.1"]}]}`, "list --versions --json\n"},
		{"invalid", "invalid", "list --versions --json\nlist --formula --versions\nlist --cask --versions\n"},
		{"empty", "  ", "list --versions --json\nlist --formula --versions\nlist --cask --versions\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, paths, warnings, bin := inventoryFixture(t)
			t.Setenv("BREW_JSON", tc.json)
			fixtureFile(t, filepath.Join(bin, "brew"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$HOME/brew-calls\"\ncase \"$*\" in\n 'list --versions --json') printf '%s\\n' \"$BREW_JSON\";;\n 'list --formula --versions') printf 'starship 1.26.0\\n';;\n 'list --cask --versions') printf 'ghostty 1.3.1\\n';;\nesac\n", 0700)
			inv := inventory(t, root, paths, warnings)
			got, err := inv.Detect("formula", "starship", "macos", "arm64")
			if err != nil {
				t.Fatal(err)
			}
			wantTool(t, got, "1.26.0", "homebrew", "package-manager")
			got, err = inv.Detect("cask", "ghostty", "macos", "arm64")
			if err != nil {
				t.Fatal(err)
			}
			wantTool(t, got, "1.3.1", "homebrew-cask", "package-manager")
			calls, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "brew-calls"))
			if err != nil {
				t.Fatal(err)
			}
			if string(calls) != tc.calls {
				t.Fatalf("calls %q", calls)
			}
		})
	}
}
func TestInventoryDirectManagedExternalAndMissing(t *testing.T) {
	root, paths, warnings, _ := inventoryFixture(t)
	fixtureFile(t, filepath.Join(root, "dependencies.conf"), "download mise 1.0 all all source checksum .local/bin/mise raw\n", 0600)
	target := filepath.Join(os.Getenv("HOME"), ".local/bin/mise")
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "none", "1.0")
	fixtureFile(t, target, "#!/bin/sh\nexit 0\n", 0700)
	got, err = inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "detected", "external", "1.0")
	fixtureFile(t, filepath.Join(paths.State, "dependencies/mise"), "1.0\n", 0600)
	got, err = inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "1.0", "selfishell", "1.0")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	got, err = inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "selfishell", "1.0")
}
func TestInventoryMiseInstalledPinsFailureAndShim(t *testing.T) {
	root, paths, warnings, bin := inventoryFixture(t)
	fixtureFile(t, filepath.Join(root, "config/shared/mise.toml"), "[tools]\nnode = \"24.18.0\"\npython = \"3.13.14\"\ngh = \"2.100.0\"\n[settings]\nnode = \"ignored\"\n", 0600)
	fixtureFile(t, filepath.Join(bin, "mise"), "#!/bin/sh\nprintf '%s|%s|%s|%s\\n' \"$*\" \"$PWD\" \"$MISE_GLOBAL_CONFIG_FILE\" \"$NO_COLOR\" >>\"$HOME/mise-calls\"\nprintf 'node 24.18.0 /config/mise.toml 24.18.0\\npython 3.13.14 /config/mise.toml 3.13.14\\npython 3.12.0 /config/mise.toml 3.12.0\\n'\nif [ -f \"$HOME/mise-fail\" ]; then printf 'mise ERROR untrusted\\nextra detail\\n' >&2; exit 1; fi\n", 0700)
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("mise", "node", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "24.18.0", "mise", "24.18.0")
	got, err = inv.Detect("mise", "python", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "3.13.14 3.12.0", "mise", "3.13.14")
	calls, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "mise-calls"))
	if err != nil {
		t.Fatal(err)
	}
	physicalShared, err := filepath.EvalSymlinks(filepath.Join(root, "config/shared"))
	if err != nil {
		t.Fatal(err)
	}
	want := "-C " + root + "/config/shared ls --current --installed --no-header --no-truncate|" + physicalShared + "|" + root + "/config/shared/mise.toml|1\n"
	if string(calls) != want {
		t.Fatalf("mise call %q, want %q", calls, want)
	}
	fixtureFile(t, filepath.Join(os.Getenv("HOME"), "mise-fail"), "", 0600)
	inv = inventory(t, root, paths, warnings)
	got, err = inv.Detect("mise", "gh", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "none", "2.100.0")
	got, err = inv.Detect("mise", "node", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "none", "24.18.0")
	if strings.Count(warnings.String(), "mise could not list installed tools") != 1 {
		t.Fatalf("warning count: %q", warnings.String())
	}
	if !strings.Contains(warnings.String(), "mise could not list installed tools: mise ERROR untrusted") {
		t.Fatalf("warning %q", warnings.String())
	}
}

func TestInventoryBrewAliasAndCaskFallback(t *testing.T) {
	root, paths, warnings, bin := inventoryFixture(t)
	fixtureFile(t, filepath.Join(bin, "brew"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$HOME/brew-calls\"\ncase \"$*\" in\n 'list --versions --json') printf '{\"formulae\":[],\"casks\":[]}' ;;\n 'list --versions aliased') printf 'canonical 1.2.3\\n' ;;\nesac\n", 0700)
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("formula", "aliased", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "1.2.3", "homebrew", "package-manager")
	got, err = inv.Detect("cask", "missing-cask", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "none", "package-manager")
	calls, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "brew-calls"))
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "list --versions --json\nlist --versions aliased\n" {
		t.Fatalf("calls %q", calls)
	}
}
func TestInventoryBrewMalformedTypedInventoryFallsBack(t *testing.T) {
	root, paths, warnings, bin := inventoryFixture(t)
	fixtureFile(t, filepath.Join(bin, "brew"), "#!/bin/sh\ncase \"$*\" in\n 'list --versions --json') printf '{\"formulae\":{},\"casks\":[]}' ;;\n 'list --formula --versions') printf 'starship 1.26.0\\n' ;;\nesac\n", 0700)
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("formula", "starship", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "1.26.0", "homebrew", "package-manager")
}
func TestInventoryDirectExternalSymlinkAndManagedInvalid(t *testing.T) {
	root, paths, warnings, _ := inventoryFixture(t)
	fixtureFile(t, filepath.Join(root, "dependencies.conf"), "download mise 1.0 all all source checksum .local/bin/mise raw\n", 0600)
	target := filepath.Join(os.Getenv("HOME"), ".local/bin/mise")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "absent"), target); err != nil {
		t.Fatal(err)
	}
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "none", "1.0")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(root, "external-mise")
	fixtureFile(t, external, "#!/bin/sh\n", 0700)
	if err := os.Symlink(external, target); err != nil {
		t.Fatal(err)
	}
	got, err = inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "detected", "external", "1.0")
	fixtureFile(t, filepath.Join(paths.State, "dependencies/mise"), "1.0\n", 0600)
	got, err = inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "selfishell", "1.0")
}
func TestInventoryDirectGitCheckoutAndPlatform(t *testing.T) {
	root, paths, warnings, _ := inventoryFixture(t)
	target := filepath.Join(os.Getenv("HOME"), "data", "zinit", "zinit.git")
	fixtureFile(t, filepath.Join(target, "zinit.zsh"), ":\n", 0600)
	fixtureFile(t, filepath.Join(root, "dependencies.conf"), "git zinit v3.15.0 all all source - .local/share/zinit/zinit.git zinit.zsh\n", 0600)
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("direct", "zinit", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "detected", "external", "v3.15.0")
	if _, err := inv.Detect("direct", "unknown", "linux", "amd64"); err == nil {
		t.Fatal("missing approved dependency accepted")
	}
	fixtureFile(t, filepath.Join(paths.State, "dependencies/zinit"), "v3.15.0\n", 0600)
	got, err = inv.Detect("direct", "zinit", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "selfishell", "v3.15.0")
}
func TestInventoryMiseShimFallbackAndReset(t *testing.T) {
	root, paths, warnings, bin := inventoryFixture(t)
	fixtureFile(t, filepath.Join(root, "config/shared/mise.toml"), "[tools]\ngh = \"2.100.0\"\nuv = \"0.12.13\"\n", 0600)
	fixtureFile(t, filepath.Join(bin, "mise"), "#!/bin/sh\nprintf 'call\\n' >>\"$HOME/mise-calls\"\ncat \"$HOME/mise-output\"\n", 0700)
	fixtureFile(t, filepath.Join(os.Getenv("HOME"), "mise-output"), "", 0600)
	shims := filepath.Join(root, "mise-data/shims")
	if err := os.MkdirAll(shims, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(bin, "mise"), filepath.Join(shims, "gh")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MISE_DATA_DIR", filepath.Join(root, "mise-data"))
	t.Setenv("PATH", shims+":"+bin+":/usr/bin:/bin")
	fixtureFile(t, filepath.Join(bin, "uv"), "#!/bin/sh\n", 0700)
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("mise", "gh", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "none", "2.100.0")
	got, err = inv.Detect("mise", "uv", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "detected", "external", "0.12.13")
	fixtureFile(t, filepath.Join(os.Getenv("HOME"), "mise-output"), "uv 0.12.13 /config 0.12.13\n", 0600)
	got, err = inv.Detect("mise", "uv", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "detected", "external", "0.12.13")
	fresh := inventory(t, root, paths, warnings)
	got, err = fresh.Detect("mise", "uv", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "0.12.13", "mise", "0.12.13")
	calls, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "mise-calls"))
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "call\ncall\n" {
		t.Fatalf("calls %q", calls)
	}
}
func TestInventoryMiseManagedBinaryOutsidePath(t *testing.T) {
	root, paths, warnings, _ := inventoryFixture(t)
	fixtureFile(t, filepath.Join(root, "config/shared/mise.toml"), "[tools]\nnode = \"24.18.0\"\n", 0600)
	fixtureFile(t, filepath.Join(os.Getenv("HOME"), ".local/bin/mise"), "#!/bin/sh\nprintf 'node 24.18.0 /config 24.18.0\\n'\n", 0700)
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("mise", "node", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "24.18.0", "mise", "24.18.0")
}
func TestToolExecutable(t *testing.T) {
	for _, tc := range []struct{ name, want string }{{"ripgrep", "rg"}, {"neovim", "nvim"}, {"kubectl@1.36.2", "kubectl"}, {"git", "git"}} {
		if got := toolExecutable(tc.name); got != tc.want {
			t.Errorf("%s => %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestInventoryDirectManagedGitChecksCommitAndTrackedChanges(t *testing.T) {
	root, paths, warnings, _ := inventoryFixture(t)
	target := filepath.Join(os.Getenv("HOME"), "data", "zinit", "zinit.git")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}} {
		cmd := exec.Command("git", append([]string{"-C", target}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	marker := filepath.Join(target, "zinit.zsh")
	fixtureFile(t, marker, ":\n", 0600)
	for _, args := range [][]string{{"add", "zinit.zsh"}, {"commit", "--quiet", "-m", "initial"}} {
		cmd := exec.Command("git", append([]string{"-C", target}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	cmd := exec.Command("git", "-C", target, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.TrimSpace(string(out))
	fixtureFile(t, filepath.Join(root, "dependencies.conf"), "git zinit v3.15.0 all all source "+sha+" .local/share/zinit/zinit.git zinit.zsh\n", 0600)
	fixtureFile(t, filepath.Join(paths.State, "dependencies/zinit"), "v3.15.0\n", 0600)
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("direct", "zinit", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "v3.15.0", "selfishell", "v3.15.0")
	fixtureFile(t, marker, "changed\n", 0600)
	got, err = inv.Detect("direct", "zinit", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "selfishell", "v3.15.0")
	fixtureFile(t, marker, ":\n", 0600)
	fixtureFile(t, filepath.Join(root, "dependencies.conf"), "git zinit v3.15.0 all all source deadbeef .local/share/zinit/zinit.git zinit.zsh\n", 0600)
	inv = inventory(t, root, paths, warnings)
	got, err = inv.Detect("direct", "zinit", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "selfishell", "v3.15.0")
}
func TestInventoryDoesNotCreateUserState(t *testing.T) {
	root, paths, warnings, _ := inventoryFixture(t)
	fixtureFile(t, filepath.Join(root, "config/shared/mise.toml"), "[tools]\nnode = \"24.18.0\"\n", 0600)
	inv := inventory(t, root, paths, warnings)
	if _, err := inv.Detect("mise", "node", "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.State); !os.IsNotExist(err) {
		t.Fatalf("state created or unexpected stat error: %v", err)
	}
}

func TestInventoryQueryCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, ok := runInventoryContext(ctx, "", nil, "sleep", "2")
	if ok {
		t.Fatal("cancelled inventory query reported success")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancelled query took %s", elapsed)
	}
}
