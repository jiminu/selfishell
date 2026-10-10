package selfishell

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
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
	t.Setenv("WSL_DISTRO_NAME", "")
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("SELFISHELL_DEPENDENCIES_FILE", filepath.Join(root, "dependencies.conf"))
	if err := testutil.WriteFile(filepath.Join(root, "dependencies.conf"), []byte(""), 0600); err != nil {
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
	if err := testutil.WriteFile(path, []byte(content), mode); err != nil {
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

func TestInventoryQueryFailureIsUnknownWithoutExternalFallback(t *testing.T) {
	for _, manager := range []string{"apt", "formula", "cask", "mise"} {
		t.Run(manager, func(t *testing.T) {
			root, paths, warnings, bin := inventoryFixture(t)
			t.Setenv("PATH", bin)
			fixtureFile(t, root+"/config/shared/mise.toml", "[tools]\nprobe = \"1.0\"\n", 0600)
			command := map[string]string{"apt": "dpkg-query", "formula": "brew", "cask": "brew", "mise": "mise"}[manager]
			fixtureFile(t, bin+"/"+command, "#!/bin/sh\nprintf 'call\\n' >>\"$HOME/query-calls\"\nprintf 'database permission denied\\n' >&2\nexit 2\n", 0700)
			fixtureFile(t, bin+"/probe", "#!/bin/sh\nexit 0\n", 0700)
			inv := inventory(t, root, paths, warnings)
			for range 2 {
				got, err := inv.Detect(manager, "probe", "linux", "amd64")
				if err == nil || !strings.Contains(err.Error(), "database permission denied") {
					t.Fatalf("query failure discarded: tool=%+v error=%v", got, err)
				}
				if got.Installed != "unknown" || got.Source != "none" {
					t.Fatalf("failed query asserted absence or external ownership: %+v", got)
				}
			}
			calls, err := os.ReadFile(os.Getenv("HOME") + "/query-calls")
			if err != nil {
				t.Fatal(err)
			}
			want := "call\n"
			if command == "brew" {
				want += "call\n" // JSON failure must still try the compatible legacy inventory.
			}
			if string(calls) != want {
				t.Fatalf("failed inventory was not cached: %q", calls)
			}
		})
	}
}

func TestInventorySuccessfulEmptyQueryAllowsMissingAndExternal(t *testing.T) {
	for _, manager := range []string{"apt", "formula", "cask", "mise"} {
		t.Run(manager, func(t *testing.T) {
			root, paths, warnings, bin := inventoryFixture(t)
			t.Setenv("PATH", bin)
			command := map[string]string{"apt": "dpkg-query", "formula": "brew", "cask": "brew", "mise": "mise"}[manager]
			fixtureFile(t, bin+"/"+command, "#!/bin/sh\nexit 0\n", 0700)
			inv := inventory(t, root, paths, warnings)
			got, err := inv.Detect(manager, "probe", "linux", "amd64")
			if err != nil || got.Installed != "missing" {
				t.Fatalf("successful empty inventory: %+v, %v", got, err)
			}
			fixtureFile(t, bin+"/probe", "#!/bin/sh\nexit 0\n", 0700)
			got, err = inv.Detect(manager, "probe", "linux", "amd64")
			if err != nil || got.Installed != "detected" || got.Source != "external" {
				t.Fatalf("successful query lost external fallback: %+v, %v", got, err)
			}
		})
	}
}

func TestInventoryMiseFailureWithoutStderrRetainsExitStatus(t *testing.T) {
	root, paths, warnings, bin := inventoryFixture(t)
	t.Setenv("PATH", bin)
	fixtureFile(t, bin+"/mise", "#!/bin/sh\nexit 7\n", 0700)
	got, err := inventory(t, root, paths, warnings).Detect("mise", "probe", "linux", "amd64")
	if got.Installed != "unknown" || err == nil || !strings.Contains(err.Error(), "exited 7") {
		t.Fatalf("silent process error lost: tool=%+v error=%v", got, err)
	}
}

func TestInventoryBrewAliasDistinguishesAbsentFromQueryFailure(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		unknown        bool
	}{
		{"absent", "exit 1", false},
		{"silent failure", "exit 7", true},
		{"inspection failure", "printf 'database locked\\n' >&2; exit 2", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, paths, warnings, bin := inventoryFixture(t)
			t.Setenv("PATH", bin)
			fixtureFile(t, bin+"/brew", "#!/bin/sh\ncase \"$*\" in\n 'list --versions --json') printf '{\"formulae\":[],\"casks\":[]}' ;;\n 'list --versions probe') "+tc.response+";;\nesac\n", 0700)
			got, err := inventory(t, root, paths, warnings).Detect("formula", "probe", "macos", "arm64")
			if tc.unknown {
				if err == nil || got.Installed != "unknown" {
					t.Fatalf("alias inspection failure discarded: tool=%+v error=%v", got, err)
				}
			} else if err != nil || got.Installed != "missing" {
				t.Fatalf("absent alias misdiagnosed: tool=%+v error=%v", got, err)
			}
		})
	}
}

func TestInventoryBrewAliasExecutionFailureIsUnknown(t *testing.T) {
	root, paths, warnings, bin := inventoryFixture(t)
	t.Setenv("PATH", bin)
	// A successful bulk query followed by a missing executable must not be
	// confused with Homebrew's ordinary silent exit 1 for an absent formula.
	fixtureFile(t, bin+"/brew", "#!/bin/sh\nprintf '{\"formulae\":[],\"casks\":[]}'\n/bin/rm \"$0\"\n", 0700)
	fixtureFile(t, bin+"/probe", "#!/bin/sh\nexit 0\n", 0700)
	got, err := inventory(t, root, paths, warnings).Detect("formula", "probe", "macos", "arm64")
	if err == nil || got.Installed != "unknown" || got.Source != "none" || !strings.Contains(err.Error(), "brew") {
		t.Fatalf("alias execution failure swallowed: tool=%+v error=%v", got, err)
	}
}
func TestInventoryBrewJSONAndLegacy(t *testing.T) {
	legacy := "list --versions --json\nlist --formula --versions\nlist --cask --versions\n"
	for _, tc := range []struct{ name, json, calls string }{
		{"json", `{"formulae":[{"name":"starship","versions":["1.26.0"]}],"casks":[{"token":"ghostty","versions":["1.3.1"]}]}`, "list --versions --json\n"},
		{"invalid", "invalid", legacy},
		{"empty", "  ", legacy},
		{"unsupported", "unsupported", legacy},
		{"null cask versions", `{"formulae":[],"casks":[{"token":"ghostty","versions":null}]}`, legacy},
		{"null formula versions", `{"formulae":[{"name":"starship","versions":null}],"casks":[{"token":"ghostty","versions":["0.0.0"]}]}`, legacy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, paths, warnings, bin := inventoryFixture(t)
			t.Setenv("BREW_JSON", tc.json)
			fixtureFile(t, filepath.Join(bin, "brew"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$HOME/brew-calls\"\ncase \"$*\" in\n 'list --versions --json') if [ \"$BREW_JSON\" = unsupported ]; then printf 'unknown option: --json\\n' >&2; exit 2; fi; printf '%s\\n' \"$BREW_JSON\";;\n 'list --formula --versions') printf 'starship 1.26.0\\n';;\n 'list --cask --versions') printf 'ghostty 1.3.1\\n';;\nesac\n", 0700)
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
	// sha256 of the fixture binary below, as install records it for raw downloads.
	sum := sha256.Sum256([]byte("#!/bin/sh\nexit 0\n"))
	fixtureFile(t, filepath.Join(root, "dependencies.conf"), "download mise 1.0 all all source "+hex.EncodeToString(sum[:])+" .local/bin/mise raw\n", 0600)
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
	// A recorded binary that no longer matches the approved checksum is not the
	// approved installation, as install already decides.
	fixtureFile(t, target, "#!/bin/sh\nexit 1\n", 0700)
	got, err = inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "selfishell", "1.0")
	// After a CLI-only update or rollback, the approved checksum describes a
	// different version, so it cannot judge the recorded binary.
	fixtureFile(t, filepath.Join(paths.State, "dependencies/mise"), "0.9\n", 0600)
	got, err = inv.Detect("direct", "mise", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "0.9", "selfishell", "1.0")
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
	t.Setenv("PATH", bin)
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
	if err == nil || !strings.Contains(err.Error(), "mise ERROR untrusted") || !strings.Contains(err.Error(), "extra detail") {
		t.Fatalf("query error lost: %v", err)
	}
	wantTool(t, got, "unknown", "none", "2.100.0")
	got, err = inv.Detect("mise", "node", "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "mise ERROR untrusted") {
		t.Fatalf("cached query error lost: %v", err)
	}
	wantTool(t, got, "unknown", "none", "24.18.0")
	if strings.Count(warnings.String(), "mise could not list installed tools") != 1 {
		t.Fatalf("warning count: %q", warnings.String())
	}
	if !strings.Contains(warnings.String(), "mise ERROR untrusted") {
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
	fixtureFile(t, filepath.Join(root, "config/shared/mise.toml"), "[tools]\nuv = \"0.12.13\"\n", 0600)
	fixtureFile(t, filepath.Join(bin, "mise"), "#!/bin/sh\nprintf 'call\\n' >>\"$HOME/mise-calls\"\ncat \"$HOME/mise-output\"\n", 0700)
	fixtureFile(t, filepath.Join(os.Getenv("HOME"), "mise-output"), "", 0600)
	fixtureFile(t, filepath.Join(bin, "uv"), "#!/bin/sh\n", 0700)
	inv := inventory(t, root, paths, warnings)
	got, err := inv.Detect("mise", "uv", "linux", "amd64")
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

func TestInventoryMiseInstallsAreNotExternalTools(t *testing.T) {
	for _, location := range []string{"installs", "symlink", "shims", "shim-alias", "shim-directory-alias", "shim-parent-alias", "configured-parent-alias", "external-sibling"} {
		t.Run(location, func(t *testing.T) {
			root, paths, warnings, bin := inventoryFixture(t)
			data := filepath.Join(root, "mise-data")
			t.Setenv("MISE_DATA_DIR", data)
			if location == "configured-parent-alias" {
				nested := filepath.Join(root, "nested")
				if err := os.MkdirAll(nested, 0700); err != nil {
					t.Fatal(err)
				}
				alias := filepath.Join(bin, "data-alias")
				if err := os.Symlink(nested, alias); err != nil {
					t.Fatal(err)
				}
				t.Setenv("MISE_DATA_DIR", alias+"/../mise-data")
			}
			fixtureFile(t, filepath.Join(root, "config/shared/mise.toml"), "[tools]\nnode = \"24.18.0\"\n", 0600)
			fixtureFile(t, filepath.Join(bin, "mise"), "#!/bin/sh\nexit 0\n", 0700)
			tool := filepath.Join(data, "installs/node/20.0.0/bin/node")
			fixtureFile(t, tool, "#!/bin/sh\nexit 0\n", 0700)
			selected := tool
			if location == "shim-alias" || location == "shim-directory-alias" || location == "shim-parent-alias" {
				shim := filepath.Join(data, "shims/node")
				if err := os.MkdirAll(filepath.Dir(shim), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(bin, "mise"), shim); err != nil {
					t.Fatal(err)
				}
				if location == "shim-parent-alias" {
					nested := filepath.Join(data, "nested")
					if err := os.MkdirAll(nested, 0700); err != nil {
						t.Fatal(err)
					}
					alias := filepath.Join(bin, "alias")
					if err := os.Symlink(nested, alias); err != nil {
						t.Fatal(err)
					}
					selected = filepath.Join(bin, "node")
					if err := os.Symlink("alias/../shims/node", selected); err != nil {
						t.Fatal(err)
					}
				} else if location == "shim-alias" {
					selected = filepath.Join(bin, "node")
					if err := os.Symlink(shim, selected); err != nil {
						t.Fatal(err)
					}
				} else {
					alias := filepath.Join(root, "shim-alias")
					if err := os.Symlink(filepath.Dir(shim), alias); err != nil {
						t.Fatal(err)
					}
					selected = filepath.Join(alias, "node")
				}
			} else if location == "symlink" || location == "shims" {
				selected = filepath.Join(bin, "node")
				if location == "shims" {
					selected = filepath.Join(data, "shims/node")
				}
				if err := os.MkdirAll(filepath.Dir(selected), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(tool, selected); err != nil {
					t.Fatal(err)
				}
			} else if location == "external-sibling" {
				selected = filepath.Join(data, "installs-personal/node")
				fixtureFile(t, selected, "#!/bin/sh\nexit 0\n", 0700)
			}
			t.Setenv("PATH", filepath.Dir(selected)+":"+bin)
			got, err := inventory(t, root, paths, warnings).Detect("mise", "node", "linux", "amd64")
			if err != nil {
				t.Fatal(err)
			}
			installed, source := "missing", "none"
			if location == "external-sibling" {
				installed, source = "detected", "external"
			}
			wantTool(t, got, installed, source, "24.18.0")
		})
	}
}
func TestInventoryMiseManagedBinaryOutsidePath(t *testing.T) {
	for _, location := range []string{"home", "home_alias", "lexical_home_alias"} {
		t.Run(location, func(t *testing.T) {
			root, paths, warnings, _ := inventoryFixture(t)
			fixtureFile(t, filepath.Join(root, "config/shared/mise.toml"), "[tools]\nnode = \"24.18.0\"\n", 0600)
			home := os.Getenv("HOME")
			if location != "home" {
				// alias/.. resolves to actual/home; a lexical clean would select work/home.
				for _, dir := range []string{root + "/actual/child", root + "/work"} {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(root+"/actual/child", root+"/work/alias"); err != nil {
					t.Fatal(err)
				}
				t.Setenv("HOME", root+"/work/alias/../home")
				home = root + "/actual/home"
				if location == "lexical_home_alias" {
					home = root + "/work/home"
				}
			}
			fixtureFile(t, home+"/.local/bin/mise", "#!/bin/sh\nprintf 'node 24.18.0 /config 24.18.0\\n'\n", 0700)
			inv := inventory(t, root, paths, warnings)
			got, err := inv.Detect("mise", "node", "linux", "amd64")
			if err != nil {
				t.Fatal(err)
			}
			if location == "lexical_home_alias" {
				if got.Source == "mise" {
					t.Fatalf("mise outside the real HOME used: %+v", got)
				}
				return
			}
			wantTool(t, got, "24.18.0", "mise", "24.18.0")
		})
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
	t.Setenv("GIT_DIR", root+"/foreign")
	t.Setenv("GIT_WORK_TREE", root+"/foreign")
	fixtureFile(t, target+"/.git/HEAD", "invalid\n", 0600)
	got, err = inv.Detect("direct", "zinit", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "selfishell", "v3.15.0")
	fixtureFile(t, target+"/.git/HEAD", sha+"\n", 0600)
	fixtureFile(t, target+"/untracked", "user data\n", 0600)
	got, err = inv.Detect("direct", "zinit", "linux", "amd64")
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
	// A newer pin's commit cannot judge the recorded checkout, but tracked
	// changes still can.
	fixtureFile(t, filepath.Join(root, "dependencies.conf"), "git zinit v3.16.0 all all source deadbeef .local/share/zinit/zinit.git zinit.zsh\n", 0600)
	inv = inventory(t, root, paths, warnings)
	got, err = inv.Detect("direct", "zinit", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "v3.15.0", "selfishell", "v3.16.0")
	fixtureFile(t, marker, "changed\n", 0600)
	got, err = inv.Detect("direct", "zinit", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wantTool(t, got, "missing", "selfishell", "v3.16.0")
}

// pathSnapshot records every entry under root without following links.
func pathSnapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %v %d", path, info.Mode(), info.ModTime().UnixNano())
		if d.Type()&fs.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			b.WriteString(" -> " + link)
		} else if d.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, " %x", sha256.Sum256(data))
		}
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
func TestInventoryDirectGitFollowsRawXDGSpelling(t *testing.T) {
	for _, shape := range []string{"managed_real_checkout", "external_real_marker", "external_lexical_marker"} {
		t.Run(shape, func(t *testing.T) {
			root, paths, warnings, _ := inventoryFixture(t)
			home := os.Getenv("HOME")
			// alias/.. resolves to actual/data; a lexical clean would select work/data.
			for _, dir := range []string{home + "/actual/child", home + "/work"} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(home+"/actual/child", home+"/work/alias"); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_DATA_HOME", home+"/work/alias/../data")
			real, lexical := home+"/actual/data/zinit/zinit.git", home+"/work/data/zinit/zinit.git"
			sha, want := "-", ToolResult{"detected", "external", "v3.15.0"}
			switch shape {
			case "managed_real_checkout":
				if err := os.MkdirAll(real, 0700); err != nil {
					t.Fatal(err)
				}
				gitCommand(t, real, "init", "--quiet")
				fixtureFile(t, real+"/zinit.zsh", ":\n", 0600)
				gitCommand(t, real, "add", "zinit.zsh")
				gitCommand(t, real, "commit", "--quiet", "-m", "initial")
				sha = gitCommand(t, real, "rev-parse", "HEAD")
				fixtureFile(t, filepath.Join(paths.State, "dependencies/zinit"), "v3.15.0\n", 0600)
				want = ToolResult{"v3.15.0", "selfishell", "v3.15.0"}
			case "external_real_marker":
				fixtureFile(t, real+"/zinit.zsh", ":\n", 0600)
			case "external_lexical_marker":
				fixtureFile(t, real+"/user-file", "user data\n", 0600)
				fixtureFile(t, lexical+"/zinit.zsh", ":\n", 0600)
				want = ToolResult{"missing", "none", "v3.15.0"}
			}
			fixtureFile(t, filepath.Join(root, "dependencies.conf"), "git zinit v3.15.0 all all source "+sha+" .local/share/zinit/zinit.git zinit.zsh\n", 0600)
			before := pathSnapshot(t, root)
			got, err := inventory(t, root, paths, warnings).Detect("direct", "zinit", "linux", "amd64")
			if err != nil {
				t.Fatal(err)
			}
			wantTool(t, got, want.Installed, want.Source, want.Approved)
			if after := pathSnapshot(t, root); after != before {
				t.Fatalf("diagnostics changed paths:\n%s\n---\n%s", before, after)
			}
		})
	}
}
