package selfishell

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const bashReference = "3bbbfa0346ee74eb47f31a81ec666340a5ef6018"
const bashLegacy = "d025710338036f1f54b948f1f3e5c17a0b3f7e38"

func isolatedStateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for key, suffix := range map[string]string{"XDG_CONFIG_HOME": "/.config", "XDG_STATE_HOME": "/.local/state", "XDG_CACHE_HOME": "/.cache", "XDG_DATA_HOME": "/.local/share"} {
		t.Setenv(key, home+suffix)
	}
	t.Setenv("SELFISHELL_TEST_INTERRUPT", "")
	t.Setenv("NO_COLOR", "1")
	return home
}

func testCommand(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %q: %v\n%s", name, args, err, out)
	}
	return out
}

func exportedBash(t *testing.T, commit string) string {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", repo, "cat-file", "-e", commit+"^{commit}").Run(); err != nil {
		t.Fatalf("missing migration reference %s; fetch full history before tests: %v", commit, err)
	}
	dir := t.TempDir()
	source := dir + "/source"
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	testCommand(t, "git", "-C", repo, "archive", commit, "--output="+dir+"/source.tar")
	testCommand(t, "tar", "-xf", dir+"/source.tar", "-C", source)
	if commit != bashLegacy {
		return source
	}
	// Use v1.3.1's own builder and native archive, then discard its source export.
	testCommand(t, "bash", source+"/scripts/build-release.sh", "--version", "1.3.1", "--output", dir+"/assets")
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	release := dir + "/retained release"
	if err := os.Mkdir(release, 0700); err != nil {
		t.Fatal(err)
	}
	testCommand(t, "tar", "-xzf", dir+"/assets/selfishell-1.3.1-"+platform+"-"+runtime.GOARCH+".tar.gz", "-C", release)
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	if got := string(testCommand(t, release+"/bin/selfishell", "version")); got != "selfishell 1.3.1\n" {
		t.Fatal(got)
	}
	return release
}

func stateBridge(t *testing.T, root string, args ...string) ([]byte, error) {
	t.Helper()
	script, err := filepath.Abs("../../tests/fixtures/go_migration/state_bridge.bash")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{script, root}, args...)...)
	return cmd.CombinedOutput()
}

func requireBridge(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	out, err := stateBridge(t, root, args...)
	if err != nil {
		t.Fatalf("bridge %q: %v\n%s", args, err, out)
	}
	return out
}

func rewriteForBash(t *testing.T, root, path, name string, state State) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Force the record to be authored by Go, rather than the identical-write no-op.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := WriteState(path, state); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("Go changed state bytes: %q %q %v", before, after, err)
	}
	if got := requireBridge(t, root, "read", name); !bytes.Equal(got, after) {
		t.Fatalf("retained Bash read differs: %q %q", got, after)
	}
}

func TestBashStateInteroperability(t *testing.T) {
	for _, commit := range []string{bashReference, bashLegacy} {
		t.Run(commit[:7], func(t *testing.T) {
			isolatedStateHome(t)
			root := exportedBash(t, commit)
			for _, kind := range []string{"file", "link", "block"} {
				t.Run(kind, func(t *testing.T) {
					home := isolatedStateHome(t)
					name := "fixture-" + kind
					target := home + "/target"
					if kind == "block" {
						name = "user-zshrc"
						target = home + "/.zshrc"
					}
					original := []byte("personal setting\r\nwithout final newline")
					if err := os.WriteFile(target, original, 0640); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(home+"/source", []byte("approved\x00content\r\n"), 0644); err != nil {
						t.Fatal(err)
					}
					t.Setenv("SELFISHELL_TEST_INTERRUPT", "1")
					if out, err := stateBridge(t, root, "install", kind); err == nil {
						t.Fatalf("interruption unexpectedly succeeded: %s", out)
					}
					path := home + "/.local/state/selfishell/resources/" + name + ".state"
					pending, err := ReadState(path)
					if err != nil || pending.Status != "pending" || pending.Kind != kind || pending.Target != target {
						t.Fatalf("Bash pending: %+v %v", pending, err)
					}
					if kind != "block" && pending.Backup == "-" {
						t.Fatal("original backup was not recorded")
					}
					rewriteForBash(t, root, path, name, pending)
					t.Setenv("SELFISHELL_TEST_INTERRUPT", "")
					requireBridge(t, root, "install", kind)
					active, err := ReadState(path)
					if err != nil || active.Status != "active" || active.Backup != pending.Backup {
						t.Fatalf("recovery lost original backup: %+v %v", active, err)
					}
					rewriteForBash(t, root, path, name, active)
					requireBridge(t, root, "install", kind)
					again, err := ReadState(path)
					if err != nil || again != active {
						t.Fatalf("reinstall changed state: %+v %v", again, err)
					}
					if kind != "block" {
						if data, err := os.ReadFile(active.Backup); err != nil || !bytes.Equal(data, original) {
							t.Fatalf("backup bytes changed: %q %v", data, err)
						}
					}
					requireBridge(t, root, "uninstall", name)
					if data, err := os.ReadFile(target); err != nil || !bytes.Equal(data, original) {
						t.Fatalf("restore bytes changed: %q %v", data, err)
					}
					if _, err := ReadState(path); !errors.Is(err, fs.ErrNotExist) {
						t.Fatalf("state remains after uninstall: %v", err)
					}
				})
			}
		})
	}
}

func formatResources(resources []Resource) string {
	var out strings.Builder
	for _, r := range resources {
		fmt.Fprintf(&out, "%s\t%s\t%s\t%s\n", r.Kind, r.Name, r.Target, r.Source)
	}
	return out.String()
}

func TestBashResourceCompatibility(t *testing.T) {
	home := isolatedStateHome(t)
	root := exportedBash(t, bashReference)
	// Preserve literal path components in the table; enumeration must not need
	// the managed targets to exist, nor clean away symlink/.. components.
	t.Setenv("XDG_CONFIG_HOME", home+"/alias/../config space")
	all, err := ManagedResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := formatResources(all), string(requireBridge(t, root, "resources")); got != want {
		t.Fatalf("resource table differs:\n%s\nwant:\n%s", got, want)
	}
	for _, platform := range []string{"macos", "ubuntu", "ubuntu-wsl"} {
		for _, ghostty := range []bool{false, true} {
			selected, err := ResourcesForPlatform(root, platform, ghostty)
			if err != nil {
				t.Fatal(err)
			}
			option := "0"
			if ghostty {
				option = "1"
			}
			if got, want := formatResources(selected), string(requireBridge(t, root, "selected-resources", platform, option)); got != want {
				t.Fatalf("%s ghostty=%v differs:\n%s\nwant:\n%s", platform, ghostty, got, want)
			}
		}
	}
}
